package editor

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path"
	"path/filepath"
	"strings"
	"time"

	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/build"
	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/media"
	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/preview"
)

// Writing. Every change is a commit made as the signed-in person and pushed
// with their token. Nothing is ever written to the branch a site publishes
// from: the first save of a change starts a draft branch, a draft goes to
// review as a pull request, and publishing merges it, but only when the
// draft passes the same checks every other route to the live site requires.

func (s *Server) apiWrite(w http.ResponseWriter, r *http.Request, sess *Session, f *ForgeConfig, id, action string) {
	st, acc, err := s.siteByID(r.Context(), sess, f, id)
	if s.signedOut(w, r, err) {
		return
	}
	if err != nil || st == nil {
		apiError(w, http.StatusNotFound, "No such site, or you can't open it.")
		return
	}
	if !acc.Write && action != "live" {
		apiError(w, http.StatusForbidden, "The platform gives you read-only access to this site.")
		return
	}
	limit := int64(2 << 20)
	if action == "media" {
		limit = media.MaxBytes + 1<<20
	}
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	switch action {
	case "media":
		s.upload(w, r, sess, f, st)
	case "save":
		s.save(w, r, sess, f, st)
	case "new":
		s.newPage(w, r, sess, f, st)
	case "live":
		s.liveRender(w, r, sess, f, st)
	case "review":
		s.review(w, r, sess, f, st)
	case "publish":
		s.publish(w, r, sess, f, st)
	default:
		apiError(w, http.StatusNotFound, "no such thing")
	}
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		apiError(w, http.StatusBadRequest, "That request couldn't be read.")
		return false
	}
	return true
}

func repoFile(st *SiteConfig, source string) string {
	return path.Join(filepath.ToSlash(st.Subdir), source)
}

// draftName picks an unused draft branch name for a page.
func (s *Server) draftName(r *http.Request, sess *Session, f *ForgeConfig, st *SiteConfig, source string) string {
	base := "draft/" + slug(strings.TrimSuffix(path.Base(source), ".md"))
	if strings.HasSuffix(source, "/index.md") || strings.HasSuffix(source, "/_index.md") {
		base = "draft/" + slug(path.Base(path.Dir(source)))
	}
	if base == "draft/" || base == "draft/index" {
		base = "draft/home"
	}
	taken := map[string]bool{}
	if bs, err := s.branches(r.Context(), f, sess.Token, st); err == nil {
		for _, b := range bs {
			taken[b.Name] = true
		}
	}
	name := base
	for i := 2; taken[name] && i < 100; i++ {
		name = fmt.Sprintf("%s-%d", base, i)
	}
	return name
}

func (s *Server) writeError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case s.signedOut(w, r, err):
	case errors.Is(err, errConflict):
		apiError(w, http.StatusConflict, "Someone changed this branch since you opened it. Your text is still here: copy it, reload to see their change, then save again.")
	default:
		var pe *platformError
		if errors.As(err, &pe) {
			apiError(w, http.StatusBadGateway, pe.Error())
			return
		}
		apiError(w, http.StatusBadGateway, trimErr(err))
	}
}

type saveRequest struct {
	Source     string `json:"source"`
	Branch     string `json:"branch"`
	BaseCommit string `json:"baseCommit"`
	Edit       Edit   `json:"edit"`
	Message    string `json:"message"`
}

