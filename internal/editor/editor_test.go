package editor

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/about"
	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/build"
	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/forms"
	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/site"

	"git.coffeylabs.org/coffey-labs/hotdog-cms/starter"
)

// fakeForge plays a Gitea: OAuth authorize/token endpoints and the API.
type fakeForge struct {
	t           *testing.T
	challenge   string
	srv         *httptest.Server
	opened      []map[string]string // pull requests opened
	merged      []string            // merge calls
	refuseMerge bool                // answer merges as a writer without merge rights
	bobGone     bool                // the platform stops taking bob's token
	listed      int                 // repository listings served
	expiring    bool                // hand out tokens that expire in a minute, with a refresh token
	refreshes   int                 // refresh-token renewals served
	refuse      bool                // refuse renewals
	requested   []string            // reviewers asked for
}

func newFakeForge(t *testing.T) *fakeForge {
	ff := &fakeForge{t: t}
	ff.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		switch {
		case r.URL.Path == "/login/oauth/access_token" && r.FormValue("grant_type") == "refresh_token":
			ff.refreshes++
			if ff.refuse || !strings.HasPrefix(r.FormValue("refresh_token"), "rt-") || r.FormValue("client_secret") != "shh" {
				w.WriteHeader(400)
				json.NewEncoder(w).Encode(map[string]string{"error": "invalid_grant"})
				return
			}
			json.NewEncoder(w).Encode(map[string]any{"access_token": "user-token", "refresh_token": fmt.Sprintf("rt-%d", ff.refreshes), "expires_in": 3600})
		case r.URL.Path == "/login/oauth/access_token":
			r.ParseForm()
			sum := sha256.Sum256([]byte(r.PostForm.Get("code_verifier")))
			code := r.PostForm.Get("code")
			if base64.RawURLEncoding.EncodeToString(sum[:]) != ff.challenge || (code != "the-code" && code != "bob-code") || r.PostForm.Get("client_secret") != "shh" {
				w.WriteHeader(400)
				json.NewEncoder(w).Encode(map[string]string{"error": "invalid_grant"})
				return
			}
			tok := "user-token"
			if code == "bob-code" {
				tok = "bob-token"
			}
			if ff.expiring {
				json.NewEncoder(w).Encode(map[string]any{"access_token": tok, "refresh_token": "rt-0", "expires_in": 60})
				return
			}
			json.NewEncoder(w).Encode(map[string]string{"access_token": tok})
		case auth != "Bearer user-token" && (auth != "Bearer bob-token" || ff.bobGone):
			w.WriteHeader(401)
		case r.URL.Path == "/api/v1/user" && auth == "Bearer bob-token":
			json.NewEncoder(w).Encode(map[string]string{"login": "bob", "full_name": "Bob"})
		case r.URL.Path == "/api/v1/user":
			json.NewEncoder(w).Encode(map[string]string{"login": "ada", "full_name": "Ada Lovelace"})
		case r.URL.Path == "/api/v1/repos/owner/site":
			// Ada can write; Bob can only read.
			json.NewEncoder(w).Encode(map[string]any{"permissions": map[string]bool{"pull": true, "push": auth == "Bearer user-token"}})
		case r.URL.Path == "/api/v1/repos/owner/secret" && auth == "Bearer bob-token":
			json.NewEncoder(w).Encode(map[string]any{"permissions": map[string]bool{"pull": true, "push": true}})
		case r.URL.Path == "/api/v1/repos/owner/secret":
			w.WriteHeader(404)
		case r.URL.Path == "/api/v1/user/repos":
			ff.listed++
			repo := func(full string, extra map[string]any) map[string]any {
				m := map[string]any{"full_name": full, "clone_url": "https://forge.test/" + full + ".git", "default_branch": "main",
					"owner": map[string]string{"login": strings.Split(full, "/")[0]}, "permissions": map[string]bool{"pull": true, "push": true}}
				for k, v := range extra {
					m[k] = v
				}
				return m
			}
			json.NewEncoder(w).Encode([]map[string]any{
				repo("owner/site", nil),  // also in editor.yaml
				repo("owner/blog", nil),  // a site
				repo("owner/notes", nil), // no site.yaml
				repo("owner/other", nil), // someone else's site.yaml
				repo("owner/attic", map[string]any{"archived": true}),
				repo("stranger/site2", nil), // outside the owners filter
				repo("owner/mirror", map[string]any{"clone_url": "https://elsewhere.example/owner/mirror.git"}),
			})
		case strings.HasSuffix(r.URL.Path, "/raw/site.yaml"):
			switch r.URL.Path {
			case "/api/v1/repos/owner/blog/raw/site.yaml", "/api/v1/repos/owner/site/raw/site.yaml", "/api/v1/repos/stranger/site2/raw/site.yaml", "/api/v1/repos/owner/attic/raw/site.yaml", "/api/v1/repos/owner/mirror/raw/site.yaml":
				w.Write([]byte("name: The Blog\nurl: https://blog.example\n"))
			case "/api/v1/repos/owner/other/raw/site.yaml":
				w.Write([]byte("title: Jekyll thing\ntheme: minima\n"))
			default:
				w.WriteHeader(404)
			}
		case r.URL.Path == "/api/v1/repos/owner/site/branches":
			json.NewEncoder(w).Encode([]map[string]any{{"name": "main", "commit": map[string]string{"id": "a1"}}, {"name": "draft/hello", "commit": map[string]string{"id": "b2"}}})
		case r.URL.Path == "/api/v1/repos/owner/site/pulls" && r.Method == "POST":
			var body map[string]string
			json.NewDecoder(r.Body).Decode(&body)
			ff.opened = append(ff.opened, body)
			w.WriteHeader(201)
			json.NewEncoder(w).Encode(map[string]any{"number": 8, "html_url": "https://forge.test/owner/site/pulls/8"})
		case r.URL.Path == "/api/v1/repos/owner/site/pulls":
			json.NewEncoder(w).Encode([]map[string]any{{"number": 7, "title": "New hello", "html_url": "https://forge.test/owner/site/pulls/7", "head": map[string]string{"ref": "draft/hello"}, "user": map[string]string{"login": "ada"}}})
		case r.URL.Path == "/api/v1/repos/owner/site/reviewers":
			json.NewEncoder(w).Encode([]map[string]string{{"login": "ada", "full_name": "Ada Lovelace"}, {"login": "carol", "full_name": "Carol"}})
		case r.URL.Path == "/api/v1/repos/owner/site/pulls/8/requested_reviewers" && r.Method == "POST":
			var body map[string][]string
			json.NewDecoder(r.Body).Decode(&body)
			ff.requested = append(ff.requested, body["reviewers"]...)
			w.WriteHeader(201)
			w.Write([]byte("{}"))
		case r.URL.Path == "/api/v1/repos/owner/site/pulls/7/merge" && r.Method == "POST":
			ff.merged = append(ff.merged, "7")
			w.WriteHeader(200)
		case r.URL.Path == "/api/v1/repos/owner/site/pulls/8/merge" && r.Method == "POST":
			if ff.refuseMerge {
				w.WriteHeader(405)
				w.Write([]byte(`{"message":"not allowed to merge"}`))
				return
			}
			ff.merged = append(ff.merged, "8")
			w.WriteHeader(200)
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
	s, ff, _ := newTestServerRepo(t)
	return s, ff
}

func newTestServerRepo(t *testing.T) (*Server, *fakeForge, string) {
	t.Helper()
	return newTestServerWith(t, "")
}

// newTestServerWith adds settings to the test platform's entry in editor.yaml.
func newTestServerWith(t *testing.T, forgeExtra string) (*Server, *fakeForge, string) {
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
  - { host: forge.test, kind: gitea, client_id: app, client_secret_env: TEST_SECRET, base: "` + ff.srv.URL + `"` + forgeExtra + ` }
sites:
  - { name: Test site, id: site, repo: "https://forge.test/owner/site.git", preview: { domain: localhost, port: "8160", scheme: http } }
  - { name: Secret site, id: secret, repo: "https://forge.test/owner/secret.git" }
`
	os.WriteFile(filepath.Join(dir, "editor.yaml"), []byte(cfg), 0o644)
	t.Setenv("TEST_SECRET", "shh")
	t.Setenv("HOTDOG_EDITOR_SECRET", strings.Repeat("e", 40))
	s, err := New(filepath.Join(dir, "editor.yaml"), os.Stderr)
	if err != nil {
		t.Fatal(err)
	}
	s.live = newLiveServer(LiveConfig{Listen: "127.0.0.1:8191"}, "http://editor.test")
	s.cloneURL = func(*SiteConfig) string { return repo }
	return s, ff, repo
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
	if !strings.HasPrefix(loc.String(), ff.srv.URL+"/login/oauth/authorize") || loc.Query().Get("code_challenge_method") != "S256" || loc.Query().Get("scope") != "read:user write:repository" {
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
	sess := cookie(rec, accountCookie(0))
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
	st.Name = accountCookie(0)
	if rec := do(s, "GET", "/api/sites", []*http.Cookie{st}, nil); rec.Code != 401 {
		t.Errorf("state cookie worked as a session: %d", rec.Code)
	}
	if rec := do(s, "POST", "/auth/logout", []*http.Cookie{sess}, nil); rec.Code != 403 {
		t.Errorf("logout without CSRF header: %d", rec.Code)
	}
	if rec := do(s, "POST", "/auth/logout", []*http.Cookie{sess}, map[string]string{"Origin": "https://evil.example", "X-HotDog": "1"}); rec.Code != 403 {
		t.Errorf("cross-site logout: %d", rec.Code)
	}
	if rec := do(s, "POST", "/auth/logout", []*http.Cookie{sess}, map[string]string{"Origin": "http://editor.test", "X-HotDog": "1"}); rec.Code != 204 {
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

func post(s *Server, path string, cookies []*http.Cookie, body any) *httptest.ResponseRecorder {
	b, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "http://editor.test"+path, strings.NewReader(string(b)))
	for _, c := range cookies {
		req.AddCookie(c)
	}
	req.Header.Set("Origin", "http://editor.test")
	req.Header.Set("X-HotDog", "1")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	return rec
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@example.test", "-c", "commit.gpgsign=false"}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func TestWriting(t *testing.T) {
	s, ff, repo := newTestServerRepo(t)
	jar := []*http.Cookie{signIn(t, s, ff)}
	mainTip := git(t, repo, "rev-parse", "main")

	// Without the editor's own header, nothing is written.
	req := httptest.NewRequest("POST", "http://editor.test/api/sites/site/save", strings.NewReader("{}"))
	req.AddCookie(jar[0])
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != 403 {
		t.Fatalf("write without CSRF header: %d", rec.Code)
	}

	// Saving on the publishing branch starts a draft, as the person.
	title := "About us"
	rec = post(s, "/api/sites/site/save", jar, map[string]any{"source": "content/about.md", "branch": "main", "baseCommit": mainTip,
		"edit": map[string]any{"set": map[string]any{"title": title, "summary": nil}, "body": "Rewritten."}})
	var saved map[string]any
	json.NewDecoder(rec.Body).Decode(&saved)
	if rec.Code != 200 || saved["branch"] != "draft/about" || saved["created"] != true {
		t.Fatalf("save: %d %v", rec.Code, saved)
	}
	if got := git(t, repo, "rev-parse", "main"); got != mainTip {
		t.Fatal("a save reached the publishing branch")
	}
	file := git(t, repo, "show", "draft/about:content/about.md")
	if !strings.Contains(file, "title: About us") || strings.Contains(file, "summary:") || !strings.HasSuffix(strings.TrimSpace(file), "Rewritten.") {
		t.Errorf("saved file:\n%s", file)
	}
	if author := git(t, repo, "log", "-1", "--format=%an <%ae>", "draft/about"); author != "Ada Lovelace <ada@noreply.forge.test>" {
		t.Errorf("author: %s", author)
	}
	// A save based on an old commit is refused, not merged over.
	rec = post(s, "/api/sites/site/save", jar, map[string]any{"source": "content/about.md", "branch": "draft/about", "baseCommit": mainTip,
		"edit": map[string]any{"body": "Clobber."}})
	if rec.Code != 409 {
		t.Errorf("stale save: %d", rec.Code)
	}

	// A new page starts its own draft.
	rec = post(s, "/api/sites/site/new", jar, map[string]any{"collection": "articles", "title": "My New Post"})
	var np map[string]any
	json.NewDecoder(rec.Body).Decode(&np)
	if rec.Code != 200 || np["source"] != "content/articles/my-new-post.md" || np["branch"] != "draft/my-new-post" {
		t.Fatalf("new page: %d %v", rec.Code, np)
	}

	// The live preview shows unsaved text, on its own origin.
	rec = post(s, "/api/sites/site/live", jar, map[string]any{"source": "content/about.md", "branch": "draft/about",
		"edit": map[string]any{"set": map[string]any{"title": "Unsaved title"}, "body": "Typing…"}})
	var lv map[string]string
	json.NewDecoder(rec.Body).Decode(&lv)
	u, err := url.Parse(lv["url"])
	if rec.Code != 200 || err != nil || !strings.HasSuffix(u.Hostname(), ".localhost") || u.Path != "/about/" {
		t.Fatalf("live: %d %v", rec.Code, lv)
	}
	lreq := httptest.NewRequest("GET", lv["url"], nil)
	lreq.Host = u.Host
	lrec := httptest.NewRecorder()
	s.live.ServeHTTP(lrec, lreq)
	if !strings.Contains(lrec.Body.String(), "Unsaved title") || lrec.Header().Get("Content-Security-Policy") != "frame-ancestors http://editor.test" {
		t.Errorf("live page: %d %s", lrec.Code, lrec.Header())
	}
	other := httptest.NewRequest("GET", "http://deadbeef.localhost:8191/about/", nil)
	orec := httptest.NewRecorder()
	s.live.ServeHTTP(orec, other)
	if orec.Code != 404 {
		t.Errorf("unknown live token: %d", orec.Code)
	}

	// Review, then publish behind the checks.
	rec = post(s, "/api/sites/site/review", jar, map[string]any{"branch": "draft/about", "title": "New about page", "description": "Shorter."})
	if rec.Code != 200 || len(ff.opened) != 1 || ff.opened[0]["base"] != "main" || !strings.Contains(ff.opened[0]["body"], "Preview: http://draft-about-") {
		t.Fatalf("review: %d %v", rec.Code, ff.opened)
	}
	// draft/hello (review #7) carries a check error: a forbidden string.
	git(t, repo, "checkout", "-q", "-b", "draft/hello")
	cfg, _ := os.ReadFile(filepath.Join(repo, "site.yaml"))
	os.WriteFile(filepath.Join(repo, "site.yaml"), []byte(strings.Replace(string(cfg), "  forbid: []", "  forbid: ['secret\\.internal']", 1)), 0o644)
	os.WriteFile(filepath.Join(repo, "content", "hello.md"), []byte("---\ntitle: Hello\n---\nOn secret.internal.\n"), 0o644)
	git(t, repo, "add", "-A")
	git(t, repo, "commit", "-q", "-m", "leak")
	git(t, repo, "checkout", "-q", "main")
	rec = post(s, "/api/sites/site/publish", jar, map[string]any{"branch": "draft/hello"})
	if rec.Code != 409 || len(ff.merged) != 0 {
		t.Fatalf("publish with a check error: %d, merged %v", rec.Code, ff.merged)
	}
	git(t, repo, "checkout", "-q", "draft/hello")
	os.WriteFile(filepath.Join(repo, "content", "hello.md"), []byte("---\ntitle: Hello\n---\nFixed.\n"), 0o644)
	git(t, repo, "commit", "-q", "-am", "fix")
	git(t, repo, "checkout", "-q", "main")
	rec = post(s, "/api/sites/site/publish", jar, map[string]any{"branch": "draft/hello"})
	if rec.Code != 200 || len(ff.merged) != 1 {
		t.Fatalf("publish: %d %s merged %v", rec.Code, rec.Body, ff.merged)
	}
	if rec := post(s, "/api/sites/site/publish", jar, map[string]any{"branch": "main"}); rec.Code != 400 {
		t.Errorf("publishing the publishing branch: %d", rec.Code)
	}
	// A writer can push a pipeline change with plain git, but can't publish it.
	git(t, repo, "checkout", "-q", "draft/hello")
	os.MkdirAll(filepath.Join(repo, ".gitea", "workflows"), 0o755)
	os.WriteFile(filepath.Join(repo, ".gitea", "workflows", "deploy.yml"), []byte("on: push\n"), 0o644)
	git(t, repo, "add", "-A")
	git(t, repo, "commit", "-q", "-m", "pipeline")
	git(t, repo, "checkout", "-q", "main")
	rec = post(s, "/api/sites/site/publish", jar, map[string]any{"branch": "draft/hello"})
	if rec.Code != 403 || len(ff.merged) != 1 || !strings.Contains(rec.Body.String(), ".gitea/workflows/deploy.yml") {
		t.Errorf("a writer published a pipeline change: %d %s merged %v", rec.Code, rec.Body, ff.merged)
	}
}

func TestCompose(t *testing.T) {
	orig := []byte("---\n# kept comment\ntitle: Old\ntags: [a, b]\nweight: 3\n---\nBody.\n")
	got, err := compose(orig, Edit{Set: rawSet(map[string]any{"title": "New", "weight": nil, "summary": "Line one"}), Body: "New body."})
	if err != nil {
		t.Fatal(err)
	}
	want := "---\n# kept comment\ntitle: New\ntags: [a, b]\nsummary: Line one\n---\n\nNew body.\n"
	if string(got) != want {
		t.Errorf("compose:\n%q\nwant\n%q", got, want)
	}
	if _, err := compose(orig, Edit{FrontMatter: strPtr("title: [unclosed"), Body: "x"}); err == nil {
		t.Error("invalid YAML accepted")
	}
}

func strPtr(s string) *string { return &s }

// addAccount signs in as another user while already signed in, the way
// "Add an account" does, and returns the cookies the browser then holds.
func addAccount(t *testing.T, s *Server, ff *fakeForge, jar []*http.Cookie, code string) []*http.Cookie {
	t.Helper()
	rec := do(s, "GET", "/auth/login?forge=forge.test&select=1&return=/", jar, nil)
	loc, _ := url.Parse(rec.Header().Get("Location"))
	ff.challenge = loc.Query().Get("code_challenge")
	rec = do(s, "GET", "/auth/callback?code="+code+"&state="+loc.Query().Get("state"), append(jar, cookie(rec, stateCookie)), nil)
	if rec.Code != 302 {
		t.Fatalf("second sign-in: %d %s", rec.Code, rec.Body)
	}
	out := map[string]*http.Cookie{}
	for _, c := range jar {
		out[c.Name] = c
	}
	for _, c := range rec.Result().Cookies() {
		if strings.HasPrefix(c.Name, accountPrefix) {
			out[c.Name] = c
		}
	}
	var res []*http.Cookie
	for _, c := range out {
		res = append(res, c)
	}
	return res
}

func TestMultiAccount(t *testing.T) {
	s, ff := newTestServer(t)
	ada := signIn(t, s, ff)
	jar := addAccount(t, s, ff, []*http.Cookie{ada}, "bob-code")
	if len(jar) != 2 {
		t.Fatalf("expected ada in slot 0 and bob in slot 1, got %d cookies", len(jar))
	}

	var sess struct {
		Accounts []map[string]string
	}
	json.NewDecoder(do(s, "GET", "/api/session", jar, nil).Body).Decode(&sess)
	if len(sess.Accounts) != 2 || sess.Accounts[0]["id"] != "forge.test/ada" || sess.Accounts[1]["id"] != "forge.test/bob" {
		t.Fatalf("session: %+v", sess)
	}

	// Each site, with the accounts that can open it.
	var sites []struct {
		ID       string
		CanWrite bool
		Accounts []siteAccount
	}
	json.NewDecoder(do(s, "GET", "/api/sites", jar, nil).Body).Decode(&sites)
	if len(sites) != 2 || sites[0].ID != "site" || len(sites[0].Accounts) != 2 || !sites[0].Accounts[0].CanWrite || sites[0].Accounts[1].CanWrite ||
		sites[1].ID != "secret" || len(sites[1].Accounts) != 1 || sites[1].Accounts[0].Login != "bob" {
		t.Fatalf("sites: %+v", sites)
	}

	// The default is the account that can write; ?as= picks another.
	asOf := func(path string) (string, bool) {
		var d struct {
			As       string
			CanWrite bool
		}
		json.NewDecoder(do(s, "GET", path, jar, nil).Body).Decode(&d)
		return d.As, d.CanWrite
	}
	if as, w := asOf("/api/sites/site"); as != "forge.test/ada" || !w {
		t.Errorf("default account: %s %v", as, w)
	}
	if as, w := asOf("/api/sites/site?as=forge.test/bob"); as != "forge.test/bob" || w {
		t.Errorf("as bob: %s %v", as, w)
	}
	if as, _ := asOf("/api/sites/site?as=forge.test/nobody"); as != "forge.test/ada" {
		t.Errorf("an account that isn't signed in falls back: %s", as)
	}
	if as, w := asOf("/api/sites/secret"); as != "forge.test/bob" || !w {
		t.Errorf("secret: %s %v", as, w)
	}
	// Writing as the read-only account is refused by the platform's answer.
	if rec := post(s, "/api/sites/site/save?as=forge.test/bob", jar, map[string]any{"source": "content/about.md", "edit": map[string]any{"body": "x"}}); rec.Code != 403 {
		t.Errorf("save as a reader: %d", rec.Code)
	}

	// Signing in as Ada again refreshes her slot instead of taking a third.
	again := addAccount(t, s, ff, jar, "the-code")
	if len(again) != 2 {
		t.Errorf("re-sign-in took a new slot: %d cookies", len(again))
	}

	// When the platform stops taking Bob's token, only Bob is signed out.
	ff.bobGone = true
	s.accessSeen = nil
	rec := do(s, "GET", "/api/sites/secret", jar, nil)
	if rec.Code != 401 || !strings.Contains(rec.Body.String(), "@bob") {
		t.Fatalf("expired account: %d %s", rec.Code, rec.Body)
	}
	if c := cookie(rec, accountCookie(1)); c == nil || c.MaxAge >= 0 {
		t.Errorf("bob's cookie wasn't cleared: %+v", c)
	}
	if c := cookie(rec, accountCookie(0)); c != nil {
		t.Errorf("ada's cookie was touched: %+v", c)
	}
	ff.bobGone = false

	// Signing out one account leaves the other.
	rec = do(s, "POST", "/auth/logout?account=forge.test/bob", jar, map[string]string{"Origin": "http://editor.test", "X-HotDog": "1"})
	if rec.Code != 204 || cookie(rec, accountCookie(1)) == nil || cookie(rec, accountCookie(0)) != nil {
		t.Errorf("sign out one: %d %v", rec.Code, rec.Result().Cookies())
	}
	rec = do(s, "POST", "/auth/logout", jar, map[string]string{"Origin": "http://editor.test", "X-HotDog": "1"})
	if rec.Code != 204 || cookie(rec, accountCookie(0)) == nil || cookie(rec, accountCookie(1)) == nil {
		t.Errorf("sign out all: %d %v", rec.Code, rec.Result().Cookies())
	}

	// GitHub is asked to let the person choose; other platforms aren't.
	if loc := do(s, "GET", "/auth/login?forge=forge.test&select=1", nil, nil).Header().Get("Location"); strings.Contains(loc, "prompt=") {
		t.Errorf("Gitea got a prompt: %s", loc)
	}
}

func TestDiscover(t *testing.T) {
	s, ff, _ := newTestServerWith(t, `, discover: { owners: [owner] }`)
	jar := []*http.Cookie{signIn(t, s, ff)}

	var sites []struct {
		ID, Name, Repo string
		Discovered     bool
		CanWrite       bool
	}
	json.NewDecoder(do(s, "GET", "/api/sites", jar, nil).Body).Decode(&sites)
	// The configured site, then the blog: not the repository without a
	// site.yaml, the other tool's, the archived one, the other owner's, or
	// one whose clone address is on another host.
	if len(sites) != 2 || sites[0].ID != "site" || sites[0].Discovered || !strings.HasPrefix(sites[1].ID, "owner-blog-") || sites[1].Name != "The Blog" || !sites[1].Discovered || !sites[1].CanWrite {
		t.Fatalf("sites: %+v", sites)
	}
	blogID := sites[1].ID

	// A found site opens like any other, by its id.
	rec := do(s, "GET", "/api/sites/"+blogID, jar, nil)
	var d struct {
		Name, Repo, As string
		CanWrite       bool
	}
	json.NewDecoder(rec.Body).Decode(&d)
	if rec.Code != 200 || d.Repo != "https://forge.test/owner/blog.git" || !d.CanWrite || d.As != "forge.test/ada" {
		t.Fatalf("found site: %d %+v %s", rec.Code, d, rec.Body)
	}

	// The listing is remembered, not repeated on every request.
	if ff.listed != 1 {
		t.Errorf("listed repositories %d times", ff.listed)
	}

	// Without discover, only editor.yaml's sites are offered.
	s2, ff2, _ := newTestServerWith(t, "")
	jar2 := []*http.Cookie{signIn(t, s2, ff2)}
	json.NewDecoder(do(s2, "GET", "/api/sites", jar2, nil).Body).Decode(&sites)
	if len(sites) != 1 || ff2.listed != 0 {
		t.Errorf("discover off: %+v, listed %d", sites, ff2.listed)
	}
	if rec := do(s2, "GET", "/api/sites/"+blogID, jar2, nil); rec.Code != 404 {
		t.Errorf("a site not offered opened: %d", rec.Code)
	}
}

func TestDiscoverSetting(t *testing.T) {
	for _, c := range []struct {
		yaml string
		on   bool
	}{{"", false}, {", discover: true", true}, {", discover: false", false}, {", discover: { limit: 20 }", true}} {
		s, _, _ := newTestServerWith(t, c.yaml)
		if got := s.cfg.Forges[0].Discover.on(); got != c.on {
			t.Errorf("%q: on = %v", c.yaml, got)
		}
	}
}

func TestSectionsKeepOrder(t *testing.T) {
	orig := []byte("---\ntitle: Home\nsections:\n  - hero:\n      heading: Hi\n      eyebrow: Hello\n---\n")
	var doc yaml.Node
	yaml.Unmarshal([]byte("title: Home\nsections:\n  - hero:\n      heading: Hi\n      eyebrow: Hello\n"), &doc)
	j, err := orderedJSON(&doc)
	if err != nil || string(j) != `{"title":"Home","sections":[{"hero":{"heading":"Hi","eyebrow":"Hello"}}]}` {
		t.Fatalf("ordered JSON: %s %v", j, err)
	}
	// A new section, reordered fields, a multi-line field and a date-looking string.
	set := map[string]json.RawMessage{"sections": json.RawMessage(`[{"text":{"heading":"Next","body":"One\nTwo"}},{"hero":{"heading":"Hi","eyebrow":"Hello","when":"2026-10-10"}}]`)}
	got, err := compose(orig, Edit{Set: set})
	if err != nil {
		t.Fatal(err)
	}
	want := "---\ntitle: Home\nsections:\n  - text:\n      heading: Next\n      body: |-\n        One\n        Two\n  - hero:\n      heading: Hi\n      eyebrow: Hello\n      when: \"2026-10-10\"\n---\n"
	if string(got) != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestEditsKeepUnchangedStyle(t *testing.T) {
	orig := []byte("---\ntitle: Home\nsections:\n  # the opening\n  - hero:\n      heading: Hi\n      buttons:\n        - { text: Go, href: /go/ }\n        - { text: Stay, href: /stay/ }\n  - closing:\n      heading: Bye\n      button: { text: Back, href: / }\n---\n")
	// The hero's heading changes and the two sections swap places.
	set := map[string]json.RawMessage{"sections": json.RawMessage(`[{"closing":{"heading":"Bye","button":{"text":"Back","href":"/"}}},{"hero":{"heading":"Hello","buttons":[{"text":"Go","href":"/go/"},{"text":"Stay","href":"/stay/"}]}}]`)}
	got, err := compose(orig, Edit{Set: set})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"button: {text: Back, href: /}", "- {text: Go, href: /go/}", "- {text: Stay, href: /stay/}", "heading: Hello", "# the opening"} {
		if !strings.Contains(string(got), want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if i, j := strings.Index(string(got), "closing:"), strings.Index(string(got), "hero:"); i > j {
		t.Errorf("order not applied:\n%s", got)
	}
}

func TestSectionsAPI(t *testing.T) {
	s, ff := newTestServer(t)
	jar := []*http.Cookie{signIn(t, s, ff)}
	var lib struct {
		Sections    []build.SectionInfo
		Data        []string
		Collections []string
	}
	rec := do(s, "GET", "/api/sites/site/sections", jar, nil)
	json.NewDecoder(rec.Body).Decode(&lib)
	names := []string{}
	for _, x := range lib.Sections {
		names = append(names, x.Name)
	}
	if rec.Code != 200 || len(lib.Sections) != 6 || strings.Join(lib.Data, ",") != "features" || strings.Join(lib.Collections, ",") != "articles" {
		t.Fatalf("sections: %d %v %v %v", rec.Code, names, lib.Data, lib.Collections)
	}
	// The page API keeps the file's key order, so sections come back as written.
	rec = do(s, "GET", "/api/sites/site/page?source=content/index.md", jar, nil)
	body := rec.Body.String()
	if i, j := strings.Index(body, `"hero"`), strings.Index(body, `"closing"`); i < 0 || j < i || !strings.Contains(body, `"eyebrow":"Built with HotDog CMS","heading"`) {
		t.Errorf("front matter order: %s", body)
	}
}

func TestLookPanel(t *testing.T) {
	s, ff, repo := newTestServerRepo(t)
	jar := []*http.Cookie{signIn(t, s, ff)}
	mainTip := git(t, repo, "rev-parse", "main")

	var lk struct {
		Look   *site.LookFile
		Values map[string]any
	}
	rec := do(s, "GET", "/api/sites/site/look", jar, nil)
	json.NewDecoder(rec.Body).Decode(&lk)
	if rec.Code != 200 || lk.Look == nil || len(lk.Look.Tokens) < 5 || len(lk.Values) != 0 {
		t.Fatalf("look: %d %+v", rec.Code, lk)
	}

	// The preview links the unsaved stylesheet, and serves it.
	rec = post(s, "/api/sites/site/look-live", jar, map[string]any{"branch": "main", "look": map[string]any{"accent": "#0a7d55"}})
	var lv map[string]string
	json.NewDecoder(rec.Body).Decode(&lv)
	u, err := url.Parse(lv["url"])
	if rec.Code != 200 || err != nil || lv["error"] != "" {
		t.Fatalf("look-live: %d %v", rec.Code, lv)
	}
	page := httptest.NewRecorder()
	req := httptest.NewRequest("GET", lv["url"], nil)
	req.Host = u.Host
	s.live.ServeHTTP(page, req)
	m := regexp.MustCompile(`href="(/look\.[0-9a-f]+\.css)"`).FindStringSubmatch(page.Body.String())
	if m == nil {
		t.Fatalf("preview page has no look stylesheet:\n%s", page.Body)
	}
	cssRec := httptest.NewRecorder()
	creq := httptest.NewRequest("GET", "http://"+u.Host+m[1], nil)
	creq.Host = u.Host
	s.live.ServeHTTP(cssRec, creq)
	if !strings.Contains(cssRec.Body.String(), "--accent: #0a7d55") || !strings.HasPrefix(cssRec.Header().Get("Content-Type"), "text/css") {
		t.Errorf("preview stylesheet: %s %s", cssRec.Header().Get("Content-Type"), cssRec.Body)
	}

	// A choice the theme doesn't allow isn't saved.
	if rec := post(s, "/api/sites/site/look-save", jar, map[string]any{"branch": "main", "baseCommit": mainTip, "look": map[string]any{"font": "comic"}}); rec.Code != 422 {
		t.Errorf("bad look saved: %d", rec.Code)
	}
	// Saving starts draft/look, with the look in site.yaml and the rest kept.
	rec = post(s, "/api/sites/site/look-save", jar, map[string]any{"branch": "main", "baseCommit": mainTip, "look": map[string]any{"accent": "#0a7d55", "font": "serif"}})
	var saved map[string]any
	json.NewDecoder(rec.Body).Decode(&saved)
	if rec.Code != 200 || saved["branch"] != "draft/look" || saved["created"] != true {
		t.Fatalf("look-save: %d %v", rec.Code, saved)
	}
	cfg := git(t, repo, "show", "draft/look:site.yaml")
	if !strings.Contains(cfg, "look:\n  accent: '#0a7d55'\n  font: serif") && !strings.Contains(cfg, "look:\n  accent: \"#0a7d55\"\n  font: serif") {
		t.Errorf("site.yaml:\n%s", cfg)
	}
	// Everything but the look is exactly as it was.
	orig := git(t, repo, "show", "main:site.yaml")
	if cfg != orig+"\nlook:\n  accent: '#0a7d55'\n  font: serif" { // git() trims the last newline
		t.Errorf("site.yaml changed beyond the look:\n%s", cfg)
	}
	// Saving again on the draft replaces the block, and an empty look removes it.
	tip := git(t, repo, "rev-parse", "draft/look")
	if rec := post(s, "/api/sites/site/look-save", jar, map[string]any{"branch": "draft/look", "baseCommit": tip, "look": map[string]any{}}); rec.Code != 200 {
		t.Fatalf("clearing the look: %d %s", rec.Code, rec.Body)
	}
	if got := git(t, repo, "show", "draft/look:site.yaml"); got != orig {
		t.Errorf("an empty look didn't restore site.yaml:\n%s", got)
	}
	if git(t, repo, "rev-parse", "main") != mainTip {
		t.Error("the look reached the publishing branch")
	}
}

func TestSetTopLevel(t *testing.T) {
	in := "name: T\nlook:\n  accent: \"#111111\"\n  font: serif\n# Menus\nmenus:\n  main:\n    - { name: Home, url: / }\n"
	got, err := setTopLevel(in, "look", map[string]any{"accent": "#222222"})
	want := "name: T\nlook:\n  accent: '#222222'\n# Menus\nmenus:\n  main:\n    - { name: Home, url: / }\n"
	if err != nil || got != want {
		t.Errorf("replace:\n%s\n%v", got, err)
	}
	if got, _ := setTopLevel(in, "look", nil); got != "name: T\n# Menus\nmenus:\n  main:\n    - { name: Home, url: / }\n" {
		t.Errorf("remove:\n%s", got)
	}
	if _, err := setTopLevel("look: { accent: '#111' }\n", "look", map[string]any{"font": "serif"}); err == nil {
		t.Error("a one-line look: was rewritten")
	}
}

func TestFilesPowerTools(t *testing.T) {
	s, ff, repo := newTestServerRepo(t)
	jar := []*http.Cookie{signIn(t, s, ff)}
	mainTip := git(t, repo, "rev-parse", "main")

	var list struct{ Files []FileInfo }
	json.NewDecoder(do(s, "GET", "/api/sites/site/files", jar, nil).Body).Decode(&list)
	paths := map[string]FileInfo{}
	for _, f := range list.Files {
		paths[f.Path] = f
	}
	if !paths["site.yaml"].Text || !paths["layouts/base.html"].Text {
		t.Fatalf("files: %v", list.Files)
	}
	for p := range paths {
		if p == ".git" || strings.HasPrefix(p, ".git/") || strings.HasPrefix(p, "public/") {
			t.Errorf("listed %s", p)
		}
	}

	var file struct {
		Content string
		Exists  bool
	}
	json.NewDecoder(do(s, "GET", "/api/sites/site/file?path=site.yaml", jar, nil).Body).Decode(&file)
	if !file.Exists || !strings.Contains(file.Content, "name: Test") {
		t.Fatalf("file: %+v", file)
	}
	edited := strings.Replace(file.Content, "name: Test", "name: Renamed", 1)

	// The diff before saving.
	var df struct{ Diff string }
	json.NewDecoder(post(s, "/api/sites/site/file-diff", jar, map[string]any{"path": "site.yaml", "content": edited}).Body).Decode(&df)
	if !strings.Contains(df.Diff, "--- a/site.yaml") || !strings.Contains(df.Diff, "-name: Test") || !strings.Contains(df.Diff, "+name: Renamed") {
		t.Fatalf("diff:\n%s", df.Diff)
	}

	// Saving starts a draft; adding and deleting files are commits on it.
	var sv map[string]any
	json.NewDecoder(post(s, "/api/sites/site/file-save", jar, map[string]any{"path": "site.yaml", "branch": "main", "baseCommit": mainTip, "content": edited}).Body).Decode(&sv)
	if sv["branch"] != "draft/site" || sv["created"] != true {
		t.Fatalf("save: %v", sv)
	}
	tip := git(t, repo, "rev-parse", "draft/site")
	if rec := post(s, "/api/sites/site/file-save", jar, map[string]any{"path": "data/team.yaml", "branch": "draft/site", "baseCommit": tip, "content": "- Ada\n"}); rec.Code != 200 {
		t.Fatalf("add: %d %s", rec.Code, rec.Body)
	}
	tip = git(t, repo, "rev-parse", "draft/site")
	if rec := post(s, "/api/sites/site/file-save", jar, map[string]any{"path": "content/contact.md", "branch": "draft/site", "baseCommit": tip, "delete": true}); rec.Code != 200 {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body)
	}
	tree := git(t, repo, "ls-tree", "-r", "--name-only", "draft/site")
	if !strings.Contains(tree, "data/team.yaml") || strings.Contains(tree, "content/contact.md") || !strings.Contains(git(t, repo, "show", "draft/site:site.yaml"), "name: Renamed") {
		t.Errorf("draft tree:\n%s", tree)
	}
	if git(t, repo, "rev-parse", "main") != mainTip {
		t.Error("a file save reached the publishing branch")
	}
	for _, bad := range []string{".git/config", "public/index.html", "../outside", "/etc/passwd", "a/../../b"} {
		if rec := post(s, "/api/sites/site/file-save", jar, map[string]any{"path": bad, "branch": "draft/site", "content": "x"}); rec.Code != 400 {
			t.Errorf("%s: %d", bad, rec.Code)
		}
	}

	// Writers can't change what runs at publish time; the fake forge makes
	// this account a writer, not a maintainer.
	for _, pipe := range []string{"publish.yaml", ".gitea/workflows/deploy.yml", ".github/workflows/ci.yml"} {
		if rec := post(s, "/api/sites/site/file-save", jar, map[string]any{"path": pipe, "branch": "draft/site", "content": "x"}); rec.Code != 403 {
			t.Errorf("%s: %d", pipe, rec.Code)
		}
	}

	// What publishing the draft would change, and its history.
	var cmp struct{ Diff, Stat string }
	json.NewDecoder(do(s, "GET", "/api/sites/site/compare?branch=draft/site", jar, nil).Body).Decode(&cmp)
	if !strings.Contains(cmp.Diff, "+name: Renamed") || !strings.Contains(cmp.Diff, "data/team.yaml") || !strings.Contains(cmp.Stat, "content/contact.md") {
		t.Errorf("compare:\n%s\n%s", cmp.Stat, cmp.Diff)
	}
	var lg struct{ Commits []Commit }
	json.NewDecoder(do(s, "GET", "/api/sites/site/log?branch=draft/site&n=5", jar, nil).Body).Decode(&lg)
	if len(lg.Commits) < 4 || lg.Commits[0].Subject != "Delete content/contact.md" || lg.Commits[1].Subject != "Add data/team.yaml" || lg.Commits[2].Subject != "Edit site.yaml" || lg.Commits[0].Author != "Ada Lovelace" {
		t.Errorf("log: %+v", lg.Commits)
	}
	json.NewDecoder(do(s, "GET", "/api/sites/site/log?branch=draft/site&path=site.yaml", jar, nil).Body).Decode(&lg)
	if len(lg.Commits) != 2 || lg.Commits[0].Subject != "Edit site.yaml" {
		t.Errorf("file log: %+v", lg.Commits)
	}
}

func TestPointAndEdit(t *testing.T) {
	l := newLiveServer(LiveConfig{Listen: "127.0.0.1:8191"}, "http://editor.test")
	u := l.show("o", t.TempDir(), "/about/", []byte("<html><body><h1>About</h1></body></html>"), nil)
	host := strings.TrimPrefix(strings.Split(u, "/")[2], "")
	get := func(p string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", "http://"+host+p, nil)
		req.Host = host
		rec := httptest.NewRecorder()
		l.ServeHTTP(rec, req)
		return rec
	}
	if b := get("/about/").Body.String(); !strings.Contains(b, `<script src="/__hotdog/jump.js" defer></script></body>`) {
		t.Errorf("page: %s", b)
	}
	js := get("/__hotdog/jump.js")
	if !strings.Contains(js.Body.String(), `const editor = "http://editor.test"`) || !strings.HasPrefix(js.Header().Get("Content-Type"), "text/javascript") {
		t.Errorf("script: %s %s", js.Header().Get("Content-Type"), js.Body)
	}
	if !strings.Contains(js.Body.String(), `replace(/\s+/g, ' ')`) {
		t.Errorf("the script's regular expression was mangled:\n%s", js.Body)
	}
}

func TestTokenRenewal(t *testing.T) {
	s, ff := newTestServer(t)
	ff.expiring = true
	jar := []*http.Cookie{signIn(t, s, ff)}

	// The token expires within the renewal window, so the next request renews
	// it first, and hands the browser the updated account.
	rec := do(s, "GET", "/api/sites", jar, nil)
	renewed := cookie(rec, accountCookie(0))
	if rec.Code != 200 || ff.refreshes != 1 || renewed == nil || renewed.MaxAge <= 0 {
		t.Fatalf("renewal: %d, %d renewals, cookie %+v", rec.Code, ff.refreshes, renewed)
	}
	// With the renewed account (good for an hour), no further renewal.
	if rec := do(s, "GET", "/api/sites", []*http.Cookie{renewed}, nil); rec.Code != 200 || ff.refreshes != 1 {
		t.Errorf("renewed again: %d, %d renewals", rec.Code, ff.refreshes)
	}
	// A burst of requests with the old account shares one renewal.
	s.renewed = nil
	for i := 0; i < 3; i++ {
		do(s, "GET", "/api/sites", jar, nil)
	}
	if ff.refreshes != 2 {
		t.Errorf("a burst renewed %d times, want once more", ff.refreshes-1)
	}
	// A refused renewal signs that account out.
	ff.refuse = true
	s.renewed = nil
	rec = do(s, "GET", "/api/sites", jar, nil)
	if c := cookie(rec, accountCookie(0)); rec.Code != 401 || c == nil || c.MaxAge >= 0 {
		t.Errorf("refused renewal: %d %+v", rec.Code, c)
	}
}

func TestMoveAndRedirects(t *testing.T) {
	s, ff, repo := newTestServerRepo(t)
	jar := []*http.Cookie{signIn(t, s, ff)}
	mainTip := git(t, repo, "rev-parse", "main")
	origCfg := git(t, repo, "show", "main:site.yaml")

	rec := post(s, "/api/sites/site/move", jar, map[string]any{"branch": "main", "baseCommit": mainTip, "source": "content/about.md", "to": "content/about-us.md"})
	var mv map[string]any
	json.NewDecoder(rec.Body).Decode(&mv)
	if rec.Code != 200 || mv["from"] != "/about/" || mv["to"] != "/about-us/" || mv["redirected"] != true || mv["created"] != true {
		t.Fatalf("move: %d %v", rec.Code, mv)
	}
	br := mv["branch"].(string)
	tree := git(t, repo, "ls-tree", "-r", "--name-only", br)
	if strings.Contains(tree, "content/about.md") || !strings.Contains(tree, "content/about-us.md") {
		t.Fatalf("tree:\n%s", tree)
	}
	cfg := git(t, repo, "show", br+":site.yaml")
	if cfg != origCfg+"\nredirects:\n  - from: /about/\n    to: /about-us/" {
		t.Errorf("site.yaml:\n%s", cfg)
	}

	// Moving it again points the first redirect at the newest address too.
	tip := git(t, repo, "rev-parse", br)
	rec = post(s, "/api/sites/site/move", jar, map[string]any{"branch": br, "baseCommit": tip, "source": "content/about-us.md", "to": "content/team.md"})
	if rec.Code != 200 {
		t.Fatalf("second move: %d %s", rec.Code, rec.Body)
	}
	cfg = git(t, repo, "show", br+":site.yaml")
	if !strings.Contains(cfg, "  - from: /about/\n    to: /team/\n  - from: /about-us/\n    to: /team/") {
		t.Errorf("chained redirects:\n%s", cfg)
	}

	// The table: read, refused when it would hide a page, saved when right.
	var rd struct{ Redirects []site.Redirect }
	json.NewDecoder(do(s, "GET", "/api/sites/site/redirects?branch="+br, jar, nil).Body).Decode(&rd)
	if len(rd.Redirects) != 2 {
		t.Fatalf("redirects: %+v", rd.Redirects)
	}
	tip = git(t, repo, "rev-parse", br)
	if rec := post(s, "/api/sites/site/redirects-save", jar, map[string]any{"branch": br, "baseCommit": tip, "redirects": []map[string]string{{"from": "/contact/", "to": "/team/"}}}); rec.Code != 422 || !strings.Contains(rec.Body.String(), "is a page") {
		t.Errorf("redirect over a page: %d %s", rec.Code, rec.Body)
	}
	if rec := post(s, "/api/sites/site/redirects-save", jar, map[string]any{"branch": br, "baseCommit": tip, "redirects": []map[string]string{{"from": "/old/", "to": "https://elsewhere.example/"}}}); rec.Code != 200 {
		t.Fatalf("save: %d %s", rec.Code, rec.Body)
	}
	if cfg := git(t, repo, "show", br+":site.yaml"); cfg != origCfg+"\nredirects:\n  - from: /old/\n    to: https://elsewhere.example/" {
		t.Errorf("after the table:\n%s", cfg)
	}
	if git(t, repo, "rev-parse", "main") != mainTip {
		t.Error("main moved")
	}
}

func TestCheckRedirects(t *testing.T) {
	for _, c := range []struct {
		list []site.Redirect
		want string
	}{
		{[]site.Redirect{{From: "old", To: "/new/"}}, "starting with /"},
		{[]site.Redirect{{From: "/a/", To: "/b/"}, {From: "/a/", To: "/c/"}}, "twice"},
		{[]site.Redirect{{From: "/a/", To: "javascript:alert(1)"}}, "should start with"},
		{[]site.Redirect{{From: "/a/", To: "/a/"}}, "itself"},
	} {
		if err := checkRedirects(c.list, map[string]bool{}); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%v: %v", c.list, err)
		}
	}
}

func TestPrivacySettings(t *testing.T) {
	s, ff, repo := newTestServerRepo(t)
	jar := []*http.Cookie{signIn(t, s, ff)}
	mainTip := git(t, repo, "rev-parse", "main")
	orig := git(t, repo, "show", "main:site.yaml")

	if rec := post(s, "/api/sites/site/privacy-save", jar, map[string]any{"branch": "main", "baseCommit": mainTip, "analytics": map[string]any{"provider": "ga4", "id": "UA-1"}}); rec.Code != 422 || !strings.Contains(rec.Body.String(), "G-ABC123") {
		t.Errorf("bad id: %d %s", rec.Code, rec.Body)
	}
	if rec := post(s, "/api/sites/site/privacy-save", jar, map[string]any{"branch": "main", "baseCommit": mainTip, "privacy": map[string]string{"contact": "not an address"}}); rec.Code != 422 {
		t.Errorf("bad contact: %d", rec.Code)
	}
	rec := post(s, "/api/sites/site/privacy-save", jar, map[string]any{"branch": "main", "baseCommit": mainTip,
		"analytics": map[string]any{"provider": "plausible", "domain": "example.org"},
		"privacy":   map[string]string{"controller": "Test Co", "contact": "privacy@example.org"}})
	var sv map[string]any
	json.NewDecoder(rec.Body).Decode(&sv)
	if rec.Code != 200 || sv["branch"] != "draft/privacy" {
		t.Fatalf("save: %d %v", rec.Code, sv)
	}
	cfg := git(t, repo, "show", "draft/privacy:site.yaml")
	if cfg != orig+"\nanalytics:\n  domain: example.org\n  provider: plausible\nprivacy:\n  contact: privacy@example.org\n  controller: Test Co" {
		t.Errorf("site.yaml:\n%s", cfg)
	}
	// Counting without asking is written down as a choice.
	tip := git(t, repo, "rev-parse", "draft/privacy")
	post(s, "/api/sites/site/privacy-save", jar, map[string]any{"branch": "draft/privacy", "baseCommit": tip,
		"analytics": map[string]any{"provider": "plausible", "domain": "example.org", "gated": false}, "privacy": map[string]string{}})
	cfg = git(t, repo, "show", "draft/privacy:site.yaml")
	if !strings.Contains(cfg, "  gated: false") || strings.Contains(cfg, "\nprivacy:") {
		t.Errorf("ungated:\n%s", cfg)
	}
	var got struct {
		Analytics *site.AnalyticsConfig
	}
	json.NewDecoder(do(s, "GET", "/api/sites/site/privacy?branch=draft/privacy", jar, nil).Body).Decode(&got)
	if got.Analytics == nil || got.Analytics.IsGated() {
		t.Errorf("read back: %+v", got.Analytics)
	}
}

func TestSearchEngineSettings(t *testing.T) {
	s, ff, repo := newTestServerRepo(t)
	jar := []*http.Cookie{signIn(t, s, ff)}
	mainTip := git(t, repo, "rev-parse", "main")
	orig := git(t, repo, "show", "main:site.yaml")
	// Pasting the whole tag is fine: the code is taken out of it.
	rec := post(s, "/api/sites/site/search-save", jar, map[string]any{"branch": "main", "baseCommit": mainTip,
		"verify": map[string]string{"google": `<meta name="google-site-verification" content="abc123XYZ">`, "bing": "B1NG2"}, "indexnow": "0123456789abcdef"})
	if rec.Code != 200 {
		t.Fatalf("save: %d %s", rec.Code, rec.Body)
	}
	cfg := git(t, repo, "show", "draft/search-engines:site.yaml")
	if cfg != orig+"\nverify:\n  bing: B1NG2\n  google: abc123XYZ\nindexnow:\n  key: 0123456789abcdef" {
		t.Errorf("site.yaml:\n%s", cfg)
	}
	if rec := post(s, "/api/sites/site/search-save", jar, map[string]any{"branch": "main", "baseCommit": mainTip, "indexnow": "short"}); rec.Code != 422 {
		t.Errorf("short key: %d", rec.Code)
	}
}

func TestFormBuilder(t *testing.T) {
	s, ff, repo := newTestServerRepo(t)
	jar := []*http.Cookie{signIn(t, s, ff)}
	mainTip := git(t, repo, "rev-parse", "main")

	var list struct {
		Forms []struct {
			Name   string
			Form   forms.Form
			UsedBy []string
		}
	}
	json.NewDecoder(do(s, "GET", "/api/sites/site/forms", jar, nil).Body).Decode(&list)
	if len(list.Forms) != 1 || list.Forms[0].Name != "contact" || len(list.Forms[0].UsedBy) != 1 || list.Forms[0].UsedBy[0] != "content/contact.md" || len(list.Forms[0].Form.Fields) < 2 {
		t.Fatalf("forms: %+v", list.Forms)
	}
	fm := list.Forms[0].Form
	fm.Submit = "Send it"
	fm.Fields = append(fm.Fields, forms.Field{Name: "phone", Type: "tel", Label: "Phone"})

	// The preview shows the edited form on the page that uses it.
	rec := post(s, "/api/sites/site/form-live", jar, map[string]any{"branch": "main", "name": "contact", "form": fm})
	var lv map[string]string
	json.NewDecoder(rec.Body).Decode(&lv)
	u, err := url.Parse(lv["url"])
	if rec.Code != 200 || err != nil || lv["error"] != "" {
		t.Fatalf("form-live: %d %v", rec.Code, lv)
	}
	req := httptest.NewRequest("GET", lv["url"], nil)
	req.Host = u.Host
	page := httptest.NewRecorder()
	s.live.ServeHTTP(page, req)
	if !strings.Contains(page.Body.String(), "Send it") || !strings.Contains(page.Body.String(), `name="phone"`) {
		t.Errorf("preview:\n%s", page.Body)
	}

	// Saving keeps the file's opening comments.
	orig := git(t, repo, "show", "main:forms/contact.yaml")
	rec = post(s, "/api/sites/site/form-save", jar, map[string]any{"branch": "main", "baseCommit": mainTip, "name": "contact", "form": fm})
	var sv map[string]any
	json.NewDecoder(rec.Body).Decode(&sv)
	if rec.Code != 200 || sv["branch"] != "draft/form-contact" {
		t.Fatalf("save: %d %v", rec.Code, sv)
	}
	saved := git(t, repo, "show", "draft/form-contact:forms/contact.yaml")
	if !strings.HasPrefix(saved, strings.SplitN(orig, "\n", 2)[0]) || !strings.Contains(saved, "submit: Send it") || !strings.Contains(saved, "- name: phone\n    type: tel") {
		t.Errorf("saved:\n%s", saved)
	}
	// A field type that doesn't exist is refused; a new form is a new file.
	tip := git(t, repo, "rev-parse", "draft/form-contact")
	bad := forms.Form{To: "a@example.org", Fields: []forms.Field{{Name: "x", Type: "colour"}}}
	if rec := post(s, "/api/sites/site/form-save", jar, map[string]any{"branch": "draft/form-contact", "baseCommit": tip, "name": "newsletter", "form": bad}); rec.Code != 422 {
		t.Errorf("bad type: %d %s", rec.Code, rec.Body)
	}
	good := forms.Form{To: "a@example.org", Fields: []forms.Field{{Name: "email", Type: "email", Required: true}}}
	if rec := post(s, "/api/sites/site/form-save", jar, map[string]any{"branch": "draft/form-contact", "baseCommit": tip, "name": "newsletter", "form": good}); rec.Code != 200 {
		t.Errorf("new form: %d %s", rec.Code, rec.Body)
	}
	if got := git(t, repo, "show", "draft/form-contact:forms/newsletter.yaml"); got != "to: a@example.org\nfields:\n  - name: email\n    type: email\n    required: true" {
		t.Errorf("new form file:\n%s", got)
	}

	// Submissions come through the endpoint's viewer, with the editor's token.
	var askedWith string
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		askedWith = r.Header.Get("Authorization")
		if r.URL.Path != "/sites/example.org/forms/contact" {
			w.WriteHeader(404)
			return
		}
		w.Write([]byte(`{"form":"contact","total":1,"submissions":[{"time":"2026-10-10T12:00:00Z","values":{"name":"Ada"}}]}`))
	}))
	defer fake.Close()
	t.Setenv("SUBS_TOKEN", strings.Repeat("s", 40))
	s.cfg.Sites[0].Submissions = SubmissionsConfig{URL: fake.URL, TokenEnv: "SUBS_TOKEN", Site: "example.org"}
	rec = do(s, "GET", "/api/sites/site/submissions?form=contact", jar, nil)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"Ada"`) || askedWith != "Bearer "+strings.Repeat("s", 40) {
		t.Fatalf("submissions: %d %s (asked with %q)", rec.Code, rec.Body, askedWith)
	}
	// Someone who can only read the site can't read its submissions.
	bob := addAccount(t, s, ff, nil, "bob-code")
	if rec := do(s, "GET", "/api/sites/site/submissions?form=contact&as=forge.test/bob", bob, nil); rec.Code != 403 {
		t.Errorf("reader: %d", rec.Code)
	}
}

