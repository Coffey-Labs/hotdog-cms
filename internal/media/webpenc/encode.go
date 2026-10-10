// Package webpenc encodes WebP pictures in pure Go.
//
// libwebp.go is libwebp compiled to WebAssembly and translated to Go by
// wasm2go, taken unchanged (apart from the package name) from
// github.com/gen2brain/webp; see README.md. Its memory is one Go byte slice
// and every access to it is length-checked, so a fault in libwebp stays
// inside that slice. This package only encodes, and only pixels that Go's own
// decoders produced: it never parses a file someone uploaded. It loads no
// system library and registers no image format.
package webpenc

import (
	"errors"
	"image"
	"io"
)

var (
	errMemWrite = errors.New("webp: couldn't hand the picture to the encoder")
	errMemRead  = errors.New("webp: couldn't read the encoder's output")
	errEncode   = errors.New("webp: encoding failed")
)

// Options choose between lossy (photographs) and lossless (screenshots and
// drawings). Quality is 1-100 for lossy; for lossless it trades time for size.
type Options struct {
	Quality  int
	Lossless bool
}

// Encode writes m as WebP.
func Encode(w io.Writer, m *image.RGBA, o Options) error {
	q := o.Quality
	if q <= 0 || q > 100 {
		q = 80
	}
	width, height := m.Rect.Dx(), m.Rect.Dy()
	data := m.Pix
	if m.Stride != 4*width || len(data) != 4*width*height {
		c := image.NewRGBA(image.Rect(0, 0, width, height))
		for y := 0; y < height; y++ {
			copy(c.Pix[y*c.Stride:(y+1)*c.Stride], m.Pix[m.PixOffset(m.Rect.Min.X, m.Rect.Min.Y+y):])
		}
		data = c.Pix
	}
	// The RGBA here is premultiplied; libwebp wants straight alpha. Only
	// pixels that are partly transparent differ.
	if !m.Opaque() {
		data = unpremultiply(data)
	}

	mod := newModule()
	in := mod.Xmalloc(int32(len(data)))
	defer mod.Xfree(in)
	if !mod.write(in, data) {
		return errMemWrite
	}
	sizePtr := mod.Xmalloc(8)
	defer mod.Xfree(sizePtr)
	mod.write(sizePtr, make([]byte, 8)) // size_t is 4 bytes in the module; keep the rest zero
	lossless := int32(0)
	if o.Lossless {
		lossless = 1
	}
	const method = 4 // libwebp's default balance of speed and size
	out := mod.Xencode(in, int32(width), int32(height), sizePtr, 0, int32(q), method, lossless, 0)
	defer mod.Xfree(out)
	size, ok := mod.readUint64(sizePtr)
	if !ok {
		return errMemRead
	}
	if size == 0 {
		return errEncode
	}
	b, ok := mod.read(out, int32(size))
	if !ok {
		return errMemRead
	}
	_, err := w.Write(b)
	return err
}

func unpremultiply(p []byte) []byte {
	out := make([]byte, len(p))
	copy(out, p)
	for i := 0; i+3 < len(out); i += 4 {
		a := uint32(out[i+3])
		if a == 0 || a == 255 {
			continue
		}
		for k := 0; k < 3; k++ {
			out[i+k] = uint8(min(255, (uint32(out[i+k])*255+a/2)/a))
		}
	}
	return out
}

func (m *module) write(ptr int32, data []byte) bool {
	if ptr < 0 || int(ptr)+len(data) > len(m.memory) {
		return false
	}
	copy(m.memory[ptr:], data)
	return true
}

func (m *module) read(ptr, size int32) ([]byte, bool) {
	if ptr < 0 || size < 0 || int(ptr)+int(size) > len(m.memory) {
		return nil, false
	}
	out := make([]byte, size)
	copy(out, m.memory[ptr:ptr+size])
	return out, true
}

func (m *module) readUint64(ptr int32) (uint64, bool) {
	if ptr < 0 || int(ptr)+8 > len(m.memory) {
		return 0, false
	}
	return load64(m.memory[ptr:]), true
}

func newModule() *module {
	mod := newModuleRaw(&wasiHost{})
	mod.X_initialize()
	return mod
}

// wasiHost answers the module's two system calls; libwebp only writes
// diagnostics, which are dropped.
type wasiHost struct{ mod *module }

func (h *wasiHost) Init(m any)                { h.mod = m.(*module) }
func (h *wasiHost) Xfd_close(fd int32) int32 { return 0 }

func (h *wasiHost) Xfd_write(fd, iovs, iovsLen, nwrittenPtr int32) int32 {
	mem := h.mod.memory
	var written uint32
	for i := int32(0); i < iovsLen; i++ {
		written += load32(mem[iovs+i*8+4:])
	}
	store32(mem[nwrittenPtr:], written)
	return 0
}
