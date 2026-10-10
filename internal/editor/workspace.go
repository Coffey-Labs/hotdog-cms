package editor

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"git.coffeylabs.org/coffey-labs/cl-cms/internal/build"
	"git.coffeylabs.org/coffey-labs/cl-cms/internal/check"
	"git.coffeylabs.org/coffey-labs/cl-cms/internal/forge"
	"git.coffeylabs.org/coffey-labs/cl-cms/internal/gitx"
	"git.coffeylabs.org/coffey-labs/cl-cms/internal/site"
)

// A workspace is one person's checkout of one branch of one site, made with
// their own token, so the editor can only ever read what the platform lets
// them read. Checkouts are a cache: deleting the folder loses nothing.

type workspaces struct {
	root  string
	mu    sync.Mutex
	locks map[string]*sync.Mutex
	built map[string]*BuildResult // by checkout and commit
}

// BuildResult is a branch built and checked.
type BuildResult struct {
	Commit   string          `json:"commit"`
	Pages    int             `json:"pages"`
	Errors   int             `json:"errors"`
	Warnings int             `json:"warnings"`
	Problems []check.Problem `json:"problems"`
	Failed   string          `json:"failed,omitempty"` // the build itself failed
	At       time.Time       `json:"at"`
}

func newWorkspaces(root string) *workspaces {
	return &workspaces{root: root, locks: map[string]*sync.Mutex{}, built: map[string]*BuildResult{}}
}

func (ws *workspaces) dir(sess *Session, sc *SiteConfig, branch string) string {
	sum := sha256.Sum256([]byte(sess.Forge + "\x00" + sess.Login + "\x00" + sc.Repo))
	b := sha256.Sum256([]byte(branch))
	return filepath.Join(ws.root, hex.EncodeToString(sum[:12]), hex.EncodeToString(b[:6]))
}

func (ws *workspaces) lock(dir string) func() {
	ws.mu.Lock()
	l := ws.locks[dir]
	if l == nil {
		l = &sync.Mutex{}
		ws.locks[dir] = l
	}
	ws.mu.Unlock()
	l.Lock()
	return l.Unlock
}

// checkout brings the person's copy of a branch up to date and loads the site.
func (ws *workspaces) checkout(sess *Session, kind forge.Kind, sc *SiteConfig, repo, branch string) (*site.Site, string, string, error) {
	dir := ws.dir(sess, sc, branch)
	defer ws.lock(dir)()
	if err := os.MkdirAll(filepath.Dir(dir), 0o700); err != nil {
		return nil, "", "", err
	}
	commit, err := gitx.Checkout(gitx.Auth{User: kind.TokenUser(), Token: sess.Token}, repo, branch, dir)
	if err != nil {
		return nil, "", "", err
	}
	siteDir := filepath.Join(dir, sc.Subdir)
	s, err := site.Load(siteDir, site.Options{Drafts: true})
	if err != nil {
		return nil, siteDir, commit, err
	}
	return s, siteDir, commit, nil
}

// buildAndCheck builds a checkout and runs the site's checks on it, once per
// commit.
func (ws *workspaces) buildAndCheck(siteDir, commit string) *BuildResult {
	key := siteDir + "@" + commit
	ws.mu.Lock()
	if r, ok := ws.built[key]; ok {
		ws.mu.Unlock()
		return r
	}
	ws.mu.Unlock()
	out := siteDir + ".editor-build"
	r := &BuildResult{Commit: commit, At: time.Now()}
	res, err := build.Run(build.Options{SiteDir: siteDir, Out: out, Drafts: true})
	if err != nil {
		r.Failed = err.Error()
	} else {
		r.Pages = res.Pages
		if rep, err := check.Run(siteDir, out, res.Site.Config); err != nil {
			r.Failed = err.Error()
		} else {
			r.Errors = rep.Errors()
			r.Warnings = len(rep.Problems) - r.Errors
			r.Problems = rep.Problems
		}
	}
	ws.mu.Lock()
	ws.built[key] = r
	ws.mu.Unlock()
	return r
}

// PageInfo is one page of a site, as the editor lists it.
type PageInfo struct {
	Path       string `json:"path"`
	Title      string `json:"title"`
	Source     string `json:"source,omitempty"`
	Collection string `json:"collection,omitempty"`
	Kind       string `json:"kind"`
	Date       string `json:"date,omitempty"`
	Draft      bool   `json:"draft,omitempty"`
	Redirect   string `json:"redirect,omitempty"`
}

// CollectionInfo is one folder of pages.
type CollectionInfo struct {
	Name  string `json:"name"`
	Title string `json:"title"`
	Count int    `json:"count"`
}

func pagesOf(s *site.Site) ([]PageInfo, []CollectionInfo) {
	var pages []PageInfo
	for _, p := range s.Pages {
		if p.Source == "" {
			continue // generated: taxonomy pages, later pages of a list
		}
		pi := PageInfo{Path: p.Path, Title: p.Title, Source: "content/" + p.Source, Collection: p.Collection, Kind: p.Kind, Draft: p.Draft, Redirect: p.RedirectTo}
		if !p.Date.IsZero() {
			pi.Date = p.Date.Format("2006-01-02")
		}
		pages = append(pages, pi)
	}
	sort.Slice(pages, func(i, j int) bool {
		if pages[i].Collection != pages[j].Collection {
			return pages[i].Collection < pages[j].Collection
		}
		if pages[i].Date != pages[j].Date {
			return pages[i].Date > pages[j].Date
		}
		return pages[i].Path < pages[j].Path
	})
	var cols []CollectionInfo
	for name, ps := range s.Collections {
		title := name
		if l := s.Lists[name]; l != nil && l.Title != "" {
			title = l.Title
		}
		cols = append(cols, CollectionInfo{Name: name, Title: title, Count: len(ps)})
	}
	sort.Slice(cols, func(i, j int) bool { return cols[i].Name < cols[j].Name })
	return pages, cols
}

// readSource reads a content file from a checkout, refusing anything that
// isn't a Markdown file under content/.
func readSource(siteDir, source string) ([]byte, error) {
	clean := filepath.ToSlash(filepath.Clean("/" + source))[1:]
	if clean != source || !strings.HasPrefix(clean, "content/") || !strings.HasSuffix(clean, ".md") {
		return nil, fmt.Errorf("not a page source: %q", source)
	}
	return os.ReadFile(filepath.Join(siteDir, filepath.FromSlash(clean)))
}
