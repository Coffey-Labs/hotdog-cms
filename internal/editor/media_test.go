package editor

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/media"
)

// phonePhoto is a JPEG with an EXIF block holding a GPS position.
func phonePhoto(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 1200, 800))
	for i := range img.Pix {
		img.Pix[i] = uint8(i * 7)
	}
	img.Set(0, 0, color.RGBA{255, 0, 0, 255})
	var b bytes.Buffer
	jpeg.Encode(&b, img, nil)
	j := b.Bytes()
	tiff := []byte{'I', 'I', 42, 0, 8, 0, 0, 0, 1, 0, 0x25, 0x88, 4, 0, 1, 0, 0, 0, 26, 0, 0, 0, 0, 0, 0, 0,
		1, 0, 1, 0, 2, 0, 2, 0, 0, 0, 'N', 0, 0, 0, 0, 0, 0, 0}
	seg := append([]byte("Exif\x00\x00"), tiff...)
	var out bytes.Buffer
	out.Write(j[:2])
	out.Write([]byte{0xFF, 0xE1})
	binary.Write(&out, binary.BigEndian, uint16(len(seg)+2))
	out.Write(seg)
	out.Write(j[2:])
	return out.Bytes()
}

func uploadReq(s *Server, jar []*http.Cookie, name string, data []byte, fields map[string]string) *httptest.ResponseRecorder {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	for k, v := range fields {
		mw.WriteField(k, v)
	}
	fw, _ := mw.CreateFormFile("file", name)
	fw.Write(data)
	mw.Close()
	req := httptest.NewRequest("POST", "http://editor.test/api/sites/site/media", &body)
	for _, c := range jar {
		req.AddCookie(c)
	}
	req.Header.Set("Origin", "http://editor.test")
	req.Header.Set("X-HotDog", "1")
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	return rec
}

func TestMedia(t *testing.T) {
	s, ff, repo := newTestServerRepo(t)
	jar := []*http.Cookie{signIn(t, s, ff)}
	mainTip := git(t, repo, "rev-parse", "main")

	// From the publishing branch, an upload starts the page's draft.
	rec := uploadReq(s, jar, "IMG_0042.JPG", phonePhoto(t), map[string]string{"branch": "main", "baseCommit": mainTip, "source": "content/about.md"})
	var up map[string]any
	json.NewDecoder(rec.Body).Decode(&up)
	if rec.Code != 200 || up["branch"] != "draft/about" || up["created"] != true || !strings.HasPrefix(up["url"].(string), "/media/") || !strings.HasSuffix(up["url"].(string), "/img-0042.jpg") {
		t.Fatalf("upload: %d %v", rec.Code, up)
	}
	if git(t, repo, "rev-parse", "main") != mainTip {
		t.Fatal("an upload reached the publishing branch")
	}
	url := up["url"].(string)
	committed := git(t, repo, "ls-tree", "-r", "--name-only", "draft/about", "static/media")
	if !strings.Contains(committed, strings.TrimPrefix(url, "/")) || !strings.Contains(committed, ".webp") {
		t.Fatalf("committed files:\n%s", committed)
	}
	raw := git(t, repo, "show", "draft/about:static"+url)
	if media.ReadJPEGMeta([]byte(raw)).GPS {
		t.Fatal("the committed picture still carries its location")
	}
	if notes, _ := json.Marshal(up["notes"]); !strings.Contains(string(notes), "location") {
		t.Errorf("notes: %s", notes)
	}

	// Not a picture: refused, nothing committed.
	tip := git(t, repo, "rev-parse", "draft/about")
	rec = uploadReq(s, jar, "x.svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`), map[string]string{"branch": "draft/about", "baseCommit": tip})
	if rec.Code != 422 || git(t, repo, "rev-parse", "draft/about") != tip {
		t.Fatalf("svg: %d %s", rec.Code, rec.Body)
	}

	// Once a page uses it, the library says where, and offers its alt text.
	rec = post(s, "/api/sites/site/save", jar, map[string]any{"source": "content/about.md", "branch": "draft/about", "baseCommit": tip,
		"edit": map[string]any{"body": "![The dock at dawn](" + url + ")"}})
	if rec.Code != 200 {
		t.Fatalf("save: %d %s", rec.Code, rec.Body)
	}
	rec = do(s, "GET", "/api/sites/site/media?branch=draft/about", jar, nil)
	var lib []MediaItem
	json.NewDecoder(rec.Body).Decode(&lib)
	if rec.Code != 200 || len(lib) != 1 || lib[0].URL != url || lib[0].Width != 1200 || lib[0].Alt != "The dock at dawn" ||
		len(lib[0].UsedBy) != 1 || lib[0].UsedBy[0] != "content/about.md" || lib[0].Files < 4 || !strings.HasSuffix(lib[0].Thumb, "-480w.jpg") {
		t.Fatalf("library: %d %+v", rec.Code, lib)
	}

	// Thumbnails: pictures only, as images, sandboxed.
	rec = do(s, "GET", "/api/sites/site/media-file?branch=draft/about&url="+lib[0].Thumb, jar, nil)
	if rec.Code != 200 || rec.Header().Get("Content-Type") != "image/jpeg" || !strings.Contains(rec.Header().Get("Content-Security-Policy"), "sandbox") {
		t.Fatalf("thumb: %d %v", rec.Code, rec.Header())
	}
	for _, bad := range []string{"/media/../site.yaml", "/site.yaml", "/media/x.html", "media/a.jpg"} {
		if rec := do(s, "GET", "/api/sites/site/media-file?branch=draft/about&url="+bad, jar, nil); rec.Code != 404 {
			t.Errorf("%s: %d", bad, rec.Code)
		}
	}
	if rec := do(s, "GET", "/api/sites/site/media?branch=draft/about", nil, nil); rec.Code != 401 {
		t.Errorf("library without signing in: %d", rec.Code)
	}
}

func TestNoWritesThroughLinks(t *testing.T) {
	outside := t.TempDir()
	dir := t.TempDir()
	os.Symlink(outside, filepath.Join(dir, "data"))
	os.WriteFile(filepath.Join(outside, "secret"), []byte("s3cret"), 0o600)
	if err := writeIn(dir, "data/planted", []byte("x")); err == nil {
		t.Error("wrote through a linked folder")
	}
	if _, err := os.Stat(filepath.Join(outside, "planted")); err == nil {
		t.Error("a file appeared outside the checkout")
	}
	if b, err := readIn(dir, "data/secret"); err == nil {
		t.Errorf("read through a linked folder: %q", b)
	}
	os.Symlink(filepath.Join(outside, "secret"), filepath.Join(dir, "page.md"))
	if _, err := readIn(dir, "page.md"); err == nil {
		t.Error("read a linked file")
	}
}
