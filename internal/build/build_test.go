package build

import (
	"fmt"
	"image/png"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/site"
	"git.coffeylabs.org/coffey-labs/hotdog-cms/starter"
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
		"tags/hotdog-cms/index.html", "404.html", "sitemap.xml", "robots.txt", "articles/feed.xml", "llms.txt"} {
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
		t.Fatal("replaced a folder hotdog-cms did not write")
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
	if !strings.Contains(read("old/index.html"), "url=/about/") || !strings.Contains(read(".hotdog-cms-redirects"), "/old/ /about/") {
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

func TestHTMLBodiesAndRedirectTo(t *testing.T) {
	dir := writeStarter(t)
	page := "---\ntitle: Raw\nformat: html\n---\n<section class=\"x\">\n\n    <p>indented, after a blank line</p>\n<script>alert(1)</script>\n</section>\n"
	os.WriteFile(filepath.Join(dir, "content", "raw.md"), []byte(page), 0o644)
	// Without trust_html or unsafe_html, an HTML body is refused.
	if _, err := Run(Options{SiteDir: dir}); err == nil || !strings.Contains(err.Error(), "trust_html") {
		t.Fatalf("HTML body accepted without permission: %v", err)
	}
	cfgPath := filepath.Join(dir, "site.yaml")
	base, _ := os.ReadFile(cfgPath)
	// Sanitized: the script goes, the indented paragraph stays a paragraph.
	os.WriteFile(cfgPath, append(base, []byte("markdown:\n  unsafe_html: true\n")...), 0o644)
	res, err := Run(Options{SiteDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	out, _ := os.ReadFile(filepath.Join(res.Out, "raw", "index.html"))
	if strings.Contains(string(out), "<script>alert") || strings.Contains(string(out), "<pre><code>") || !strings.Contains(string(out), "<p>indented, after a blank line</p>") {
		t.Errorf("sanitized HTML body wrong:\n%s", out)
	}
	// A link post: listed and in the feed at its destination, built as a redirect, not in the sitemap.
	os.WriteFile(cfgPath, base, 0o644)
	os.Remove(filepath.Join(dir, "content", "raw.md"))
	os.WriteFile(filepath.Join(dir, "content", "articles", "elsewhere.md"), []byte("---\ntitle: Written elsewhere\ndate: 2026-10-11\nredirect_to: https://example.net/post/\n---\n"), 0o644)
	if res, err = Run(Options{SiteDir: dir}); err != nil {
		t.Fatal(err)
	}
	read := func(p string) string { b, _ := os.ReadFile(filepath.Join(res.Out, p)); return string(b) }
	if !strings.Contains(read("articles/elsewhere/index.html"), "url=https://example.net/post/") {
		t.Error("redirect_to page is not a redirect")
	}
	if strings.Contains(read("sitemap.xml"), "/articles/elsewhere/") {
		t.Error("redirect_to page is in the sitemap")
	}
	if !strings.Contains(read("articles/feed.xml"), "https://example.net/post/") {
		t.Error("feed doesn't link the redirect_to destination")
	}
}

func TestLook(t *testing.T) {
	dir := writeStarter(t)
	res, err := Run(Options{SiteDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	home, _ := os.ReadFile(filepath.Join(res.Out, "index.html"))
	if strings.Contains(string(home), "/look.") || strings.Contains(string(home), "brand-logo") {
		t.Error("a site that changes nothing gets a look stylesheet or a logo")
	}

	cfg, _ := os.ReadFile(filepath.Join(dir, "site.yaml"))
	os.WriteFile(filepath.Join(dir, "site.yaml"), append(cfg, []byte("look:\n  accent: \"#0a7d55\"\n  font: serif\n  logo: /logo.svg\n")...), 0o644)
	if res, err = Run(Options{SiteDir: dir}); err != nil {
		t.Fatal(err)
	}
	home, _ = os.ReadFile(filepath.Join(res.Out, "index.html"))
	link := regexp.MustCompile(`<link rel="stylesheet" href="(/look\.[0-9a-f]{10}\.css)">`).FindStringSubmatch(string(home))
	if link == nil || !strings.Contains(string(home), `<img class="brand-logo" src="/logo.svg" alt="">`) {
		t.Fatalf("home page:\n%s", home)
	}
	css, err := os.ReadFile(filepath.Join(res.Out, link[1]))
	if err != nil || !strings.Contains(string(css), "--accent: #0a7d55;") || !strings.Contains(string(css), "--font: Charter") {
		t.Errorf("look.css: %v\n%s", err, css)
	}
	// LookCSS, for the editor's preview, names the same file.
	u, data, err := LookCSS(dir, map[string]any{"accent": "#0a7d55", "font": "serif", "logo": "/logo.svg"})
	if err != nil || u != link[1] || string(data) != string(css) {
		t.Errorf("LookCSS: %s %v", u, err)
	}

	// A choice the theme doesn't allow stops the build, saying why.
	os.WriteFile(filepath.Join(dir, "site.yaml"), append(cfg, []byte("look:\n  font: comic\n")...), 0o644)
	if _, err := Run(Options{SiteDir: dir}); err == nil || !strings.Contains(err.Error(), `"comic" isn't one of`) {
		t.Errorf("bad look: %v", err)
	}
}

func TestSocialCards(t *testing.T) {
	dir := writeStarter(t)
	// A page with its own picture keeps it.
	about := filepath.Join(dir, "content", "about.md")
	src, _ := os.ReadFile(about)
	os.WriteFile(about, []byte(strings.Replace(string(src), "title: About", "title: About\nimage: /own.png\nimage_alt: Our own", 1)), 0o644)
	res, err := Run(Options{SiteDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	home, _ := os.ReadFile(filepath.Join(res.Out, "index.html"))
	m := regexp.MustCompile(`<meta property="og:image" content="https://example.org(/cards/home\.[0-9a-f]{8}\.png)">`).FindStringSubmatch(string(home))
	if m == nil || !strings.Contains(string(home), `<meta property="og:image:alt" content="Home">`) {
		t.Fatalf("home head:\n%s", home)
	}
	f, err := os.Open(filepath.Join(res.Out, m[1]))
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(f)
	f.Close()
	if err != nil || img.Bounds().Dx() != 1200 || img.Bounds().Dy() != 630 {
		t.Fatalf("card: %v %v", err, img.Bounds())
	}
	// The site's band color fills it; the accent runs down the left edge.
	if r, g, b, _ := img.At(600, 20).RGBA(); r>>8 != 0x1d || g>>8 != 0x2b || b>>8 != 0x3a {
		t.Errorf("background %x %x %x", r>>8, g>>8, b>>8)
	}
	aboutHTML, _ := os.ReadFile(filepath.Join(res.Out, "about", "index.html"))
	if !strings.Contains(string(aboutHTML), `content="https://example.org/own.png"`) || strings.Contains(string(aboutHTML), "/cards/") {
		t.Error("a page with its own image got a card")
	}
	if _, err := os.Stat(filepath.Join(res.Out, "articles", "hello-world")); err != nil {
		t.Fatal(err)
	}
	cards, _ := filepath.Glob(filepath.Join(res.Out, "cards", "*.png"))
	// Home, the articles list, the article, contact; not 404, tags or about.
	if len(cards) != 4 {
		t.Errorf("%d cards: %v", len(cards), cards)
	}
	// A new title is a new address, so previews refresh.
	idx := filepath.Join(dir, "content", "index.md")
	src, _ = os.ReadFile(idx)
	os.WriteFile(idx, []byte(strings.Replace(string(src), "title: Home", "title: Welcome", 1)), 0o644)
	res, _ = Run(Options{SiteDir: dir})
	home2, _ := os.ReadFile(filepath.Join(res.Out, "index.html"))
	if strings.Contains(string(home2), m[1]) {
		t.Error("the card's address didn't change with its title")
	}
}