func TestPeopleAndUndo(t *testing.T) {
	s, ff, repo := newTestServerRepo(t)
	jar := []*http.Cookie{signIn(t, s, ff)}

	var people struct{ People []Person }
	json.NewDecoder(do(s, "GET", "/api/sites/site/people", jar, nil).Body).Decode(&people)
	if len(people.People) != 1 || people.People[0].Login != "carol" {
		t.Fatalf("people: %+v", people.People)
	}
	var d struct{ Role string }
	json.NewDecoder(do(s, "GET", "/api/sites/site", jar, nil).Body).Decode(&d)
	if d.Role != "writer" {
		t.Errorf("role: %q", d.Role)
	}

	// A review that asks Carol: the platform is asked to notify her.
	git(t, repo, "checkout", "-q", "-b", "draft/x")
	git(t, repo, "commit", "-q", "--allow-empty", "-m", "x")
	git(t, repo, "checkout", "-q", "main")
	rec := post(s, "/api/sites/site/review", jar, map[string]any{"branch": "draft/x", "title": "X", "description": "", "reviewers": []string{"carol"}})
	if rec.Code != 200 || len(ff.requested) != 1 || ff.requested[0] != "carol" {
		t.Fatalf("review with reviewers: %d %s %v", rec.Code, rec.Body, ff.requested)
	}

	// A published change, then undone on a draft.
	before := git(t, repo, "show", "main:content/about.md")
	os.WriteFile(filepath.Join(repo, "content", "about.md"), []byte(before+"\nA line that was a mistake.\n"), 0o644)
	git(t, repo, "commit", "-q", "-am", "Oops")
	bad := git(t, repo, "rev-parse", "HEAD")
	mainTip := bad
	rec = post(s, "/api/sites/site/revert", jar, map[string]any{"sha": bad})
	var rv map[string]any
	json.NewDecoder(rec.Body).Decode(&rv)
	if rec.Code != 200 || !strings.HasPrefix(rv["branch"].(string), "draft/undo-") {
		t.Fatalf("revert: %d %v", rec.Code, rv)
	}
	if got := git(t, repo, "show", rv["branch"].(string)+":content/about.md"); got != strings.TrimRight(before, "\n") {
		t.Errorf("undone file:\n%s", got)
	}
	if git(t, repo, "rev-parse", "main") != mainTip {
		t.Error("undo reached the publishing branch")
	}
	if subj := git(t, repo, "log", "-1", "--format=%s", rv["branch"].(string)); !strings.HasPrefix(subj, `Revert "Oops"`) {
		t.Errorf("subject: %s", subj)
	}
	if rec := post(s, "/api/sites/site/revert", jar, map[string]any{"sha": "not-a-sha"}); rec.Code != 400 {
		t.Errorf("bad sha: %d", rec.Code)
	}
}

