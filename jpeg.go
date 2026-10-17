package main

// Reading a JPEG marker by marker, for the copies of the picture it keeps
// and for where it really ends.

import (
	"bytes"
	"encoding/binary"
)

type jpegFile struct {
	width, height int
	// end is the byte after the end marker of the picture: where viewers
	// stop, and where phones put everything else.
	end    int
	exif   []byte   // the TIFF structure inside APP1, if any
	exifAt int      // where it starts in the file
	xmp    []string // the standard packet and the extended one, joined
	mpf    []byte   // the TIFF structure inside APP2 MPF, if any
	mpfAt  int
	broken string
}

var (
	exifID   = []byte("Exif\x00\x00")
	xmpID    = []byte("http://ns.adobe.com/xap/1.0/\x00")
	xmpExtID = []byte("http://ns.adobe.com/xmp/extension/\x00")
	mpfID    = []byte("MPF\x00")
)

func readJPEG(data []byte) *jpegFile {
	j := &jpegFile{}
	extended := map[string][]extPart{}
	i := 2
	for {
		// Markers can be padded with any number of 0xFF.
		for i < len(data) && data[i] == 0xff && i+1 < len(data) && data[i+1] == 0xff {
			i++
		}
		// The end marker is two bytes and is usually the last two in the
		// file, so it is looked for before asking for a segment's length.
		if i+2 <= len(data) && data[i] == 0xff && data[i+1] == 0xd9 {
			j.end = i + 2
			break
		}
		if i+4 > len(data) || data[i] != 0xff {
			j.broken = "the markers stop making sense before the picture ends"
			j.end = len(data)
			return j
		}
		m := data[i+1]
		if m >= 0xd0 && m <= 0xd7 || m == 0x01 {
			i += 2
			continue
		}
		n := int(binary.BigEndian.Uint16(data[i+2:]))
		if n < 2 || i+2+n > len(data) {
			j.broken = "a segment claims more bytes than the file has"
			j.end = len(data)
			return j
		}
		body := data[i+4 : i+2+n]
		switch {
		case m == 0xe1 && bytes.HasPrefix(body, exifID) && j.exif == nil:
			j.exif = body[len(exifID):]
			j.exifAt = i + 4 + len(exifID)
		case m == 0xe1 && bytes.HasPrefix(body, xmpID):
			j.xmp = append(j.xmp, string(body[len(xmpID):]))
		case m == 0xe1 && bytes.HasPrefix(body, xmpExtID):
			// GUID (32), full length (4), offset (4), then the part.
			rest := body[len(xmpExtID):]
			if len(rest) > 40 {
				guid := string(rest[:32])
				extended[guid] = append(extended[guid], extPart{int(binary.BigEndian.Uint32(rest[36:])), rest[40:]})
			}
		case m == 0xe2 && bytes.HasPrefix(body, mpfID) && j.mpf == nil:
			j.mpf = body[len(mpfID):]
			j.mpfAt = i + 4 + len(mpfID)
		case m >= 0xc0 && m <= 0xcf && m != 0xc4 && m != 0xc8 && m != 0xcc:
			if len(body) >= 5 {
				j.height = int(binary.BigEndian.Uint16(body[1:]))
				j.width = int(binary.BigEndian.Uint16(body[3:]))
			}
		}
		i += 2 + n
		if m == 0xda {
			// The scan. Its data runs until a marker that is not a stuffed
			// zero or a restart, and a progressive JPEG has several scans.
			k := i
			for k+1 < len(data) {
				if data[k] == 0xff {
					next := data[k+1]
					if next != 0 && !(next >= 0xd0 && next <= 0xd7) && next != 0xff {
						break
					}
				}
				k++
			}
			if k+1 >= len(data) {
				j.broken = "the picture stops before its end marker"
				j.end = len(data)
				return j
			}
			i = k
		}
	}
	for _, parts := range extended {
		j.xmp = append(j.xmp, joinExtended(parts))
	}
	return j
}

type extPart struct {
	offset int
	body   []byte
}

// joinExtended puts the extended XMP back together. Each part says where
// it goes, and phones write them in order but nothing requires it.
func joinExtended(parts []extPart) string {
	end := 0
	for _, p := range parts {
		end = max(end, p.offset+len(p.body))
	}
	if end > 256<<20 {
		return ""
	}
	whole := make([]byte, end)
	for _, p := range parts {
		copy(whole[p.offset:], p.body)
	}
	return string(whole)
}

// The thumbnail in the EXIF block: IFD1, with its offset and length
// counted from the start of the TIFF header.
func (j *jpegFile) thumbnail() []byte {
	t, err := parseTIFF(j.exif)
	if err != nil || t.ifd1 == nil {
		return nil
	}
	off, ok1 := t.ifd1.uint(0x0201)
	n, ok2 := t.ifd1.uint(0x0202)
	if !ok1 || !ok2 || n == 0 || int(off)+int(n) > len(j.exif) || int(off) < 0 {
		return nil
	}
	return j.exif[off : off+n]
}

// mpfEntry is one image in the Multi-Picture index. The first is the
// picture itself; the others follow it after its end, at offsets counted
// from the start of the MPF block's TIFF header.
type mpfEntry struct {
	kind   uint32
	at, n  int
	inFile []byte
}

func (j *jpegFile) mpfImages(data []byte) []mpfEntry {
	t, err := parseTIFF(j.mpf)
	if err != nil {
		return nil
	}
	e, ok := t.ifd0[0xb002]
	if !ok {
		return nil
	}
	raw := t.bytes(e)
	var out []mpfEntry
	for k := 0; k+16 <= len(raw); k += 16 {
		attr := t.order.Uint32(raw[k:])
		n := int(t.order.Uint32(raw[k+4:]))
		off := int(t.order.Uint32(raw[k+8:]))
		if off == 0 {
			continue // the first entry, the picture itself
		}
		at := j.mpfAt + off
		if at < 0 || n <= 0 || at+n > len(data) {
			continue
		}
		out = append(out, mpfEntry{kind: attr & 0xffffff, at: at, n: n, inFile: data[at : at+n]})
	}
	return out
}
