package endpoint

import (
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/about"
	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/forms"
)

// Server answers /_hotdog/... for every configured site.
type Server struct {
	cfg       *Config
	sites     map[string]*siteRuntime
	tokens    tokens
	limit     *limiter
	mail      *mailer
	store     store
	trusted   []*net.IPNet
	turnstile string // HOTDOG_TURNSTILE_SECRET
	search    *searcher
	now       func() time.Time
	log       *log.Logger
}

// New builds a server from endpoint.yaml. The form secret must be at least 32
// characters: every endpoint behind one load balancer needs the same one.
func New(configFile string, logw io.Writer) (*Server, error) {
	cfg, sites, err := LoadConfig(configFile)
	if err != nil {
		return nil, err
	}
	secret := os.Getenv("HOTDOG_FORM_SECRET")
	if len(secret) < 32 {
		return nil, errors.New("HOTDOG_FORM_SECRET must be set, at least 32 characters (openssl rand -hex 32)")
	}
	s := &Server{
		cfg: cfg, sites: sites, tokens: tokens{secret: []byte(secret)},
		limit: newLimiter(cfg.RateLimit.Count, cfg.RateLimit.Window),
		mail:  newMailer(cfg.SMTP), turnstile: os.Getenv("HOTDOG_TURNSTILE_SECRET"),
		now: time.Now, log: log.New(logw, "", log.LstdFlags),
	}
	for _, p := range cfg.TrustedProxies {
		_, n, _ := net.ParseCIDR(p)
		s.trusted = append(s.trusted, n)
	}
	s.search = newSearcher()
	return s, nil
}

// Listen is the configured address.
func (s *Server) Listen() string { return s.cfg.Listen }

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Cache-Control", "no-store")
	if r.URL.Path == "/_hotdog/health" {
		fmt.Fprintln(w, "ok")
		return
	}
	if r.URL.Path == "/_hotdog/source" {
		http.Redirect(w, r, about.Get(s.cfg.Source, nil).Source, http.StatusFound)
		return
	}
	host := strings.ToLower(r.Host)
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	rt := s.sites[host]
	if rt == nil {
		http.Error(w, "unknown site", http.StatusNotFound)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/_hotdog/")
	switch {
	case strings.HasPrefix(path, "forms/"):
		rest := strings.TrimPrefix(path, "forms/")
		name, tail, _ := strings.Cut(rest, "/")
		f := rt.forms[name]
		if f == nil {
			http.Error(w, "no such form", http.StatusNotFound)
			return
		}
		switch {
		case tail == "token" && r.Method == http.MethodGet:
			writeJSON(w, http.StatusOK, map[string]string{"token": s.tokens.issue(host, name, s.now())})
		case tail == "" && r.Method == http.MethodPost:
			s.submit(w, r, rt, host, f)
		default:
			http.Error(w, "not here", http.StatusMethodNotAllowed)
		}
	case strings.HasPrefix(path, "search/") && r.Method == http.MethodGet:
		s.handleSearch(w, r, rt, strings.TrimSuffix(strings.TrimPrefix(path, "search/"), "/"))
	default:
		http.NotFound(w, r)
	}
}

// clientIP is the visitor's address: the peer's, unless the peer is a
// trusted proxy, in which case the nearest address in X-Forwarded-For that
// isn't one of ours.
func (s *Server) clientIP(r *http.Request) string {
	peer, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		peer = r.RemoteAddr
	}
	if !s.isTrusted(peer) {
		return peer
	}
	hops := strings.Split(r.Header.Get("X-Forwarded-For"), ",")
	for i := len(hops) - 1; i >= 0; i-- {
		h := strings.TrimSpace(hops[i])
		if h == "" {
			continue
		}
		if net.ParseIP(h) == nil {
			return peer
		}
		if !s.isTrusted(h) {
			return h
		}
	}
	return peer
}

func (s *Server) isTrusted(ip string) bool {
	p := net.ParseIP(ip)
	if p == nil {
		return false
	}
	for _, n := range s.trusted {
		if n.Contains(p) {
			return true
		}
	}
	return false
}

// sameSite reports whether the request came from a page on the site itself.
// Browsers send Origin with every form POST; a post from another site, or
// from a script that doesn't bother, is refused.
func sameSite(r *http.Request, rt *siteRuntime) bool {
	o := r.Header.Get("Origin")
	if o == "" || o == "null" {
		return false
	}
	u, err := url.Parse(o)
	if err != nil {
		return false
	}
	h := strings.ToLower(u.Hostname())
	for _, ok := range rt.hosts {
		if h == strings.ToLower(ok) {
			return true
		}
	}
	return false
}

