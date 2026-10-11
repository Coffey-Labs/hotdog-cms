package editor

import (
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/preview"
	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/site"
)

// Redirects, and moving a page. When a page moves, its old address keeps
// working: a redirect is added in the same commit, and redirects that
// pointed at the old address are pointed at the new one, so there are no
// chains. Redirects live in site.yaml, and only that block is rewritten.

func (s *Server) apiRedirects(w http.ResponseWriter, r *http.Request, sess *Session, f *ForgeConfig, id string) {
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
	list := loaded.Config.Redirects
	if list == nil {
		list = []site.Redirect{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"redirects": list, "commit": commit, "branch": branch, "canWrite": acc.Write})
}

// checkRedirects says what's wrong with a list of redirects, or nothing.
func checkRedirects(list []site.Redirect, pages map[string]bool) error {
	seen := map[string]bool{}
	for i, rd := range list {
		n := i + 1
		switch {
		case !strings.HasPrefix(rd.From, "/") || strings.Contains(rd.From, ".."):
			return fmt.Errorf("redirect %d: %q should be an address on the site, starting with /", n, rd.From)
		case seen[rd.From]:
			return fmt.Errorf("redirect %d: %s is redirected twice", n, rd.From)
		case !strings.HasPrefix(rd.To, "/") && !strings.HasPrefix(rd.To, "https://") && !strings.HasPrefix(rd.To, "http://"):
			return fmt.Errorf("redirect %d: %q should start with / or https://", n, rd.To)
		case rd.From == rd.To:
			return fmt.Errorf("redirect %d: %s redirects to itself", n, rd.From)
		case pages[rd.From] || pages[strings.TrimSuffix(rd.From, "/")+"/"]:
			return fmt.Errorf("redirect %d: %s is a page; a redirect there would hide it", n, rd.From)
		}
		seen[rd.From] = true
	}
	return nil
}

func redirectsValue(list []site.Redirect) []map[string]string {
	out := []map[string]string{}
	for _, rd := range list {
		out = append(out, map[string]string{"from": rd.From, "to": rd.To})
	}
	return out
}

type redirectsRequest struct {
	Branch     string          `json:"branch"`
	BaseCommit string          `json:"baseCommit"`
	Redirects  []site.Redirect `json:"redirects"`
}

func (s *Server) redirectsSave(w http.ResponseWriter, r *http.Request, sess *Session, f *ForgeConfig, st *SiteConfig) {
	var req redirectsRequest
	if !decode(w, r, &req) {
		return
	}
	if req.Branch == "" {
		req.Branch = st.Branch
	}
	loaded, siteDir, _, err := s.ws.checkout(sess, f.Kind, st, s.cloneURL(st), req.Branch)
	if loaded == nil {
		s.writeError(w, r, err)
		return
	}
	for i := range req.Redirects {
		req.Redirects[i].From = strings.TrimSpace(req.Redirects[i].From)
		req.Redirects[i].To = strings.TrimSpace(req.Redirects[i].To)
	}
	if err := checkRedirects(req.Redirects, pagePaths(loaded)); err != nil {
		apiError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	cfg, err := os.ReadFile(filepath.Join(siteDir, "site.yaml"))
	if err != nil {
		apiError(w, http.StatusBadGateway, "Couldn't read site.yaml.")
		return
	}
	out, err := setTopLevel(string(cfg), "redirects", redirectsValue(req.Redirects))
	if err != nil {
		apiError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	target, create := req.Branch, false
	if req.Branch == st.Branch {
		target, create = s.draftName(r, sess, f, st, "redirects.md"), true
	}
	commit, err := s.ws.save(sess, f.Kind, st, s.cloneURL(st), req.Branch, target, create, req.BaseCommit, map[string][]byte{repoFile(st, "site.yaml"): []byte(out)}, "Update redirects")
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"branch": target, "commit": commit, "created": create})
}

