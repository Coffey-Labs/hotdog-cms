// Package media prepares pictures that come in through the editor.
//
// Nothing is published as uploaded. Each picture is decoded (in Go, never by
// a C library), turned the right way up, resized to a few widths and encoded
// afresh, so what reaches the site is pixels and nothing else: no EXIF, no
// location, no comments, nothing hidden after the image data. SVG is refused,
// because it's a document that can carry scripts; it goes in through git,
// where a person reviews it.
package media

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"math"
	"net/http"
	"path"
	"regexp"
	"strings"

	"golang.org/x/image/draw"
	xwebp "golang.org/x/image/webp"

	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/media/webpenc"
)

const (
	// MaxBytes is the largest file accepted.
	MaxBytes = 25 << 20
	// MaxPixels bounds the decoded size, checked from the header before any
	// decoding, so a small file that claims to be enormous is refused cheaply.
	MaxPixels = 60_000_000
	// JPEGQuality is the re-encoding quality for photographs.
	JPEGQuality = 82
)

// WebPQuality is the lossy WebP quality for photographs. Pictures kept as
// PNG get lossless WebP instead.
const WebPQuality = 80

// Widths are the sizes a picture is published at. The largest is the main
// file; the smaller ones are offered to browsers through srcset. A picture
// narrower than one of these isn't enlarged to it.
var Widths = []int{480, 960, 1600}

// File is one encoded size of a picture.
type File struct {
	Path   string // under the site's static folder, e.g. media/2026/10/dock-960w.jpg
	Width  int
	Height int
	Data   []byte
	WebP   bool // the WebP copy of the size before it
}

// Image is a picture ready to commit: its files, main file first.
type Image struct {
	URL    string // the main file's address on the site
	Width  int
	Height int
	Type   string   // image/jpeg or image/png
	Files  []File   // each size, followed by its WebP copy where that's smaller
	Notes  []string // what was done to it, in words, for the person who uploaded it
}

// ErrNotPicture is returned for anything that isn't a JPEG, PNG, GIF or WebP.
var ErrNotPicture = errors.New("only JPEG, PNG, GIF and WebP pictures can be uploaded here; SVG and other files go in through git, where they're reviewed")

var sem = make(chan struct{}, 2) // decoding is memory-hungry; two at a time