func TestDiscoverFindsNewRepos(t *testing.T) {
	s, ff, _ := newTestServerWith(t, `, discover: { owners: [owner] }`)
	jar := []*http.Cookie{signIn(t, s, ff)}
	do(s, "GET", "/api/sites", jar, nil) // caches the listing
	if ff.listed != 1 {
		t.Fatalf("listed %d", ff.listed)
	}
	// Age the cached listing past 30 seconds: an unknown id looks again.
	s.found.mu.Lock()
	for k, d := range s.found.seen {
		d.at = d.at.Add(-time.Minute)
		s.found.seen[k] = d
	}
	s.found.mu.Unlock()
	if rec := do(s, "GET", "/api/sites/owner-newsite", jar, nil); rec.Code != 404 || ff.listed != 2 {
		t.Errorf("unknown id: %d, listed %d", rec.Code, ff.listed)
	}
	// Straight after, it doesn't ask again.
	do(s, "GET", "/api/sites/owner-newsite", jar, nil)
	if ff.listed != 2 {
		t.Errorf("asked again at once: listed %d", ff.listed)
	}
}

func TestPipelineFile(t *testing.T) {
	for p, want := range map[string]bool{
		"publish.yaml": true, "site/publish.yml": true, ".gitea/workflows/a.yml": true, ".github/dependabot.yml": true,
		".forgejo/workflows/b.yaml": true, ".gitlab-ci.yml": true, ".woodpecker/build.yml": true, ".woodpecker.yml": true,
		"Jenkinsfile": true, "content/publish.md": false, "site.yaml": false, "docs/.github.md": false, "assets/ci.css": false,
	} {
		if pipelineFile(p) != want {
			t.Errorf("%s: want %v", p, want)
		}
	}
}

