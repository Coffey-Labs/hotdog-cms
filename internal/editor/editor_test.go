package editor

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"git.coffeylabs.org/coffey-labs/cl-cms/starter"
)

// fakeForge plays a Gitea: OAuth authorize/token endpoints and the API.
type fakeForge struct {
	t         *testing.T
	challenge string
	srv       *httptest.Server
}

func newFakeForge(t *testing.T) *fakeForge {
	ff := &fakeForge{t: t}
	ff.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		switch {
		case r.URL.Path == "/login/oauth/access_token":
			r.ParseForm()
			sum := sha256.Sum256([]byte(r.PostForm.Get("code_verifier")))
			if base64.RawURLEncoding.EncodeToString(sum[:]) != ff.challenge || r.PostForm.Get("code") != "the-code" || r.PostForm.Get("client_secret") != "shh" {
				w.WriteHeader(400)
				json.NewEncoder(w).Encode(map[string]string{"error": "invalid_grant"})
				return
			}
			json.NewEncoder(w).Encode(map[string]string{"access_token": "user-token"})
		case auth != "Bearer user-token":
			w.WriteHeader(401)
		case r.URL.Path == "/api/v1/user":
			json.NewEncoder(w).Encode(map[string]string{"login": "ada", "full_name": "Ada Lovelace"})
		case r.URL.Path == "/api/v1/repos/owner/site":
			json.NewEncoder(w).Encode(map[string]any{"permissions": map[string]bool{"pull": true, "push": true}})
		case r.URL.Path == "/api/v1/repos/owner/secret":
			w.WriteHeader(404)
		case r.URL.Path == "/api/v1/repos/owner/site/branches":
			json.NewEncoder(w).Encode([]map[string]any{{"name": "main", "commit": map[string]string{"id": "a1"}}, {"name": "draft/hello", "commit": map[string]string{"id": "b2"}}})
		case r.URL.Path == "/api/v1/repos/owner/site/pulls":
			json.NewEncoder(w).Encode([]map[string]any{{"number": 7, "title": "New hello", "html_url": "https://forge.test/owner/site/pulls/7", "head": map[string]string{"ref": "draft/hello"}, "user": map[string]string{"login": "ada"}}})
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(ff.srv.Close)
	return ff
}

func starterRepo(t *testing.T) string {
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
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"add", "-A"}, {"commit", "-q", "-m", "site"}} {
		cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@example.test", "-c", "commit.gpgsign=false"}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	return dir
}

func newTestServer(t *testing.T) (*Server, *fakeForge) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	ff := newFakeForge(t)
	repo := starterRepo(t)
	dir := t.TempDir()
	cfg := `listen: 127.0.0.1:8190
public_url: http://editor.test
cache: ` + filepath.Join(dir, "cache") + `
forges:
  - { host: forge.test, kind: gitea, client_id: app, client_secret_env: TEST_SECRET, base: "` + ff.srv.URL + `" }
sites:
  - { name: Test site, id: site, repo: "https://forge.test/owner/site.git", preview: { domain: localhost, port: "8160", scheme: http } }
  - { name: Secret site, id: secret, repo: "https://forge.test/owner/secret.git" }
`
	os.WriteFile(filepath.Join(dir, "editor.yaml"), []byte(cfg), 0o644)
	t.Setenv("TEST_SECRET", "shh")
	t.Setenv("CLCMS_EDITOR_SECRET", strings.Repeat("e", 40))
	s, err := New(filepath.Join(dir, "editor.yaml"), os.Stderr)
	if err != nil {
		t.Fatal(err)
	}
	s.cloneURL = func(*SiteConfig) string { return repo }
	return s, ff
}

func do(s *Server, method, path string, cookies []*http.Cookie, hdr map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "http://editor.test"+path, nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	return rec
}

func cookie(rec *httptest.ResponseRecorder, name string) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == name {
			return c
		}
	}
	return nil
}

func signIn(t *testing.T, s *Server, ff *fakeForge) *http.Cookie {
	t.Helper()
	rec := do(s, "GET", "/auth/login?forge=forge.test&return=/sites/site", nil, nil)
	if rec.Code != 302 {
		t.Fatalf("login: %d", rec.Code)
	}
	loc, _ := url.Parse(rec.Header().Get("Location"))
	if !strings.HasPrefix(loc.String(), ff.srv.URL+"/login/oauth/authorize") || loc.Query().Get("code_challenge_method") != "S256" || loc.Query().Get("scope") != "read:user read:repository" {
		t.Fatalf("authorize redirect: %s", loc)
	}
	ff.challenge = loc.Query().Get("code_challenge")
	st := cookie(rec, stateCookie)
	// A wrong state is refused.
	if rec := do(s, "GET", "/auth/callback?code=the-code&state=wrong", []*http.Cookie{st}, nil); rec.Code != 400 {
		t.Fatalf("wrong state accepted: %d", rec.Code)
	}
	rec = do(s, "GET", "/auth/callback?code=the-code&state="+loc.Query().Get("state"), []*http.Cookie{st}, nil)
	if rec.Code != 302 || rec.Header().Get("Location") != "/sites/site" {
		t.Fatalf("callback: %d %s %s", rec.Code, rec.Header().Get("Location"), rec.Body)
	}
	sess := cookie(rec, sessionCookie)
	if sess == nil || !sess.HttpOnly || sess.SameSite != http.SameSiteLaxMode || strings.Contains(sess.Value, "user-token") {
		t.Fatalf("session cookie wrong: %+v", sess)
	}
	return sess
}

