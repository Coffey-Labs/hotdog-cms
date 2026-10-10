package editor

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/media"
	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/site"
)

// Pictures. An upload is re-encoded (internal/media) and committed to the
// draft like any other change, so a picture reaches the site the same way
// words do: on a branch, previewed, checked and reviewed. The library lists
// what's in static/media/ and where each picture is used.

// upload takes one picture from a multipart form: file, branch, baseCommit,
// and source (the page it's for, which names a new draft).
func (s *Server) upload(w http.ResponseWriter, r *http.Request, sess *Session, f *ForgeConfig, st *SiteConfig) {
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		apiError(w, http.StatusRequestEntityTooLarge, "That upload couldn't be read, or is over the limit of 25 MB.")
		return
	}
	defer r.MultipartForm.RemoveAll()
	file, hdr, err := r.FormFile("file")
	if err != nil {
		apiError(w, http.StatusBadRequest, "Choose a picture to upload.")
		return
	}
	data, err := io.ReadAll(io.LimitReader(file, media.MaxBytes+1))
	file.Close()
	if err != nil {
		apiError(w, http.StatusBadRequest, "That upload couldn't be read.")
		return
	}
	branch := r.FormValue("branch")
	if branch == "" {
		branch = st.Branch
	}
	_, siteDir, _, err := s.ws.checkout(sess, f.Kind, st, s.cloneURL(st), branch)
	if siteDir == "" {
		s.writeError(w, r, err)
		return
	}
	taken := func(p string) bool {
		_, err := os.Stat(filepath.Join(siteDir, filepath.FromSlash(p)))
		return err == nil
	}
	pic, err := media.Process(data, hdr.Filename, time.Now().Format("2006/01"), taken)
	if err != nil {
		apiError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	files := map[string][]byte{}
	var paths []string
	for _, pf := range pic.Files {
		rel := path.Join("static", pf.Path)
		files[repoFile(st, rel)] = pf.Data
		paths = append(paths, rel)
	}
	target, create := branch, false
	if branch == st.Branch {
		source := r.FormValue("source")
		if source == "" {
			source = "media.md"
		}
		target, create = s.draftName(r, sess, f, st, source), true
	}
	commit, err := s.ws.save(sess, f.Kind, st, s.cloneURL(st), branch, target, create, r.FormValue("baseCommit"), files, "Add picture "+pic.URL)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	s.log.Printf("%s added %s to %s@%s (%s)", sess.Login, pic.URL, st.ID, target, shortSHA(commit))
	writeJSON(w, http.StatusOK, map[string]any{
		"branch": target, "commit": commit, "created": create,
		"url": pic.URL, "width": pic.Width, "height": pic.Height, "notes": pic.Notes, "files": paths,
	})
}

// MediaItem is one picture in the library.
type MediaItem struct {
	URL    string           `json:"url"`
	Width  int              `json:"width"`
	Height int              `json:"height"`
	Thumb  string           `json:"thumb"` // the smallest size, to show in the library
	Bytes  int64            `json:"bytes"` // every file of it together
	Files  int              `json:"files"`
	UsedBy []string         `json:"usedBy"` // files that refer to it
	Alt    string           `json:"alt"`    // alt text it's been given before, to start from
	Sizes  []site.MediaSize `json:"-"`
}

var mdImageRe = regexp.MustCompile(`!\[([^\]]*)\]\(\s*<?([^)\s>]+)`)

func (s *Server) apiMedia(w http.ResponseWriter, r *http.Request, sess *Session, f *ForgeConfig, id string) {
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
	if siteDir == "" {
		apiError(w, http.StatusBadGateway, "Couldn't fetch the site: "+trimErr(err))
		return
	}
	writeJSON(w, http.StatusOK, mediaLibrary(siteDir))
}

// mediaLibrary lists the pictures in static/media/ with where they're used:
// any page, template, data file or setting that names one, or one of its
// sizes.
func mediaLibrary(siteDir string) []*MediaItem {
	pics := site.LoadMedia(filepath.Join(siteDir, "static"))
	items := []*MediaItem{}
	byStem := map[string]*MediaItem{}
	for u, p := range pics {
		it := &MediaItem{URL: u, Width: p.Width, Height: p.Height, Sizes: p.Sizes, UsedBy: []string{}}
		for _, sz := range p.Sizes {
			if fi, err := os.Stat(filepath.Join(siteDir, "static", filepath.FromSlash(sz.URL))); err == nil {
				it.Bytes += fi.Size()
				it.Files++
			}
		}
		for _, sz := range p.Sizes { // narrowest first; prefer the JPEG or PNG
			if !sz.WebP {
				it.Thumb = sz.URL
				break
			}
		}
		items = append(items, it)
		byStem[strings.TrimSuffix(u, path.Ext(u))] = it
	}
	_ = filepath.WalkDir(siteDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(siteDir, p)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			switch rel {
			case ".git", "public", "static", "node_modules":
				return fs.SkipDir
			}
			return nil
		}
		switch path.Ext(rel) {
		case ".md", ".html", ".yaml", ".yml", ".json", ".toml", ".css", ".txt":
		default:
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil || !bytes.Contains(data, []byte("/media/")) {
			return nil
		}
		for stem, it := range byStem {
			if bytes.Contains(data, []byte(stem)) {
				it.UsedBy = append(it.UsedBy, rel)
			}
		}
		for _, m := range mdImageRe.FindAllSubmatch(data, -1) {
			u := string(m[2])
			if it := byStem[strings.TrimSuffix(u, path.Ext(u))]; it != nil && it.Alt == "" {
				it.Alt = string(m[1])
			}
		}
		return nil
	})
	for _, it := range items {
		sort.Strings(it.UsedBy)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].URL > items[j].URL }) // newest month first
	return items
}

// mediaTypes are the only files the editor serves out of a checkout.
var mediaTypes = map[string]string{".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".png": "image/png", ".webp": "image/webp", ".gif": "image/gif"}

// apiMediaFile serves a picture from the person's checkout, for the
// library's thumbnails. Only pictures under static/media/, only as images,
// and sandboxed: a file that isn't really a picture can't run as a page.
func (s *Server) apiMediaFile(w http.ResponseWriter, r *http.Request, sess *Session, f *ForgeConfig, id string) {
	st, _, err := s.siteByID(r.Context(), sess, f, id)
	if s.signedOut(w, r, err) {
		return
	}
	if err != nil || st == nil {
		http.NotFound(w, r)
		return
	}
	u := r.URL.Query().Get("url")
	clean := path.Clean("/" + u)
	typ := mediaTypes[strings.ToLower(path.Ext(clean))]
	if clean != u || !strings.HasPrefix(clean, "/media/") || typ == "" {
		http.NotFound(w, r)
		return
	}
	branch := r.URL.Query().Get("branch")
	if branch == "" {
		branch = st.Branch
	}
	dir := filepath.Join(s.ws.dir(sess, st, branch), st.Subdir)
	data, err := os.ReadFile(filepath.Join(dir, "static", filepath.FromSlash(clean)))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "unreadable", http.StatusInternalServerError)
		return
	}
	h := w.Header()
	h.Set("Content-Type", typ)
	h.Set("Content-Security-Policy", "sandbox; default-src 'none'")
	h.Set("Cache-Control", "private, max-age=300")
	_, _ = w.Write(data)
}
