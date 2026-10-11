package publish

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/build"
)

func TestIndexNow(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	var pings []map[string]any
	status := http.StatusOK
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		pings = append(pings, body)
		w.WriteHeader(status)
	}))
	defer srv.Close()
	old := indexNowEndpoint
	indexNowEndpoint = srv.URL
	defer func() { indexNowEndpoint = old }()

	site := t.TempDir()
	starterSite(t, site)
	cfg, _ := os.ReadFile(filepath.Join(site, "site.yaml"))
	os.WriteFile(filepath.Join(site, "site.yaml"), append(cfg, []byte("indexnow: { key: abcdef0123456789 }\nverify: { google: g00gleCode, bing: B1NG }\n")...), 0o644)
	www := filepath.Join(t.TempDir(), "www")
	publishOnce := func() string {
		res, err := build.Run(build.Options{SiteDir: site})
		if err != nil {
			t.Fatal(err)
		}
		var log bytes.Buffer
		if err := Run("live", Target{Type: "dir", Path: www, IndexNow: true}, res.Out, &log); err != nil {
			t.Fatal(err)
		}
		return log.String()
	}

	log := publishOnce()
	if len(pings) != 1 || !strings.Contains(log, "told search engines about") {
		t.Fatalf("first publish: %d pings, log %s", len(pings), log)
	}
	p := pings[0]
	urls, _ := p["urlList"].([]any)
	if p["host"] != "example.org" || p["key"] != "abcdef0123456789" || p["keyLocation"] != "https://example.org/abcdef0123456789.txt" || len(urls) < 5 {
		t.Fatalf("ping: %v", p)
	}
	// The key file and the verification tags are in the build.
	res, _ := build.Run(build.Options{SiteDir: site})
	if b, _ := os.ReadFile(filepath.Join(res.Out, "abcdef0123456789.txt")); string(b) != "abcdef0123456789" {
		t.Errorf("key file: %q", b)
	}
	home, _ := os.ReadFile(filepath.Join(res.Out, "index.html"))
	if !strings.Contains(string(home), `<meta name="google-site-verification" content="g00gleCode"><meta name="msvalidate.01" content="B1NG">`) {
		t.Error("verification tags missing")
	}

	// Nothing changed: nothing sent.
	if log := publishOnce(); len(pings) != 1 || !strings.Contains(log, "no pages changed") {
		t.Errorf("unchanged: %d pings, %s", len(pings), log)
	}
	// One page changed: only that page.
	about := filepath.Join(site, "content", "about.md")
	src, _ := os.ReadFile(about)
	os.WriteFile(about, append(src, []byte("\nOne more line.\n")...), 0o644)
	publishOnce()
	if len(pings) != 2 {
		t.Fatalf("after an edit: %d pings", len(pings))
	}
	urls, _ = pings[1]["urlList"].([]any)
	if len(urls) != 1 || urls[0] != "https://example.org/about/" {
		t.Errorf("changed: %v", urls)
	}
	// A refused ping is reported; the publish still succeeds.
	status = http.StatusForbidden
	os.WriteFile(about, append(src, []byte("\nAnother line.\n")...), 0o644)
	if log := publishOnce(); !strings.Contains(log, "403") || !strings.Contains(log, "publish itself went fine") {
		t.Errorf("refused: %s", log)
	}
	// A target that didn't ask for it never pings.
	res, _ = build.Run(build.Options{SiteDir: site})
	n := len(pings)
	var quiet bytes.Buffer
	Run("staging", Target{Type: "dir", Path: filepath.Join(t.TempDir(), "s")}, res.Out, &quiet)
	if len(pings) != n || strings.Contains(quiet.String(), "indexnow") {
		t.Error("a target without indexnow: true pinged")
	}
}
