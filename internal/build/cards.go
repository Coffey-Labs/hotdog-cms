package build

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/jpeg" // a logo can be a JPEG
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"

	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/site"
)

// Social cards: the picture a link preview shows (Mastodon, LinkedIn,
// Bluesky, chat apps), drawn at build time for every page that doesn't name
// its own image. 1200×630, the page's title large, the site's name small, in
// the site's colors. Drawn in Go with the Go fonts unless the site brings its
// own, so the build needs nothing installed. The file name carries a hash of
// what's on the card, so a changed title is a new address and previews
// refresh.

const cardW, cardH = 1200, 630

// cardPath is where a page's card is published, or "" if it gets none.
func cardPath(s *site.Site, p *site.Page) string {
	cc := s.Config.Cards
	if !cc.On() || p.Image != "" || p.RedirectTo != "" || p.NoIndex || (p.Kind != site.KindPage && p.Kind != site.KindList) {
		return ""
	}
	name := strings.Trim(strings.ReplaceAll(p.Path, "/", "-"), "-")
	name = strings.TrimSuffix(name, ".html")
	if name == "" {
		name = "home"
	}
	sum := sha256.Sum256([]byte(p.Title + "\x00" + p.Summary + "\x00" + s.Config.Name + "\x00" + cc.Signature()))
	return "/cards/" + name + "." + hex.EncodeToString(sum[:4]) + ".png"
}

// assignCards points every page that will get a card at it, so templates
// write og:image for it, and returns the cards to draw. The live preview does
// this too, without drawing.
func assignCards(s *site.Site) map[*site.Page]string {
	cards := map[*site.Page]string{}
	for _, p := range s.Pages {
		if c := cardPath(s, p); c != "" {
			cards[p] = c
			p.Image = c
			if p.ImageAlt == "" {
				p.ImageAlt = p.Title
			}
		}
	}
	return cards
}

type cardStyle struct {
	bg, fg, accent color.Color
	title, small   *opentype.Font
	logo           image.Image
}

func parseHex(h string, def color.Color) color.Color {
	h = strings.TrimPrefix(h, "#")
	if len(h) == 3 {
		h = string([]byte{h[0], h[0], h[1], h[1], h[2], h[2]})
	}
	v, err := strconv.ParseUint(h, 16, 32)
	if len(h) != 6 || err != nil {
		return def
	}
	return color.RGBA{uint8(v >> 16), uint8(v >> 8), uint8(v), 255}
}

func loadCardStyle(siteDir string, s *site.Site) (*cardStyle, error) {
	cc := s.Config.Cards
	// Colors: the card's own, else the site's look (band and its text, which
	// read well large), else a dark default.
	look := map[string]string{}
	if lf, err := site.LoadLook(siteDir); err == nil && lf != nil {
		vals, _ := lf.Values(s.Config.Look)
		for _, t := range lf.Tokens {
			look[t.Name] = lf.Get(vals, t.Name)
		}
	}
	pick := func(own string, names ...string) string {
		if own != "" {
			return own
		}
		for _, n := range names {
			if look[n] != "" {
				return look[n]
			}
		}
		return ""
	}
	st := &cardStyle{
		bg:     parseHex(pick(cc.Background, "band", "text"), color.RGBA{0x1d, 0x2b, 0x3a, 255}),
		fg:     parseHex(pick(cc.Text, "band-text", "bg"), color.RGBA{0xf5, 0xf1, 0xea, 255}),
		accent: parseHex(pick(cc.Accent, "accent"), color.RGBA{0xf4, 0xbe, 0x2c, 255}),
	}
	var err error
	if st.title, err = loadFont(siteDir, cc.Font, gobold.TTF); err != nil {
		return nil, err
	}
	if st.small, err = loadFont(siteDir, cc.FontSmall, goregular.TTF); err != nil {
		return nil, err
	}
	logo := cc.Logo
	if logo == "" {
		logo = look["logo"]
	}
	if logo != "" && !strings.HasSuffix(strings.ToLower(logo), ".svg") {
		for _, base := range []string{"static", "assets"} {
			if f, err := os.Open(filepath.Join(siteDir, base, filepath.FromSlash(strings.TrimPrefix(logo, "/")))); err == nil {
				st.logo, _, _ = image.Decode(f)
				f.Close()
				break
			}
		}
	}
	return st, nil
}

