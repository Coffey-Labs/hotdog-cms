package check

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/site"
)

func mkSite(t *testing.T, files map[string]string, extra string) (string, string, *site.Config) {
	t.Helper()
	dir := t.TempDir()
	out := filepath.Join(dir, "public")
	for name, body := range files {
		p := filepath.Join(out, filepath.FromSlash(name))
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(body), 0o644)
	}
	os.WriteFile(filepath.Join(dir, "site.yaml"), []byte("name: T\nurl: https://example.org\n"+extra), 0o644)
	cfg, err := site.LoadConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	return dir, out, cfg
}

const head = `<title>Home</title><meta name="description" content="d"><link rel="canonical" href="https://example.org/"><meta property="og:image" content="/card.png"><meta property="og:image:alt" content="a card">`

func rules(r *Report) map[string]int {
	m := map[string]int{}
	for _, p := range r.Problems {
		m[string(p.Level)+" "+p.Rule]++
	}
	return m
}

func TestRules(t *testing.T) {
	expires := time.Now().AddDate(0, 0, 10).UTC().Format(time.RFC3339)
	dir, out, cfg := mkSite(t, map[string]string{
		"index.html": `<html><head>` + head + `<script async src="https://www.googletagmanager.com/gtag/js?id=G-X"></script>
<script>window.dataLayer = window.dataLayer || [];</script></head><body style="x">
<img src="/missing.png"><a href="/nowhere/">x</a><a onclick="go()">y</a>
<script src="http://cdn.example.net/a.js"></script>
<p>Write to hello@example.org or @me@social.example</p><!--email_off--><p>safe@example.org</p><!--/email_off-->
<iframe src="https://video.example.com/embed"></iframe></body></html>`,
		"card.png":                 "png",
		"about/index.html":         `<html><head>` + head + `</head><body></body></html>`,
		"thin/index.html":          `<html><head><meta name="robots" content="noindex"></head><body></body></html>`,
		"sitemap.xml":              `<urlset><url><loc>https://example.org/thin/</loc></url></urlset>`,
		"robots.txt":               "User-agent: *\n",
		".well-known/security.txt": "Contact: mailto:security@example.org\nExpires: " + expires + "\n",
	}, "check:\n  cloudflare: true\n  allow_third_party: [video.example.com]\n")
	r, err := Run(dir, out, cfg)
	if err != nil {
		t.Fatal(err)
	}
	got := rules(r)
	want := map[string]int{
		"error tracker-before-consent":  2, // the gtag script, and the inline dataLayer code
		"error broken-link":             2, // /missing.png and /nowhere/
		"error inline-handler":          1,
		"error mixed-content":           1,
		"error noindex-in-sitemap":      1,
		"error privacy-page":            1,
		"warning inline-script":         1,
		"warning inline-style":          1,
		"warning img-alt":               1,
		"warning third-party":           1, // cdn.example.net; video.example.com is allowed
		"warning cloudflare-email":      2, // hello@ and the handle; safe@ is inside markers
		"warning robots-sitemap":        1,
		"warning security-txt":          1, // expires within 30 days
		"warning duplicate-title":       1,
		"warning duplicate-description": 1,
	}
	for k, n := range want {
		if got[k] != n {
			t.Errorf("%s: got %d, want %d", k, got[k], n)
		}
	}
	for k := range got {
		if _, ok := want[k]; !ok {
			t.Errorf("unexpected finding %s (%d)", k, got[k])
		}
	}
	if !strings.Contains(r.CSP, "script-src 'self' 'unsafe-inline' http://cdn.example.net https://www.googletagmanager.com") || !strings.Contains(r.CSP, "frame-src https://video.example.com") {
		t.Errorf("csp: %s", r.CSP)
	}
	// Ignoring a rule switches it off; strict CSP makes inline code an error.
	cfg.Check.Ignore = []string{"img-alt"}
	cfg.Check.CSP = "strict"
	r, _ = Run(dir, out, cfg)
	got = rules(r)
	if got["warning img-alt"] != 0 || got["error inline-script"] != 1 || got["error inline-style"] != 1 {
		t.Errorf("ignore/strict not applied: %v", got)
	}
}

func TestCleanSitePasses(t *testing.T) {
	dir, out, cfg := mkSite(t, map[string]string{
		"index.html":               `<html><head>` + head + `</head><body><img src="/card.png" alt=""></body></html>`,
		"card.png":                 "png",
		"sitemap.xml":              `<urlset><url><loc>https://example.org/</loc></url></urlset>`,
		"robots.txt":               "Sitemap: https://example.org/sitemap.xml\n",
		".well-known/security.txt": "Expires: " + time.Now().AddDate(1, 0, 0).UTC().Format(time.RFC3339) + "\n",
	}, "")
	r, err := Run(dir, out, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Problems) != 0 {
		t.Errorf("clean site has findings: %v", r.Problems)
	}
	if !strings.HasPrefix(r.CSP, "default-src 'self'; script-src 'self'; style-src 'self';") {
		t.Errorf("clean site's CSP isn't strict: %s", r.CSP)
	}
}

// gpsJPEG is the start of a JPEG whose EXIF block has a GPS entry: enough
// for the rule, which reads only the header.
func gpsJPEG() string {
	tiff := []byte{'I', 'I', 42, 0, 8, 0, 0, 0,
		1, 0, // IFD0: one entry, the GPS pointer
		0x25, 0x88, 4, 0, 1, 0, 0, 0, 26, 0, 0, 0,
		0, 0, 0, 0,
		1, 0, // GPS IFD: one entry
		1, 0, 2, 0, 2, 0, 0, 0, 'N', 0, 0, 0,
		0, 0, 0, 0}
	seg := append([]byte("Exif\x00\x00"), tiff...)
	b := []byte{0xFF, 0xD8, 0xFF, 0xE1, byte((len(seg) + 2) >> 8), byte(len(seg) + 2)}
	b = append(b, seg...)
	return string(append(b, 0xFF, 0xDA, 0, 2, 0xFF, 0xD9))
}

func TestImageRules(t *testing.T) {
	dir, out, cfg := mkSite(t, map[string]string{
		"index.html":    `<html><head>` + head + `</head><body></body></html>`,
		"card.png":      "png",
		"img/home.jpg":  gpsJPEG(),
		"img/plain.jpg": "\xFF\xD8\xFF\xDA\x00\x02\xFF\xD9",
		"img/big.png":   strings.Repeat("x", 600<<10),
	}, "")
	r, err := Run(dir, out, cfg)
	if err != nil {
		t.Fatal(err)
	}
	got := rules(r)
	if got["error image-location"] != 1 || got["warning image-weight"] != 1 {
		t.Fatalf("got %v", got)
	}
	for _, p := range r.Problems {
		if p.Rule == "image-location" && p.File != "public/img/home.jpg" {
			t.Errorf("flagged %s", p.File)
		}
	}
}
