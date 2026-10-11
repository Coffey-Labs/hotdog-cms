package packs

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/build"
	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/site"
	"git.coffeylabs.org/coffey-labs/hotdog-cms/starter"
)

func write(t *testing.T, dir string, files map[string]string) string {
	t.Helper()
	for rel, body := range files {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(body), 0o644)
	}
	return dir
}

func starterSite(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	fs.WalkDir(starter.Files, "site", func(p string, d fs.DirEntry, err error) error {
		target := filepath.Join(dir, strings.TrimPrefix(p, "site"))
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, _ := starter.Files.ReadFile(p)
		return os.WriteFile(target, []byte(strings.ReplaceAll(string(data), "{{SITE_NAME}}", "Test")), 0o644)
	})
	return dir
}

const faqSection = `{{/* section
label: FAQ
fields:
  - name: items
    type: list
    fields:
      - { name: q, type: text, label: Question }
      - { name: a, type: markdown, label: Answer }
*/}}<link rel="stylesheet" href="{{ asset "packs/faq/faq.css" }}">
<section class="faq">{{ range .Data.items }}<details><summary>{{ .q }}</summary>{{ markdownify .a }}</details>{{ end }}</section>`

func TestPacks(t *testing.T) {
	pack := write(t, t.TempDir(), map[string]string{
		"pack.yaml":         "name: faq\nversion: 1.0.0\ntitle: FAQ\nlicense: MIT\n",
		"sections/faq.html": faqSection,
		"assets/faq.css":    ".faq details { margin: 1rem 0; }\n",
		"assets/old.css":    "/* only in 1.0 */\n",
		"README.md":         "# FAQ pack\n",
	})
	m, files, err := Check(pack)
	if err != nil || m.Name != "faq" || len(files) != 3 || files[0].To != "assets/packs/faq/faq.css" {
		t.Fatalf("check: %v %+v", err, files)
	}
	site1 := starterSite(t)
	ref, err := Add(site1, pack, false)
	if err != nil {
		t.Fatal(err)
	}
	if ref.Version != "1.0.0" || len(ref.Files) != 3 {
		t.Fatalf("ref: %+v", ref)
	}
	cfg, _ := site.LoadConfig(site1)
	if len(cfg.Packs) != 1 || cfg.Packs[0].Name != "faq" || cfg.Packs[0].Source != "" {
		t.Fatalf("site.yaml packs: %+v", cfg.Packs)
	}
	lib, _ := build.SectionLibrary(site1)
	for _, si := range lib {
		if (si.Name == "faq") != (si.Pack == "faq") {
			t.Errorf("section library: %s from pack %q", si.Name, si.Pack)
		}
	}
	// A page uses the pack's section, and the site builds.
	idx := filepath.Join(site1, "content", "index.md")
	src, _ := os.ReadFile(idx)
	os.WriteFile(idx, []byte(strings.Replace(string(src), "sections:\n", "sections:\n  - faq:\n      items:\n        - q: Is it free?\n          a: Yes, **AGPL**.\n", 1)), 0o644)
	res, err := build.Run(build.Options{SiteDir: site1})
	if err != nil {
		t.Fatal(err)
	}
	home, _ := os.ReadFile(filepath.Join(res.Out, "index.html"))
	if !strings.Contains(string(home), "<summary>Is it free?</summary><p>Yes, <strong>AGPL</strong>.</p>") || !strings.Contains(string(home), "/packs/faq/faq.") {
		t.Errorf("built page:\n%s", home)
	}

	// An upgrade replaces its files and drops the ones it no longer has.
	os.Remove(filepath.Join(pack, "assets", "old.css"))
	os.WriteFile(filepath.Join(pack, "pack.yaml"), []byte("name: faq\nversion: 1.1.0\n"), 0o644)
	if _, err := Add(site1, pack, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(site1, "assets", "packs", "faq", "old.css")); err == nil {
		t.Error("a file the new version dropped is still there")
	}
	cfg, _ = site.LoadConfig(site1)
	if cfg.Packs[0].Version != "1.1.0" || len(cfg.Packs[0].Files) != 2 {
		t.Errorf("after upgrade: %+v", cfg.Packs)
	}

	// Another pack can't claim the same file; a site's own file isn't overwritten.
	other := write(t, t.TempDir(), map[string]string{"pack.yaml": "name: other\nversion: 1.0.0\n", "sections/faq.html": "<p>{{ .Data.x }}</p>"})
	if _, err := Add(site1, other, false); err == nil || !strings.Contains(err.Error(), "already installed by the faq pack") {
		t.Errorf("clash between packs: %v", err)
	}
	own := write(t, t.TempDir(), map[string]string{"pack.yaml": "name: hero2\nversion: 1.0.0\n", "sections/hero.html": "<p>mine</p>"})
	if _, err := Add(site1, own, false); err == nil || !strings.Contains(err.Error(), "already in the site") {
		t.Errorf("overwrote the site's own file: %v", err)
	}

	// Removing it takes its files away and forgets it.
	if err := Remove(site1, "faq"); err != nil {
		t.Fatal(err)
	}
	cfg, _ = site.LoadConfig(site1)
	if _, err := os.Stat(filepath.Join(site1, "sections", "faq.html")); err == nil || len(cfg.Packs) != 0 {
		t.Errorf("after remove: %v %+v", err, cfg.Packs)
	}
	if raw, _ := os.ReadFile(filepath.Join(site1, "site.yaml")); strings.Contains(string(raw), "\npacks:") {
		t.Error("site.yaml still lists packs")
	}
}

