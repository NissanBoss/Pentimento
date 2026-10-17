package main

// Reading a PNG chunk by chunk, to find where it really ends and what is
// kept in its text chunks.

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"io"
	"unicode/utf8"
)

var pngMagic = []byte("\x89PNG\r\n\x1a\n")

type pngFile struct {
	width, height int
	depth, colour int
	interlaced    bool
	// end is the byte after the IEND chunk: where every viewer stops
	// reading, and where anything a careless save left behind begins.
	end   int
	texts []pngText
	// broken says why the chunks stopped making sense, if they did.
	broken string
}

type pngText struct {
	key, value string
}

func readPNG(data []byte) *pngFile {
	p := &pngFile{}
	i := len(pngMagic)
	for {
		if i+8 > len(data) {
			p.broken = "the file stops in the middle of a chunk"
			p.end = len(data)
			return p
		}
		n := int(binary.BigEndian.Uint32(data[i:]))
		kind := string(data[i+4 : i+8])
		if n < 0 || i+12+n > len(data) {
			p.broken = "a chunk claims more bytes than the file has"
			p.end = len(data)
			return p
		}
		body := data[i+8 : i+8+n]
		switch kind {
		case "IHDR":
			if n >= 13 {
				p.width = int(binary.BigEndian.Uint32(body))
				p.height = int(binary.BigEndian.Uint32(body[4:]))
				p.depth, p.colour = int(body[8]), int(body[9])
				p.interlaced = body[12] == 1
			}
		case "tEXt":
			if key, value, ok := bytes.Cut(body, []byte{0}); ok {
				p.texts = append(p.texts, pngText{string(key), latin1(value)})
			}
		case "zTXt":
			if key, rest, ok := bytes.Cut(body, []byte{0}); ok && len(rest) > 1 {
				if value, err := unzlib(rest[1:], 64<<20); err == nil {
					p.texts = append(p.texts, pngText{string(key), latin1(value)})
				}
			}
		case "iTXt":
			if key, value, ok := readITXt(body); ok {
				p.texts = append(p.texts, pngText{key, value})
			}
		}
		i += 12 + n
		if kind == "IEND" {
			p.end = i
			return p
		}
	}
}

// readITXt unpicks the international text chunk: a key, a compression
// flag and method, a language and a translated key, then the text.
func readITXt(body []byte) (string, string, bool) {
	key, rest, ok := bytes.Cut(body, []byte{0})
	if !ok || len(rest) < 2 {
		return "", "", false
	}
	compressed := rest[0] == 1
	rest = rest[2:]
	if _, rest, ok = bytes.Cut(rest, []byte{0}); !ok {
		return "", "", false
	}
	if _, rest, ok = bytes.Cut(rest, []byte{0}); !ok {
		return "", "", false
	}
	if compressed {
		out, err := unzlib(rest, 64<<20)
		if err != nil {
			return "", "", false
		}
		rest = out
	}
	return string(key), string(rest), true
}

// unzlib inflates with a ceiling, because the size inside a text chunk is
// whatever whoever wrote it says.
func unzlib(data []byte, limit int64) ([]byte, error) {
	z, err := zlib.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer z.Close()
	return io.ReadAll(io.LimitReader(z, limit))
}

// latin1 is what tEXt is written in by the standard. In practice it is
// often UTF-8, and a key full of non-ASCII bytes that already reads as
// UTF-8 is left alone.
func latin1(b []byte) string {
	ascii := true
	for _, c := range b {
		if c >= 0x80 {
			ascii = false
			break
		}
	}
	if ascii {
		return string(b)
	}
	s := string(b)
	if utf8.ValidString(s) {
		return s
	}
	r := make([]rune, len(b))
	for i, c := range b {
		r[i] = rune(c)
	}
	return string(r)
}

// bytesPerPixel for the colour types a screenshot is saved as. Below
// eight bits a pixel is a fraction of a byte, and the filter works on
// whole bytes, so those count as one.
func (p *pngFile) bytesPerPixel() int {
	channels := map[int]int{0: 1, 2: 3, 3: 1, 4: 2, 6: 4}[p.colour]
	if channels == 0 {
		return 0
	}
	return max(1, channels*p.depth/8)
}

func (p *pngFile) rowBytes(width int) int {
	channels := map[int]int{0: 1, 2: 3, 3: 1, 4: 2, 6: 4}[p.colour]
	return (width*channels*p.depth + 7) / 8
}

func (c *run) png(where string, data []byte, depth int) {
	p := readPNG(data)
	if p.broken != "" {
		c.gap(where, p.broken)
	}
	if _, _, ok := samsungDir(data, p.end); ok {
		// Samsung puts the same trailer after a screenshot as after a
		// photo, and in a screenshot it names the app that was open.
		c.phoneTrailer(where, samsungTrailer(data, p.end))
	} else if l := leftoverPNG(p, data); l != nil {
		c.leftover(where, l, p)
	}
	c.drawingInPNG(where, p, depth)
	c.pixels(where, data)
}

func (c *run) leftover(where string, l *leftover, p *pngFile) {
	if l.png == nil {
		c.hit("tail-other", where, fmt.Sprintf("%s after the end: %s", size(int64(l.size)), l.kind))
		return
	}
	o := l.png
	if o.sealed {
		c.hit("crop-sealed", where, fmt.Sprintf("%s of the compressed rows of an earlier picture, behind this one of %dx%d.",
			size(int64(o.streamBytes)), p.width, p.height),
			"They are the middle of one compressed block, and the table that decodes it was at the start, in the part the new picture overwrote.")
		return
	}
	if o.width == 0 {
		line := fmt.Sprintf("%s of an earlier picture's compressed rows after the end", size(int64(o.streamBytes)))
		if o.decoded > 0 {
			line += fmt.Sprintf(", decoding to %s", size(int64(o.decoded)))
		}
		c.hit("crop-tail", where, line, "The rows were not counted: "+o.why+".")
		return
	}
	line := fmt.Sprintf("The bottom %s rows of an earlier picture %s pixels wide, behind this one of %dx%d.",
		commas(o.rows), commas(o.width), p.width, p.height)
	var more string
	switch {
	case o.wholeRows == o.rows:
		more = "Every pixel of them comes back exactly."
	case o.wholeRows > 0:
		more = fmt.Sprintf("%s of the rows come back exactly, and %s of the pixels across all of them.",
			commas(o.wholeRows), percent(o.known, 1))
	default:
		more = fmt.Sprintf("%s of their pixels come back exactly; the rest copied parts of the picture that are gone.", percent(o.known, 1))
	}
	if !o.sure {
		more += " Other widths fit the rows almost as well, so the width is the likeliest one, not a certain one."
	}
	c.hit("crop-tail", where, line, more)
}
