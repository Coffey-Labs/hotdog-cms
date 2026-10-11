package editor

import (
	"net/http"

	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/build"
	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/isolate"
	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/preview"
	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/site"
	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/siteyaml"
)

// The Look panel: what the theme's look.yaml lets change, the site's choices
// from site.yaml, a live preview of new ones, and saving them as a commit to
// site.yaml on a draft, like any other change.

func (s *Server) apiLook(w http.ResponseWriter, r *http.Request, sess *Session, f *ForgeConfig, id string) {
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
	if loaded == nil {
		apiError(w, http.StatusBadGateway, "Couldn't load the site: "+trimErr(err))
		return
	}
	lf, err := site.LoadLook(siteDir)
	if err != nil {
		apiError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	values := loaded.Config.Look
	if values == nil {
		values = map[string]any{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"look": lf, "values": values, "branch": branch, "commit": commit, "canWrite": acc.Write, "minContrast": site.MinContrast})
}

type lookRequest struct {
	Branch     string         `json:"branch"`
	BaseCommit string         `json:"baseCommit"`
	Look       map[string]any `json:"look"`
}

// lookLive shows the home page with the look as chosen, before it's saved.
func (s *Server) lookLive(w http.ResponseWriter, r *http.Request, sess *Session, f *ForgeConfig, st *SiteConfig) {
	var req lookRequest
	if !decode(w, r, &req) {
		return
	}
	if req.Branch == "" {
		req.Branch = st.Branch
	}
	if req.Look == nil {
		req.Look = map[string]any{}
	}
	loaded, siteDir, commit, err := s.ws.checkout(sess, f.Kind, st, s.cloneURL(st), req.Branch)
	if loaded == nil {
		s.writeError(w, r, err)
		return
	}
	s.ws.buildAndCheck(siteDir, commit)
	cssPath, css, err := build.LookCSS(siteDir, req.Look)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"error": err.Error()})
		return
	}
	home, err := readSource(siteDir, "content/index.md")
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"error": "This site has no content/index.md to preview the look on."})
		return
	}
	html, pagePath, err := isolate.RenderPage(siteDir, "content/index.md", home, build.Options{URL: loaded.Config.URL, Look: req.Look})
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"error": err.Error()})
		return
	}
	extra := map[string][]byte{}
	if cssPath != "" {
		extra[cssPath] = css
	}
	owner := sess.Forge + "|" + sess.Login + "|" + st.ID + "|" + req.Branch + "|look"
	url := s.live.show(owner, siteDir+".editor-build", pagePath, html, extra)
	writeJSON(w, http.StatusOK, map[string]any{"url": url})
}

// lookSave writes the look into site.yaml, on the draft (starting one from
// the publishing branch), after checking the theme allows it.
func (s *Server) lookSave(w http.ResponseWriter, r *http.Request, sess *Session, f *ForgeConfig, st *SiteConfig) {
	var req lookRequest
	if !decode(w, r, &req) {
		return
	}
	if req.Branch == "" {
		req.Branch = st.Branch
	}
	_, siteDir, _, err := s.ws.checkout(sess, f.Kind, st, s.cloneURL(st), req.Branch)
	if siteDir == "" {
		s.writeError(w, r, err)
		return
	}
	lf, err := site.LoadLook(siteDir)
	if err != nil || lf == nil {
		apiError(w, http.StatusUnprocessableEntity, "This site's theme has no look.yaml, so there's no look to change.")
		return
	}
	if _, err := lf.Values(req.Look); err != nil {
		apiError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	cfg, err := readIn(siteDir, "site.yaml")
	if err != nil {
		apiError(w, http.StatusBadGateway, "Couldn't read site.yaml.")
		return
	}
	out, err := setTopLevel(string(cfg), "look", req.Look)
	if err != nil {
		apiError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	target, create := req.Branch, false
	if req.Branch == st.Branch {
		target, create = s.draftName(r, sess, f, st, "look.md"), true
	}
	commit, err := s.ws.save(sess, f.Kind, st, s.cloneURL(st), req.Branch, target, create, req.BaseCommit, map[string][]byte{repoFile(st, "site.yaml"): []byte(out)}, "Change the look")
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	s.log.Printf("%s changed the look of %s@%s (%s)", sess.Login, st.ID, target, shortSHA(commit))
	writeJSON(w, http.StatusOK, map[string]any{"branch": target, "commit": commit, "created": create, "preview": st.Preview.url(preview.Slug(target))})
}

// setTopLevel is siteyaml.SetTopLevel (shared with the importer).
func setTopLevel(text, key string, value any) (string, error) {
	return siteyaml.SetTopLevel(text, key, value)
}
