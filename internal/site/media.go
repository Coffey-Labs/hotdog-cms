package site

import (
	"fmt"
	"html"
	"image"
	_ "image/jpeg" // DecodeConfig for the sizes of pictures in media/
	_ "image/png"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Pictures in static/media/ are published as the editor writes them: a main
// file (dock.jpg), smaller sizes beside it (dock-480w.jpg, dock-960w.jpg) and
// WebP copies of each (dock.webp, dock-480w.webp). A Markdown image that
// points at the main file is published as a <picture> offering all of them,
// with its width and height so the page doesn't jump while it loads.

// MediaPicture is one picture in media/ and the sizes it's published at.
type MediaPicture struct {
	Width, Height int
	Sizes         []MediaSize // narrowest first
}

// MediaSize is one published file of a picture.
type MediaSize struct {
	URL   string
	Width int
	WebP  bool
}

var mediaVariantRe = regexp.MustCompile(`^(.+)-(\d+)w\.(jpg|png|webp)$`)

// LoadMedia indexes static/media/, by the main file's address.
func LoadMedia(staticDir string) map[string]*MediaPicture {
	root := filepath.Join(staticDir, "media")
	type found struct {
		main     string // its address, if the main JPEG or PNG is there
		mainWebP bool
		sizes    []MediaSize
	}
	byStem := map[string]*found{}
	get := func(stem string) *found {
		if byStem[stem] == nil {
			byStem[stem] = &found{}
		}
		return byStem[stem]
	}
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(staticDir, p)
		u := "/" + filepath.ToSlash(rel)
		dir, name := path.Split(u)
		if m := mediaVariantRe.FindStringSubmatch(name); m != nil {
			w, _ := strconv.Atoi(m[2])
			get(dir + m[1]).sizes = append(get(dir+m[1]).sizes, MediaSize{URL: u, Width: w, WebP: m[3] == "webp"})
			return nil
		}
		ext := path.Ext(name)
		stem := dir + strings.TrimSuffix(name, ext)
		switch strings.ToLower(ext) {
		case ".jpg", ".jpeg", ".png":
			get(stem).main = u
		case ".webp":
			get(stem).mainWebP = true
		}
		return nil
	})
	out := map[string]*MediaPicture{}
	for stem, f := range byStem {
		if f.main == "" {
			continue
		}
		file, err := os.Open(filepath.Join(staticDir, filepath.FromSlash(f.main)))
		if err != nil {
			continue
		}
		cfg, _, err := image.DecodeConfig(file)
		file.Close()
		if err != nil {
			continue
		}
		pic := &MediaPicture{Width: cfg.Width, Height: cfg.Height}
		mainExt := path.Ext(f.main)
		for _, sz := range f.sizes {
			// Only sizes of this main file's kind, or its WebP copies.
			if sz.Width < cfg.Width && (sz.WebP || path.Ext(sz.URL) == mainExt) {
				pic.Sizes = append(pic.Sizes, sz)
			}
		}
		pic.Sizes = append(pic.Sizes, MediaSize{URL: f.main, Width: cfg.Width})
		if f.mainWebP {
			pic.Sizes = append(pic.Sizes, MediaSize{URL: stem + ".webp", Width: cfg.Width, WebP: true})
		}
		sort.SliceStable(pic.Sizes, func(i, j int) bool { return pic.Sizes[i].Width < pic.Sizes[j].Width })
		out[f.main] = pic
	}
	return out
}

const defaultImageSizes = "(min-width: 50rem) 50rem, 100vw"

var imgTagRe = regexp.MustCompile(`<img src="(/media/[^"]+)"([^>]*?)\s*/?>`)

// pictures turns <img> tags pointing at media/ into responsive pictures.
func (m *Markdown) pictures(out []byte) []byte {
	if len(m.media) == 0 {
		return out
	}
	return imgTagRe.ReplaceAllFunc(out, func(tag []byte) []byte {
		sub := imgTagRe.FindSubmatch(tag)
		src := html.UnescapeString(string(sub[1]))
		pic := m.media[src]
		if pic == nil {
			return tag
		}
		rest := string(sub[2])
		var plain, webp []string
		for _, sz := range pic.Sizes {
			entry := fmt.Sprintf("%s %dw", html.EscapeString(sz.URL), sz.Width)
			if sz.WebP {
				webp = append(webp, entry)
			} else {
				plain = append(plain, entry)
			}
		}
		sizes := html.EscapeString(m.imageSizes)
		var b strings.Builder
		img := fmt.Sprintf(`<img src="%s"%s width="%d" height="%d"`, html.EscapeString(src), rest, pic.Width, pic.Height)
		if len(plain) > 1 {
			img += fmt.Sprintf(` srcset="%s" sizes="%s"`, strings.Join(plain, ", "), sizes)
		}
		if !strings.Contains(rest, "loading=") {
			img += ` loading="lazy" decoding="async"`
		}
		img += ">"
		if len(webp) == 0 {
			return []byte(img)
		}
		fmt.Fprintf(&b, `<picture><source type="image/webp" srcset="%s" sizes="%s">%s</picture>`, strings.Join(webp, ", "), sizes, img)
		return []byte(b.String())
	})
}