// Process prepares an uploaded picture. name is the uploaded file name, used
// for the published name; dir is the folder under media/ (such as
// "2026/10"); taken reports whether a path is already used in the site, so
// an existing picture is never overwritten.
func Process(data []byte, name, dir string, taken func(string) bool) (*Image, error) {
	if len(data) > MaxBytes {
		return nil, fmt.Errorf("the picture is %d MB; the limit is %d MB", len(data)>>20, MaxBytes>>20)
	}
	sem <- struct{}{}
	defer func() { <-sem }()

	kind := http.DetectContentType(data)
	var (
		cfg    image.Config
		decode func() (image.Image, error)
		err    error
		notes  []string
		meta   = JPEGMeta{Orientation: 1}
	)
	r := func() *bytes.Reader { return bytes.NewReader(data) }
	switch kind {
	case "image/jpeg":
		cfg, err = jpeg.DecodeConfig(r())
		decode = func() (image.Image, error) { return jpeg.Decode(r()) }
		meta = ReadJPEGMeta(data)
	case "image/png":
		cfg, err = png.DecodeConfig(r())
		decode = func() (image.Image, error) { return png.Decode(r()) }
	case "image/gif":
		cfg, err = gif.DecodeConfig(r())
		decode = func() (image.Image, error) { return gif.Decode(r()) }
		if gifFrames(data) > 1 {
			notes = append(notes, "This GIF was animated; only its first frame is kept.")
		}
	case "image/webp":
		cfg, err = xwebp.DecodeConfig(r())
		decode = func() (image.Image, error) { return xwebp.Decode(r()) }
	default:
		return nil, ErrNotPicture
	}
	if err != nil {
		return nil, fmt.Errorf("the picture couldn't be read: %v", err)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width*cfg.Height > MaxPixels {
		return nil, fmt.Errorf("the picture is %d×%d; the limit is %d megapixels", cfg.Width, cfg.Height, MaxPixels/1_000_000)
	}
	src, err := decode()
	if err != nil {
		return nil, fmt.Errorf("the picture couldn't be read: %v", err)
	}
	if meta.GPS {
		notes = append(notes, "Removed the location it was taken at.")
	}
	if meta.EXIF {
		notes = append(notes, "Removed camera details (EXIF).")
	}
	if meta.Orientation != 1 {
		notes = append(notes, "Turned it the right way up.")
	}

	// Sizes, worked out the right way up.
	ow, oh := cfg.Width, cfg.Height
	if meta.Orientation >= 5 {
		ow, oh = oh, ow
	}
	mainW := min(ow, Widths[len(Widths)-1])
	widths := []int{mainW}
	for _, w := range Widths {
		if w < mainW {
			widths = append(widths, w)
		}
	}

	// The format: photographs stay JPEG; PNG and GIF stay PNG, since they're
	// usually screenshots and drawings that JPEG would smear; WebP becomes
	// JPEG unless it has transparency.
	typ, ext := "image/png", ".png"
	if kind == "image/jpeg" {
		typ, ext = "image/jpeg", ".jpg"
	}

	base := slug(strings.TrimSuffix(path.Base(name), path.Ext(name)))
	stem := path.Join("media", dir, base)
	for i := 2; taken("static/"+stem+ext) && i < 1000; i++ {
		stem = path.Join("media", dir, fmt.Sprintf("%s-%d", base, i))
	}

	img := &Image{URL: "/" + stem + ext}
	for k, w := range widths {
		h := max(1, int(math.Round(float64(oh)*float64(w)/float64(ow))))
		rw, rh := w, h
		if meta.Orientation >= 5 {
			rw, rh = h, w
		}
		px := orient(scale(src, rw, rh), meta.Orientation)
		if k == 0 && kind == "image/webp" && px.Opaque() {
			typ, ext = "image/jpeg", ".jpg"
			img.URL = "/" + stem + ext
		}
		var buf bytes.Buffer
		if typ == "image/jpeg" {
			err = jpeg.Encode(&buf, px, &jpeg.Options{Quality: JPEGQuality})
		} else {
			err = (&png.Encoder{CompressionLevel: png.BestCompression}).Encode(&buf, px)
		}
		if err != nil {
			return nil, err
		}
		p := stem + ext
		if k > 0 {
			p = fmt.Sprintf("%s-%dw%s", stem, w, ext)
		}
		img.Files = append(img.Files, File{Path: p, Width: w, Height: h, Data: buf.Bytes()})
		// A WebP beside it, for browsers that take one, when it's smaller.
		var wb bytes.Buffer
		if err := webpenc.Encode(&wb, px, webpenc.Options{Quality: WebPQuality, Lossless: typ == "image/png"}); err != nil {
			return nil, err
		}
		if wb.Len() < buf.Len() {
			img.Files = append(img.Files, File{Path: strings.TrimSuffix(p, ext) + ".webp", Width: w, Height: h, Data: wb.Bytes(), WebP: true})
		}
	}
	img.Width, img.Height, img.Type = img.Files[0].Width, img.Files[0].Height, typ
	if mainW < ow {
		notes = append(notes, fmt.Sprintf("Resized from %d px wide to %d.", ow, mainW))
	}
	img.Notes = notes
	return img, nil
}

// VariantRe matches the smaller sizes Process writes beside a main file:
// name-480w.jpg next to name.jpg.
var VariantRe = regexp.MustCompile(`^(.+)-(\d+)w\.(jpg|png|webp)$`)

// scale resizes to w×h. A large reduction is done in halving steps, each an
// exact two-by-two average, and the last step with a careful filter: quick,
// and without the shimmer that one big jump leaves in fine detail.
func scale(src image.Image, w, h int) *image.RGBA {
	b := src.Bounds()
	cur := src
	for cw, ch := b.Dx(), b.Dy(); cw/2 >= w*2 && ch/2 >= h*2; {
		cw, ch = cw/2, ch/2
		half := image.NewRGBA(image.Rect(0, 0, cw, ch))
		draw.ApproxBiLinear.Scale(half, half.Rect, cur, cur.Bounds(), draw.Src, nil)
		cur = half
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	if cur.Bounds().Dx() == w && cur.Bounds().Dy() == h {
		draw.Draw(dst, dst.Rect, cur, cur.Bounds().Min, draw.Src)
	} else {
		draw.CatmullRom.Scale(dst, dst.Rect, cur, cur.Bounds(), draw.Src, nil)
	}
	return dst
}

// orient turns pixels the way an EXIF orientation says the picture should be
// shown, so the published file needs no orientation tag.
func orient(src *image.RGBA, o int) *image.RGBA {
	if o <= 1 || o > 8 {
		return src
	}
	w, h := src.Rect.Dx(), src.Rect.Dy()
	dw, dh := w, h
	if o >= 5 {
		dw, dh = h, w
	}
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var dx, dy int
			switch o {
			case 2: // mirrored
				dx, dy = w-1-x, y
			case 3: // upside down
				dx, dy = w-1-x, h-1-y
			case 4: // mirrored, upside down
				dx, dy = x, h-1-y
			case 5: // mirrored, on its side
				dx, dy = y, x
			case 6: // on its side: a quarter turn clockwise puts it right
				dx, dy = h-1-y, x
			case 7:
				dx, dy = h-1-y, w-1-x
			case 8: // a quarter turn anticlockwise puts it right
				dx, dy = y, w-1-x
			}
			si := src.PixOffset(x, y)
			di := dst.PixOffset(dx, dy)
			copy(dst.Pix[di:di+4], src.Pix[si:si+4])
		}
	}
	return dst
}

// gifFrames counts a GIF's frames by walking its blocks, without decoding
// any of them.
func gifFrames(d []byte) int {
	if len(d) < 13 {
		return 0
	}
	i := 13
	if d[10]&0x80 != 0 {
		i += 3 << (d[10]&7 + 1)
	}
	skipBlocks := func() bool {
		for i < len(d) {
			n := int(d[i])
			i++
			if n == 0 {
				return true
			}
			i += n
		}
		return false
	}
	frames := 0
	for i < len(d) {
		switch d[i] {
		case 0x21: // extension: label, then sub-blocks
			i += 2
			if !skipBlocks() {
				return frames
			}
		case 0x2C: // a frame: descriptor, local colors, LZW size, sub-blocks
			frames++
			if i+10 > len(d) {
				return frames
			}
			flags := d[i+9]
			i += 10
			if flags&0x80 != 0 {
				i += 3 << (flags&7 + 1)
			}
			i++
			if !skipBlocks() {
				return frames
			}
		default: // 0x3B, the end, or something unreadable
			return frames
		}
	}
	return frames
}

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

func slug(s string) string {
	s = strings.Trim(slugRe.ReplaceAllString(strings.ToLower(s), "-"), "-")
	if len(s) > 60 {
		s = strings.TrimRight(s[:60], "-")
	}
	if s == "" {
		return "image"
	}
	return s
}
