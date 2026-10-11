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