func TestAboutBeforeSignIn(t *testing.T) {
	s, _ := newTestServer(t)
	s.cfg.Links = []about.Link{{Name: "Privacy", URL: "https://example.org/privacy/"}}
	rec := do(s, "GET", "/api/about", nil, nil)
	var a about.Info
	json.NewDecoder(rec.Body).Decode(&a)
	if rec.Code != 200 || a.License != "AGPL-3.0-or-later" || !strings.HasPrefix(a.Source, about.DefaultSource) || a.CompanyURL != "https://coffeylabs.org" || len(a.Links) != 1 {
		t.Errorf("about: %d %+v", rec.Code, a)
	}
}

func TestQuickPosts(t *testing.T) {
	s, ff, repo := newTestServerRepo(t)
	jar := []*http.Cookie{signIn(t, s, ff)}

	var info QuickInfo
	json.NewDecoder(do(s, "GET", "/api/sites/site/quick", jar, nil).Body).Decode(&info)
	if info.Timezone != "UTC" || len(info.Collections) == 0 || info.Collections[0].Name != "articles" {
		t.Fatalf("quick info: %+v", info)
	}

	// A banner, posted and published in one go.
	var out map[string]any
	rec := post(s, "/api/sites/site/quick", jar, map[string]any{"kind": "banner", "publish": true, "text": "Closed Monday.", "tone": "alert", "link": "/about/", "linkText": "Hours", "until": "2099-01-01 00:00"})
	json.NewDecoder(rec.Body).Decode(&out)
	if rec.Code != 200 || out["published"] != true || len(ff.merged) != 1 {
		t.Fatalf("banner: %d %v merged %v", rec.Code, out, ff.merged)
	}
	branch := out["branch"].(string)
	banners := git(t, repo, "show", branch+":data/banners.yaml")
	if !strings.Contains(banners, "text: Closed Monday.") || !strings.Contains(banners, "tone: alert") || !strings.Contains(banners, "until: 2099-01-01 00:00") {
		t.Errorf("banners.yaml:\n%s", banners)
	}
	if !strings.HasPrefix(ff.opened[len(ff.opened)-1]["title"], "Quick post: Banner: Closed Monday.") {
		t.Errorf("review title: %v", ff.opened[len(ff.opened)-1])
	}

	// A news post, left for review when the platform won't let this person merge.
	ff.refuseMerge = true
	out = map[string]any{}
	rec = post(s, "/api/sites/site/quick", jar, map[string]any{"kind": "post", "publish": true, "collection": "articles", "title": "Holiday hours", "summary": "When we're open.", "body": "Closed **Monday**."})
	json.NewDecoder(rec.Body).Decode(&out)
	if rec.Code != 200 || out["published"] == true || !strings.Contains(out["message"].(string), "couldn't be published from here") {
		t.Fatalf("post: %d %v", rec.Code, out)
	}
	page := git(t, repo, "show", out["branch"].(string)+":content/articles/holiday-hours.md")
	if !strings.HasPrefix(page, "---\ntitle: Holiday hours\ndate: ") || !strings.Contains(page, "summary: When we're open.\n---\nClosed **Monday**.") {
		t.Errorf("post:\n%s", page)
	}

	// Mistakes are caught before anything is committed.
	for _, bad := range []map[string]any{
		{"kind": "banner", "text": ""},
		{"kind": "banner", "text": "x", "link": "javascript:alert(1)"},
		{"kind": "banner", "text": "x", "until": "2000-01-01 00:00"},
		{"kind": "post", "collection": "articles", "title": ""},
		{"kind": "post", "collection": "../etc", "title": "x"},
		{"kind": "event", "collection": "articles", "title": "x", "date": "2026-11-07"},
	} {
		if rec := post(s, "/api/sites/site/quick", jar, bad); rec.Code != 400 {
			t.Errorf("%v: %d %s", bad, rec.Code, rec.Body)
		}
	}
}

