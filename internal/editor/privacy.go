package editor

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/site"
)

// The analytics wizard and the privacy settings: analytics: and privacy: in
// site.yaml, written as two blocks, the rest of the file left as it was.
// Statistics wait for consent unless the person chooses otherwise, and that
// choice is spelled out in the file.

func (s *Server) apiPrivacy(w http.ResponseWriter, r *http.Request, sess *Session, f *ForgeConfig, id string) {
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
	p := loaded.Config.Privacy
	writeJSON(w, http.StatusOK, map[string]any{
		"analytics": loaded.Config.Analytics,
		"privacy":   map[string]string{"controller": p.Controller, "contact": p.Contact, "updated": p.Updated},
		"commit":    commit, "branch": branch, "canWrite": acc.Write,
	})
}

type privacyRequest struct {
	Branch     string                `json:"branch"`
	BaseCommit string                `json:"baseCommit"`
	Analytics  *site.AnalyticsConfig `json:"analytics"`
	Privacy    map[string]string     `json:"privacy"`
}

var emailish = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

func (s *Server) privacySave(w http.ResponseWriter, r *http.Request, sess *Session, f *ForgeConfig, st *SiteConfig) {
	var req privacyRequest
	if !decode(w, r, &req) {
		return
	}
	if req.Branch == "" {
		req.Branch = st.Branch
	}
	if err := req.Analytics.Validate(); err != nil {
		apiError(w, http.StatusUnprocessableEntity, "Analytics: "+err.Error())
		return
	}
	if c := strings.TrimSpace(req.Privacy["contact"]); c != "" && !emailish.MatchString(c) {
		apiError(w, http.StatusUnprocessableEntity, fmt.Sprintf("%q doesn't look like an email address.", c))
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
	var an map[string]any
	if a := req.Analytics; a != nil {
		an = map[string]any{"provider": a.Provider}
		for k, v := range map[string]string{"domain": a.Domain, "src": a.Src, "url": a.URL, "site_id": a.SiteID, "id": a.ID} {
			if v = strings.TrimSpace(v); v != "" {
				an[k] = v
			}
		}
		if !a.IsGated() {
			an["gated"] = false // counting without asking: a choice, written down
		}
	}
	pv := map[string]any{}
	for _, k := range []string{"controller", "contact", "updated"} {
		if v := strings.TrimSpace(req.Privacy[k]); v != "" {
			pv[k] = v
		}
	}
	out, err := setTopLevel(string(raw), "analytics", an)
	if err == nil {
		out, err = setTopLevel(out, "privacy", pv)
	}
	if err != nil {
		apiError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	target, create := req.Branch, false
	if req.Branch == st.Branch {
		target, create = s.draftName(r, sess, f, st, "privacy.md"), true
	}
	commit, err := s.ws.save(sess, f.Kind, st, s.cloneURL(st), req.Branch, target, create, req.BaseCommit, map[string][]byte{repoFile(st, "site.yaml"): []byte(out)}, "Update analytics and privacy settings")
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"branch": target, "commit": commit, "created": create})
}
