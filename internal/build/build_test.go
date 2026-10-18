package build

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
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

// A tag with punctuation links to its own page: "LINUX.com" lives at
// /tags/linuxcom/, not /tags/linux-com/.
func TestStarterTagLinks(t *testing.T) {
	dir := writeStarter(t)
	page := "---\ntitle: News\ndate: 2026-08-10\ntags: [LINUX.com, Open Source]\n---\n\nHello.\n"
	if err := os.WriteFile(filepath.Join(dir, "content", "articles", "news.md"), []byte(page), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := Run(Options{SiteDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	out, _ := os.ReadFile(filepath.Join(res.Out, "articles", "news", "index.html"))
	for _, want := range []string{`href="/tags/linuxcom/"`, `href="/tags/open-source/"`} {
		if !strings.Contains(string(out), want) {
			t.Errorf("article doesn't link %s", want)
		}
	}
	for _, p := range []string{"tags/linuxcom/index.html", "tags/open-source/index.html"} {
		if _, err := os.Stat(filepath.Join(res.Out, p)); err != nil {
			t.Errorf("missing %s", p)
		}
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
	cfg = []byte(strings.Replace(string(cfg), "\npaginate: 12\n", "\npaginate: 5\n", 1))
	extra := "\nterm_index_min: 20\nredirects:\n  - { from: /old/, to: /about/ }\n"
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
	p2 := read("articles/page/2/index.html")
	for _, want := range []string{"<title>Articles · page 2 · Test</title>", `<link rel="prev" href="https://example.org/articles/">`, `<link rel="next" href="https://example.org/articles/page/3/">`, `<a href="/articles/page/2/" aria-current="page">2</a>`, `<a class="older" href="/articles/page/3/" rel="next">`} {
		if !strings.Contains(p2, want) {
			t.Errorf("page 2 lacks %s", want)
		}
	}
	if strings.Count(p2, `<li class="card">`) != 6 {
		t.Errorf("page 2 lists %d cards, want 6", strings.Count(p2, `<li class="card">`))
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
	// Home, the articles list, the article, contact, privacy; not 404, tags or about.
	if len(cards) != 5 {
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

	// A card the last build drew is copied, not drawn again: its name says
	// what's on it.
	homeCard := func() string {
		h, _ := os.ReadFile(filepath.Join(dir, "public", "index.html"))
		m := regexp.MustCompile(`/cards/home\.[0-9a-f]{8}\.png`).FindString(string(h))
		if m == "" {
			t.Fatal("no home card")
		}
		return m
	}
	c2 := homeCard()
	os.WriteFile(filepath.Join(dir, "public", c2), []byte("kept"), 0o644)
	if _, err := Run(Options{SiteDir: dir}); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "public", c2)); string(b) != "kept" {
		t.Error("the card was drawn again")
	}

	// A logo replaced under the same name draws a different card, so it
	// gets a new name.
	logo := func(c color.Color) {
		img := image.NewRGBA(image.Rect(0, 0, 4, 4))
		draw.Draw(img, img.Bounds(), &image.Uniform{c}, image.Point{}, draw.Src)
		f, _ := os.Create(filepath.Join(dir, "static", "logo.png"))
		png.Encode(f, img)
		f.Close()
	}
	cfg, _ := os.ReadFile(filepath.Join(dir, "site.yaml"))
	os.WriteFile(filepath.Join(dir, "site.yaml"), []byte(strings.Replace(string(cfg), "cards: true", "cards:\n  logo: /logo.png", 1)), 0o644)
	logo(color.RGBA{255, 0, 0, 255})
	if _, err := Run(Options{SiteDir: dir}); err != nil {
		t.Fatal(err)
	}
	c3 := homeCard()
	logo(color.RGBA{0, 0, 255, 255})
	if _, err := Run(Options{SiteDir: dir}); err != nil {
		t.Fatal(err)
	}
	if c4 := homeCard(); c3 == c4 || c3 == c2 {
		t.Errorf("cards %s, %s, %s: a new logo should be a new name", c2, c3, c4)
	}
}

func TestHumanSize(t *testing.T) {
	for n, want := range map[int64]string{0: "0 bytes", 999: "999 bytes", 1000: "1 KB", 14400: "14 KB", 999_499: "999 KB", 1_000_000: "1 MB", 1_250_000: "1.2 MB", 2_000_000_000: "2 GB"} {
		if got := humanSize(n); got != want {
			t.Errorf("humanSize(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestHeadingsAndMinutes(t *testing.T) {
	h := headings(`<h2 id="a">First <em>part</em></h2><p>x</p><h3 id="b">Detail</h3><h4 id="c">No</h4><h2>No id</h2>`)
	if len(h) != 2 || h[0] != (Heading{2, "a", "First part"}) || h[1] != (Heading{3, "b", "Detail"}) {
		t.Errorf("headings: %+v", h)
	}
	for in, want := range map[any]string{15: "PT15M", 60: "PT1H", 75: "PT1H15M", 0: "", "x": ""} {
		if got := minutes(in); got != want {
			t.Errorf("minutes(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestMoney(t *testing.T) {
	for _, c := range []struct {
		v    any
		cur  string
		want string
	}{{24, "USD", "$24.00"}, {24.5, "eur", "€24.50"}, {"1200", "JPY", "¥1200"}, {9, "SEK", "9.00 SEK"}, {7, "", "$7.00"}, {"Ask us", "USD", "Ask us"}} {
		if got := money(c.v, c.cur); got != c.want {
			t.Errorf("money(%v, %q) = %q, want %q", c.v, c.cur, got, c.want)
		}
	}
}

func TestPlainText(t *testing.T) {
	for in, want := range map[string]string{
		"Give us **three days' notice**, and *tell* us.": "Give us three days' notice, and tell us.",
		"See [our menu](/menu/) or `call`.":              "See our menu or call.",
		"- one\n- two\n\n> quoted":                       "one two quoted",
		"snake_case stays, _this_ goes.":                 "snake_case stays, this goes.",
	} {
		if got := plainText(in); got != want {
			t.Errorf("plainText(%q) = %q, want %q", in, got, want)
		}
	}
}

// A search index carries each page's section headings, so a result can go
// to the section.
func TestSearchHeadings(t *testing.T) {
	dir := writeStarter(t)
	os.MkdirAll(filepath.Join(dir, "content", "docs"), 0o755)
	os.WriteFile(filepath.Join(dir, "content", "docs", "install.md"), []byte("---\ntitle: Install\n---\n## On Linux\n\nx\n\n### With *Docker*\n\ny\n"), 0o644)
	cfg, _ := os.ReadFile(filepath.Join(dir, "site.yaml"))
	os.WriteFile(filepath.Join(dir, "site.yaml"), []byte(strings.Replace(string(cfg), "collections:\n", "collections:\n  docs:\n    search: true\n", 1)), 0o644)
	res, err := Run(Options{SiteDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(res.Out, "docs", "search.json"))
	if !strings.Contains(string(b), `"h":[["on-linux","On Linux"],["with-docker","With Docker"]]`) {
		t.Errorf("search.json has no headings:\n%s", b)
	}
}

// A picture beside a page goes out beside it, where its collection's path:
// puts the page; folders with nothing to publish aren't written.
func TestContentFilesFollowThePath(t *testing.T) {
	dir := writeStarter(t)
	os.MkdirAll(filepath.Join(dir, "content", "docs", "run"), 0o755)
	os.MkdirAll(filepath.Join(dir, "content", "docs", "empty"), 0o755)
	os.WriteFile(filepath.Join(dir, "content", "docs", "run", "sizing.md"), []byte("---\ntitle: Sizing\n---\n![Chart](chart.png)\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "content", "docs", "run", "chart.png"), []byte("png"), 0o644)
	os.WriteFile(filepath.Join(dir, "content", "docs", "empty", "only.md"), []byte("---\ntitle: Only\n---\n"), 0o644)
	os.Remove(filepath.Join(dir, "content", "index.md"))
	cfg, _ := os.ReadFile(filepath.Join(dir, "site.yaml"))
	os.WriteFile(filepath.Join(dir, "site.yaml"), []byte(strings.Replace(string(cfg), "collections:\n", "collections:\n  docs:\n    path: /\n", 1)), 0o644)
	res, err := Run(Options{SiteDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(res.Out, "run", "chart.png")); err != nil {
		t.Errorf("the picture isn't beside its page at /run/: %v", err)
	}
	if _, err := os.Stat(filepath.Join(res.Out, "docs")); err == nil {
		t.Error("an empty /docs/run/ or /docs/empty/ was written")
	}
	filepath.WalkDir(res.Out, func(p string, d fs.DirEntry, err error) error {
		if d != nil && d.IsDir() {
			if entries, _ := os.ReadDir(p); len(entries) == 0 {
				t.Errorf("empty folder %s", p)
			}
		}
		return nil
	})
}

// data reads files in folders under data/, one by path or a folder at once.
func TestDataFolderFunc(t *testing.T) {
	dir := writeStarter(t)
	os.MkdirAll(filepath.Join(dir, "data", "exams"), 0o755)
	os.WriteFile(filepath.Join(dir, "data", "exams", "alpha.yaml"), []byte("questions: 60\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "data", "exams", "beta.yaml"), []byte("questions: 40\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "layouts", "exams.html"),
		[]byte(`{{ define "main" }}one={{ (data "exams/alpha").questions }};{{ range $k, $v := data "exams" }}{{ $k }}={{ $v.questions }};{{ end }}{{ end }}`), 0o644)
	os.WriteFile(filepath.Join(dir, "content", "exams.md"), []byte("---\ntitle: Exams\nlayout: exams\n---\n"), 0o644)
	res, err := Run(Options{SiteDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	out, _ := os.ReadFile(filepath.Join(res.Out, "exams", "index.html"))
	if !strings.Contains(string(out), "one=60;alpha=60;beta=40;") {
		t.Errorf("data folders: %s", out)
	}
}
