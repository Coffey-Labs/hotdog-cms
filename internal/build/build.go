// Package build renders a loaded site into a folder of static files.
package build

import (
	"bytes"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/forms"
	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/site"
)

// marker is written into every output folder. A build only ever replaces a
// folder that has one, so pointing --out at the wrong place cannot wipe it.
const marker = ".hotdog-cms-build"

// Options for one build.
type Options struct {
	SiteDir string
	Out     string // default <site>/public
	Drafts  bool
	URL     string // replace site.yaml's url (local preview)
	Dev     bool
	Now     time.Time         // judge scheduled pages against this time; default now
	Look    map[string]any    // replace site.yaml's look: (the editor's Look preview)
	Forms   map[string][]byte // replace forms/<name>.yaml (the editor's form preview)
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

	s, err := site.Load(siteDir, site.Options{Drafts: opt.Drafts, URL: opt.URL, Dev: opt.Dev, Now: opt.Now})
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

	cards := assignCards(s)
	tpl, err := prepare(siteDir, s, work)
	if err != nil {
		return nil, err
	}
	if err := writeCards(siteDir, work, s, cards); err != nil {
		return nil, err
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
	res := &Result{Site: s, Out: out}
	for _, p := range s.Pages {
		name := p.Source
		if name == "" {
			name = p.Path
		} else {
			name = "content/" + name
		}
		if p.RedirectTo != "" {
			dst := pageFile(work, p.Path)
			if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
				return nil, err
			}
			var buf bytes.Buffer
			if err := redirectTpl.Execute(&buf, p.RedirectTo); err != nil {
				return nil, err
			}
			if err := os.WriteFile(dst, buf.Bytes(), 0o644); err != nil {
				return nil, err
			}
			res.Pages++
			continue
		}
		layout, err := tpl.layoutFor(p, s.Config)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		if err := tpl.checkSections(p); err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		buf := &capped{max: maxPage}
		if err := tpl.layouts[layout].ExecuteTemplate(buf, "base", p); err != nil {
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
	if err := writeCalendars(s, work); err != nil {
		return nil, err
	}
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
		if cc.Search && s.Collection(name) != nil {
			if err := writeSearch(s, name, work); err != nil {
				return nil, fmt.Errorf("search index for %s: %w", name, err)
			}
		}
		// A feed exists as soon as the collection does (its folder has a page
		// of its own, or any pages), so links to it work before the first
		// post: subscribers simply see nothing yet.
		if !cc.Feed || (s.Collection(name) == nil && s.Lists[name] == nil) {
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
	if err := writeSearchFiles(work, s.Config); err != nil {
		return nil, err
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

// prepare fingerprints the assets (writing them into out, unless out is
// empty) and loads the templates with everything they can call.
func prepare(siteDir string, s *site.Site, out string) (*Templates, error) {
	return prepareWith(siteDir, s, out, nil)
}

func prepareWith(siteDir string, s *site.Site, out string, formOverrides map[string][]byte) (*Templates, error) {
	assets, err := writeAssets(filepath.Join(siteDir, "assets"), out)
	if err != nil {
		return nil, fmt.Errorf("assets: %w", err)
	}
	icons, err := loadIcons(filepath.Join(siteDir, "icons"))
	if err != nil {
		return nil, err
	}
	siteForms, err := forms.Load(siteDir)
	if err != nil {
		return nil, err
	}
	for name, raw := range formOverrides {
		f, err := forms.Parse(name, raw)
		if err != nil {
			return nil, fmt.Errorf("forms/%s.yaml: %w", name, err)
		}
		siteForms[name] = f
	}
	if len(siteForms) > 0 {
		if err := assets.add(out, "hotdog-forms.js", forms.Script); err != nil {
			return nil, err
		}
		if err := assets.add(out, "hotdog-forms.css", forms.Style); err != nil {
			return nil, err
		}
	}
	if s.Config.Analytics != nil {
		if err := assets.add(out, "hotdog-consent.js", consentScript); err != nil {
			return nil, err
		}
		if err := assets.add(out, "hotdog-consent.css", consentStyle); err != nil {
			return nil, err
		}
	}
	// The site's look: a stylesheet of what it changes, after the theme's own.
	look, err := site.LoadLook(siteDir)
	if err != nil {
		return nil, err
	}
	var lookVals map[string]site.LookValue
	if look != nil {
		if lookVals, err = look.Values(s.Config.Look); err != nil {
			return nil, err
		}
		if css := look.CSS(lookVals); css != nil {
			if err := assets.add(out, "look.css", css); err != nil {
				return nil, err
			}
		}
	} else if len(s.Config.Look) > 0 {
		return nil, fmt.Errorf("site.yaml has look: but the site has no look.yaml saying what can change")
	}
	fm := funcs(s, assets, s.Markdown, icons)
	// lookCSS is the look stylesheet's address, or "" when nothing's changed;
	// look "logo" is a token's value, the site's choice or the theme's default.
	fm["lookCSS"] = func() string {
		u, _ := assets.URL("look.css")
		return u
	}
	fm["consent"] = func() template.HTML { return consentTags(s.Config, assets) }
	fm["verificationTags"] = func() template.HTML { return verificationTags(s.Config) }
	fm["consentActive"] = func() bool { return s.Config.Analytics != nil }
	fm["privacyFacts"] = func() []site.PrivacyFact { return privacyFacts(s.Config, siteForms) }
	fm["look"] = func(name string) string {
		if look == nil {
			return ""
		}
		return look.Get(lookVals, name)
	}
	// form "contact" renders forms/contact.yaml.
	fm["form"] = func(name string) (template.HTML, error) {
		f, ok := siteForms[name]
		if !ok {
			return "", fmt.Errorf("no forms/%s.yaml", name)
		}
		js, _ := assets.URL("hotdog-forms.js")
		css, _ := assets.URL("hotdog-forms.css")
		return f.Render(js, css)
	}
	return loadTemplates(siteDir, fm)
}

// RenderPage renders one page of a site with its source replaced by content,
// without building the rest: the live preview of a page being edited. Asset
// addresses match a full build of the same site, so the page can be shown
// beside that build's files. It returns the HTML and the page's address.
func RenderPage(siteDir, source string, content []byte, opt Options) ([]byte, string, error) {
	rel := strings.TrimPrefix(filepath.ToSlash(source), "content/")
	s, err := site.Load(siteDir, site.Options{Drafts: true, URL: opt.URL, Now: opt.Now, Overrides: map[string][]byte{rel: content}, Look: opt.Look})
	if err != nil {
		return nil, "", err
	}
	assignCards(s)
	tpl, err := prepareWith(siteDir, s, "", opt.Forms)
	if err != nil {
		return nil, "", err
	}
	for _, p := range s.Pages {
		if p.Source != rel {
			continue
		}
		if p.RedirectTo != "" {
			var buf bytes.Buffer
			err := redirectTpl.Execute(&buf, p.RedirectTo)
			return buf.Bytes(), p.Path, err
		}
		layout, err := tpl.layoutFor(p, s.Config)
		if err != nil {
			return nil, "", err
		}
		if err := tpl.checkSections(p); err != nil {
			return nil, "", err
		}
		buf := &capped{max: maxPage}
		if err := tpl.layouts[layout].ExecuteTemplate(buf, "base", p); err != nil {
			return nil, "", fmt.Errorf("layout %s: %w", layout, err)
		}
		return buf.Bytes(), p.Path, nil
	}
	return nil, "", fmt.Errorf("%s isn't a page of this site", source)
}

// maxPage bounds one rendered page. A template is something anyone who can
// write to the site can change, and a loop in one shouldn't be able to use
// up the memory of the machine building it (the editor builds everyone's
// drafts).
const maxPage = 32 << 20

// capped is a buffer that refuses to grow past max.
type capped struct {
	bytes.Buffer
	max int
}

func (c *capped) Write(p []byte) (int, error) {
	if c.Len()+len(p) > c.max {
		return 0, fmt.Errorf("the page is over %d MB; is a template looping?", c.max>>20)
	}
	return c.Buffer.Write(p)
}

func pageFile(root, urlPath string) string {
	// path.Clean of an absolute path can't climb above it: whatever reaches
	// here stays inside root.
	clean := path.Clean("/" + urlPath)
	if strings.HasSuffix(urlPath, "/") {
		return filepath.Join(root, filepath.FromSlash(clean), "index.html")
	}
	return filepath.Join(root, filepath.FromSlash(clean))
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
		return fmt.Errorf("%s has files that hotdog-cms did not write; refusing to replace it", out)
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
// or an earlier hotdog-cms build.
func Install(src, dst string) error {
	if _, err := os.Stat(filepath.Join(src, marker)); err != nil {
		return fmt.Errorf("%s is not a hotdog-cms build", src)
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

// LookCSS is the look stylesheet a site would get with these choices, and the
// address a build would publish it at: for previewing a look before it's
// saved. Empty when the choices change nothing.
func LookCSS(siteDir string, look map[string]any) (string, []byte, error) {
	lf, err := site.LoadLook(siteDir)
	if err != nil || lf == nil {
		return "", nil, err
	}
	vals, err := lf.Values(look)
	if err != nil {
		return "", nil, err
	}
	css := lf.CSS(vals)
	if css == nil {
		return "", nil, nil
	}
	a := &Assets{urls: map[string]string{}}
	if err := a.add("", "look.css", css); err != nil {
		return "", nil, err
	}
	u, _ := a.URL("look.css")
	return u, css, nil
}