func (s *Server) save(w http.ResponseWriter, r *http.Request, sess *Session, f *ForgeConfig, st *SiteConfig) {
	var req saveRequest
	if !decode(w, r, &req) {
		return
	}
	if req.Branch == "" {
		req.Branch = st.Branch
	}
	_, siteDir, _, err := s.ws.checkout(sess, f.Kind, st, s.cloneURL(st), req.Branch)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	original, err := readSource(siteDir, req.Source)
	if err != nil {
		apiError(w, http.StatusNotFound, "No such page on this branch.")
		return
	}
	content, err := compose(original, req.Edit)
	if err != nil {
		apiError(w, http.StatusBadRequest, err.Error())
		return
	}
	msg := strings.TrimSpace(req.Message)
	if msg == "" {
		msg = "Edit " + req.Source
	}
	target, create := req.Branch, false
	if req.Branch == st.Branch {
		target, create = s.draftName(r, sess, f, st, req.Source), true
	}
	commit, err := s.ws.save(sess, f.Kind, st, s.cloneURL(st), req.Branch, target, create, req.BaseCommit, map[string][]byte{repoFile(st, req.Source): content}, msg)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	s.log.Printf("%s saved %s to %s@%s (%s)", sess.Login, req.Source, st.ID, target, shortSHA(commit))
	writeJSON(w, http.StatusOK, map[string]any{"branch": target, "commit": commit, "created": create, "preview": st.Preview.url(preview.Slug(target))})
}

func shortSHA(c string) string {
	if len(c) > 10 {
		return c[:10]
	}
	return c
}

type newRequest struct {
	Collection string `json:"collection"`
	Title      string `json:"title"`
}

func (s *Server) newPage(w http.ResponseWriter, r *http.Request, sess *Session, f *ForgeConfig, st *SiteConfig) {
	var req newRequest
	if !decode(w, r, &req) {
		return
	}
	req.Title = strings.TrimSpace(req.Title)
	name := slug(req.Title)
	if req.Title == "" || name == "" {
		apiError(w, http.StatusBadRequest, "Give the page a title with some letters or numbers in it.")
		return
	}
	if req.Collection != "" && (slug(req.Collection) != req.Collection) {
		apiError(w, http.StatusBadRequest, "No such collection.")
		return
	}
	source := path.Join("content", req.Collection, name+".md")
	_, siteDir, base, err := s.ws.checkout(sess, f.Kind, st, s.cloneURL(st), st.Branch)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	if _, err := readSource(siteDir, source); err == nil {
		apiError(w, http.StatusConflict, "A page with that address already exists: "+source)
		return
	}
	set := map[string]any{"title": req.Title}
	if req.Collection != "" {
		set["date"] = time.Now().Format("2006-01-02")
	}
	content, err := compose(nil, Edit{Set: set})
	if err != nil {
		apiError(w, http.StatusBadRequest, err.Error())
		return
	}
	target := s.draftName(r, sess, f, st, source)
	commit, err := s.ws.save(sess, f.Kind, st, s.cloneURL(st), st.Branch, target, true, base, map[string][]byte{repoFile(st, source): content}, "New page: "+req.Title)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	s.log.Printf("%s started %s on %s@%s", sess.Login, source, st.ID, target)
	writeJSON(w, http.StatusOK, map[string]any{"branch": target, "commit": commit, "source": source})
}

type liveRequest struct {
	Source string `json:"source"`
	Branch string `json:"branch"`
	Edit   *Edit  `json:"edit"` // nil: show the page as saved
}

// liveRender renders the page as edited, onto the branch's last build.
func (s *Server) liveRender(w http.ResponseWriter, r *http.Request, sess *Session, f *ForgeConfig, st *SiteConfig) {
	var req liveRequest
	if !decode(w, r, &req) {
		return
	}
	if req.Branch == "" {
		req.Branch = st.Branch
	}
	loaded, siteDir, commit, err := s.ws.checkout(sess, f.Kind, st, s.cloneURL(st), req.Branch)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	s.ws.buildAndCheck(siteDir, commit)
	original, err := readSource(siteDir, req.Source)
	if err != nil {
		apiError(w, http.StatusNotFound, "No such page on this branch.")
		return
	}
	content := original
	if req.Edit != nil {
		if content, err = compose(original, *req.Edit); err != nil {
			writeJSON(w, http.StatusOK, map[string]any{"error": err.Error()})
			return
		}
	}
	html, pagePath, err := build.RenderPage(siteDir, req.Source, content, build.Options{URL: loaded.Config.URL})
	if err != nil {
		// A page that doesn't render yet (a typo in front matter, mid-edit)
		// is reported, not an error: the preview keeps its last good version.
		writeJSON(w, http.StatusOK, map[string]any{"error": err.Error()})
		return
	}
	owner := sess.Forge + "|" + sess.Login + "|" + st.ID + "|" + req.Branch
	url := s.live.show(owner, siteDir+".editor-build", pagePath, html)
	writeJSON(w, http.StatusOK, map[string]any{"url": url, "path": pagePath})
}

