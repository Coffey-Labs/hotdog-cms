package site

import (
	"strings"
	"testing"
)

// A file in content/tags/ describes its term instead of being a collection,
// and tagged keeps the order it is given.
func TestTermDocsAndTagged(t *testing.T) {
	dir := writeSite(t, map[string]string{
		"site.yaml":                  "name: T\nurl: https://example.org\ntaxonomies: [tags]\ncollections:\n  products:\n    sort: weight\n",
		"content/products/b.md":      "---\ntitle: B\nweight: 2\ntags: [Open Source]\n---\n",
		"content/products/a.md":      "---\ntitle: A\nweight: 1\ntags: [open-source, Mugs]\n---\n",
		"content/products/c.md":      "---\ntitle: C\nweight: 3\ntags: [Mugs]\n---\n",
		"content/tags/_index.md":     "---\ntitle: Shelves\n---\nEvery shelf.\n",
		"content/tags/opensource.md": "---\ntitle: Free software\nlayout: shelf\nlook: green\n---\nShare it.\n",
		"content/tags/later.md":      "---\ntitle: Later\n---\nNothing yet.\n",
	})
	s, err := Load(dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Collections["tags"]; ok {
		t.Error("content/tags/ became a collection")
	}
	tp := s.TermPage("tags", "Open Source")
	if tp == nil {
		t.Fatal("no page for Open Source")
	}
	if tp.Title != "Free software" || tp.Layout != "shelf" || tp.Params["look"] != "green" || !strings.Contains(string(tp.Content), "Share it.") || tp.Source != "tags/opensource.md" {
		t.Errorf("term page = %q, layout %q, params %v, content %q, source %q", tp.Title, tp.Layout, tp.Params, tp.Content, tp.Source)
	}
	if len(tp.Pages) != 2 {
		t.Errorf("open source lists %d pages, want 2", len(tp.Pages))
	}
	var got []string
	for _, p := range s.Tagged("tags", "OPEN SOURCE", s.Collection("products")) {
		got = append(got, p.Title)
	}
	if strings.Join(got, ",") != "A,B" {
		t.Errorf("tagged = %v, want A,B in the collection's order", got)
	}
	if later := s.TermPage("tags", "later"); later == nil || later.Title != "Later" || len(later.Pages) != 0 {
		t.Errorf("a described term nobody uses: %+v", later)
	}
	for _, info := range s.Terms("tags") {
		if info.Slug == "later" {
			t.Error("an unused term is listed")
		}
	}
	paths := map[string]*Page{}
	for _, p := range s.Pages {
		if paths[p.Path] != nil {
			t.Errorf("two pages at %s", p.Path)
		}
		paths[p.Path] = p
	}
	if idx := paths["/tags/"]; idx == nil || idx.Title != "Shelves" {
		t.Errorf("tags index = %+v", idx)
	}
	if paths["/tags/later/"] == nil {
		t.Error("no /tags/later/")
	}
	if s.TermPage("tags", "nothing") != nil || s.Tagged("colors", "red", s.Collection("products")) != nil {
		t.Error("unknown terms or taxonomies should find nothing")
	}
}
