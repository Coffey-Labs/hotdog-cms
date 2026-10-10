package editor

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"git.coffeylabs.org/coffey-labs/cl-cms/internal/forge"
	"git.coffeylabs.org/coffey-labs/cl-cms/internal/preview"
)

//go:embed all:ui/dist
var uiFiles embed.FS

// Server is the editor.
type Server struct {
	cfg  *Config
	seal *sealer
	ws   *workspaces
	http *http.Client
	log  *log.Logger
	ui   fs.FS
	csp  string
	// cloneURL is where a site is cloned from; the site's repo, except in tests.
	cloneURL func(*SiteConfig) string
}

// New builds the editor from editor.yaml. CLCMS_EDITOR_SECRET seals sessions.
func New(configFile string, logw io.Writer) (*Server, error) {
	cfg, err := LoadConfig(configFile)
	if err != nil {
		return nil, err
	}
	seal, err := newSealer(os.Getenv("CLCMS_EDITOR_SECRET"))
	if err != nil {
		return nil, err
	}
	for _, f := range cfg.Forges {
		if os.Getenv(f.ClientSecretEnv) == "" {
			return nil, fmt.Errorf("%s isn't set: the client secret for %s", f.ClientSecretEnv, f.Host)
		}
	}
	ui, _ := fs.Sub(uiFiles, "ui/dist")
	s := &Server{cfg: cfg, seal: seal, ws: newWorkspaces(cfg.Cache), http: &http.Client{Timeout: 20 * time.Second}, log: log.New(logw, "", log.LstdFlags), ui: ui,
		cloneURL: func(sc *SiteConfig) string { return sc.Repo }}
	// The editor's own pages get the strictest policy that still lets them
	// show previews: nothing inline, nothing from anywhere else, and frames
	// only from the configured preview servers.
	frames := []string{}
	for _, st := range cfg.Sites {
		if st.Preview.Domain != "" {
			u, _ := url.Parse(st.Preview.url("x"))
			frames = append(frames, u.Scheme+"://*."+st.Preview.Domain+portSuffix(st.Preview.Port))
		}
	}
	frameSrc := "'none'"
	if len(frames) > 0 {
		frameSrc = strings.Join(frames, " ")
	}
	s.csp = "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; font-src 'self'; connect-src 'self'; frame-src " + frameSrc + "; object-src 'none'; base-uri 'none'; form-action 'self'; frame-ancestors 'none'"
	return s, nil
}

func portSuffix(p string) string {
	if p == "" {
		return ""
	}
	return ":" + p
}

// Listen is the configured address.
func (s *Server) Listen() string { return s.cfg.Listen }

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h := w.Header()
	h.Set("Content-Security-Policy", s.csp)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("X-Frame-Options", "DENY")
	h.Set("Referrer-Policy", "same-origin")
	h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
	switch {
	case r.URL.Path == "/auth/login" && r.Method == http.MethodGet:
		s.login(w, r)
	case r.URL.Path == "/auth/callback" && r.Method == http.MethodGet:
		s.callback(w, r)
	case r.URL.Path == "/auth/logout" && r.Method == http.MethodPost:
		if !s.sameOrigin(r) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		clearCookie(w, r, sessionCookie)
		w.WriteHeader(http.StatusNoContent)
	case strings.HasPrefix(r.URL.Path, "/api/"):
		h.Set("Cache-Control", "no-store")
		s.apiRoute(w, r)
	default:
		s.serveUI(w, r)
	}
}

// sameOrigin guards anything that changes state: the request must come from
// the editor's own pages, and carry the header only its own script sends.
func (s *Server) sameOrigin(r *http.Request) bool {
	return r.Header.Get("Origin") == s.cfg.PublicURL && r.Header.Get("X-CLCMS") == "1"
}

