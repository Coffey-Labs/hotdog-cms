package site

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPageLinks(t *testing.T) {
	all := make([]*Page, 10)
	for i := range all {
		all[i] = &Page{Path: fmt.Sprintf("/a/page/%d/", i+1)}
	}
	row := func(n int) string {
		var b []string
		for _, l := range (&Paginator{Number: n, TotalPages: 10, All: all}).Links(1) {
			switch {
			case l.Gap:
				b = append(b, "…")
			case l.Current:
				b = append(b, fmt.Sprintf("[%d]", l.Number))
			default:
				b = append(b, fmt.Sprint(l.Number))
			}
		}
		return strings.Join(b, " ")
	}
	for n, want := range map[int]string{
		1:  "[1] 2 … 10",
		5:  "1 … 4 [5] 6 … 10",
		3:  "1 2 [3] 4 … 10",
		10: "1 … 9 [10]",
	} {
		if got := row(n); got != want {
			t.Errorf("page %d: %q, want %q", n, got, want)
		}
	}
}

func TestNoLinks(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "static"), 0o755)
	os.WriteFile(filepath.Join(dir, "site.yaml"), []byte("name: x\nurl: https://example.org\n"), 0o644)
	os.Symlink("/etc/passwd", filepath.Join(dir, "static", "passwd.txt"))
	if _, err := Load(dir, Options{}); err == nil || !strings.Contains(err.Error(), "static/passwd.txt is a symbolic link") {
		t.Errorf("loaded a site with a link in it: %v", err)
	}
}

func TestMarkdownAttributes(t *testing.T) {
	md := NewMarkdown(MarkdownConfig{}, "example.org")
	h, err := md.Render([]byte(`## Hi {#top .big style="background:url(https://evil.example/x)" onclick="x"}`))
	if err != nil {
		t.Fatal(err)
	}
	if s := string(h); !strings.Contains(s, `id="top"`) || !strings.Contains(s, `class="big"`) || strings.Contains(s, "style") || strings.Contains(s, "onclick") {
		t.Errorf("attributes: %s", s)
	}
}

// A folder's _index.md inside a collection is that folder's page: it lists
// the folder's pages, and the collection's own page keeps the collection.
func TestFolderListPages(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"site.yaml":                     "name: T\nurl: https://example.org\npaginate: 2\n",
		"content/docs/_index.md":        "---\ntitle: Docs\n---\n",
		"content/docs/a.md":             "---\ntitle: A\n---\n",
		"content/docs/b/_index.md":      "---\ntitle: B\n---\n",
		"content/docs/b/one.md":         "---\ntitle: One\n---\n",
		"content/docs/b/two.md":         "---\ntitle: Two\n---\n",
		"content/docs/b/three/index.md": "---\ntitle: Three\n---\n",
		"content/docs/b/c/deep.md":      "---\ntitle: Deep\n---\n",
	}
	for name, body := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(body), 0o644)
	}
	s, err := Load(dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if l := s.Lists["docs"]; l == nil || l.Path != "/docs/" || len(l.Pages) != 5 {
		t.Fatalf("the docs' list: %+v", l)
	}
	b := s.PageAt("/docs/b/")
	var got []string
	for _, p := range b.Pages {
		got = append(got, p.Title)
	}
	if strings.Join(got, ",") != "One,Three,Two" && strings.Join(got, ",") != "One,Two,Three" {
		t.Errorf("/docs/b/ lists %v", got)
	}
	if b.Paginator.TotalPages != 1 || len(b.Paginator.Items) != 3 {
		t.Errorf("/docs/b/ is split: %+v", b.Paginator)
	}
	for _, p := range s.Pages {
		if strings.HasPrefix(p.Path, "/docs/b/page/") {
			t.Errorf("stray page %s", p.Path)
		}
	}
	if s.PageAt("/docs/page/2/") == nil {
		t.Error("the docs' own list is still split")
	}
}

// paginate: 0 on a collection keeps its list on one page whatever the site's.
func TestPaginateOff(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"site.yaml":              "name: T\nurl: https://example.org\npaginate: 2\ncollections:\n  docs:\n    paginate: 0\n",
		"content/docs/_index.md": "---\ntitle: Docs\n---\n",
		"content/docs/a.md":      "---\ntitle: A\n---\n",
		"content/docs/b.md":      "---\ntitle: B\n---\n",
		"content/docs/c.md":      "---\ntitle: C\n---\n",
		"content/news/a.md":      "---\ntitle: A\n---\n",
		"content/news/b.md":      "---\ntitle: B\n---\n",
		"content/news/c.md":      "---\ntitle: C\n---\n",
	}
	for name, body := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(body), 0o644)
	}
	s, err := Load(dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if s.PageAt("/docs/page/2/") != nil || len(s.PageAt("/docs/").Paginator.Items) != 3 {
		t.Error("the docs were split")
	}
	if s.PageAt("/news/page/2/") == nil {
		t.Error("the site's paginate no longer applies")
	}
}