func TestPacksCarryNoCode(t *testing.T) {
	bad := write(t, t.TempDir(), map[string]string{
		"pack.yaml":           "name: bad\nversion: 1.0.0\n",
		"sections/a.html":     `<div onclick="steal()">x</div>`,
		"sections/b.html":     `<script>alert(1)</script>`,
		"partials/c.html":     `<a href="javascript:alert(1)">x</a>`,
		"assets/tracker.js":   "steal()",
		"assets/logo.svg":     "<svg><script>x</script></svg>",
		"content/extra.md":    "not a pack's business",
		"sections/sub/d.html": "<p>nested</p>",
	})
	_, _, err := Check(bad)
	if err == nil {
		t.Fatal("a pack with code was accepted")
	}
	for _, want := range []string{"sections/a.html", "sections/b.html", "partials/c.html", "assets/tracker.js: JavaScript", "assets/logo.svg: SVG", "content/extra.md", "sections/sub/d.html"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("missing %q in:\n%v", want, err)
		}
	}
}

func TestBundledCalendar(t *testing.T) {
	if !Bundled("calendar") || Bundled("../calendar") || Bundled("nope") {
		t.Fatal("bundled pack names")
	}
	dir := starterSite(t)
	ref, err := Add(dir, "calendar", false)
	if err != nil {
		t.Fatal(err)
	}
	if ref.Source != "hotdog-cms:calendar" || len(ref.AddedCollections) != 1 || ref.Notes == "" {
		t.Errorf("ref: %+v", ref)
	}
	cfg, _ := site.LoadConfig(dir)
	if cc := cfg.Collections["events"]; !cc.Calendar || cc.Layout != "event" || cc.ListLayout != "events" {
		t.Errorf("events collection: %+v", cc)
	}
	if raw, _ := os.ReadFile(filepath.Join(dir, "site.yaml")); !strings.Contains(string(raw), "    # paginate: 20\n  events:\n") {
		t.Errorf("site.yaml's comments moved:\n%s", raw)
	}
	os.MkdirAll(filepath.Join(dir, "content", "events"), 0o755)
	os.WriteFile(filepath.Join(dir, "content", "events", "fair.md"), []byte("---\ntitle: Fair\nstart: 2099-05-01\n---\n"), 0o644)
	res, err := build.Run(build.Options{SiteDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	list, _ := os.ReadFile(filepath.Join(res.Out, "events", "index.html"))
	for _, want := range []string{"Coming up", `href="/events/fair/"`, "webcal://example.org/events/calendar.ics", "Friday, May 1, 2099"} {
		if !strings.Contains(string(list), want) {
			t.Errorf("events page lacks %q", want)
		}
	}
	// Installing again keeps the site's own settings for the collection.
	if ref, err := Add(dir, "calendar", false); err != nil || len(ref.AddedCollections) != 0 {
		t.Errorf("reinstall: %v %+v", err, ref)
	}
}