func TestSafeReturn(t *testing.T) {
	for in, want := range map[string]string{
		"/sites/x?branch=draft/a": "/sites/x?branch=draft/a",
		"/":                       "/",
		"//evil.example/":         "/",
		"/\t/evil.example/":       "/",
		"/\\evil.example":         "/",
		"/%09/evil.example":       "/%09/evil.example", // stays a path on the editor: browsers don't decode %09 here
		"https://evil.example/":   "/",
		"/ /x":                    "/",
		"javascript:alert(1)":     "/",
		"":                        "/",
	} {
		if got := safeReturn(in); got != want {
			t.Errorf("safeReturn(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestHostCookiesOverHTTPS(t *testing.T) {
	sl, err := newSealer(strings.Repeat("k", 32))
	if err != nil {
		t.Fatal(err)
	}
	sl.secure = true
	rec := httptest.NewRecorder()
	if err := sl.setCookie(rec, accountCookie(0), map[string]string{"a": "b"}, time.Hour); err != nil {
		t.Fatal(err)
	}
	c := rec.Result().Cookies()[0]
	if c.Name != "__Host-hotdog_a0" || !c.Secure || c.Path != "/" || c.Domain != "" || !c.HttpOnly {
		t.Errorf("cookie over https: %+v", c)
	}
}

func TestNewPageOnDraft(t *testing.T) {
	s, ff, repo := newTestServerRepo(t)
	jar := []*http.Cookie{signIn(t, s, ff)}
	var a, b map[string]any
	json.NewDecoder(post(s, "/api/sites/site/new", jar, map[string]any{"collection": "", "title": "First"}).Body).Decode(&a)
	if a["branch"] != "draft/first" {
		t.Fatalf("new page: %v", a)
	}
	json.NewDecoder(post(s, "/api/sites/site/new", jar, map[string]any{"collection": "", "title": "Second", "branch": "draft/first"}).Body).Decode(&b)
	if b["branch"] != "draft/first" {
		t.Fatalf("second page started its own draft: %v", b)
	}
	if tree := git(t, repo, "ls-tree", "-r", "--name-only", "draft/first"); !strings.Contains(tree, "content/first.md") || !strings.Contains(tree, "content/second.md") {
		t.Errorf("draft tree:\n%s", tree)
	}
}
