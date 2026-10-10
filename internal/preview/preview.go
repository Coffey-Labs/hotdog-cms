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
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"git.coffeylabs.org/coffey-labs/cl-cms/internal/build"
	"git.coffeylabs.org/coffey-labs/cl-cms/internal/check"
	"git.coffeylabs.org/coffey-labs/cl-cms/internal/forge"
	"git.coffeylabs.org/coffey-labs/cl-cms/internal/gitx"
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

	Git        gitx.Auth     // for private HTTPS repositories
	Forge      *forge.Client // posts each preview's address as a commit status; nil for none
	HookSecret string        // the webhook secret; empty turns the hook off
	// FrameAncestors are origins allowed to show previews in a frame, such as
	// the editor (http://127.0.0.1:8190). Anyone else is refused, as before.
	FrameAncestors []string
}

// Preview is one built branch.
type Preview struct {
	Branch   string
	Slug     string
	Commit   string
	Built    time.Time
	Pages    int
	Error    string // the last build's error; the previous good build is still served
	Errors   int    // check errors in the current build: publishing it would be refused
	Warnings int    // check warnings in the current build
	URL      string
	dir      string
	pending  string // commit of a failed build, so it isn't retried until the branch moves
}

// Builder keeps the set of previews current.
type Builder struct {
	o        Options
	mu       sync.RWMutex
	previews map[string]*Preview // by slug
	syncMu   sync.Mutex          // one sync at a time
	kick     chan struct{}       // a webhook asking for a sync now
}

// NewBuilder prepares a builder; call Sync to build.
func NewBuilder(o Options) (*Builder, error) {
	if err := gitx.Available(); err != nil {
		return nil, fmt.Errorf("previews need git: %w", err)
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
	return &Builder{o: o, previews: map[string]*Preview{}, kick: make(chan struct{}, 1)}, nil
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

// heads lists the wanted branches and their commits.
func (b *Builder) heads() (map[string]string, error) {
	all, err := gitx.Heads(b.o.Git, b.o.Repo)
	if err != nil {
		return nil, err
	}
	heads := map[string]string{}
	for br, c := range all {
		if b.wanted(br) {
			heads[br] = c
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
	b.syncMu.Lock()
	defer b.syncMu.Unlock()
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

	b.status(commit, forge.Pending, url+"/", "Building the preview")
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
		go b.status(commit, forge.Failure, "", "Preview build failed: "+err.Error())
		return
	}
	p.Commit, p.Built, p.Pages, p.Error, p.pending = commit, time.Now(), res.Pages, "", ""
	p.Errors, p.Warnings = 0, 0
	if rep, err := check.Run(filepath.Join(src, b.o.Subdir), out, res.Site.Config); err == nil {
		p.Errors = rep.Errors()
		p.Warnings = len(rep.Problems) - p.Errors
	}
	fmt.Fprintf(b.o.Log, "preview %s (%s @ %s): %d pages at %s/\n", slug, branch, short(commit), res.Pages, url)
	go b.status(commit, forge.Success, url+"/", fmt.Sprintf("Preview ready, %d pages", res.Pages))
}

// status tells the platform how a commit's preview went, so the pull request
// shows it with a link. Failures to post are logged, never fatal.
func (b *Builder) status(commit string, st forge.State, url, desc string) {
	if b.o.Forge == nil || !b.o.Forge.Supported() || commit == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := b.o.Forge.SetStatus(ctx, commit, forge.Status{State: st, URL: url, Description: desc}); err != nil {
		fmt.Fprintf(b.o.Log, "commit status for %s: %v\n", short(commit), err)
	}
}

// Kick asks for a sync as soon as possible; extra kicks while one is
// pending fold into it.
func (b *Builder) Kick() {
	select {
	case b.kick <- struct{}{}:
	default:
	}
}

func (b *Builder) checkout(src, branch string) error {
	if err := os.MkdirAll(filepath.Dir(src), 0o755); err != nil {
		return err
	}
	_, err := gitx.Checkout(b.o.Git, b.o.Repo, branch, src)
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