func pagePaths(s *site.Site) map[string]bool {
	out := map[string]bool{}
	for _, p := range s.Pages {
		if p.RedirectTo == "" {
			out[p.Path] = true
		}
	}
	return out
}

type moveRequest struct {
	Branch     string `json:"branch"`
	BaseCommit string `json:"baseCommit"`
	Source     string `json:"source"`
	To         string `json:"to"` // the new source path, content/…md
}

// movePage renames a page's file, and redirects its old address to its new
// one, in one commit.
func (s *Server) movePage(w http.ResponseWriter, r *http.Request, sess *Session, f *ForgeConfig, st *SiteConfig) {
	var req moveRequest
	if !decode(w, r, &req) {
		return
	}
	if req.Branch == "" {
		req.Branch = st.Branch
	}
	loaded, siteDir, _, err := s.ws.checkout(sess, f.Kind, st, s.cloneURL(st), req.Branch)
	if loaded == nil {
		s.writeError(w, r, err)
		return
	}
	content, err := readSource(siteDir, req.Source)
	if err != nil {
		apiError(w, http.StatusNotFound, "No such page on this branch.")
		return
	}
	to := path.Clean(strings.TrimSpace(req.To))
	if _, err := readSource(siteDir, to); err == nil {
		apiError(w, http.StatusConflict, "There's already a page at "+to+".")
		return
	}
	if c := filepath.ToSlash(filepath.Clean("/" + to))[1:]; c != to || !strings.HasPrefix(to, "content/") || !strings.HasSuffix(to, ".md") || to == req.Source {
		apiError(w, http.StatusBadRequest, "The new place should be a different file under content/, ending in .md.")
		return
	}
	oldPath, newPath := "", ""
	for _, p := range loaded.Pages {
		if "content/"+p.Source == req.Source {
			oldPath = p.Path
		}
	}
	// Where the page will be, worked out by the build's own rules.
	moved, err := site.Load(siteDir, site.Options{Drafts: true, Skip: map[string]bool{strings.TrimPrefix(req.Source, "content/"): true}, Overrides: map[string][]byte{strings.TrimPrefix(to, "content/"): content}})
	if err != nil {
		apiError(w, http.StatusUnprocessableEntity, "The page wouldn't load there: "+trimErr(err))
		return
	}
	for _, p := range moved.Pages {
		if "content/"+p.Source == to {
			newPath = p.Path
		}
	}
	files := map[string][]byte{repoFile(st, req.Source): nil, repoFile(st, to): content}
	redirected := false
	if oldPath != "" && newPath != "" && oldPath != newPath {
		list := []site.Redirect{}
		for _, rd := range loaded.Config.Redirects {
			if rd.From == newPath {
				continue // the new address is a page now
			}
			if rd.To == oldPath {
				rd.To = newPath // no chains
			}
			list = append(list, rd)
		}
		list = append(list, site.Redirect{From: oldPath, To: newPath})
		cfg, err := os.ReadFile(filepath.Join(siteDir, "site.yaml"))
		if err != nil {
			apiError(w, http.StatusBadGateway, "Couldn't read site.yaml.")
			return
		}
		out, err := setTopLevel(string(cfg), "redirects", redirectsValue(list))
		if err != nil {
			apiError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		files[repoFile(st, "site.yaml")] = []byte(out)
		redirected = true
	}
	target, create := req.Branch, false
	if req.Branch == st.Branch {
		target, create = s.draftName(r, sess, f, st, to), true
	}
	commit, err := s.ws.save(sess, f.Kind, st, s.cloneURL(st), req.Branch, target, create, req.BaseCommit, files, fmt.Sprintf("Move %s to %s", req.Source, to))
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	s.log.Printf("%s moved %s to %s on %s@%s (%s)", sess.Login, req.Source, to, st.ID, target, shortSHA(commit))
	writeJSON(w, http.StatusOK, map[string]any{"branch": target, "commit": commit, "created": create, "source": to, "from": oldPath, "to": newPath, "redirected": redirected, "preview": st.Preview.url(preview.Slug(target))})
}
