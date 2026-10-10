package media

import (
	"bytes"
	"encoding/binary"
)

// JPEGMeta is what a JPEG's EXIF block says that matters here: which way up
// the picture goes, and whether it carries the place it was taken.
type JPEGMeta struct {
	Orientation int  // 1-8 as EXIF defines them; 1 when absent
	GPS         bool // a GPS block with at least one entry
	EXIF        bool // any EXIF block at all
}

// ReadJPEGMeta walks a JPEG's segments up to the image data and reads the
// EXIF block, if there is one. It never fails: anything it can't make sense
// of is treated as absent.
func ReadJPEGMeta(data []byte) JPEGMeta {
	m := JPEGMeta{Orientation: 1}
	if len(data) < 4 || data[0] != 0xFF || data[1] != 0xD8 {
		return m
	}
	for i := 2; i+4 <= len(data); {
		if data[i] != 0xFF {
			return m
		}
		marker := data[i+1]
		if marker == 0xFF { // fill byte
			i++
			continue
		}
		if marker == 0xDA || marker == 0xD9 { // image data, or the end
			return m
		}
		if marker >= 0xD0 && marker <= 0xD7 || marker == 0x01 { // no length
			i += 2
			continue
		}
		n := int(binary.BigEndian.Uint16(data[i+2:]))
		if n < 2 || i+2+n > len(data) {
			return m
		}
		seg := data[i+4 : i+2+n]
		if marker == 0xE1 && bytes.HasPrefix(seg, []byte("Exif\x00\x00")) {
			m.EXIF = true
			readTIFF(seg[6:], &m)
		}
		i += 2 + n
	}
	return m
}

func readTIFF(t []byte, m *JPEGMeta) {
	if len(t) < 8 {
		return
	}
	var bo binary.ByteOrder
	switch string(t[:2]) {
	case "II":
		bo = binary.LittleEndian
	case "MM":
		bo = binary.BigEndian
	default:
		return
	}
	if bo.Uint16(t[2:]) != 42 {
		return
	}
	entries := func(off uint32) [][]byte {
		if off < 8 || int(off)+2 > len(t) {
			return nil
		}
		count := int(bo.Uint16(t[off:]))
		var out [][]byte
		for k := 0; k < count; k++ {
			p := int(off) + 2 + k*12
			if p+12 > len(t) {
				break
			}
			out = append(out, t[p:p+12])
		}
		return out
	}
	for _, e := range entries(bo.Uint32(t[4:])) {
		switch bo.Uint16(e) {
		case 0x0112: // Orientation, a SHORT held in the entry itself
			if o := int(bo.Uint16(e[8:])); o >= 1 && o <= 8 {
				m.Orientation = o
			}
		case 0x8825: // pointer to the GPS block
			if len(entries(bo.Uint32(e[8:]))) > 0 {
				m.GPS = true
			}
		}
	}
}
