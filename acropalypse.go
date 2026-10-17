package main

// The picture before the crop, still behind the end of the file.
//
// In 2023 Simon Aarons and David Buchanan found that the Pixel's Markup
// editor saved a cropped screenshot over the old file without making the
// file shorter (CVE-2023-21036), and then that Windows' Snipping Tool did
// the same (CVE-2023-28303). The new, smaller picture ends with its IEND
// chunk; every byte after it is the old picture's, the tail end of its
// compressed rows. Viewers stop at IEND and never show it.
//
// Saying "there are bytes after the end" is what every PNG checker does.
// What matters is whether those bytes are a picture, and how much of it:
// so this finds the old IDAT chunks in the leftovers, finds a point in the
// compressed stream where a block begins, decodes from there to the end,
// works out how wide the old picture was, and counts the rows that come
// back whole. It never writes them anywhere.

import (
	"bytes"
	"encoding/binary"
)

type leftover struct {
	// what the bytes after the end are, in words, when they are not the
	// old picture
	kind string
	size int
	// set when they are the tail of an earlier PNG
	png *oldPicture
}

type oldPicture struct {
	streamBytes int // compressed bytes recovered from the old chunks
	decoded     int // bytes they decode to, filter bytes included
	// geometry, when it could be worked out
	width     int
	rows      int
	wholeRows int     // rows in which every pixel came back
	known     float64 // the share of their bytes that came back exactly
	sure      bool
	// sealed is old data with no block start in it: the middle of one
	// compressed block whose code table went with the part overwritten.
	sealed bool
	// why the rows could not be counted, when they could not
	why string
}

// leftoverPNG looks at what follows IEND.
func leftoverPNG(p *pngFile, data []byte) *leftover {
	rest := data[p.end:]
	if len(rest) == 0 {
		return nil
	}
	l := &leftover{size: len(rest)}
	if allZero(rest) {
		// Some writers pad to a block. Nothing of anybody's is in zeros.
		if len(rest) <= 4096 {
			return nil
		}
		l.kind = "zeros"
		return l
	}
	if what := knownFile(rest); what != "" {
		l.kind = what
		return l
	}
	stream, ok := oldChunks(rest)
	if !ok {
		l.kind = "data that is not a picture this can name"
		return l
	}
	l.png = decodeOld(stream, p)
	return l
}

// oldChunks pulls the compressed data out of what is left of the old
// picture's IDAT chunks. The leftovers begin in the middle of one chunk,
// at whatever byte the new file ended, so the first whole chunk is found
// by walking forward from each candidate and asking whether the lengths
// lead, chunk by chunk, to an IEND at the very end.
func oldChunks(rest []byte) ([]byte, bool) {
	for at := 0; at+12 <= len(rest); at++ {
		at = indexFrom(rest, []byte("IDAT"), at+4) - 4
		if at < 0 {
			break
		}
		var datas [][]byte
		i := at
		for i+12 <= len(rest) {
			n := int(binary.BigEndian.Uint32(rest[i:]))
			kind := string(rest[i+4 : i+8])
			if n < 0 || i+12+n > len(rest) {
				break
			}
			if kind == "IEND" {
				// The tail of the chunk the leftovers begin in is data too,
				// up to its four bytes of CRC.
				var stream []byte
				if at >= 4 {
					stream = append(stream, rest[:at-4]...)
				}
				for _, d := range datas {
					stream = append(stream, d...)
				}
				return stream, len(stream) > 0
			}
			if kind != "IDAT" {
				break
			}
			datas = append(datas, rest[i+8:i+8+n])
			i += 12 + n
		}
	}
	// Windows 10's Snipping Tool writes the whole picture as one IDAT, so
	// the leftovers are the middle of that one chunk, its CRC and an IEND,
	// with no chunk header anywhere to walk from.
	iend := []byte("\x00\x00\x00\x00IEND\xae\x42\x60\x82")
	if len(rest) > 16+len(iend) && bytes.HasSuffix(rest, iend) {
		return rest[:len(rest)-len(iend)-4], true
	}
	return nil, false
}

func indexFrom(b, sep []byte, from int) int {
	if from >= len(b) {
		return -1
	}
	i := bytes.Index(b[from:], sep)
	if i < 0 {
		return -1
	}
	return from + i
}

// decodeOld finds where a block of the old stream begins and decodes from
// there. A stream started in the right place ends exactly where the data
// does, four bytes of checksum short: that is the test, and a false start
// almost never passes it.
func decodeOld(stream []byte, p *pngFile) *oldPicture {
	o := &oldPicture{streamBytes: len(stream)}
	// A block that zlib writes for a screenshot is a few kilobytes, so a
	// start turns up near the front. Past this much without one, the data
	// is not what it looks like.
	tries := min(len(stream)*8, 512<<10*8)
	buf := make([]int16, 0, 1<<20)
	limit := 256 << 20
	for bit := 0; bit < tries; bit++ {
		got, err := inflateFrom(stream, bit, limit, buf)
		if err != nil {
			continue
		}
		left := len(stream)*8 - got.end
		if left < 32 || left > 39 {
			continue
		}
		o.decoded = len(got.out)
		geometry(o, got.out, p)
		return o
	}
	o.sealed = true
	return o
}

