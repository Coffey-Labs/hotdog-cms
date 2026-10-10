// Package build renders a loaded site into a folder of static files.
package build

import (
	"bytes"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"git.coffeylabs.org/coffey-labs/cl-cms/internal/forms"
	"git.coffeylabs.org/coffey-labs/cl-cms/internal/site"
)

// marker is written into every output folder. A build only ever replaces a
// folder that has one, so pointing --out at the wrong place cannot wipe it.
const marker = ".cl-cms-build"

// Options for one build.
type Options struct {
	SiteDir string
	Out     string // default <site>/public
	Drafts  bool
	URL     string // replace site.yaml's url (local preview)
	Dev     bool
}

// Result says what a build produced.
type Result struct {
	Site     *site.Site
	Out      string
	Pages    int
	Files    int
	Feeds    []string
	Duration time.Duration
}

// Run builds a site.
func Run(opt Options) (*Result, error) {
	start := time.Now()
	siteDir, err := filepath.Abs(opt.SiteDir)
	if err != nil {
		return nil, err
	}
	out := opt.Out
	if out == "" {
		out = filepath.Join(siteDir, "public")
	}
	if out, err = filepath.Abs(out); err != nil {
		return nil, err
	}
	if err := safeOut(siteDir, out); err != nil {
		return nil, err
	}

	s, err := site.Load(siteDir, site.Options{Drafts: opt.Drafts, URL: opt.URL, Dev: opt.Dev})
	if err != nil {
		return nil, err
	}

	// Built beside the output and swapped in at the end, so a build that
	// fails halfway leaves the previous site untouched.
	work := fmt.Sprintf("%s.new-%d", out, os.Getpid())
	_ = os.RemoveAll(work)
	defer os.RemoveAll(work)
	if err := os.MkdirAll(work, 0o755); err != nil {
		return nil, err
	}

	assets, err := writeAssets(filepath.Join(siteDir, "assets"), work)
	if err != nil {
		return nil, fmt.Errorf("assets: %w", err)
	}
	if err := copyTree(filepath.Join(siteDir, "static"), work, nil); err != nil {
		return nil, fmt.Errorf("static: %w", err)
	}
	// Images and downloads kept beside a page in content/ go out beside it.
	if err := copyTree(filepath.Join(siteDir, "content"), work, func(rel string, d fs.DirEntry) bool {
		return !d.IsDir() && strings.HasSuffix(rel, ".md")
	}); err != nil {
		return nil, fmt.Errorf("content: %w", err)
	}

	md := s.Markdown
	icons, err := loadIcons(filepath.Join(siteDir, "icons"))
	if err != nil {
		return nil, err
	}
	siteForms, err := forms.Load(siteDir)
	if err != nil {
		return nil, err
	}
	if len(siteForms) > 0 {
		if err := assets.add(work, "cl-forms.js", forms.Script); err != nil {
			return nil, err
		}
		if err := assets.add(work, "cl-forms.css", forms.Style); err != nil {
			return nil, err
		}
	}
	fm := funcs(s, assets, md, icons)
	// form "contact" renders forms/contact.yaml.
	fm["form"] = func(name string) (template.HTML, error) {
		f, ok := siteForms[name]
		if !ok {
			return "", fmt.Errorf("no forms/%s.yaml", name)
		}
		js, _ := assets.URL("cl-forms.js")
		css, _ := assets.URL("cl-forms.css")
		return f.Render(js, css)
	}
	tpl, err := loadTemplates(siteDir, fm)
	if err != nil {
		return nil, err
	}
	res := &Result{Site: s, Out: out}
	for _, p := range s.Pages {
		name := p.Source
		if name == "" {
			name = p.Path
		} else {
			name = "content/" + name
		}
		layout, err := tpl.layoutFor(p, s.Config)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		if err := tpl.checkSections(p); err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		var buf bytes.Buffer
		if err := tpl.layouts[layout].ExecuteTemplate(&buf, "base", p); err != nil {
			return nil, fmt.Errorf("%s (layout %s): %w", name, layout, err)
		}
		dst := pageFile(work, p.Path)
		if _, err := os.Stat(dst); err == nil {
			return nil, fmt.Errorf("%s: %s is already taken by a file in static/ or content/", name, p.Path)
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(dst, buf.Bytes(), 0o644); err != nil {
			return nil, err
		}
		res.Pages++
	}

	cfg := s.Config
	if cfg.Outputs.SitemapOn() {
		if err := writeSitemap(s, work); err != nil {
			return nil, err
		}
	}
	if cfg.Outputs.RobotsOn() {
		if _, err := os.Stat(filepath.Join(work, "robots.txt")); errors.Is(err, fs.ErrNotExist) {
			if err := writeRobots(s, work); err != nil {
				return nil, err
			}
		}
	}
	for name, cc := range cfg.Collections {
		if s.Collection(name) == nil {
			continue
		}
		if cc.Search {
			if err := writeSearch(s, name, work); err != nil {
				return nil, fmt.Errorf("search index for %s: %w", name, err)
			}
		}
		if !cc.Feed {
			continue
		}
		write := writeFeed
		if cc.FeedFormat == "rss" {
			write = writeRSS
		}
		p, err := write(s, name, cc, work)
		if err != nil {
			return nil, fmt.Errorf("feed for %s: %w", name, err)
		}
		res.Feeds = append(res.Feeds, p)
	}
	if err := writeRedirects(s, work); err != nil {
		return nil, err
	}
	if cfg.Outputs.LLMs {
		if err := writeLLMs(s, work); err != nil {
			return nil, err
		}
	}
	if err := os.WriteFile(filepath.Join(work, marker), []byte(s.BuildTime.Format(time.RFC3339)+"\n"), 0o644); err != nil {
		return nil, err
	}
	_ = filepath.WalkDir(work, func(_ string, d fs.DirEntry, _ error) error {
		if d != nil && !d.IsDir() {
			res.Files++
		}
		return nil
	})

	if err := swap(work, out); err != nil {
		return nil, err
	}
	res.Duration = time.Since(start)
	return res, nil
}

func pageFile(root, urlPath string) string {
	if strings.HasSuffix(urlPath, "/") {
		return filepath.Join(root, filepath.FromSlash(urlPath), "index.html")
	}
	return filepath.Join(root, filepath.FromSlash(urlPath))
}

// safeOut refuses output folders that would overwrite the site itself.
func safeOut(siteDir, out string) error {
	rel, err := filepath.Rel(out, siteDir)
	if err == nil && (rel == "." || !strings.HasPrefix(rel, "..")) {
		return fmt.Errorf("output %s would contain the site itself; pick another folder", out)
	}
	for _, d := range []string{"content", "layouts", "partials", "sections", "assets", "static", "data", "forms", "icons"} {
		if r, err := filepath.Rel(filepath.Join(siteDir, d), out); err == nil && !strings.HasPrefix(r, "..") {
			return fmt.Errorf("output %s is inside the site's %s/ folder", out, d)
		}
	}
	return nil
}

// claimed reports whether dir is missing, empty, or an earlier build.
func claimed(dir string) (bool, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	if len(entries) == 0 {
		return true, nil
	}
	_, err = os.Stat(filepath.Join(dir, marker))
	return err == nil, nil
}

// swap puts the new build in place with two renames, so a web server reading
// the folder sees the old site or the new one, never half of each.
func swap(work, out string) error {
	ok, err := claimed(out)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%s has files that cl-cms did not write; refusing to replace it", out)
	}
	old := fmt.Sprintf("%s.old-%d", out, os.Getpid())
	if _, err := os.Stat(out); err == nil {
		if err := os.Rename(out, old); err != nil {
			return err
		}
	}
	if err := os.Rename(work, out); err != nil {
		_ = os.Rename(old, out)
		return err
	}
	return os.RemoveAll(old)
}

// Install copies a finished build into dst the same way Run writes its own
// output: beside it first, then swapped in whole. dst must be empty, missing,
// or an earlier cl-cms build.
func Install(src, dst string) error {
	if _, err := os.Stat(filepath.Join(src, marker)); err != nil {
		return fmt.Errorf("%s is not a cl-cms build", src)
	}
	work := fmt.Sprintf("%s.new-%d", dst, os.Getpid())
	_ = os.RemoveAll(work)
	defer os.RemoveAll(work)
	if err := copyTree(src, work, nil); err != nil {
		return err
	}
	return swap(work, dst)
}

// Marker is the name of the file every build carries, for publishers that
// need to recognise a folder they may replace.
const Marker = marker

// CopyTree copies a folder as it is.
func CopyTree(src, dst string) error { return copyTree(src, dst, nil) }