type reviewRequest struct {
	Branch      string `json:"branch"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

func (s *Server) review(w http.ResponseWriter, r *http.Request, sess *Session, f *ForgeConfig, st *SiteConfig) {
	var req reviewRequest
	if !decode(w, r, &req) {
		return
	}
	if req.Branch == "" || req.Branch == st.Branch {
		apiError(w, http.StatusBadRequest, "Only a draft can go to review.")
		return
	}
	if strings.TrimSpace(req.Title) == "" {
		apiError(w, http.StatusBadRequest, "Give the review a title.")
		return
	}
	_, siteDir, commit, err := s.ws.checkout(sess, f.Kind, st, s.cloneURL(st), req.Branch)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	res := s.ws.buildAndCheck(siteDir, commit)
	body := strings.TrimSpace(req.Description)
	if u := st.Preview.url(preview.Slug(req.Branch)); u != "" {
		body += "\n\nPreview: " + u
	}
	switch {
	case res.Failed != "":
		body += "\n\nThis branch doesn't build yet: " + res.Failed
	case res.Errors > 0:
		body += fmt.Sprintf("\n\nChecks: %d error(s), %d warning(s). It can't be published until the errors are fixed.", res.Errors, res.Warnings)
	default:
		body += fmt.Sprintf("\n\nChecks: passing, %d warning(s).", res.Warnings)
	}
	body += "\n\nOpened from the HotDog CMS editor by @" + sess.Login + "."
	rv, err := s.openReview(r.Context(), f, sess.Token, st, req.Branch, strings.TrimSpace(req.Title), strings.TrimSpace(body))
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	s.log.Printf("%s opened review #%d for %s@%s", sess.Login, rv.Number, st.ID, req.Branch)
	writeJSON(w, http.StatusOK, rv)
}

type publishRequest struct {
	Branch string `json:"branch"`
}

// publish merges a draft's review, after running the checks on the draft
// exactly as the publishing pipeline will.
func (s *Server) publish(w http.ResponseWriter, r *http.Request, sess *Session, f *ForgeConfig, st *SiteConfig) {
	var req publishRequest
	if !decode(w, r, &req) {
		return
	}
	if req.Branch == "" || req.Branch == st.Branch {
		apiError(w, http.StatusBadRequest, "Only a draft can be published.")
		return
	}
	reviews, err := s.reviews(r.Context(), f, sess.Token, st)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	number := 0
	for _, rv := range reviews {
		if rv.Branch == req.Branch {
			number = rv.Number
		}
	}
	if number == 0 {
		apiError(w, http.StatusConflict, "Submit this draft for review first; publishing merges the review.")
		return
	}
	_, siteDir, commit, err := s.ws.checkout(sess, f.Kind, st, s.cloneURL(st), req.Branch)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	res := s.ws.buildAndCheck(siteDir, commit)
	if res.Failed != "" || res.Errors > 0 {
		msg := fmt.Sprintf("Not published: %d check error(s). Fix them on the draft, or switch the rule off in site.yaml.", res.Errors)
		if res.Failed != "" {
			msg = "Not published: the draft doesn't build. " + res.Failed
		}
		writeJSON(w, http.StatusConflict, map[string]any{"error": msg, "problems": res.Problems})
		return
	}
	if err := s.merge(r.Context(), f, sess.Token, st, number); err != nil {
		s.writeError(w, r, err)
		return
	}
	s.log.Printf("%s published %s@%s (review #%d, %s)", sess.Login, st.ID, req.Branch, number, shortSHA(commit))
	writeJSON(w, http.StatusOK, map[string]any{"published": true, "number": number})
}