func TestSignInAndBrowse(t *testing.T) {
	s, ff := newTestServer(t)
	if rec := do(s, "GET", "/api/sites", nil, nil); rec.Code != 401 {
		t.Fatalf("signed-out API: %d", rec.Code)
	}
	sess := signIn(t, s, ff)
	jar := []*http.Cookie{sess}

	var sites []map[string]any
	json.NewDecoder(do(s, "GET", "/api/sites", jar, nil).Body).Decode(&sites)
	if len(sites) != 1 || sites[0]["id"] != "site" || sites[0]["canWrite"] != true {
		t.Fatalf("sites: %v", sites)
	}
	if rec := do(s, "GET", "/api/sites/secret", jar, nil); rec.Code != 404 {
		t.Errorf("a site the platform hides opened: %d", rec.Code)
	}

	rec := do(s, "GET", "/api/sites/site", jar, nil)
	var d struct {
		Pages   []PageInfo
		Drafts  []struct{ Branch, Preview string }
		Reviews []map[string]any
		Build   BuildResult
		Preview string
	}
	json.NewDecoder(rec.Body).Decode(&d)
	if rec.Code != 200 || len(d.Pages) < 4 || d.Build.Pages == 0 || d.Build.Failed != "" {
		t.Fatalf("site: %d %+v", rec.Code, d)
	}
	if len(d.Drafts) != 1 || d.Drafts[0].Branch != "draft/hello" || !strings.HasPrefix(d.Drafts[0].Preview, "http://draft-hello-") {
		t.Errorf("drafts: %+v", d.Drafts)
	}
	if len(d.Reviews) != 1 || d.Preview == "" {
		t.Errorf("reviews/preview: %+v %q", d.Reviews, d.Preview)
	}

	var pg PageDetail
	json.NewDecoder(do(s, "GET", "/api/sites/site/page?source=content/about.md", jar, nil).Body).Decode(&pg)
	if pg.FrontMatter["title"] != "About" || !strings.Contains(pg.Body, "content/about.md") {
		t.Errorf("page: %+v", pg)
	}
	for _, bad := range []string{"../site.yaml", "content/../site.yaml", "/etc/passwd", "content/about.md/../../site.yaml", "layouts/base.html"} {
		if rec := do(s, "GET", "/api/sites/site/page?source="+url.QueryEscape(bad), jar, nil); rec.Code != 404 {
			t.Errorf("source %q: %d", bad, rec.Code)
		}
	}
}

type PageDetail struct {
	FrontMatter map[string]any
	Body        string
}

func TestSessionAndCSRF(t *testing.T) {
	s, ff := newTestServer(t)
	sess := signIn(t, s, ff)
	tampered := *sess
	tampered.Value = sess.Value[:len(sess.Value)-4] + "AAAA"
	if rec := do(s, "GET", "/api/sites", []*http.Cookie{&tampered}, nil); rec.Code != 401 {
		t.Errorf("tampered session accepted: %d", rec.Code)
	}
	// The sealed state cookie can't stand in for a session.
	rec := do(s, "GET", "/auth/login?forge=forge.test", nil, nil)
	st := cookie(rec, stateCookie)
	st.Name = sessionCookie
	if rec := do(s, "GET", "/api/sites", []*http.Cookie{st}, nil); rec.Code != 401 {
		t.Errorf("state cookie worked as a session: %d", rec.Code)
	}
	if rec := do(s, "POST", "/auth/logout", []*http.Cookie{sess}, nil); rec.Code != 403 {
		t.Errorf("logout without CSRF header: %d", rec.Code)
	}
	if rec := do(s, "POST", "/auth/logout", []*http.Cookie{sess}, map[string]string{"Origin": "https://evil.example", "X-CLCMS": "1"}); rec.Code != 403 {
		t.Errorf("cross-site logout: %d", rec.Code)
	}
	if rec := do(s, "POST", "/auth/logout", []*http.Cookie{sess}, map[string]string{"Origin": "http://editor.test", "X-CLCMS": "1"}); rec.Code != 204 {
		t.Errorf("logout: %d", rec.Code)
	}
	if rec := do(s, "GET", "/auth/login?forge=forge.test&return=//evil.example/", nil, nil); rec.Code != 302 {
		t.Fatal(rec.Code)
	}
	if got := safeReturn("//evil.example/"); got != "/" {
		t.Errorf("open redirect: %s", got)
	}
	h := do(s, "GET", "/", nil, nil).Header()
	if !strings.Contains(h.Get("Content-Security-Policy"), "frame-ancestors 'none'") || !strings.Contains(h.Get("Content-Security-Policy"), "frame-src http://*.localhost:8160") || h.Get("X-Frame-Options") != "DENY" {
		t.Errorf("headers: %v", h)
	}
}