func loadFont(siteDir, rel string, fallback []byte) (*opentype.Font, error) {
	data := fallback
	if rel != "" {
		b, err := os.ReadFile(filepath.Join(siteDir, filepath.FromSlash(rel)))
		if err != nil {
			return nil, fmt.Errorf("cards: font %s: %w", rel, err)
		}
		data = b
	}
	f, err := opentype.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("cards: font %s: %w", rel, err)
	}
	return f, nil
}

func face(f *opentype.Font, size float64) font.Face {
	fc, _ := opentype.NewFace(f, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingFull})
	return fc
}

// wrap breaks text into lines no wider than width.
func wrap(fc font.Face, text string, width int) []string {
	var lines []string
	line := ""
	for _, w := range strings.Fields(text) {
		try := strings.TrimSpace(line + " " + w)
		if font.MeasureString(fc, try).Ceil() > width && line != "" {
			lines = append(lines, line)
			line = w
		} else {
			line = try
		}
	}
	if line != "" {
		lines = append(lines, line)
	}
	return lines
}

// drawCard renders one card.
func drawCard(st *cardStyle, title, summary, siteName string) ([]byte, error) {
	img := image.NewRGBA(image.Rect(0, 0, cardW, cardH))
	draw.Draw(img, img.Bounds(), &image.Uniform{st.bg}, image.Point{}, draw.Src)
	// The accent: a bar down the left edge.
	draw.Draw(img, image.Rect(0, 0, 18, cardH), &image.Uniform{st.accent}, image.Point{}, draw.Src)

	const margin = 90
	width := cardW - 2*margin
	// The largest size at which the title fits in four lines.
	size := 76.0
	var fc font.Face
	var lines []string
	for ; size >= 40; size -= 4 {
		fc = face(st.title, size)
		lines = wrap(fc, title, width)
		if len(lines) <= 4 {
			break
		}
	}
	if len(lines) > 4 {
		lines = append(lines[:3], strings.TrimSpace(lines[3])+"…")
		lines = lines[:4]
	}
	lineH := int(size * 1.18)
	top := margin + 40
	if st.logo != nil {
		top += 30
	}
	d := &font.Drawer{Dst: img, Src: &image.Uniform{st.fg}, Face: fc}
	for i, l := range lines {
		d.Dot = fixed.P(margin, top+int(size)+i*lineH)
		d.DrawString(l)
	}
	// The summary, softer, under a title short enough to leave room.
	if summary != "" && len(lines) <= 2 {
		sf := face(st.small, 34)
		r, g, b, _ := st.fg.RGBA()
		br, bg, bb, _ := st.bg.RGBA()
		soft := color.RGBA{uint8((r*3 + br) / 4 >> 8), uint8((g*3 + bg) / 4 >> 8), uint8((b*3 + bb) / 4 >> 8), 255}
		ds := &font.Drawer{Dst: img, Src: &image.Uniform{soft}, Face: sf}
		y := top + int(size) + (len(lines)-1)*lineH + 70
		sl := wrap(sf, summary, width)
		if len(sl) > 3 {
			sl = append(sl[:2], strings.TrimSpace(sl[2])+"…")
		}
		for i, l := range sl {
			ds.Dot = fixed.P(margin, y+i*46)
			ds.DrawString(l)
		}
	}

	small := face(st.small, 34)
	ds := &font.Drawer{Dst: img, Src: &image.Uniform{st.fg}, Face: small}
	x := margin
	if st.logo != nil {
		// The logo, 56 px tall, before the site's name at the bottom.
		b := st.logo.Bounds()
		w := b.Dx() * 56 / max(1, b.Dy())
		r := image.Rect(margin, cardH-margin-56+8, margin+w, cardH-margin+8)
		xdraw.CatmullRom.Scale(img, r, st.logo, b, xdraw.Over, nil)
		x += w + 18
	}
	ds.Dot = fixed.P(x, cardH-margin)
	ds.DrawString(siteName)

	var buf bytes.Buffer
	if err := (&png.Encoder{CompressionLevel: png.BestCompression}).Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// writeCards draws the cards assignCards gave out.
func writeCards(siteDir, out string, s *site.Site, cards map[*site.Page]string) error {
	if len(cards) == 0 {
		return nil
	}
	st, err := loadCardStyle(siteDir, s)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(out, "cards"), 0o755); err != nil {
		return err
	}
	for p, c := range cards {
		data, err := drawCard(st, p.Title, p.Summary, s.Config.Name)
		if err != nil {
			return fmt.Errorf("card for %s: %w", p.Path, err)
		}
		if err := os.WriteFile(filepath.Join(out, filepath.FromSlash(strings.TrimPrefix(c, "/"))), data, 0o644); err != nil {
			return err
		}
	}
	return nil
}
