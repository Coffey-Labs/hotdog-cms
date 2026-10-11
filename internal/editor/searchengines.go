package editor

import (
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Search engines: the consoles' verification codes and the IndexNow key, as
// verify: and indexnow: in site.yaml (only those blocks rewritten).

func (s *Server) apiSearch(w http.ResponseWriter, r *http.Request, sess *Session, f *ForgeConfig, id string) {
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
	loaded, _, commit, err := s.ws.checkout(sess, f.Kind, st, s.cloneURL(st), branch)
	if loaded == nil {
		apiError(w, http.StatusBadGateway, "Couldn't load the site: "+trimErr(err))
		return
	}
	c := loaded.Config
	writeJSON(w, http.StatusOK, map[string]any{
		"verify": c.Verify, "indexnow": c.IndexNow.Key, "siteURL": c.URL, "sitemap": strings.TrimRight(c.URL, "/") + "/sitemap.xml",
		"commit": commit, "branch": branch, "canWrite": acc.Write,
	})
}

type searchRequest struct {
	Branch     string            `json:"branch"`
	BaseCommit string            `json:"baseCommit"`
	Verify     map[string]string `json:"verify"`
	IndexNow   string            `json:"indexnow"`
}

var (
	keyRe  = regexp.MustCompile(`^[A-Za-z0-9-]{8,128}$`)
	codeRe = regexp.MustCompile(`^[A-Za-z0-9_.=+/-]{4,200}$`)
)

func (s *Server) searchSave(w http.ResponseWriter, r *http.Request, sess *Session, f *ForgeConfig, st *SiteConfig) {
	var req searchRequest
	if !decode(w, r, &req) {
		return
	}
	if req.Branch == "" {
		req.Branch = st.Branch
	}
	verify := map[string]any{}
	for _, k := range []string{"google", "bing", "yandex"} {
		v := strings.TrimSpace(req.Verify[k])
		// Pasted the whole tag? Take the code out of it.
		if m := regexp.MustCompile(`content="([^"]+)"`).FindStringSubmatch(v); m != nil {
			v = m[1]
		}
		if v == "" {
			continue
		}
		if !codeRe.MatchString(v) {
			apiError(w, http.StatusUnprocessableEntity, "The "+k+" code doesn't look like a verification code.")
			return
		}
		verify[k] = v
	}
	key := strings.TrimSpace(req.IndexNow)
	if key != "" && !keyRe.MatchString(key) {
		apiError(w, http.StatusUnprocessableEntity, "The IndexNow key should be 8 to 128 letters, digits or dashes.")
		return
	}
	_, siteDir, _, err := s.ws.checkout(sess, f.Kind, st, s.cloneURL(st), req.Branch)
	if siteDir == "" {
		s.writeError(w, r, err)
		return
	}
	raw, err := os.ReadFile(filepath.Join(siteDir, "site.yaml"))
	if err != nil {
		apiError(w, http.StatusBadGateway, "Couldn't read site.yaml.")
		return
	}
	var in map[string]any
	if key != "" {
		in = map[string]any{"key": key}
	}
	out, err := setTopLevel(string(raw), "verify", verify)
	if err == nil {
		out, err = setTopLevel(out, "indexnow", in)
	}
	if err != nil {
		apiError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	target, create := req.Branch, false
	if req.Branch == st.Branch {
		target, create = s.draftName(r, sess, f, st, "search-engines.md"), true
	}
	commit, err := s.ws.save(sess, f.Kind, st, s.cloneURL(st), req.Branch, target, create, req.BaseCommit, map[string][]byte{repoFile(st, "site.yaml"): []byte(out)}, "Update search engine settings")
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"branch": target, "commit": commit, "created": create})
}
