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
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/forge"
	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/preview"
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
	live     *liveServer

	accessMu   sync.Mutex
	accessSeen map[string]accessHit
	found      discoverCache

	renewMu sync.Mutex
	renewed map[string]renewal // recent token renewals, by the refresh token used
}

// New builds the editor from editor.yaml. HOTDOG_EDITOR_SECRET seals sessions.
func New(configFile string, logw io.Writer) (*Server, error) {
	cfg, err := LoadConfig(configFile)
	if err != nil {
		return nil, err
	}
	seal, err := newSealer(os.Getenv("HOTDOG_EDITOR_SECRET"))
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
	s.live = newLiveServer(cfg.Live, cfg.PublicURL)
	frames = append(frames, s.live.origin())
	frameSrc := strings.Join(frames, " ")
	s.csp = "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data: blob:; font-src 'self'; connect-src 'self'; frame-src " + frameSrc + "; object-src 'none'; base-uri 'none'; form-action 'self'; frame-ancestors 'none'"
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

// Live is the live preview server and where it listens. It must be served
// on its own address: its origin keeps sites' scripts away from the editor.
func (s *Server) Live() (http.Handler, string) { return s.live, s.live.cfg.Listen }

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
		// One account (?account=platform/login), or all of them.
		which := r.URL.Query().Get("account")
		for _, a := range s.seal.accounts(r) {
			if which == "" || a.ID() == which {
				clearCookie(w, r, accountCookie(a.slot))
			}
		}
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
	return r.Header.Get("Origin") == s.cfg.PublicURL && r.Header.Get("X-HotDog") == "1"
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
	accts := s.freshen(w, r, s.seal.accounts(r))
	if len(accts) == 0 {
		apiError(w, http.StatusUnauthorized, "Sign in to continue.")
		return
	}
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 1 && parts[0] == "sites" && r.Method == http.MethodGet {
		s.apiSites(w, r, accts)
		return
	}
	// Everything else is about one site, worked on as one account.
	if len(parts) < 2 || parts[0] != "sites" {
		apiError(w, http.StatusNotFound, "no such thing")
		return
	}
	st := s.findSite(r.Context(), accts, parts[1])
	sess := s.accountFor(r, accts, st)
	if sess == nil {
		apiError(w, http.StatusNotFound, "No such site, or you can't open it.")
		return
	}
	f := s.cfg.forge(sess.Forge)
	if f == nil {
		apiError(w, http.StatusNotFound, "No such site, or you can't open it.")
		return
	}
	r = r.WithContext(context.WithValue(r.Context(), reqKey{}, &reqInfo{sess: sess, accts: accts, site: st}))
	if r.Method == http.MethodPost {
		if !s.sameOrigin(r) {
			apiError(w, http.StatusForbidden, "That request didn't come from the editor.")
			return
		}
		if len(parts) == 3 && parts[0] == "sites" {
			s.apiWrite(w, r, sess, f, parts[1], parts[2])
			return
		}
		apiError(w, http.StatusNotFound, "no such thing")
		return
	}
	if r.Method != http.MethodGet {
		apiError(w, http.StatusMethodNotAllowed, "not here")
		return
	}
	switch {
	case len(parts) == 2 && parts[0] == "sites":
		s.apiSite(w, r, sess, f, parts[1])
	case len(parts) == 3 && parts[0] == "sites" && parts[2] == "page":
		s.apiPage(w, r, sess, f, parts[1])
	case len(parts) == 3 && parts[0] == "sites" && parts[2] == "privacy":
		s.apiPrivacy(w, r, sess, f, parts[1])
	case len(parts) == 3 && parts[0] == "sites" && parts[2] == "redirects":
		s.apiRedirects(w, r, sess, f, parts[1])
	case len(parts) == 3 && parts[0] == "sites" && parts[2] == "files":
		s.apiFiles(w, r, sess, f, parts[1])
	case len(parts) == 3 && parts[0] == "sites" && parts[2] == "file":
		s.apiFile(w, r, sess, f, parts[1])
	case len(parts) == 3 && parts[0] == "sites" && parts[2] == "compare":
		s.apiCompare(w, r, sess, f, parts[1])
	case len(parts) == 3 && parts[0] == "sites" && parts[2] == "log":
		s.apiLog(w, r, sess, f, parts[1])
	case len(parts) == 3 && parts[0] == "sites" && parts[2] == "look":
		s.apiLook(w, r, sess, f, parts[1])
	case len(parts) == 3 && parts[0] == "sites" && parts[2] == "sections":
		s.apiSections(w, r, sess, f, parts[1])
	case len(parts) == 3 && parts[0] == "sites" && parts[2] == "media":
		s.apiMedia(w, r, sess, f, parts[1])
	case len(parts) == 3 && parts[0] == "sites" && parts[2] == "media-file":
		s.apiMediaFile(w, r, sess, f, parts[1])
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
	accounts := []map[string]string{}
	for _, a := range s.seal.accounts(r) {
		accounts = append(accounts, map[string]string{"id": a.ID(), "login": a.Login, "name": a.Name, "forge": a.Forge})
	}
	writeJSON(w, http.StatusOK, map[string]any{"signedIn": len(accounts) > 0, "accounts": accounts, "forges": forges})
}

// reqInfo is the account a request works as, and every account signed in.
type reqKey struct{}

type reqInfo struct {
	sess  *Session
	accts []*Session
	site  *SiteConfig // the site the request is about
}

func infoFrom(r *http.Request) *reqInfo {
	i, _ := r.Context().Value(reqKey{}).(*reqInfo)
	return i
}

// candidates are the signed-in accounts on a site's platform.
func candidates(accts []*Session, st *SiteConfig) []*Session {
	u, err := url.Parse(st.Repo)
	if err != nil {
		return nil
	}
	var out []*Session
	for _, a := range accts {
		if strings.EqualFold(a.Forge, u.Hostname()) {
			out = append(out, a)
		}
	}
	return out
}

// accountFor picks the account a request about a site works as: the one
// named by ?as=platform/login if it's signed in, otherwise the first that
// can write to the site, otherwise the first that can read it.
func (s *Server) accountFor(r *http.Request, accts []*Session, st *SiteConfig) *Session {
	if st == nil {
		return nil
	}
	cands := candidates(accts, st)
	if len(cands) == 0 {
		return nil
	}
	if want := r.URL.Query().Get("as"); want != "" {
		for _, c := range cands {
			if c.ID() == want {
				return c
			}
		}
	}
	if len(cands) == 1 {
		return cands[0]
	}
	var reader, gone *Session
	for _, c := range cands {
		f := s.cfg.forge(c.Forge)
		if f == nil {
			continue
		}
		acc, err := s.accessCached(r.Context(), f, c.Token, st)
		if errors.Is(err, errSignedOut) && gone == nil {
			gone = c
		}
		if err != nil {
			continue
		}
		if acc.Write {
			return c
		}
		if acc.Read && reader == nil {
			reader = c
		}
	}
	if reader != nil {
		return reader
	}
	if gone != nil {
		return gone // so the person hears that this sign-in ended
	}
	return cands[0]
}

// siteAccount is an account that can open a site, as the interface lists it.
type siteAccount struct {
	ID       string `json:"id"`
	Login    string `json:"login"`
	Forge    string `json:"forge"`
	CanWrite bool   `json:"canWrite"`
}

// accountsFor lists the signed-in accounts that can open a site. An account
// whose token the platform no longer takes is signed out on the way.
func (s *Server) accountsFor(w http.ResponseWriter, r *http.Request, accts []*Session, st *SiteConfig) []siteAccount {
	out := []siteAccount{}
	for _, c := range candidates(accts, st) {
		f := s.cfg.forge(c.Forge)
		if f == nil {
			continue
		}
		acc, err := s.accessCached(r.Context(), f, c.Token, st)
		if errors.Is(err, errSignedOut) {
			clearCookie(w, r, accountCookie(c.slot))
			continue
		}
		if err == nil && acc.Read {
			out = append(out, siteAccount{ID: c.ID(), Login: c.Login, Forge: c.Forge, CanWrite: acc.Write})
		}
	}
	return out
}

// apiSites lists the configured sites any signed-in account can open, and
// which accounts can.
func (s *Server) apiSites(w http.ResponseWriter, r *http.Request, accts []*Session) {
	out := []map[string]any{}
	for _, st := range s.allSites(r.Context(), accts) {
		who := s.accountsFor(w, r, accts, st)
		if len(who) == 0 {
			continue
		}
		canWrite := false
		for _, a := range who {
			canWrite = canWrite || a.CanWrite
		}
		out = append(out, map[string]any{"id": st.ID, "name": st.Name, "repo": st.Repo, "branch": st.Branch, "canWrite": canWrite, "accounts": who, "discovered": st.discovered})
	}
	writeJSON(w, http.StatusOK, out)
}

// signedOut signs out the account in use when the platform has stopped
// accepting its token. Other accounts stay signed in.
func (s *Server) signedOut(w http.ResponseWriter, r *http.Request, err error) bool {
	if errors.Is(err, errSignedOut) {
		who := "Your sign-in"
		if i := infoFrom(r); i != nil {
			clearCookie(w, r, accountCookie(i.sess.slot))
			who = "The sign-in for @" + i.sess.Login + " on " + i.sess.Forge
		}
		apiError(w, http.StatusUnauthorized, who+" has ended. Sign in again.")
		return true
	}
	return false
}

func (s *Server) siteByID(ctx context.Context, sess *Session, f *ForgeConfig, id string) (*SiteConfig, Access, error) {
	st := s.cfg.site(id)
	if i, _ := ctx.Value(reqKey{}).(*reqInfo); i != nil && i.site != nil && i.site.ID == id {
		st = i.site // a site found in an account's repositories
	}
	if st == nil {
		return nil, Access{}, nil
	}
	u, _ := url.Parse(st.Repo)
	if !strings.EqualFold(u.Hostname(), f.Host) {
		return nil, Access{}, nil
	}
	acc, err := s.accessCached(ctx, f, sess.Token, st)
	if err != nil || !acc.Read {
		return nil, Access{}, err
	}
	return st, acc, nil
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
		"as": sess.ID(), "accounts": s.accountsFor(w, r, infoFrom(r).accts, st),
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
	_, siteDir, commit, err := s.ws.checkout(sess, f.Kind, st, s.cloneURL(st), branch)
	if err != nil {
		apiError(w, http.StatusBadGateway, "Couldn't fetch the site: "+trimErr(err))
		return
	}
	raw, err := readSource(siteDir, r.URL.Query().Get("source"))
	if err != nil {
		apiError(w, http.StatusNotFound, "No such page.")
		return
	}
	fmRaw, body := splitSource(raw)
	var fm any = map[string]any{}
	var doc yaml.Node
	if yaml.Unmarshal([]byte(fmRaw), &doc) == nil && len(doc.Content) == 1 && doc.Content[0].Kind == yaml.MappingNode {
		if j, err := orderedJSON(&doc); err == nil {
			fm = j // keys in the file's order
		}
	} else {
		fm, _ = splitFrontMatter(raw)
	}
	writeJSON(w, http.StatusOK, map[string]any{"source": r.URL.Query().Get("source"), "frontMatter": fm, "frontMatterRaw": fmRaw, "body": body, "branch": branch, "commit": commit})
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
