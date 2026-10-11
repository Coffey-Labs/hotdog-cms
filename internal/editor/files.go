package editor

import (
	"bytes"
	"errors"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/gitx"
	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/preview"
)

// The power interface's file access: every file of a site, any text file
// edited as raw text with a diff before saving, files added and deleted, a
// draft compared with the published branch, and history. All of it is git:
// a save is a commit on a draft, like every other change, and nothing here
// runs anything but git.

const maxTextFile = 1 << 20

// FileInfo is one file of a site.
type FileInfo struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
	Text bool   `json:"text"`
}

// cleanSitePath accepts a path inside the site, never in .git or the build
// output.
func cleanSitePath(p string) (string, bool) {
	c := path.Clean("/" + strings.ReplaceAll(p, "\\", "/"))[1:]
	if c == "" || c != p || c == ".git" || strings.HasPrefix(c, ".git/") || c == "public" || strings.HasPrefix(c, "public/") || strings.Contains(c, "\x00") {
		return "", false
	}
	return c, true
}

// pipelineFile says whether a repository path is one that decides what runs
// when the site is built or published: the operator's publish targets (and
// their before and after steps) and CI pipelines. Changing one is changing
// what runs with the operator's credentials, so only maintainers may.
func pipelineFile(repoPath string) bool {
	base := path.Base(repoPath)
	top, _, _ := strings.Cut(repoPath, "/")
	switch {
	case base == "publish.yaml" || base == "publish.yml":
		return true
	case top == ".github" || top == ".gitea" || top == ".forgejo" || top == ".circleci":
		return true
	case repoPath == ".gitlab-ci.yml" || repoPath == "Jenkinsfile" || repoPath == ".drone.yml" || repoPath == "bitbucket-pipelines.yml":
		return true
	case top == ".woodpecker" || repoPath == ".woodpecker.yml" || repoPath == ".woodpecker.yaml":
		return true
	}
	return false
}

func isText(data []byte) bool {
	head := data
	if len(head) > 8192 {
		head = head[:8192]
	}
	return !bytes.ContainsRune(head, 0) && utf8.Valid(head)
}

func (s *Server) siteFor(w http.ResponseWriter, r *http.Request, sess *Session, f *ForgeConfig, id string) (*SiteConfig, Access, string, string, bool) {
	st, acc, err := s.siteByID(r.Context(), sess, f, id)
	if s.signedOut(w, r, err) {
		return nil, acc, "", "", false
	}
	if err != nil || st == nil {
		apiError(w, http.StatusNotFound, "No such site, or you can't open it.")
		return nil, acc, "", "", false
	}
	branch := r.URL.Query().Get("branch")
	if branch == "" {
		branch = st.Branch
	}
	_, siteDir, commit, err := s.ws.checkout(sess, f.Kind, st, s.cloneURL(st), branch)
	if siteDir == "" {
		apiError(w, http.StatusBadGateway, "Couldn't fetch the site: "+trimErr(err))
		return nil, acc, "", "", false
	}
	return st, acc, siteDir, commit, true
}

func (s *Server) apiFiles(w http.ResponseWriter, r *http.Request, sess *Session, f *ForgeConfig, id string) {
	_, _, siteDir, commit, ok := s.siteFor(w, r, sess, f, id)
	if !ok {
		return
	}
	files := []FileInfo{}
	_ = filepath.WalkDir(siteDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(siteDir, p)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			switch rel {
			case ".git", "public", "node_modules":
				return fs.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(rel, ".editor-build") || d.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		fi := FileInfo{Path: rel, Size: info.Size()}
		if info.Size() <= maxTextFile {
			if data, err := os.ReadFile(p); err == nil {
				fi.Text = isText(data)
			}
		}
		files = append(files, fi)
		return nil
	})
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	writeJSON(w, http.StatusOK, map[string]any{"files": files, "commit": commit})
}

func (s *Server) apiFile(w http.ResponseWriter, r *http.Request, sess *Session, f *ForgeConfig, id string) {
	_, _, siteDir, commit, ok := s.siteFor(w, r, sess, f, id)
	if !ok {
		return
	}
	rel, valid := cleanSitePath(r.URL.Query().Get("path"))
	if !valid {
		apiError(w, http.StatusNotFound, "No such file.")
		return
	}
	data, err := os.ReadFile(filepath.Join(siteDir, filepath.FromSlash(rel)))
	if errors.Is(err, fs.ErrNotExist) {
		writeJSON(w, http.StatusOK, map[string]any{"path": rel, "exists": false, "content": "", "commit": commit})
		return
	}
	if err != nil {
		apiError(w, http.StatusNotFound, "No such file.")
		return
	}
	if len(data) > maxTextFile || !isText(data) {
		apiError(w, http.StatusUnprocessableEntity, "That file isn't text the editor can show (or is over 1 MB).")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"path": rel, "exists": true, "content": string(data), "commit": commit})
}