// geometry works out how wide the old picture was and counts the rows
// that came back.
//
// The decoded bytes are the bottom of the old picture's rows, each one a
// filter byte followed by the pixels. The filter byte is 0 to 4. At the
// right width every row starts with one; at a wrong width the positions
// land in the pixels, and a pixel byte is soon more than 4.
//
// Flat parts of a screenshot fool that, because the filtered bytes of a
// flat row are all zero and a zero passes for a filter byte. So the width
// that wins is the one with the most rows that start with a filter other
// than zero. An encoder chooses those for rows with something in them,
// and a wrong width lands on a byte that small in a row with something in
// it almost never. Choosing the width by how well the rows line up was
// tried first, and on the real Snipping Tool files it picked 2,281 for a
// picture 1,366 wide: most of the bytes there are unknown, and the
// comparison was made on the few that were not.
func geometry(o *oldPicture, out []int16, p *pngFile) {
	if p.interlaced {
		o.why = "the picture is interlaced, and its rows are not in order"
		return
	}
	// The old picture is read with the new one's colour type first. An
	// editor can save the crop as RGB when the screenshot was RGBA, so the
	// two common ones are tried as well.
	layouts := []*pngFile{p}
	for _, colour := range []int{6, 2} {
		if p.colour != colour || p.depth != 8 {
			layouts = append(layouts, &pngFile{width: p.width, colour: colour, depth: 8})
		}
	}
	type candidate struct {
		layout       *pngFile
		width, valid int
	}
	var best, second candidate
	for _, l := range layouts {
		if l.bytesPerPixel() == 0 || l.depth < 8 {
			continue
		}
		// The old picture was at least as wide as the crop of it.
		for w := max(1, l.width); w <= 16384; w++ {
			stride := 1 + l.rowBytes(w)
			rows := len(out) / stride
			if rows < 4 {
				break
			}
			valid, bad := 0, false
			for k := 1; k <= min(rows, 512); k++ {
				f := out[len(out)-k*stride]
				if f == unknown {
					continue
				}
				if f > 4 {
					bad = true
					break
				}
				if f != 0 {
					valid++
				}
			}
			switch {
			case bad:
			case valid > best.valid:
				best, second = candidate{l, w, valid}, best
			case valid > second.valid:
				second = candidate{l, w, valid}
			}
		}
	}
	if best.valid < 4 {
		o.why = "no width makes the rows line up"
		return
	}
	o.width = best.width
	o.sure = second.valid*5 < best.valid*4
	o.rows, o.wholeRows, o.known = countRows(out, best.layout, best.width)
}

// unfiltered undoes the PNG filters on the rows that were decoded,
// leaving unknown wherever a byte depended on one.
func unfiltered(out []int16, p *pngFile, width int) ([][]int16, []int) {
	bpp := p.bytesPerPixel()
	stride := 1 + p.rowBytes(width)
	rows := len(out) / stride
	first := len(out) - rows*stride
	recon := make([][]int16, rows)
	filters := make([]int, rows)
	var prior []int16
	for r := 0; r < rows; r++ {
		line := out[first+r*stride : first+(r+1)*stride]
		f := int(line[0])
		filters[r] = f
		cur := make([]int16, stride-1)
		for i := range cur {
			x := line[i+1]
			if x == unknown || f == unknown {
				cur[i] = unknown
				continue
			}
			var a, b, c int16
			if i >= bpp {
				a = cur[i-bpp]
			}
			if prior != nil {
				b = prior[i]
				if i >= bpp {
					c = prior[i-bpp]
				}
			} else {
				b, c = unknown, unknown
			}
			cur[i] = unfilter(f, x, a, b, c)
		}
		recon[r] = cur
		prior = cur
	}
	return recon, filters
}

func unfilter(f int, x, a, b, c int16) int16 {
	switch f {
	case 0:
		return x
	case 1:
		if a == unknown {
			return unknown
		}
		return (x + a) & 0xff
	case 2:
		if b == unknown {
			return unknown
		}
		return (x + b) & 0xff
	case 3:
		if a == unknown || b == unknown {
			return unknown
		}
		return (x + (a+b)/2) & 0xff
	case 4:
		if a == unknown || b == unknown || c == unknown {
			return unknown
		}
		return (x + paeth(a, b, c)) & 0xff
	}
	return unknown
}

func paeth(a, b, c int16) int16 {
	p := a + b - c
	pa, pb, pc := abs16(p-a), abs16(p-b), abs16(p-c)
	switch {
	case pa <= pb && pa <= pc:
		return a
	case pb <= pc:
		return b
	}
	return c
}

func abs16(v int16) int16 {
	if v < 0 {
		return -v
	}
	return v
}

// countRows counts the rows that came back, the ones in which every byte
// did, and the share of all their bytes that did. A byte counted here is
// exact: anything that depended on what came before the start is left
// unknown rather than guessed.
func countRows(out []int16, p *pngFile, width int) (rows, whole int, known float64) {
	recon, _ := unfiltered(out, p, width)
	all, got := 0, 0
	for _, row := range recon {
		n := 0
		for _, v := range row {
			if v != unknown {
				n++
			}
		}
		if n == len(row) {
			whole++
		}
		all += len(row)
		got += n
	}
	if all > 0 {
		known = float64(got) / float64(all)
	}
	return len(recon), whole, known
}

func allZero(b []byte) bool {
	for _, c := range b {
		if c != 0 {
			return false
		}
	}
	return true
}

// knownFile names a whole file that was appended on purpose, which is a
// different story from a crop that left the old picture behind.
func knownFile(b []byte) string {
	switch {
	case bytes.HasPrefix(b, []byte("PK\x03\x04")):
		return "a zip archive"
	case bytes.HasPrefix(b, pngMagic):
		return "another PNG"
	case bytes.HasPrefix(b, []byte{0xff, 0xd8, 0xff}):
		return "a JPEG"
	case len(b) > 12 && string(b[4:8]) == "ftyp":
		return "a video"
	case bytes.HasPrefix(b, []byte("%PDF")):
		return "a PDF"
	case bytes.HasPrefix(b, []byte("7z\xbc\xaf\x27\x1c")):
		return "a 7z archive"
	case bytes.HasPrefix(b, []byte("Rar!")):
		return "a RAR archive"
	}
	return ""
}