type reply struct {
	OK       bool            `json:"ok"`
	Message  string          `json:"message,omitempty"`
	Problems []forms.Problem `json:"problems,omitempty"`
	Retoken  bool            `json:"retoken,omitempty"`
}

func (s *Server) submit(w http.ResponseWriter, r *http.Request, rt *siteRuntime, host string, f *forms.Form) {
	wantsJSON := strings.Contains(r.Header.Get("Accept"), "application/json")
	answer := func(code int, rp reply) {
		if wantsJSON {
			writeJSON(w, code, rp)
			return
		}
		if rp.OK && f.Success != "" {
			http.Redirect(w, r, f.Success, http.StatusSeeOther)
			return
		}
		writeHTML(w, code, rp)
	}
	if !sameSite(r, rt) {
		answer(http.StatusForbidden, reply{Message: "This form can only be sent from its own page."})
		return
	}
	ip := s.clientIP(r)
	if !s.limit.allow(host+"|"+ip, s.now()) {
		answer(http.StatusTooManyRequests, reply{Message: "Too many messages from your connection. Please wait a few minutes and try again."})
		return
	}
	if ct, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); ct != "application/x-www-form-urlencoded" {
		answer(http.StatusUnsupportedMediaType, reply{Message: "This form takes text fields only."})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if err := r.ParseForm(); err != nil {
		answer(http.StatusRequestEntityTooLarge, reply{Message: "That's more than this form accepts."})
		return
	}
	values := r.PostForm
	// The trap field: a person never sees it; a bot fills in everything.
	// Answer as if it worked, so the bot learns nothing.
	if values.Get("_website") != "" {
		s.log.Printf("%s %s: trap field filled, dropped", host, f.Name)
		answer(http.StatusOK, reply{OK: true, Message: "Thanks, your message was sent."})
		return
	}
	if err := s.tokens.check(values.Get("_t"), host, f.Name, s.now()); err != nil {
		msg := "This form needs JavaScript to send. Please reload the page and try again."
		if errors.Is(err, errTokenYoung) {
			msg = "That was quicker than a person types. Please wait a moment and send again."
		} else if errors.Is(err, errTokenOld) {
			msg = "This page has been open a long time. Please reload it and send again."
		}
		answer(http.StatusForbidden, reply{Message: msg, Retoken: true})
		return
	}
	if f.Turnstile != "" && !turnstileOK(r.Context(), s.turnstile, values.Get("cf-turnstile-response"), ip) {
		answer(http.StatusForbidden, reply{Message: "Please complete the check above the button and send again."})
		return
	}
	sub, problems := f.Check(values)
	if problems != nil {
		answer(http.StatusBadRequest, reply{Message: "Please check the highlighted fields.", Problems: problems})
		return
	}
	stored := false
	if f.Store {
		if err := s.store.append(rt.cfg.Store, sub, s.now()); err != nil {
			s.log.Printf("%s %s: storing failed: %v", host, f.Name, err)
		} else {
			stored = true
		}
	}
	if err := s.mail.deliver(rt.site.Name, sub); err != nil {
		s.log.Printf("%s %s: delivery failed: %v", host, f.Name, err)
		if !stored {
			msg := "The message couldn't be sent just now. Please try again later."
			if f.Mailto != "" {
				msg += " You can also email " + f.Mailto + "."
			}
			answer(http.StatusBadGateway, reply{Message: msg})
			return
		}
	}
	s.log.Printf("%s %s: sent", host, f.Name)
	answer(http.StatusOK, reply{OK: true, Message: "Thanks, your message was sent."})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

var replyPage = template.Must(template.New("reply").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="robots" content="noindex"><title>{{ if .OK }}Sent{{ else }}Not sent{{ end }}</title>
<style>body{font:17px/1.6 system-ui,sans-serif;max-width:36rem;margin:3rem auto;padding:0 1rem}</style></head><body>
<h1>{{ if .OK }}Sent{{ else }}Not sent yet{{ end }}</h1>
<p>{{ .Message }}</p>
{{ with .Problems }}<ul>{{ range . }}<li>{{ .Message }}</li>{{ end }}</ul>{{ end }}
<p>Use your browser's Back button to return to the form; what you typed is still there.</p>
</body></html>
`))

func writeHTML(w http.ResponseWriter, code int, rp reply) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(code)
	_ = replyPage.Execute(w, rp)
}
