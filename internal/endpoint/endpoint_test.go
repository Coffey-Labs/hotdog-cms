package endpoint

import (
	"bufio"
	"encoding/json"
	"net"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type sent struct {
	from string
	to   []string
	msg  string
}

func setup(t *testing.T, extraSite string) (*Server, *[]sent, string) {
	t.Helper()
	dir := t.TempDir()
	siteDir := filepath.Join(dir, "site")
	os.MkdirAll(filepath.Join(siteDir, "forms"), 0o755)
	os.WriteFile(filepath.Join(siteDir, "site.yaml"), []byte("name: Example\nurl: https://example.org\n"), 0o644)
	os.WriteFile(filepath.Join(siteDir, "forms", "contact.yaml"), []byte(`to: hello@example.org
subject: "Contact from {{ .name }}"
reply_to: email
store: true
fields:
  - { name: name, required: true }
  - { name: email, type: email, required: true }
  - { name: message, type: textarea, required: true }
`), 0o644)
	pub := filepath.Join(dir, "public")
	os.MkdirAll(filepath.Join(pub, "articles"), 0o755)
	os.WriteFile(filepath.Join(pub, "articles", "search.json"), []byte(`[
{"u":"/articles/a/","t":"Mail servers at home","d":"2026-10-01","g":[["mail","/tags/mail/"]],"x":"Running a mail server. Mail mail mail."},
{"u":"/articles/b/","t":"Kubernetes","d":"2026-09-01","x":"Nothing about it."},
{"u":"/articles/c/","t":"Notes","d":"2026-08-01","x":"A mail server, once."}]`), 0o644)
	cfg := `listen: 127.0.0.1:0
trusted_proxies: [127.0.0.1/32]
smtp: { host: 127.0.0.1, port: 2525, from: "Website <forms@example.org>", tls: none }
rate_limit: { count: 3, window: 1m }
sites:
  - { dir: site, public: public, store: store }
` + extraSite
	os.WriteFile(filepath.Join(dir, "endpoint.yaml"), []byte(cfg), 0o644)
	t.Setenv("HOTDOG_FORM_SECRET", strings.Repeat("k", 40))
	s, err := New(filepath.Join(dir, "endpoint.yaml"), os.Stderr)
	if err != nil {
		t.Fatal(err)
	}
	var mails []sent
	var mu sync.Mutex
	s.mail.send = func(addr, from string, to []string, msg []byte) error {
		mu.Lock()
		defer mu.Unlock()
		mails = append(mails, sent{from, to, string(msg)})
		return nil
	}
	return s, &mails, dir
}

func token(t *testing.T, s *Server) string {
	t.Helper()
	req := httptest.NewRequest("GET", "http://example.org/_hotdog/forms/contact/token", nil)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	var d map[string]string
	json.NewDecoder(rec.Body).Decode(&d)
	return d["token"]
}

func post(s *Server, v url.Values, origin, accept string, ip string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("POST", "http://example.org/_hotdog/forms/contact", strings.NewReader(v.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	if ip != "" {
		req.RemoteAddr = ip + ":5000"
	}
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	return rec
}

func TestSubmitFlow(t *testing.T) {
	s, mails, dir := setup(t, "")
	start := time.Now()
	s.now = func() time.Time { return start }
	tok := token(t, s)
	good := url.Values{"name": {"Ada"}, "email": {"ada@example.net"}, "message": {"Hello"}, "_t": {tok}, "_website": {""}}

	if rec := post(s, good, "https://example.org", "application/json", "203.0.113.1"); rec.Code != 403 || !strings.Contains(rec.Body.String(), "retoken") {
		t.Fatalf("instant submission: %d %s", rec.Code, rec.Body)
	}
	s.now = func() time.Time { return start.Add(5 * time.Second) }
	if rec := post(s, good, "https://evil.example", "application/json", "203.0.113.2"); rec.Code != 403 {
		t.Fatalf("cross-site post: %d", rec.Code)
	}
	if rec := post(s, good, "", "application/json", "203.0.113.2"); rec.Code != 403 {
		t.Fatalf("post without Origin: %d", rec.Code)
	}
	forged := url.Values{}
	for k, v := range good {
		forged[k] = v
	}
	forged.Set("_t", "v1.1."+strings.Repeat("A", 43))
	if rec := post(s, forged, "https://example.org", "application/json", "203.0.113.3"); rec.Code != 403 {
		t.Fatalf("forged token: %d", rec.Code)
	}
	if rec := post(s, good, "https://www.example.org", "application/json", "203.0.113.4"); rec.Code != 200 {
		t.Fatalf("good submission: %d %s", rec.Code, rec.Body)
	}
	if len(*mails) != 1 {
		t.Fatalf("%d mails sent", len(*mails))
	}
	m := (*mails)[0]
	for _, want := range []string{"To: <hello@example.org>", "Reply-To: <ada@example.net>", "Subject: Contact from Ada", "Name: Ada"} {
		if !strings.Contains(m.msg, want) {
			t.Errorf("mail is missing %q:\n%s", want, m.msg)
		}
	}
	stored, err := os.ReadFile(filepath.Join(dir, "store", "contact.jsonl"))
	if err != nil || !strings.Contains(string(stored), `"name":"Ada"`) {
		t.Errorf("not stored: %v %s", err, stored)
	}
	if fi, _ := os.Stat(filepath.Join(dir, "store", "contact.jsonl")); fi.Mode().Perm() != 0o600 {
		t.Errorf("store file is %v, want 0600", fi.Mode().Perm())
	}

	// The trap field: answered as success, nothing sent.
	trap := url.Values{}
	for k, v := range good {
		trap[k] = v
	}
	trap.Set("_website", "http://spam.example")
	if rec := post(s, trap, "https://example.org", "application/json", "203.0.113.5"); rec.Code != 200 || len(*mails) != 1 {
		t.Fatalf("trap field: %d, %d mails", rec.Code, len(*mails))
	}
	// Header injection through a single-line field.
	inj := url.Values{}
	for k, v := range good {
		inj[k] = v
	}
	inj.Set("name", "Ada\r\nBcc: victim@example.com")
	if rec := post(s, inj, "https://example.org", "application/json", "203.0.113.6"); rec.Code != 400 || len(*mails) != 1 {
		t.Fatalf("header injection: %d, %d mails", rec.Code, len(*mails))
	}
	// Without JavaScript: an HTML page, not JSON.
	if rec := post(s, inj, "https://example.org", "text/html", "203.0.113.7"); rec.Code != 400 || !strings.Contains(rec.Header().Get("Content-Type"), "text/html") {
		t.Fatalf("html reply: %d %s", rec.Code, rec.Header().Get("Content-Type"))
	}
}

func TestRateLimitAndProxy(t *testing.T) {
	s, _, _ := setup(t, "")
	start := time.Now()
	s.now = func() time.Time { return start }
	tok := token(t, s)
	s.now = func() time.Time { return start.Add(5 * time.Second) }
	v := url.Values{"name": {"Ada"}, "email": {"ada@example.net"}, "message": {"Hi"}, "_t": {tok}}
	codes := []int{}
	for i := 0; i < 4; i++ {
		// From the trusted proxy, for the same visitor.
		req := httptest.NewRequest("POST", "http://example.org/_hotdog/forms/contact", strings.NewReader(v.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Origin", "https://example.org")
		req.Header.Set("X-Forwarded-For", "198.51.100.9")
		req.RemoteAddr = "127.0.0.1:4000"
		rec := httptest.NewRecorder()
		s.ServeHTTP(rec, req)
		codes = append(codes, rec.Code)
	}
	if codes[3] != 429 || codes[0] != 200 {
		t.Fatalf("rate limit codes %v", codes)
	}
	// A client that isn't the proxy can't pick its own address.
	req := httptest.NewRequest("POST", "http://example.org/", nil)
	req.RemoteAddr = "203.0.113.50:1"
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	if ip := s.clientIP(req); ip != "203.0.113.50" {
		t.Fatalf("spoofed X-Forwarded-For believed: %s", ip)
	}
}

func TestSearchAndHosts(t *testing.T) {
	s, _, _ := setup(t, "")
	req := httptest.NewRequest("GET", "http://example.org/_hotdog/search/articles?q=mail+server", nil)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	var d struct {
		Total   int
		Results []struct{ U, Snippet string }
	}
	json.NewDecoder(rec.Body).Decode(&d)
	if d.Total != 2 || d.Results[0].U != "/articles/a/" || !strings.Contains(d.Results[0].Snippet, "mail server") {
		t.Fatalf("search: %d %+v", rec.Code, d)
	}
	for _, path := range []string{"/_hotdog/search/../../etc", "/_hotdog/search/nope"} {
		rec := httptest.NewRecorder()
		s.ServeHTTP(rec, httptest.NewRequest("GET", "http://example.org"+path+"?q=x", nil))
		if rec.Code != 404 {
			t.Errorf("%s: %d", path, rec.Code)
		}
	}
	rec = httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest("GET", "http://other.example/_hotdog/forms/contact/token", nil))
	if rec.Code != 404 {
		t.Errorf("unknown host: %d", rec.Code)
	}
}

func TestSecretRequired(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "endpoint.yaml"), []byte("sites: []\n"), 0o644)
	t.Setenv("HOTDOG_FORM_SECRET", "short")
	if _, err := New(filepath.Join(dir, "endpoint.yaml"), os.Stderr); err == nil {
		t.Fatal("started without a proper secret")
	}
}

// A real SMTP exchange with a small server that records what it receives.
func TestSMTPDelivery(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	got := make(chan string, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		r := bufio.NewReader(c)
		say := func(s string) { c.Write([]byte(s + "\r\n")) }
		say("220 test")
		var data strings.Builder
		inData := false
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			if inData {
				if line == ".\r\n" {
					inData = false
					say("250 ok")
					got <- data.String()
					continue
				}
				data.WriteString(line)
				continue
			}
			switch cmd := strings.ToUpper(strings.TrimSpace(line)); {
			case strings.HasPrefix(cmd, "EHLO"), strings.HasPrefix(cmd, "HELO"):
				say("250 test")
			case strings.HasPrefix(cmd, "MAIL"), strings.HasPrefix(cmd, "RCPT"):
				say("250 ok")
			case cmd == "DATA":
				inData = true
				say("354 go")
			case cmd == "QUIT":
				say("221 bye")
				return
			default:
				say("502 no")
			}
		}
	}()
	s, _, _ := setup(t, "")
	port := ln.Addr().(*net.TCPAddr).Port
	s.mail = newMailer(SMTPConfig{Host: "127.0.0.1", Port: port, From: "forms@example.org", TLS: "none"})
	f := s.sites["example.org"].forms["contact"]
	sub, problems := f.Check(map[string][]string{"name": {"Zoë"}, "email": {"z@example.net"}, "message": {"Grüße"}})
	if problems != nil {
		t.Fatal(problems)
	}
	if err := s.mail.deliver("Example", sub); err != nil {
		t.Fatal(err)
	}
	select {
	case msg := <-got:
		if !strings.Contains(msg, "Subject: =?utf-8?q?Contact_from_Zo=C3=AB?=") || !strings.Contains(msg, "Gr=C3=BC=C3=9Fe") {
			t.Errorf("message encoding wrong:\n%s", msg)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no mail arrived")
	}
}

func TestSubmissionsViewer(t *testing.T) {
	t.Setenv("VIEWER_TOKEN", strings.Repeat("v", 40))
	s, _, _ := setup(t, "viewer: { listen: 127.0.0.1:8182, token_env: VIEWER_TOKEN }\n")
	start := time.Now()
	s.now = func() time.Time { return start }
	for i, name := range []string{"Ada", "Grace"} {
		s.now = func() time.Time { return start }
		tok := token(t, s)
		s.now = func() time.Time { return start.Add(time.Duration(5+i) * time.Second) }
		v := url.Values{"name": {name}, "email": {strings.ToLower(name) + "@example.net"}, "message": {"Hello from " + name}, "_t": {tok}, "_website": {""}}
		if rec := post(s, v, "https://example.org", "application/json", "203.0.113.9"); rec.Code != 200 {
			t.Fatalf("submit %s: %d %s", name, rec.Code, rec.Body)
		}
	}
	h, addr, err := s.Viewer()
	if err != nil || h == nil || addr != "127.0.0.1:8182" {
		t.Fatalf("viewer: %v %v %s", h, err, addr)
	}
	get := func(path, tok string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", "http://"+addr+path, nil)
		if tok != "" {
			req.Header.Set("Authorization", "Bearer "+tok)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	if rec := get("/sites/example.org/forms/contact", ""); rec.Code != 401 {
		t.Errorf("no token: %d", rec.Code)
	}
	if rec := get("/sites/example.org/forms/contact", strings.Repeat("x", 40)); rec.Code != 401 {
		t.Errorf("wrong token: %d", rec.Code)
	}
	rec := get("/sites/example.org/forms/contact", strings.Repeat("v", 40))
	var d struct {
		Total       int
		Fields      []map[string]string
		Submissions []struct {
			Time   string
			Values map[string]string
		}
	}
	json.NewDecoder(rec.Body).Decode(&d)
	if rec.Code != 200 || d.Total != 2 || len(d.Submissions) != 2 || d.Submissions[0].Values["name"] != "Grace" || len(d.Fields) != 3 {
		t.Fatalf("read: %d %+v", rec.Code, d)
	}
	if rec := get("/sites/example.org/forms/contact?limit=1&offset=1", strings.Repeat("v", 40)); !strings.Contains(rec.Body.String(), `"Ada"`) || strings.Contains(rec.Body.String(), `"Grace"`) {
		t.Errorf("paging: %s", rec.Body)
	}
	// The public endpoint knows nothing of it.
	req := httptest.NewRequest("GET", "http://example.org/sites/example.org/forms/contact", nil)
	req.Header.Set("Authorization", "Bearer "+strings.Repeat("v", 40))
	pub := httptest.NewRecorder()
	s.ServeHTTP(pub, req)
	if pub.Code == 200 {
		t.Error("the public endpoint served submissions")
	}
	// A public address is refused.
	if err := (ViewerConfig{Listen: "0.0.0.0:8182", TokenEnv: "X"}).check(); err == nil {
		t.Error("viewer on 0.0.0.0 accepted")
	}
	if err := (ViewerConfig{Listen: "203.0.113.5:8182", TokenEnv: "X"}).check(); err == nil {
		t.Error("viewer on a public address accepted")
	}
	if err := (ViewerConfig{Listen: "10.0.0.5:8182", TokenEnv: "X"}).check(); err != nil {
		t.Errorf("private address refused: %v", err)
	}
}

func TestSourceLink(t *testing.T) {
	s, _, _ := setup(t, "")
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest("GET", "http://anything.example/_hotdog/source", nil))
	if rec.Code != 302 || !strings.HasPrefix(rec.Header().Get("Location"), "https://git.coffeylabs.org/coffey-labs/hotdog-cms") {
		t.Errorf("source: %d %s", rec.Code, rec.Header().Get("Location"))
	}
	s.cfg.Source = "https://git.example.org/me/hotdog-fork"
	rec = httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest("GET", "http://example.org/_hotdog/source", nil))
	if rec.Header().Get("Location") != "https://git.example.org/me/hotdog-fork" {
		t.Errorf("operator's source: %s", rec.Header().Get("Location"))
	}
}

func TestRecipients(t *testing.T) {
	sc := SiteConfig{Recipients: []string{"hello@example.org", "@team.example.org"}}
	for addr, want := range map[string]bool{"hello@example.org": true, "HELLO@example.org": true, "ann@team.example.org": true, "x@evil.example": false, "hello@example.org.evil": false, "x@notteam.example.org": false} {
		if sc.allowed(addr) != want {
			t.Errorf("%s: want %v", addr, want)
		}
	}
}

func TestLimitKeyAndTerms(t *testing.T) {
	if limitKey("2001:db8:1:2:aaaa::1") != limitKey("2001:db8:1:2:bbbb::9") || limitKey("2001:db8:1:3::1") == limitKey("2001:db8:1:2::1") {
		t.Error("IPv6 isn't counted by /64")
	}
	if limitKey("203.0.113.7") != "203.0.113.7" {
		t.Error("IPv4 changed")
	}
	if n := len(queryTerms("aa bb cc dd ee ff gg hh ii jj kk aa aa")); n != maxTerms {
		t.Errorf("%d terms", n)
	}
}