type fileSaveRequest struct {
	Branch     string `json:"branch"`
	BaseCommit string `json:"baseCommit"`
	Path       string `json:"path"`
	Content    string `json:"content"`
	Delete     bool   `json:"delete"`
	Message    string `json:"message"`
}

func (s *Server) fileSave(w http.ResponseWriter, r *http.Request, sess *Session, f *ForgeConfig, st *SiteConfig, acc Access) {
	var req fileSaveRequest
	if !decode(w, r, &req) {
		return
	}
	rel, ok := cleanSitePath(req.Path)
	if !ok {
		apiError(w, http.StatusBadRequest, "That isn't a path in the site (and .git and public/ can't be written).")
		return
	}
	if pipelineFile(repoFile(st, rel)) && !acc.Admin {
		apiError(w, http.StatusForbidden, rel+" decides what runs when the site is published, so only a maintainer of the repository can change it.")
		return
	}
	if len(req.Content) > maxTextFile {
		apiError(w, http.StatusRequestEntityTooLarge, "The file is over 1 MB.")
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
	_, statErr := os.Stat(filepath.Join(siteDir, filepath.FromSlash(rel)))
	exists := statErr == nil
	msg := strings.TrimSpace(req.Message)
	var data []byte
	switch {
	case req.Delete:
		if !exists {
			apiError(w, http.StatusNotFound, "There's no such file to delete.")
			return
		}
		if msg == "" {
			msg = "Delete " + rel
		}
	default:
		data = []byte(req.Content)
		if msg == "" {
			msg = map[bool]string{true: "Edit ", false: "Add "}[exists] + rel
		}
	}
	target, create := req.Branch, false
	if req.Branch == st.Branch {
		target, create = s.draftName(r, sess, f, st, rel), true
	}
	commit, err := s.ws.save(sess, f.Kind, st, s.cloneURL(st), req.Branch, target, create, req.BaseCommit, map[string][]byte{repoFile(st, rel): data}, msg)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	s.log.Printf("%s: %s on %s@%s (%s)", sess.Login, msg, st.ID, target, shortSHA(commit))
	writeJSON(w, http.StatusOK, map[string]any{"branch": target, "commit": commit, "created": create, "preview": st.Preview.url(preview.Slug(target))})
}

// fileDiff shows what saving would change: the branch's file against the
// text being edited, as a unified diff.
func (s *Server) fileDiff(w http.ResponseWriter, r *http.Request, sess *Session, f *ForgeConfig, st *SiteConfig) {
	var req fileSaveRequest
	if !decode(w, r, &req) {
		return
	}
	rel, ok := cleanSitePath(req.Path)
	if !ok {
		apiError(w, http.StatusBadRequest, "That isn't a path in the site.")
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
	old, _ := os.ReadFile(filepath.Join(siteDir, filepath.FromSlash(rel)))
	var now []byte
	if !req.Delete {
		now = []byte(req.Content)
	}
	d, err := unifiedDiff(rel, old, now)
	if err != nil {
		apiError(w, http.StatusBadGateway, trimErr(err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"diff": d})
}

// unifiedDiff is git's diff of two versions of one file.
func unifiedDiff(rel string, old, now []byte) (string, error) {
	dir, err := os.MkdirTemp("", "hotdog-cms-diff-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)
	a, b := filepath.Join(dir, "a"), filepath.Join(dir, "b")
	if err := os.WriteFile(a, old, 0o600); err != nil {
		return "", err
	}
	if err := os.WriteFile(b, now, 0o600); err != nil {
		return "", err
	}
	var out bytes.Buffer
	cmd := exec.Command("git", "diff", "--no-index", "--no-color", "-U3", "--src-prefix=a/", "--dst-prefix=b/", "a", "b")
	cmd.Dir, cmd.Stdout = dir, &out
	err = cmd.Run()
	var ee *exec.ExitError
	if err != nil && !(errors.As(err, &ee) && ee.ExitCode() == 1) { // 1: they differ
		return "", err
	}
	// Name the file as it is in the site.
	d := strings.Replace(out.String(), "diff --git a/a b/b", "diff --git a/"+rel+" b/"+rel, 1)
	d = strings.Replace(d, "--- a/a", "--- a/"+rel, 1)
	d = strings.Replace(d, "+++ b/b", "+++ b/"+rel, 1)
	return d, nil
}

// apiCompare is a draft against the branch the site publishes from: what
// publishing it would change.
func (s *Server) apiCompare(w http.ResponseWriter, r *http.Request, sess *Session, f *ForgeConfig, id string) {
	st, _, _, _, ok := s.siteFor(w, r, sess, f, id)
	if !ok {
		return
	}
	branch := r.URL.Query().Get("branch")
	if branch == "" || branch == st.Branch {
		writeJSON(w, http.StatusOK, map[string]any{"diff": "", "stat": "", "base": st.Branch, "branch": st.Branch})
		return
	}
	dir := s.ws.dir(sess, st, branch)
	unlock := s.ws.lock(dir)
	defer unlock()
	auth := gitx.Auth{User: f.Kind.TokenUser(), Token: sess.Token}
	if _, err := gitx.Run(auth, dir, "fetch", "--quiet", "--depth", "200", "origin",
		"+refs/heads/"+st.Branch+":refs/hotdog/base", "+refs/heads/"+branch+":refs/hotdog/head"); err != nil {
		apiError(w, http.StatusBadGateway, "Couldn't fetch the branches: "+trimErr(err))
		return
	}
	scope := []string{}
	if st.Subdir != "" {
		scope = []string{"--", st.Subdir}
	}
	// Changes since the draft started; if their common start is deeper than
	// fetched, the two tips directly.
	rng := "refs/hotdog/base...refs/hotdog/head"
	if _, err := gitx.Run(auth, dir, "merge-base", "refs/hotdog/base", "refs/hotdog/head"); err != nil {
		rng = "refs/hotdog/base..refs/hotdog/head"
	}
	stat, _ := gitx.Run(auth, dir, append([]string{"diff", "--no-color", "--stat", rng}, scope...)...)
	d, err := gitx.Run(auth, dir, append([]string{"diff", "--no-color", "-U3", rng}, scope...)...)
	if err != nil {
		apiError(w, http.StatusBadGateway, trimErr(err))
		return
	}
	if len(d) > 2<<20 {
		d = d[:2<<20] + "\n… (the rest of the diff is too long to show)"
	}
	writeJSON(w, http.StatusOK, map[string]any{"diff": d, "stat": stat, "base": st.Branch, "branch": branch})
}

// Commit is one entry of a branch's history.
type Commit struct {
	SHA     string    `json:"sha"`
	Author  string    `json:"author"`
	When    time.Time `json:"when"`
	Subject string    `json:"subject"`
}

func (s *Server) apiLog(w http.ResponseWriter, r *http.Request, sess *Session, f *ForgeConfig, id string) {
	st, _, _, _, ok := s.siteFor(w, r, sess, f, id)
	if !ok {
		return
	}
	branch := r.URL.Query().Get("branch")
	if branch == "" {
		branch = st.Branch
	}
	n, _ := strconv.Atoi(r.URL.Query().Get("n"))
	if n <= 0 || n > 200 {
		n = 30
	}
	args := []string{"log", "-n", strconv.Itoa(n), "--format=%H%x1f%an%x1f%aI%x1f%s", "refs/hotdog/log"}
	if p := r.URL.Query().Get("path"); p != "" {
		rel, ok := cleanSitePath(p)
		if !ok {
			apiError(w, http.StatusBadRequest, "That isn't a path in the site.")
			return
		}
		args = append(args, "--", repoFile(st, rel))
	} else if st.Subdir != "" {
		args = append(args, "--", st.Subdir)
	}
	dir := s.ws.dir(sess, st, branch)
	unlock := s.ws.lock(dir)
	defer unlock()
	auth := gitx.Auth{User: f.Kind.TokenUser(), Token: sess.Token}
	if _, err := gitx.Run(auth, dir, "fetch", "--quiet", "--depth", strconv.Itoa(n+1), "origin", "+refs/heads/"+branch+":refs/hotdog/log"); err != nil {
		apiError(w, http.StatusBadGateway, "Couldn't fetch the history: "+trimErr(err))
		return
	}
	out, err := gitx.Run(auth, dir, args...)
	if err != nil {
		apiError(w, http.StatusBadGateway, trimErr(err))
		return
	}
	commits := []Commit{}
	for _, line := range strings.Split(out, "\n") {
		p := strings.Split(line, "\x1f")
		if len(p) != 4 {
			continue
		}
		when, _ := time.Parse(time.RFC3339, p[2])
		commits = append(commits, Commit{SHA: p[0], Author: p[1], When: when, Subject: p[3]})
	}
	writeJSON(w, http.StatusOK, map[string]any{"branch": branch, "commits": commits})
}
