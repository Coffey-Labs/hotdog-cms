package preview

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/forge"
	"git.coffeylabs.org/coffey-labs/hotdog-cms/starter"
)

type repo struct {
	t   *testing.T
	dir string
}

func newRepo(t *testing.T) *repo {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	r := &repo{t: t, dir: t.TempDir()}
	err := fs.WalkDir(starter.Files, "site", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		target := filepath.Join(r.dir, strings.TrimPrefix(p, "site"))
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, _ := starter.Files.ReadFile(p)
		return os.WriteFile(target, []byte(strings.ReplaceAll(string(data), "{{SITE_NAME}}", "Test")), 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	r.git("init", "-q", "-b", "main")
	r.git("add", "-A")
	r.git("commit", "-q", "-m", "site")
	return r
}

func (r *repo) git(args ...string) {
	r.t.Helper()
	cmd := exec.Command("git", append([]string{"-C", r.dir, "-c", "user.name=t", "-c", "user.email=t@example.test", "-c", "commit.gpgsign=false"}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		r.t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func (r *repo) write(rel, body string) {
	r.t.Helper()
	if err := os.WriteFile(filepath.Join(r.dir, rel), []byte(body), 0o644); err != nil {
		r.t.Fatal(err)
	}
}

func get(t *testing.T, h http.Handler, host, path string) (*http.Response, string) {
	t.Helper()
	req := httptest.NewRequest("GET", "http://"+host+path, nil)
	req.Host = host
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	body, _ := io.ReadAll(rec.Result().Body)
	return rec.Result(), string(body)
}

func TestPreviews(t *testing.T) {
	r := newRepo(t)
	r.git("checkout", "-q", "-b", "new-about")
	r.write("content/about.md", "---\ntitle: About, rewritten\n---\nA draft of the new page.\n")
	r.git("commit", "-q", "-am", "rewrite about")
	r.git("checkout", "-q", "main")

	b, err := NewBuilder(Options{Repo: r.dir, Root: t.TempDir(), Port: "8160"})
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Sync(); err != nil {
		t.Fatal(err)
	}
	h := b.Handler(nil)
	mainHost, branchHost := Slug("main")+".localhost:8160", Slug("new-about")+".localhost:8160"

	res, body := get(t, h, mainHost, "/about/")
	if res.StatusCode != 200 || !strings.Contains(body, "<h1>About</h1>") {
		t.Fatalf("main preview: %d", res.StatusCode)
	}
	if !strings.Contains(res.Header.Get("X-Robots-Tag"), "noindex") || res.Header.Get("Cache-Control") != "no-store" {
		t.Error("preview is indexable or cacheable")
	}
	if _, body := get(t, h, branchHost, "/about/"); !strings.Contains(body, "About, rewritten") {
		t.Fatal("branch preview does not show the branch")
	}
	// Absolute links point at the preview, not the production site.
	if _, body := get(t, h, branchHost, "/"); !strings.Contains(body, `href="http://`+branchHost+`/"`) {
		t.Error("canonical link does not point at the preview")
	}
	if res, _ := get(t, h, branchHost, "/no-such-page/"); res.StatusCode != 404 {
		t.Errorf("missing page: %d", res.StatusCode)
	}
	if res, _ := get(t, h, branchHost, "/.hotdog-cms-build"); res.StatusCode != 404 {
		t.Errorf("build marker is served")
	}
	if _, body := get(t, h, branchHost, "/robots.txt"); !strings.Contains(body, "Disallow: /") {
		t.Error("robots.txt lets crawlers in")
	}
	if res, _ := get(t, h, "nope.localhost:8160", "/"); res.StatusCode != 404 {
		t.Errorf("unknown preview: %d", res.StatusCode)
	}
	if _, body := get(t, h, "localhost:8160", "/"); !strings.Contains(body, "new-about") || !strings.Contains(body, ">main<") {
		t.Error("index does not list both branches")
	}

	// A commit that breaks the build keeps the last good preview and says why.
	r.git("checkout", "-q", "new-about")
	r.write("content/broken.md", "---\ntitle: x\nlayout: nope\n---\n")
	r.git("add", "-A")
	r.git("commit", "-q", "-m", "broken")
	r.git("checkout", "-q", "main")
	if err := b.Sync(); err != nil {
		t.Fatal(err)
	}
	if _, body := get(t, h, branchHost, "/about/"); !strings.Contains(body, "About, rewritten") {
		t.Fatal("a failed build took the previous preview down")
	}
	if _, body := get(t, h, "localhost:8160", "/"); !strings.Contains(body, "layout") {
		t.Error("index does not show the build error")
	}

	// A deleted branch loses its preview.
	r.git("branch", "-q", "-D", "new-about")
	if err := b.Sync(); err != nil {
		t.Fatal(err)
	}
	if res, _ := get(t, h, branchHost, "/"); res.StatusCode != 404 {
		t.Error("preview of a deleted branch is still served")
	}
}

func TestAuthAndListening(t *testing.T) {
	r := newRepo(t)
	b, err := NewBuilder(Options{Repo: r.dir, Root: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Serve("0.0.0.0:0", nil, time.Hour); err == nil || !strings.Contains(err.Error(), "credentials") {
		t.Fatalf("listened off loopback without credentials: %v", err)
	}
	h := b.Handler(&Auth{User: "editor", Pass: "a long preview password"})
	if res, _ := get(t, h, "localhost", "/"); res.StatusCode != 401 {
		t.Fatalf("no credentials: %d", res.StatusCode)
	}
	req := httptest.NewRequest("GET", "http://localhost/", nil)
	req.SetBasicAuth("editor", "a long preview password")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("right credentials: %d", rec.Code)
	}
}

func TestSlug(t *testing.T) {
	a, b := Slug("feature/x"), Slug("feature-x")
	if a == b {
		t.Error("two branches share a preview address")
	}
	long := Slug(strings.Repeat("very-long-branch-name-", 10))
	if len(long) > 63 {
		t.Errorf("slug %q is longer than a DNS label", long)
	}
}

func TestWebhookAndStatus(t *testing.T) {
	r := newRepo(t)
	var mu sync.Mutex
	var statuses []map[string]string
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var body map[string]string
		_ = json.NewDecoder(req.Body).Decode(&body)
		body["path"] = req.URL.Path
		mu.Lock()
		statuses = append(statuses, body)
		mu.Unlock()
		w.WriteHeader(201)
	}))
	defer api.Close()
	fc := &forge.Client{Repo: forge.Repo{Kind: forge.GitHub, Path: "acme/site", API: api.URL}, Token: "tok"}
	b, err := NewBuilder(Options{Repo: r.dir, Root: t.TempDir(), Port: "8160", Forge: fc, HookSecret: "hook-secret"})
	if err != nil {
		t.Fatal(err)
	}
	h := b.Handler(&Auth{User: "editor", Pass: "a long preview password"})

	body := []byte(`{"ref":"refs/heads/main","after":"` + strings.Repeat("b", 40) + `"}`)
	mac := hmac.New(sha256.New, []byte("hook-secret"))
	mac.Write(body)
	send := func(sig string) int {
		req := httptest.NewRequest("POST", "http://localhost:8160/_hotdog/hook", bytes.NewReader(body))
		req.Header.Set("X-GitHub-Event", "push")
		req.Header.Set("X-Hub-Signature-256", sig)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}
	if code := send("sha256=" + strings.Repeat("0", 64)); code != 401 {
		t.Fatalf("forged webhook: %d", code)
	}
	// The hook gets past the preview sign-in: platforms can't send it, and
	// the signature is the proof instead.
	if code := send("sha256=" + hex.EncodeToString(mac.Sum(nil))); code != 202 {
		t.Fatalf("signed webhook: %d", code)
	}
	select {
	case <-b.kick:
	default:
		t.Fatal("a signed push did not ask for a sync")
	}
	if err := b.Sync(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		mu.Lock()
		n := len(statuses)
		mu.Unlock()
		if n >= 2 || time.Now().After(deadline) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	var sawPending, sawSuccess bool
	for _, s := range statuses {
		if !strings.HasPrefix(s["path"], "/repos/acme/site/statuses/") {
			t.Errorf("status went to %s", s["path"])
		}
		switch s["state"] {
		case "pending":
			sawPending = true
		case "success":
			sawSuccess = strings.HasPrefix(s["target_url"], "http://"+Slug("main")+".localhost:8160")
		}
	}
	if !sawPending || !sawSuccess {
		t.Fatalf("statuses posted: %v", statuses)
	}
}

func TestFrameAncestors(t *testing.T) {
	r := newRepo(t)
	b, _ := NewBuilder(Options{Repo: r.dir, Root: t.TempDir(), FrameAncestors: []string{"http://127.0.0.1:8190"}})
	res, _ := get(t, b.Handler(nil), "localhost", "/")
	if res.Header.Get("X-Frame-Options") != "" || res.Header.Get("Content-Security-Policy") != "frame-ancestors 'self' http://127.0.0.1:8190" {
		t.Errorf("framing headers: %v", res.Header)
	}
	b2, _ := NewBuilder(Options{Repo: r.dir, Root: t.TempDir()})
	if res, _ := get(t, b2.Handler(nil), "localhost", "/"); res.Header.Get("X-Frame-Options") != "SAMEORIGIN" {
		t.Error("previews frameable by anyone by default")
	}
}
