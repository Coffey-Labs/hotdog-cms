package media

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"strings"
	"testing"
)

// photo makes a JPEG w×h whose left half is red and right half blue, so its
// orientation can be read back from the pixels.
func photo(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := color.RGBA{220, 20, 20, 255}
			if x >= w/2 {
				c = color.RGBA{20, 20, 220, 255}
			}
			img.Set(x, y, c)
		}
	}
	var b bytes.Buffer
	if err := jpeg.Encode(&b, img, &jpeg.Options{Quality: 95}); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// withEXIF puts an EXIF block (orientation, and a GPS block with a latitude)
// and a comment right after a JPEG's start marker, the way a phone does.
func withEXIF(jpg []byte, orientation uint16, gps bool) []byte {
	bo := binary.LittleEndian
	var t bytes.Buffer
	t.WriteString("II")
	binary.Write(&t, bo, uint16(42))
	binary.Write(&t, bo, uint32(8))
	n := uint16(1)
	if gps {
		n = 2
	}
	binary.Write(&t, bo, n)
	// Orientation: tag, type SHORT, count 1, value.
	binary.Write(&t, bo, []uint16{0x0112, 3})
	binary.Write(&t, bo, uint32(1))
	binary.Write(&t, bo, []uint16{orientation, 0})
	if gps {
		gpsOff := uint32(8 + 2 + 2*12 + 4)
		binary.Write(&t, bo, []uint16{0x8825, 4})
		binary.Write(&t, bo, uint32(1))
		binary.Write(&t, bo, gpsOff)
	}
	binary.Write(&t, bo, uint32(0)) // no next IFD
	if gps {
		binary.Write(&t, bo, uint16(1))
		binary.Write(&t, bo, []uint16{0x0001, 2}) // GPSLatitudeRef "N"
		binary.Write(&t, bo, uint32(2))
		t.Write([]byte{'N', 0, 0, 0})
		binary.Write(&t, bo, uint32(0))
	}
	seg := append([]byte("Exif\x00\x00"), t.Bytes()...)
	var out bytes.Buffer
	out.Write(jpg[:2])
	out.Write([]byte{0xFF, 0xE1})
	binary.Write(&out, binary.BigEndian, uint16(len(seg)+2))
	out.Write(seg)
	com := []byte("secret comment")
	out.Write([]byte{0xFF, 0xFE})
	binary.Write(&out, binary.BigEndian, uint16(len(com)+2))
	out.Write(com)
	out.Write(jpg[2:])
	return out.Bytes()
}

func none(string) bool { return false }

func TestReadJPEGMeta(t *testing.T) {
	m := ReadJPEGMeta(withEXIF(photo(t, 40, 20), 6, true))
	if m.Orientation != 6 || !m.GPS || !m.EXIF {
		t.Fatalf("got %+v", m)
	}
	m = ReadJPEGMeta(photo(t, 40, 20))
	if m.Orientation != 1 || m.GPS || m.EXIF {
		t.Fatalf("plain JPEG: got %+v", m)
	}
	if m := ReadJPEGMeta([]byte{0xFF, 0xD8, 0xFF, 0xE1, 0xFF}); m.Orientation != 1 {
		t.Fatalf("truncated: got %+v", m)
	}
}

func TestStripsAndOrients(t *testing.T) {
	in := withEXIF(photo(t, 2400, 1200), 6, true) // stored on its side
	img, err := Process(in, "Dock at Dawn.JPG", "2026/10", none)
	if err != nil {
		t.Fatal(err)
	}
	if img.URL != "/media/2026/10/dock-at-dawn.jpg" || img.Type != "image/jpeg" {
		t.Fatalf("url %s type %s", img.URL, img.Type)
	}
	// Shown the right way up it's 1200 wide and 2400 tall: 1200 is under the
	// largest width, so that's the main size, with 960 and 480 beside it.
	if img.Width != 1200 || img.Height != 2400 || len(img.Files) != 3 {
		t.Fatalf("main %dx%d, %d files", img.Width, img.Height, len(img.Files))
	}
	if img.Files[1].Path != "media/2026/10/dock-at-dawn-480w.jpg" && img.Files[1].Path != "media/2026/10/dock-at-dawn-960w.jpg" {
		t.Fatalf("variant path %s", img.Files[1].Path)
	}
	for _, f := range img.Files {
		m := ReadJPEGMeta(f.Data)
		if m.EXIF || m.GPS || bytes.Contains(f.Data, []byte("secret comment")) {
			t.Fatalf("%s still carries metadata", f.Path)
		}
		got, err := jpeg.Decode(bytes.NewReader(f.Data))
		if err != nil {
			t.Fatal(err)
		}
		if got.Bounds().Dx() != f.Width || got.Bounds().Dy() != f.Height {
			t.Fatalf("%s is %v, says %dx%d", f.Path, got.Bounds(), f.Width, f.Height)
		}
		// Red was on the left of the stored picture; a quarter turn clockwise
		// puts it on top.
		r, _, b, _ := got.At(f.Width/2, f.Height/8).RGBA()
		if r < b {
			t.Fatalf("%s: top isn't red, so it wasn't turned", f.Path)
		}
	}
	notes := strings.Join(img.Notes, " ")
	for _, want := range []string{"location", "EXIF", "right way up"} {
		if !strings.Contains(notes, want) {
			t.Errorf("notes %q don't mention %q", notes, want)
		}
	}
}

