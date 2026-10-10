// Package preview builds every branch of a site's repository the way
// production would build it, and serves each at its own address, so a change
// can be looked at for real before it is published.
//
// A preview is the production build with one difference, the site's address,
// so links stay on the preview. Nothing is injected into the pages. Every
// response says noindex and no-store, robots.txt turns crawlers away, and the
// server listens on loopback unless it is given credentials to ask for.
package preview

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"git.coffeylabs.org/coffey-labs/cl-cms/internal/build"
)

// Options configure the preview builder.
type Options struct {
	Repo   string   // anything git can clone
	Subdir string   // the site's folder inside the repository
	Refs   []string // branch patterns to preview (path.Match); default every branch
	Root   string   // where previews are built
	Domain string   // previews are served at <slug>.<Domain>; "localhost" by default
	Port   string   // the port browsers use, for the preview's own links; "" for 80/443
	Scheme string   // http or https
	Drafts bool     // include draft pages
	Log    io.Writer
}

// Preview is one built branch.
type Preview struct {
	Branch  string
	Slug    string
	Commit  string
	Built   time.Time
	Pages   int
	Error   string // the last build's error; the previous good build is still served
	URL     string
	dir     string
	pending string // commit of a failed build, so it isn't retried until the branch moves
}

// Builder keeps the set of previews current.
type Builder struct {
	o        Options
	mu       sync.RWMutex
	previews map[string]*Preview // by slug
}

// NewBuilder prepares a builder; call Sync to build.
func NewBuilder(o Options) (*Builder, error) {
	if _, err := exec.LookPath("git"); err != nil {
		return nil, fmt.Errorf("previews need git installed")
	}
	if o.Domain == "" {
		o.Domain = "localhost"
	}
	if o.Scheme == "" {
		o.Scheme = "http"
	}
	if o.Log == nil {
		o.Log = io.Discard
	}
	if err := os.MkdirAll(o.Root, 0o755); err != nil {
		return nil, err
	}
	return &Builder{o: o, previews: map[string]*Preview{}}, nil
}

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

// Slug turns a branch name into a DNS label: readable, and unique per branch
// through a short hash, so feature/x and feature-x never share a preview.
func Slug(branch string) string {
	s := strings.Trim(slugRe.ReplaceAllString(strings.ToLower(branch), "-"), "-")
	if len(s) > 48 {
		s = strings.Trim(s[:48], "-")
	}
	sum := sha256.Sum256([]byte(branch))
	if s == "" {
		s = "branch"
	}
	return s + "-" + hex.EncodeToString(sum[:])[:6]
}

func (b *Builder) git(dir string, args ...string) (string, error) {
	var out, errb bytes.Buffer
	if dir != "" {
		args = append([]string{"-C", dir}, args...)
	}
	cmd := exec.Command("git", args...)
	cmd.Stdout, cmd.Stderr = &out, &errb
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %v: %s", args[0], err, strings.TrimSpace(errb.String()))
	}
	return strings.TrimSpace(out.String()), nil
}

// heads lists the repository's branches and their commits.
func (b *Builder) heads() (map[string]string, error) {
	out, err := b.git("", "ls-remote", "--heads", b.o.Repo)
	if err != nil {
		return nil, err
	}
	heads := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) != 2 || !strings.HasPrefix(f[1], "refs/heads/") {
			continue
		}
		branch := strings.TrimPrefix(f[1], "refs/heads/")
		if b.wanted(branch) {
			heads[branch] = f[0]
		}
	}
	return heads, nil
}

func (b *Builder) wanted(branch string) bool {
	if len(b.o.Refs) == 0 {
		return true
	}
	for _, p := range b.o.Refs {
		if ok, _ := path.Match(p, branch); ok {
			return true
		}
	}
	return false
}

// Sync builds every wanted branch whose commit has moved and removes the
// previews of branches that are gone.
func (b *Builder) Sync() error {
	heads, err := b.heads()
	if err != nil {
		return err
	}
	branches := make([]string, 0, len(heads))
	for br := range heads {
		branches = append(branches, br)
	}
	sort.Strings(branches)
	live := map[string]bool{}
	for _, br := range branches {
		slug := Slug(br)
		live[slug] = true
		b.mu.RLock()
		p := b.previews[slug]
		b.mu.RUnlock()
		if p != nil && (p.Commit == heads[br] || p.pending == heads[br]) {
			continue
		}
		b.buildBranch(br, slug, heads[br])
	}
	b.mu.Lock()
	for slug, p := range b.previews {
		if !live[slug] {
			fmt.Fprintf(b.o.Log, "preview %s: branch %s is gone, removing\n", slug, p.Branch)
			_ = os.RemoveAll(p.dir)
			_ = os.RemoveAll(filepath.Join(b.o.Root, ".src", slug))
			delete(b.previews, slug)
		}
	}
	b.mu.Unlock()
	return nil
}

func (b *Builder) buildBranch(branch, slug, commit string) {
	src := filepath.Join(b.o.Root, ".src", slug)
	out := filepath.Join(b.o.Root, slug)
	host := slug + "." + b.o.Domain
	if b.o.Port != "" {
		host += ":" + b.o.Port
	}
	url := b.o.Scheme + "://" + host

	err := b.checkout(src, branch)
	var res *build.Result
	if err == nil {
		res, err = build.Run(build.Options{SiteDir: filepath.Join(src, b.o.Subdir), Out: out, Drafts: b.o.Drafts, URL: url})
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	p := b.previews[slug]
	if p == nil {
		p = &Preview{Branch: branch, Slug: slug, URL: url + "/", dir: out}
		b.previews[slug] = p
	}
	if err != nil {
		p.Error, p.pending = err.Error(), commit
		fmt.Fprintf(b.o.Log, "preview %s (%s @ %s) failed: %v\n", slug, branch, short(commit), err)
		return
	}
	p.Commit, p.Built, p.Pages, p.Error, p.pending = commit, time.Now(), res.Pages, "", ""
	fmt.Fprintf(b.o.Log, "preview %s (%s @ %s): %d pages at %s/\n", slug, branch, short(commit), res.Pages, url)
}

func (b *Builder) checkout(src, branch string) error {
	if _, err := os.Stat(filepath.Join(src, ".git")); err != nil {
		if err := os.MkdirAll(filepath.Dir(src), 0o755); err != nil {
			return err
		}
		_, err := b.git("", "clone", "--quiet", "--depth", "1", "--branch", branch, "--single-branch", b.o.Repo, src)
		return err
	}
	if _, err := b.git(src, "fetch", "--quiet", "--depth", "1", "origin", branch); err != nil {
		return err
	}
	if _, err := b.git(src, "reset", "--quiet", "--hard", "FETCH_HEAD"); err != nil {
		return err
	}
	_, err := b.git(src, "clean", "-qfdx")
	return err
}

// List returns the previews, by branch name.
func (b *Builder) List() []Preview {
	b.mu.RLock()
	defer b.mu.RUnlock()
	out := make([]Preview, 0, len(b.previews))
	for _, p := range b.previews {
		out = append(out, *p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Branch < out[j].Branch })
	return out
}

// dirFor returns the build folder for a slug, if it has a good build.
func (b *Builder) dirFor(slug string) (string, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	p, ok := b.previews[slug]
	if !ok || p.Commit == "" {
		return "", false
	}
	return p.dir, true
}

func short(h string) string {
	if len(h) > 10 {
		return h[:10]
	}
	return h
}
