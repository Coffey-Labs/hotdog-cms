package build

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"git.coffeylabs.org/coffey-labs/cl-cms/internal/site"
	"git.coffeylabs.org/coffey-labs/cl-cms/starter"
)

// writeStarter puts the starter site in a temporary folder.
func writeStarter(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	err := fs.WalkDir(starter.Files, "site", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		target := filepath.Join(dir, strings.TrimPrefix(p, "site"))
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, _ := starter.Files.ReadFile(p)
		return os.WriteFile(target, []byte(strings.ReplaceAll(string(data), "{{SITE_NAME}}", "Test")), 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestStarterBuilds(t *testing.T) {
	dir := writeStarter(t)
	res, err := Run(Options{SiteDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"index.html", "about/index.html", "articles/index.html", "articles/hello-world/index.html",
		"tags/cl-cms/index.html", "404.html", "sitemap.xml", "robots.txt", "articles/feed.xml", "llms.txt"} {
		if _, err := os.Stat(filepath.Join(res.Out, f)); err != nil {
			t.Errorf("missing %s", f)
		}
	}
	home, _ := os.ReadFile(filepath.Join(res.Out, "index.html"))
	if strings.Contains(string(home), `href="/styles.css"`) || !strings.Contains(string(home), `/styles.`) {
		t.Error("stylesheet is not fingerprinted")
	}
	// A second build replaces the first.
	if _, err := Run(Options{SiteDir: dir}); err != nil {
		t.Fatalf("rebuild: %v", err)
	}
}

func TestTemplatesEscape(t *testing.T) {
	dir := writeStarter(t)
	page := "---\ntitle: \"<script>alert(1)</script>\"\nsummary: '\"><img src=x onerror=alert(1)>'\n---\n\n<script>alert(2)</script>\n\n[x](javascript:alert(3))\n"
	if err := os.WriteFile(filepath.Join(dir, "content", "evil.md"), []byte(page), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := Run(Options{SiteDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	out, _ := os.ReadFile(filepath.Join(res.Out, "evil", "index.html"))
	for _, bad := range []string{"<script>alert", "<img src=x", "javascript:alert"} {
		if strings.Contains(string(out), bad) {
			t.Errorf("page contains %q", bad)
		}
	}
}

func TestRefusesForeignOutput(t *testing.T) {
	dir := writeStarter(t)
	other := t.TempDir()
	if err := os.WriteFile(filepath.Join(other, "precious.txt"), []byte("keep me"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(Options{SiteDir: dir, Out: other}); err == nil {
		t.Fatal("replaced a folder cl-cms did not write")
	}
	if _, err := os.Stat(filepath.Join(other, "precious.txt")); err != nil {
		t.Fatal("the folder's file is gone")
	}
	if _, err := Run(Options{SiteDir: dir, Out: filepath.Join(dir, "content", "out")}); err == nil {
		t.Fatal("wrote output inside content/")
	}
	if _, err := Run(Options{SiteDir: dir, Out: filepath.Dir(dir)}); err == nil {
		t.Fatal("wrote output over the site's parent")
	}
}

func TestPaginationTermsRedirectsRSS(t *testing.T) {
	dir := writeStarter(t)
	// Fourteen articles, tagged in three spellings of one subject.
	spellings := []string{"open source", "#OpenSource", "open-source"}
	for i := 0; i < 14; i++ {
		body := fmt.Sprintf("---\ntitle: Article %02d\ndate: 2026-09-%02d\ntags: [%q, linux]\n---\nSee https://example.com and [elsewhere](https://example.net/x).\n", i, i+1, spellings[i%3])
		os.WriteFile(filepath.Join(dir, "content", "articles", fmt.Sprintf("a%02d.md", i)), []byte(body), 0o644)
	}
	cfg, _ := os.ReadFile(filepath.Join(dir, "site.yaml"))
	extra := "\npaginate: 5\nterm_index_min: 20\nredirects:\n  - { from: /old/, to: /about/ }\n"
	cfg = []byte(strings.Replace(string(cfg), "    feed: true\n", "    feed: true\n    feed_format: rss\n    paginate: 6\n", 1) + extra)
	cfg = []byte(strings.Replace(string(cfg), "markdown:", "markdown:", 1))
	cfg = append(cfg, []byte("markdown:\n  autolink: false\n  external_links: new_tab\n")...)
	os.WriteFile(filepath.Join(dir, "site.yaml"), cfg, 0o644)

	res, err := Run(Options{SiteDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	read := func(p string) string { b, _ := os.ReadFile(filepath.Join(res.Out, p)); return string(b) }
	// 15 articles (14 + hello-world), 6 per page: 3 pages.
	for _, p := range []string{"articles/index.html", "articles/page/2/index.html", "articles/page/3/index.html"} {
		if read(p) == "" {
			t.Errorf("missing %s", p)
		}
	}
	if read("articles/page/4/index.html") != "" {
		t.Error("a fourth page of 15 items at 6 a page")
	}
	// Three spellings, one page, under the slug used most.
	if read("tags/open-source/index.html") == "" {
		t.Error("open source spellings did not share /tags/open-source/")
	}
	for _, p := range []string{"tags/opensource/index.html"} {
		if read(p) != "" {
			t.Errorf("%s exists; spellings were not merged", p)
		}
	}
	if !strings.Contains(read("tags/linux/index.html"), `content="noindex`) {
		t.Error("thin term page is not noindex")
	}
	if strings.Contains(read("sitemap.xml"), "/tags/linux/") {
		t.Error("noindex term page is in the sitemap")
	}
	if !strings.Contains(read("old/index.html"), "url=/about/") || !strings.Contains(read(".cl-cms-redirects"), "/old/ /about/") {
		t.Error("redirect not written")
	}
	if feed := read("articles/feed.xml"); !strings.Contains(feed, `<rss version="2.0"`) || !strings.Contains(feed, "<category>linux</category>") {
		t.Error("feed is not RSS with categories")
	}
	art := read("articles/a00/index.html")
	if strings.Contains(art, `href="https://example.com"`) {
		t.Error("bare URL was autolinked with autolink off")
	}
	if !strings.Contains(art, `href="https://example.net/x" target="_blank" rel="noopener noreferrer"`) {
		t.Error("external link does not open in a new tab")
	}
}

func TestOwnHostLinksStayInTab(t *testing.T) {
	md := site.NewMarkdown(site.MarkdownConfig{ExternalLinks: "new_tab"}, "example.org")
	h, _ := md.Render([]byte("[a](https://www.example.org/x) [b](https://example.net/) [c](/local/)"))
	if strings.Count(string(h), `target="_blank"`) != 1 || !strings.Contains(string(h), `href="https://example.net/" target="_blank"`) {
		t.Errorf("new-tab rule wrong: %s", h)
	}
}