func TestResizesLargePhoto(t *testing.T) {
	img, err := Process(photo(t, 4000, 3000), "big.jpg", "2026/10", none)
	if err != nil {
		t.Fatal(err)
	}
	var widths []int
	for _, f := range img.Files {
		widths = append(widths, f.Width)
	}
	if len(widths) != 3 || widths[0] != 1600 || img.Height != 1200 {
		t.Fatalf("widths %v, main height %d", widths, img.Height)
	}
}

func TestNeverOverwrites(t *testing.T) {
	taken := func(p string) bool { return p == "static/media/2026/10/a.jpg" || p == "static/media/2026/10/a-2.jpg" }
	img, err := Process(photo(t, 100, 50), "a.jpg", "2026/10", taken)
	if err != nil {
		t.Fatal(err)
	}
	if img.URL != "/media/2026/10/a-3.jpg" || len(img.Files) != 1 {
		t.Fatalf("got %s with %d files", img.URL, len(img.Files))
	}
}

func TestPNGStaysPNGWithTransparency(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 64, 32))
	src.Set(1, 1, color.NRGBA{0, 0, 0, 255})
	var b bytes.Buffer
	png.Encode(&b, src)
	b.WriteString("trailing payload after IEND")
	img, err := Process(b.Bytes(), "logo.png", "2026/10", none)
	if err != nil {
		t.Fatal(err)
	}
	if img.Type != "image/png" || !strings.HasSuffix(img.URL, ".png") {
		t.Fatalf("got %s %s", img.Type, img.URL)
	}
	if bytes.Contains(img.Files[0].Data, []byte("trailing payload")) {
		t.Fatal("data after the image survived")
	}
	got, _ := png.Decode(bytes.NewReader(img.Files[0].Data))
	if _, _, _, a := got.At(10, 10).RGBA(); a != 0 {
		t.Fatal("transparency lost")
	}
}

func TestAnimatedGIFNoted(t *testing.T) {
	pal := color.Palette{color.Black, color.White}
	a := &gif.GIF{}
	for i := 0; i < 3; i++ {
		a.Image = append(a.Image, image.NewPaletted(image.Rect(0, 0, 10, 10), pal))
		a.Delay = append(a.Delay, 10)
	}
	var b bytes.Buffer
	gif.EncodeAll(&b, a)
	if n := gifFrames(b.Bytes()); n != 3 {
		t.Fatalf("counted %d frames", n)
	}
	img, err := Process(b.Bytes(), "spin.gif", "2026/10", none)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(img.Notes, " "), "first frame") {
		t.Fatalf("notes: %v", img.Notes)
	}
}

func TestRefuses(t *testing.T) {
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`)
	if _, err := Process(svg, "x.svg", "2026/10", none); err != ErrNotPicture {
		t.Fatalf("svg: %v", err)
	}
	if _, err := Process([]byte("<html><script>"), "x.jpg", "2026/10", none); err != ErrNotPicture {
		t.Fatalf("html named .jpg: %v", err)
	}
	// A PNG header claiming 100000×100000 pixels is refused before decoding.
	var b bytes.Buffer
	png.Encode(&b, image.NewGray(image.Rect(0, 0, 1, 1)))
	h := b.Bytes()
	binary.BigEndian.PutUint32(h[16:], 100000)
	binary.BigEndian.PutUint32(h[20:], 100000)
	binary.BigEndian.PutUint32(h[29:], crc32.ChecksumIEEE(h[12:29]))
	if _, err := Process(h, "bomb.png", "2026/10", none); err == nil || !strings.Contains(err.Error(), "megapixels") {
		t.Fatalf("bomb: %v", err)
	}
}
