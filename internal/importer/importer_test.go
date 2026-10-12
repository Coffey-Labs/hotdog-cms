package importer

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/build"
	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/check"
	"git.coffeylabs.org/coffey-labs/hotdog-cms/starter"
)

func starterSite(t *testing.T) string {
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

func jpg() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 800, 500))
	for i := range img.Pix {
		img.Pix[i] = uint8(i)
	}
	img.Set(1, 1, color.RGBA{200, 10, 10, 255})
	var b bytes.Buffer
	jpeg.Encode(&b, img, nil)
	return b.Bytes()
}

func read(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func fakeWordPress(t *testing.T, password string) *httptest.Server {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if password != "" && strings.HasPrefix(r.URL.Path, "/wp-json/") {
			if u, p, ok := r.BasicAuth(); !ok || u != "ed" || p != password {
				w.WriteHeader(401)
				return
			}
		}
		img := srv.URL + "/wp-content/uploads/2026/09/dock.jpg"
		post := func(id int, slug, title, content string, status string) map[string]any {
			return map[string]any{"id": id, "type": "post", "slug": slug, "status": status, "link": srv.URL + "/2026/09/" + slug + "/",
				"date_gmt": "2026-09-14T08:30:00", "modified_gmt": "2026-09-20T10:00:00", "parent": 0,
				"title": map[string]string{"rendered": title}, "content": map[string]string{"rendered": content},
				"excerpt": map[string]string{"rendered": "<p>A short summary [&hellip;]</p>"},
				"_embedded": map[string]any{"author": []map[string]string{{"name": "Ed Writer"}},
					"wp:featuredmedia": []map[string]string{{"source_url": img, "alt_text": "The dock"}},
					"wp:term":          [][]map[string]string{{{"name": "Uncategorized", "taxonomy": "category"}, {"name": "Boats", "taxonomy": "category"}}, {{"name": "Harbor &amp; sea", "taxonomy": "post_tag"}}}}}
		}
		switch r.URL.Path {
		case "/wp-json/wp/v2/posts":
			w.Header().Set("X-WP-TotalPages", "1")
			json.NewEncoder(w).Encode([]any{
				post(1, "hello-dock", "Hello &#8220;Dock&#8221;", `<!-- wp:paragraph --><p>Morning at the <strong>dock</strong>.</p><!-- /wp:paragraph --><figure class="wp-block-image"><img src="`+img+`" alt="Boats"><figcaption>Calm water</figcaption></figure>`, "publish"),
				post(2, "with-video", "With a video", `<p>Watch:</p><figure class="wp-block-embed"><iframe src="https://www.youtube.com/embed/x"></iframe></figure>`, "publish"),
			})
		case "/wp-json/wp/v2/pages":
			w.Header().Set("X-WP-TotalPages", "1")
			json.NewEncoder(w).Encode([]any{
				map[string]any{"id": 10, "type": "page", "slug": "company", "status": "publish", "link": srv.URL + "/company/", "parent": 0, "date_gmt": "2026-01-01T00:00:00",
					"title": map[string]string{"rendered": "Company"}, "content": map[string]string{"rendered": "<p>Who we are.</p>"}, "excerpt": map[string]string{"rendered": ""}},
				map[string]any{"id": 11, "type": "page", "slug": "team", "status": "publish", "link": srv.URL + "/company/team/", "parent": 10, "date_gmt": "2026-01-02T00:00:00",
					"title": map[string]string{"rendered": "Team"}, "content": map[string]string{"rendered": "<ul><li>Ed</li><li>Jo</li></ul>"}, "excerpt": map[string]string{"rendered": ""}},
			})
		case "/wp-content/uploads/2026/09/dock.jpg":
			w.Write(jpg())
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestWordPressImport(t *testing.T) {
	srv := fakeWordPress(t, "app pass word")
	if _, err := FromWordPress(WordPressOptions{URL: srv.URL, User: "ed", Password: "wrong"}); err == nil || !strings.Contains(err.Error(), "Application Password") {
		t.Errorf("bad password: %v", err)
	}
	items, err := FromWordPress(WordPressOptions{URL: srv.URL, User: "ed", Password: "app pass word"})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 4 || items[0].Title != "Hello “Dock”" || items[0].Summary != "A short summary" || strings.Join(items[0].Tags, ",") != "Boats,Harbor & sea" || items[3].Dir != "company" {
		t.Fatalf("items: %+v", items)
	}
	site := starterSite(t)
	rep, err := Write(items, Options{SiteDir: site, Base: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	post := read(t, filepath.Join(site, "content", "articles", "hello-dock.md"))
	for _, want := range []string{"title: Hello “Dock”", "date: \"2026-09-14\"", "updated: \"2026-09-20\"", "tags: [Boats, Harbor & sea]", "author: Ed Writer", "image: /media/2026/09/dock.jpg", "image_alt: The dock", "Morning at the **dock**.", "![Boats](/media/2026/09/dock.jpg)", "*Calm water*"} {
		if !strings.Contains(post, want) {
			t.Errorf("post missing %q:\n%s", want, post)
		}
	}
	if !strings.Contains(read(t, filepath.Join(site, "content", "articles", "with-video.md")), "format: html") || rep.KeptHTML["content/articles/with-video.md"] == nil {
		t.Error("the embed page wasn't kept as HTML")
	}
	if team := read(t, filepath.Join(site, "content", "company", "team.md")); !strings.Contains(team, "- Ed\n- Jo") {
		t.Errorf("team:\n%s", team)
	}
	if _, err := os.Stat(filepath.Join(site, "static", "media", "2026", "09", "dock.webp")); err != nil || rep.Pictures != 1 {
		t.Errorf("picture: %v, %d brought", err, rep.Pictures)
	}
	cfg := read(t, filepath.Join(site, "site.yaml"))
	for _, want := range []string{"  - from: /2026/09/hello-dock/\n    to: /articles/hello-dock/", "markdown:", "unsafe_html: true"} {
		if !strings.Contains(cfg, want) {
			t.Errorf("site.yaml missing %q", want)
		}
	}
	if strings.Contains(cfg, "from: /company/\n") {
		t.Error("a redirect from an address that didn't change")
	}
	// The imported site builds and passes the checks.
	res, err := build.Run(build.Options{SiteDir: site})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	ck, _ := check.Run(site, res.Out, res.Site.Config)
	if ck.Errors() > 0 {
		t.Errorf("check errors: %v", ck.Problems)
	}
	if md := rep.Markdown(srv.URL); !strings.Contains(md, "> "+Ownership) {
		t.Errorf("the report doesn't open with the ownership note:\n%s", md)
	}
	if md := rep.Markdown(srv.URL); !strings.Contains(md, "with-video.md`: ") || !strings.Contains(md, "an embed") {
		t.Errorf("report:\n%s", md)
	}
	// Importing again skips what's there.
	rep, _ = Write(items, Options{SiteDir: site, Base: srv.URL})
	if len(rep.Skipped) != 4 || len(rep.Written) != 0 {
		t.Errorf("second import: %d skipped, %d written", len(rep.Skipped), len(rep.Written))
	}
}

func TestWXR(t *testing.T) {
	x := `<?xml version="1.0"?><rss xmlns:content="http://purl.org/rss/1.0/modules/content/" xmlns:excerpt="http://wordpress.org/export/1.2/excerpt/" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:wp="http://wordpress.org/export/1.2/"><channel><link>https://old.example</link>
<item><title>Classic post</title><link>https://old.example/2020/05/classic-post/</link><dc:creator>ed</dc:creator>
<content:encoded><![CDATA[First paragraph
still the first.

[caption id="a" align="alignnone"]<img src="file:///etc/passwd" alt="x"> A caption[/caption]

[gallery ids="1,2"]]]></content:encoded><excerpt:encoded><![CDATA[]]></excerpt:encoded>
<wp:post_id>5</wp:post_id><wp:post_date_gmt>2020-05-01 10:00:00</wp:post_date_gmt><wp:post_name>classic-post</wp:post_name><wp:status>publish</wp:status><wp:post_parent>0</wp:post_parent><wp:post_type>post</wp:post_type>
<category domain="post_tag" nicename="old"><![CDATA[Old]]></category></item>
<item><title>Trashed</title><wp:post_id>6</wp:post_id><wp:status>trash</wp:status><wp:post_type>post</wp:post_type></item>
</channel></rss>`
	items, link, err := FromWXR(strings.NewReader(x), "")
	if err != nil || len(items) != 1 || link != "https://old.example" {
		t.Fatalf("wxr: %v %d %s", err, len(items), link)
	}
	it := items[0]
	if !strings.Contains(it.HTML, "<p>First paragraph<br>still the first.</p>") || !strings.Contains(it.HTML, "<figcaption>A caption</figcaption>") || len(it.Notes) != 1 || !strings.Contains(it.Notes[0], "[gallery]") {
		t.Fatalf("wxr item: %q %v", it.HTML, it.Notes)
	}
	site := starterSite(t)
	rep, err := Write(items, Options{SiteDir: site, Base: link})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Failed["file:///etc/passwd"] == "" {
		t.Errorf("a file:// picture outside the import was read: %v", rep.Failed)
	}
	if md := read(t, filepath.Join(site, "content", "articles", "classic-post.md")); !strings.Contains(md, "First paragraph\\\nstill the first.") {
		t.Errorf("classic post:\n%s", md)
	}
}

func TestHugoJekyllGhost(t *testing.T) {
	// Hugo: TOML front matter, aliases, a page bundle with its picture, a shortcode.
	hugo := t.TempDir()
	os.MkdirAll(filepath.Join(hugo, "content", "posts", "trip"), 0o755)
	os.WriteFile(filepath.Join(hugo, "content", "posts", "trip", "index.md"), []byte("+++\ntitle = \"A trip\"\ndate = 2025-06-01\ntags = [\"travel\", \"boats\"]\naliases = [\"/old/trip/\"]\n[params]\nhidden = true\n+++\n\n![The boat](boat.jpg)\n\n{{< youtube abc >}}\n"), 0o644)
	os.WriteFile(filepath.Join(hugo, "content", "posts", "trip", "boat.jpg"), jpg(), 0o644)
	os.WriteFile(filepath.Join(hugo, "content", "about.md"), []byte("---\ntitle: About\n---\nHi.\n"), 0o644)
	items, err := FromHugo(hugo, map[string]string{"posts": "articles"})
	if err != nil || len(items) != 2 {
		t.Fatalf("hugo: %v %d", err, len(items))
	}
	site := starterSite(t)
	rep, err := Write(items, Options{SiteDir: site, LocalRoot: hugo, Overwrite: true})
	if err != nil {
		t.Fatal(err)
	}
	trip := read(t, filepath.Join(site, "content", "articles", "trip.md"))
	if !strings.Contains(trip, "![The boat](/media/2025/06/boat.jpg)") || !strings.Contains(trip, "tags: [travel, boats]") || rep.Notes["content/articles/trip.md"] == nil {
		t.Errorf("hugo trip:\n%s %v", trip, rep.Notes)
	}
	cfg := read(t, filepath.Join(site, "site.yaml"))
	if !strings.Contains(cfg, "from: /old/trip/\n    to: /articles/trip/") || !strings.Contains(cfg, "from: /posts/trip/\n    to: /articles/trip/") {
		t.Errorf("hugo redirects:\n%s", cfg)
	}

	// Jekyll: the old permalink from the date, redirect_from, Liquid.
	jk := t.TempDir()
	os.MkdirAll(filepath.Join(jk, "_posts"), 0o755)
	os.WriteFile(filepath.Join(jk, "_config.yml"), []byte("permalink: date\n"), 0o644)
	os.WriteFile(filepath.Join(jk, "_posts", "2024-03-05-spring.md"), []byte("---\ntitle: Spring\ncategories: [news]\nredirect_from: [/spring/]\n---\nIt's {{ site.title }} time.\n"), 0o644)
	items, err = FromJekyll(jk, "")
	if err != nil || len(items) != 1 || items[0].OldURL != "/news/2024/03/05/spring.html" || items[0].Date.Format("2006-01-02") != "2024-03-05" || len(items[0].Notes) == 0 {
		t.Fatalf("jekyll: %v %+v", err, items)
	}

	// Ghost: placeholders and tags.
	g := `{"db":[{"data":{"posts":[{"id":"p1","title":"Ghostly","slug":"ghostly","status":"published","type":"post","html":"<p>Hi <img src=\"__GHOST_URL__/content/images/a.jpg\" alt=\"a\"></p>","published_at":"2025-01-02T03:04:05.000Z","updated_at":"2025-01-03T03:04:05.000Z","custom_excerpt":"Boo"}],"tags":[{"id":"t1","name":"spooky"},{"id":"t2","name":"#hidden"}],"posts_tags":[{"post_id":"p1","tag_id":"t1"},{"post_id":"p1","tag_id":"t2"}],"users":[{"id":"u1","name":"Casper"}],"posts_authors":[{"post_id":"p1","author_id":"u1"}]}}]}`
	items, err = FromGhost(strings.NewReader(g), "https://old.ghost", "")
	if err != nil || len(items) != 1 || !strings.Contains(items[0].HTML, "https://old.ghost/content/images/a.jpg") || strings.Join(items[0].Tags, ",") != "spooky" || items[0].Author != "Casper" || items[0].Dir != "articles" {
		t.Fatalf("ghost: %v %+v", err, items)
	}
}
