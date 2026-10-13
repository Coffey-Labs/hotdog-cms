package site

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeSite(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(body), 0o644)
	}
	return dir
}

func titles(items []*NavItem) string {
	var out []string
	for _, it := range items {
		out = append(out, it.Title)
	}
	return strings.Join(out, ", ")
}

// A docs collection with path: / sits at the root of its host, as a site
// moved from MkDocs does, and the tree places each page in it.
func TestNavTreeAtRoot(t *testing.T) {
	dir := writeSite(t, map[string]string{
		"site.yaml":                  "name: T\nurl: https://docs.example.org\ncollections:\n  docs:\n    path: /\n",
		"content/docs/_index.md":     "---\ntitle: Home\n---\nRead [sizing](run/sizing.md).\n",
		"content/docs/run/_index.md": "---\ntitle: Running it\n---\n",
		"content/docs/run/sizing.md": "---\ntitle: Sizing\n---\n",
		"content/docs/run/backup.md": "---\ntitle: Backups\n---\n",
		"content/docs/run/later.md":  "---\ntitle: Later\ndraft: true\n---\n",
		"content/docs/using.md":      "---\ntitle: Getting started\n---\n",
		"content/docs/away.md":       "---\ntitle: Elsewhere\nurl: /elsewhere/\n---\n",
		"data/docs-nav.yaml": `
- page: docs/_index.md
- docs/using.md
- title: Run it
  page: docs/run/_index.md
  icon: server
  children:
    - docs/run/sizing.md
    - docs/run/later.md
    - title: Compliance
      children:
        - title: Backups and undelete
          page: docs/run/backup.md
- title: Back to the site
  url: https://example.org/
`,
	})
	s, err := Load(dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	for src, want := range map[string]string{"docs/_index.md": "/", "docs/run/sizing.md": "/run/sizing/", "docs/run/_index.md": "/run/", "docs/using.md": "/using/", "docs/away.md": "/elsewhere/"} {
		if p := s.PageFrom(src); p == nil || p.Path != want {
			t.Errorf("%s: want %s, got %+v", src, want, p)
		}
	}
	if c := string(s.PageFrom("docs/_index.md").Content); !strings.Contains(c, `href="/run/sizing/"`) {
		t.Errorf("link to a moved page: %s", c)
	}

	nav, err := s.NavFor("docs-nav", s.PageFrom("docs/run/backup.md"))
	if err != nil {
		t.Fatal(err)
	}
	if got := titles(nav.Items); got != "Home, Getting started, Run it, Back to the site" {
		t.Errorf("top level: %s", got)
	}
	if got := titles(nav.Items[2].Children); got != "Sizing, Compliance" {
		t.Errorf("a draft is left out: %s", got)
	}
	if got := titles(nav.Trail); got != "Run it, Compliance" {
		t.Errorf("trail: %s", got)
	}
	if nav.Here == nil || nav.Here.Title != "Backups and undelete" || !nav.Here.Here {
		t.Errorf("here: %+v", nav.Here)
	}
	if nav.Prev == nil || nav.Prev.Title != "Sizing" || nav.Next != nil {
		t.Errorf("prev %+v next %+v", nav.Prev, nav.Next)
	}
	if nav.Items[2].Params["icon"] != "server" || len(nav.Items[0].Params) != 0 {
		t.Errorf("params: %v, %v", nav.Items[2].Params, nav.Items[0].Params)
	}
	if !nav.Items[2].Open || nav.Items[1].Open {
		t.Error("only the headings over the page are open")
	}
	if out := nav.Items[3]; !out.External || out.URL != "https://example.org/" {
		t.Errorf("link elsewhere: %+v", out)
	}

	nav, _ = s.NavFor("docs-nav", s.PageFrom("docs/run/_index.md"))
	if nav.Prev == nil || nav.Prev.Title != "Getting started" || nav.Next == nil || nav.Next.Title != "Sizing" {
		t.Errorf("a heading's page comes before what's under it: prev %+v next %+v", nav.Prev, nav.Next)
	}
}

// The docs pack's first format, parts and page names, reads as a tree.
func TestNavPartsFormat(t *testing.T) {
	dir := writeSite(t, map[string]string{
		"site.yaml":                        "name: T\nurl: https://example.org\n",
		"content/docs/_index.md":           "---\ntitle: Docs\n---\n",
		"content/docs/start/_index.md":     "---\ntitle: Start here\n---\n",
		"content/docs/start/what-it-is.md": "---\ntitle: What it is\n---\n",
		"content/docs/start/how/index.md":  "---\ntitle: How it works\n---\n",
		"content/docs/install/_index.md":   "---\ntitle: Install\n---\n",
		"content/docs/install/linux.md":    "---\ntitle: Linux\n---\n",
		"data/docs-nav.yaml": `
- part: start
  title: Start here
  blurb: What it is.
  pages: [what-it-is, how, missing]
- part: install
  title: Install
  pages: [linux]
`,
	})
	s, err := Load(dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	nav, err := s.NavFor("docs-nav", s.PageFrom("docs/start/how/index.md"))
	if err != nil {
		t.Fatal(err)
	}
	if got := titles(nav.Items[0].Children); got != "What it is, How it works" {
		t.Errorf("pages: %s", got)
	}
	if nav.Items[0].URL != "/docs/start/" || nav.Items[0].Blurb != "What it is." {
		t.Errorf("part: %+v", nav.Items[0])
	}
	if nav.Prev.Title != "What it is" || nav.Next.Title != "Install" {
		t.Errorf("prev %+v next %+v", nav.Prev, nav.Next)
	}
}

func TestCollectionPathChecks(t *testing.T) {
	dir := writeSite(t, map[string]string{
		"site.yaml":              "name: T\nurl: https://example.org\ncollections:\n  docs:\n    path: /\n",
		"content/index.md":       "---\ntitle: Home\n---\n",
		"content/docs/_index.md": "---\ntitle: Docs\n---\n",
	})
	if _, err := Load(dir, Options{}); err == nil || !strings.Contains(err.Error(), "both want /") {
		t.Errorf("a docs home at / beside the site's home: %v", err)
	}
	for _, bad := range []string{"docs", "/docs", "/../x/"} {
		dir := writeSite(t, map[string]string{
			"site.yaml":        "name: T\nurl: https://example.org\ncollections:\n  docs:\n    path: " + bad + "\n",
			"content/index.md": "x\n",
		})
		if _, err := Load(dir, Options{}); err == nil || !strings.Contains(err.Error(), "should be a folder") {
			t.Errorf("path %q: %v", bad, err)
		}
	}
}
