package site

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/media"
)

// mediaSite writes a site whose static/media/ holds a picture as the editor
// would have committed it.
func mediaSite(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	img := image.NewRGBA(image.Rect(0, 0, 2000, 1000))
	for y := 0; y < 1000; y++ {
		for x := 0; x < 2000; x++ {
			img.Set(x, y, color.RGBA{uint8(x), uint8(y), 90, 255})
		}
	}
	var b bytes.Buffer
	jpeg.Encode(&b, img, &jpeg.Options{Quality: 90})
	pic, err := media.Process(b.Bytes(), "dock.jpg", "2026/10", func(string) bool { return false })
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range pic.Files {
		p := filepath.Join(dir, "static", filepath.FromSlash(f.Path))
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, f.Data, 0o644)
	}
	os.MkdirAll(filepath.Join(dir, "content"), 0o755)
	os.WriteFile(filepath.Join(dir, "site.yaml"), []byte("name: T\nurl: https://example.org\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "content", "index.md"), []byte("---\ntitle: Home\n---\n\n![A dock at \"dawn\"](/media/2026/10/dock.jpg)\n\n![Elsewhere](/img/x.jpg)\n"), 0o644)
	return dir
}

func TestMediaPictures(t *testing.T) {
	s, err := Load(mediaSite(t), Options{})
	if err != nil {
		t.Fatal(err)
	}
	var home *Page
	for _, p := range s.Pages {
		if p.Path == "/" {
			home = p
		}
	}
	if home == nil {
		t.Fatal("no home page")
	}
	got := string(home.Content)
	for _, want := range []string{
		`<picture><source type="image/webp" srcset="/media/2026/10/dock-480w.webp 480w, /media/2026/10/dock-960w.webp 960w, /media/2026/10/dock.webp 1600w" sizes="(min-width: 50rem) 50rem, 100vw">`,
		`<img src="/media/2026/10/dock.jpg" alt="A dock at &quot;dawn&quot;" width="1600" height="800"`,
		`srcset="/media/2026/10/dock-480w.jpg 480w, /media/2026/10/dock-960w.jpg 960w, /media/2026/10/dock.jpg 1600w"`,
		`loading="lazy" decoding="async"></picture>`,
		`<img src="/img/x.jpg" alt="Elsewhere">`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %s\nin %s", want, got)
		}
	}
}