func (s *Server) serveUI(w http.ResponseWriter, r *http.Request) {
	p := strings.TrimPrefix(r.URL.Path, "/")
	if p != "" {
		if f, err := s.ui.Open(p); err == nil {
			st, _ := f.Stat()
			f.Close()
			if st != nil && !st.IsDir() {
				if strings.HasPrefix(p, "assets/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				}
				http.ServeFileFS(w, r, s.ui, p)
				return
			}
		}
	}
	index, err := fs.ReadFile(s.ui, "index.html")
	if err != nil {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprintln(w, "The editor's interface isn't built into this binary. Run `make ui` (or npm run build in editor-ui/) and build again.")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(index)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func apiError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func (s *Server) apiRoute(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/")
	if path == "session" {
		s.apiSession(w, r)
		return
	}
	sess := s.seal.session(r)
	if sess == nil {
		apiError(w, http.StatusUnauthorized, "Sign in to continue.")
		return
	}
	f := s.cfg.forge(sess.Forge)
	if f == nil {
		apiError(w, http.StatusUnauthorized, "Sign in to continue.")
		return
	}
	if r.Method != http.MethodGet {
		apiError(w, http.StatusMethodNotAllowed, "read-only for now")
		return
	}
	parts := strings.Split(strings.Trim(path, "/"), "/")
	switch {
	case len(parts) == 1 && parts[0] == "sites":
		s.apiSites(w, r, sess, f)
	case len(parts) == 2 && parts[0] == "sites":
		s.apiSite(w, r, sess, f, parts[1])
	case len(parts) == 3 && parts[0] == "sites" && parts[2] == "page":
		s.apiPage(w, r, sess, f, parts[1])
	default:
		apiError(w, http.StatusNotFound, "no such thing")
	}
}

func (s *Server) apiSession(w http.ResponseWriter, r *http.Request) {
	type forgeInfo struct {
		Host string     `json:"host"`
		Kind forge.Kind `json:"kind"`
	}
	var forges []forgeInfo
	for _, f := range s.cfg.Forges {
		forges = append(forges, forgeInfo{f.Host, f.Kind})
	}
	out := map[string]any{"signedIn": false, "forges": forges}
	if sess := s.seal.session(r); sess != nil {
		out["signedIn"] = true
		out["user"] = map[string]string{"login": sess.Login, "name": sess.Name, "forge": sess.Forge}
	}
	writeJSON(w, http.StatusOK, out)
}

// sitesFor lists the configured sites on the person's platform that the
// platform lets them read.
func (s *Server) sitesFor(ctx context.Context, sess *Session, f *ForgeConfig) ([]map[string]any, error) {
	var out []map[string]any
	for i := range s.cfg.Sites {
		st := &s.cfg.Sites[i]
		u, _ := url.Parse(st.Repo)
		if !strings.EqualFold(u.Hostname(), f.Host) {
			continue
		}
		acc, err := s.access(ctx, f, sess.Token, st)
		if err != nil {
			return nil, err
		}
		if !acc.Read {
			continue
		}
		out = append(out, map[string]any{"id": st.ID, "name": st.Name, "repo": st.Repo, "branch": st.Branch, "canWrite": acc.Write})
	}
	return out, nil
}

func (s *Server) apiSites(w http.ResponseWriter, r *http.Request, sess *Session, f *ForgeConfig) {
	sites, err := s.sitesFor(r.Context(), sess, f)
	if s.signedOut(w, r, err) {
		return
	}
	if err != nil {
		apiError(w, http.StatusBadGateway, "Couldn't ask the platform which sites you can open.")
		return
	}
	if sites == nil {
		sites = []map[string]any{}
	}
	writeJSON(w, http.StatusOK, sites)
}

// signedOut ends the session when the platform has stopped accepting its token.
func (s *Server) signedOut(w http.ResponseWriter, r *http.Request, err error) bool {
	if errors.Is(err, errSignedOut) {
		clearCookie(w, r, sessionCookie)
		apiError(w, http.StatusUnauthorized, "Your sign-in has ended. Sign in again.")
		return true
	}
	return false
}

func (s *Server) siteByID(ctx context.Context, sess *Session, f *ForgeConfig, id string) (*SiteConfig, Access, error) {
	for i := range s.cfg.Sites {
		st := &s.cfg.Sites[i]
		if st.ID != id {
			continue
		}
		u, _ := url.Parse(st.Repo)
		if !strings.EqualFold(u.Hostname(), f.Host) {
			break
		}
		acc, err := s.access(ctx, f, sess.Token, st)
		if err != nil {
			return nil, Access{}, err
		}
		if !acc.Read {
			break
		}
		return st, acc, nil
	}
	return nil, Access{}, nil
}

func (s *Server) apiSite(w http.ResponseWriter, r *http.Request, sess *Session, f *ForgeConfig, id string) {
	st, acc, err := s.siteByID(r.Context(), sess, f, id)
	if s.signedOut(w, r, err) {
		return
	}
	if err != nil || st == nil {
		apiError(w, http.StatusNotFound, "No such site, or you can't open it.")
		return
	}
	branch := r.URL.Query().Get("branch")
	if branch == "" {
		branch = st.Branch
	}
	loaded, siteDir, commit, err := s.ws.checkout(sess, f.Kind, st, s.cloneURL(st), branch)
	if err != nil {
		s.log.Printf("%s opening %s@%s: %v", sess.Login, st.ID, branch, err)
		apiError(w, http.StatusBadGateway, "Couldn't fetch the site: "+trimErr(err))
		return
	}
	pages, cols := pagesOf(loaded)
	result := s.ws.buildAndCheck(siteDir, commit)

	type draft struct {
		Branch  string `json:"branch"`
		Commit  string `json:"commit"`
		Preview string `json:"preview,omitempty"`
	}
	var drafts []draft
	var reviews []map[string]any
	notYet := ""
	if bs, err := s.branches(r.Context(), f, sess.Token, st); err == nil {
		for _, b := range bs {
			if b.Name != st.Branch {
				drafts = append(drafts, draft{Branch: b.Name, Commit: b.Commit, Preview: st.Preview.url(preview.Slug(b.Name))})
			}
		}
	} else if errors.Is(err, errNotYet) {
		notYet = err.Error()
	}
	if rs, err := s.reviews(r.Context(), f, sess.Token, st); err == nil {
		for _, rv := range rs {
			reviews = append(reviews, map[string]any{"number": rv.Number, "title": rv.Title, "branch": rv.Branch, "author": rv.Author, "url": rv.URL, "preview": st.Preview.url(preview.Slug(rv.Branch))})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id": st.ID, "name": st.Name, "repo": st.Repo, "branch": branch, "publishBranch": st.Branch,
		"commit": commit, "canWrite": acc.Write, "siteName": loaded.Config.Name, "siteURL": loaded.Config.URL,
		"collections": cols, "pages": pages, "drafts": drafts, "reviews": reviews, "build": result,
		"preview": st.Preview.url(preview.Slug(branch)), "notYet": notYet,
	})
}

func (s *Server) apiPage(w http.ResponseWriter, r *http.Request, sess *Session, f *ForgeConfig, id string) {
	st, _, err := s.siteByID(r.Context(), sess, f, id)
	if s.signedOut(w, r, err) {
		return
	}
	if err != nil || st == nil {
		apiError(w, http.StatusNotFound, "No such site, or you can't open it.")
		return
	}
	branch := r.URL.Query().Get("branch")
	if branch == "" {
		branch = st.Branch
	}
	_, siteDir, _, err := s.ws.checkout(sess, f.Kind, st, s.cloneURL(st), branch)
	if err != nil {
		apiError(w, http.StatusBadGateway, "Couldn't fetch the site: "+trimErr(err))
		return
	}
	raw, err := readSource(siteDir, r.URL.Query().Get("source"))
	if err != nil {
		apiError(w, http.StatusNotFound, "No such page.")
		return
	}
	fm, body := splitFrontMatter(raw)
	writeJSON(w, http.StatusOK, map[string]any{"source": r.URL.Query().Get("source"), "frontMatter": fm, "body": body})
}

// splitFrontMatter separates the YAML block from the body, for display.
func splitFrontMatter(raw []byte) (map[string]any, string) {
	s := strings.TrimPrefix(string(raw), "\ufeff")
	fm := map[string]any{}
	if strings.HasPrefix(s, "---\n") || strings.HasPrefix(s, "---\r\n") {
		rest := s[strings.Index(s, "\n")+1:]
		if i := strings.Index(rest, "\n---"); i >= 0 {
			_ = yaml.Unmarshal([]byte(rest[:i]), &fm)
			body := rest[i+4:]
			body = strings.TrimPrefix(strings.TrimPrefix(body, "\r"), "\n")
			return fm, body
		}
	}
	return fm, s
}

func trimErr(err error) string {
	msg := err.Error()
	if len(msg) > 200 {
		msg = msg[:200] + "…"
	}
	return msg
}
