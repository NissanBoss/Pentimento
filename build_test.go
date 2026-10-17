package main

// The tests build the files they check, so what makes each case is in the
// test and not in a binary nobody can read. The real files, the Snipping
// Tool's and the phones', are in samples_test.go.

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"math/rand"
	"strings"
	"testing"
)

// page is something like a screenshot of text: dark marks of letter size
// in lines on a light background, different for every seed.
func page(w, h int, seed int64) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Bounds(), image.NewUniform(color.NRGBA{246, 246, 240, 255}), image.Point{}, draw.Src)
	r := rand.New(rand.NewSource(seed))
	for y := 10; y+12 < h; y += 22 {
		for x := 8; x+10 < w; {
			word := 2 + r.Intn(8)
			for k := 0; k < word && x+8 < w; k++ {
				// a letter: a few strokes in a 7 by 11 cell
				for s := 0; s < 3; s++ {
					x0, y0 := x+r.Intn(5), y+r.Intn(8)
					for t := 0; t < 4+r.Intn(5); t++ {
						px, py := x0+t*r.Intn(2), y0+t*(1-r.Intn(2))
						if px < w && py < h {
							img.SetNRGBA(px, py, color.NRGBA{30, 30, 40, 255})
						}
					}
				}
				x += 9
			}
			x += 9
		}
	}
	return img
}

// photo is something like a photograph: smooth shapes and some texture,
// which is what thumbnails and comparisons are tried on.
func photo(w, h int, seed int64) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	r := rand.New(rand.NewSource(seed))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			v := uint8(80 + 100*y/h + r.Intn(6))
			img.SetNRGBA(x, y, color.NRGBA{v, uint8(int(v) * 9 / 10), uint8(255 - int(v)/2), 255})
		}
	}
	for k := 0; k < 14; k++ {
		cx, cy, rad := r.Intn(w), r.Intn(h), 10+r.Intn(w/5)
		c := color.NRGBA{uint8(r.Intn(256)), uint8(r.Intn(256)), uint8(r.Intn(256)), 255}
		for y := max(0, cy-rad); y < min(h, cy+rad); y++ {
			for x := max(0, cx-rad); x < min(w, cx+rad); x++ {
				if (x-cx)*(x-cx)+(y-cy)*(y-cy) < rad*rad {
					img.SetNRGBA(x, y, c)
				}
			}
		}
	}
	return img
}

func encodePNG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func encodeJPEG(t *testing.T, img image.Image, quality int) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := jpeg.Encode(&b, img, &jpeg.Options{Quality: quality}); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func cropped(img *image.NRGBA, r image.Rectangle) *image.NRGBA {
	out := image.NewNRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	draw.Draw(out, out.Bounds(), img, r.Min, draw.Src)
	return out
}

// shrunk averages the pixels that fall in each new one, the way a camera
// or ImageMagick makes a thumbnail. Picking one pixel in each instead made
// thumbnails of text that no real thumbnail looks like.
func shrunk(img image.Image, w, h int) *image.NRGBA {
	b := img.Bounds()
	out := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			x0, x1 := b.Min.X+x*b.Dx()/w, b.Min.X+(x+1)*b.Dx()/w
			y0, y1 := b.Min.Y+y*b.Dy()/h, b.Min.Y+(y+1)*b.Dy()/h
			var sr, sg, sb, n uint32
			for yy := y0; yy < max(y1, y0+1); yy++ {
				for xx := x0; xx < max(x1, x0+1); xx++ {
					r, g, bl, _ := img.At(xx, yy).RGBA()
					sr, sg, sb, n = sr+r>>8, sg+g>>8, sb+bl>>8, n+1
				}
			}
			out.SetNRGBA(x, y, color.NRGBA{uint8(sr / n), uint8(sg / n), uint8(sb / n), 255})
		}
	}
	return out
}

func boxed(img *image.NRGBA, r image.Rectangle, c color.NRGBA, opacity float64) *image.NRGBA {
	out := cropped(img, img.Bounds())
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			o := out.NRGBAAt(x, y)
			mix := func(a, b uint8) uint8 { return uint8(float64(a)*(1-opacity) + float64(b)*opacity + 0.5) }
			out.SetNRGBA(x, y, color.NRGBA{mix(o.R, c.R), mix(o.G, c.G), mix(o.B, c.B), 255})
		}
	}
	return out
}

// withSegments puts APP segments straight after the SOI of a JPEG.
func withSegments(jpg []byte, segments ...[]byte) []byte {
	out := append([]byte{}, jpg[:2]...)
	for _, s := range segments {
		out = append(out, s...)
	}
	return append(out, jpg[2:]...)
}

func segment(marker byte, body []byte) []byte {
	s := []byte{0xff, marker, 0, 0}
	binary.BigEndian.PutUint16(s[2:], uint16(len(body)+2))
	return append(s, body...)
}

// exifWithThumbnail is an APP1 EXIF segment whose IFD1 holds a thumbnail.
func exifWithThumbnail(thumb []byte) []byte {
	var t bytes.Buffer
	le := binary.LittleEndian
	t.WriteString("II*\x00")
	binary.Write(&t, le, uint32(8))
	// IFD0, empty, pointing at IFD1
	binary.Write(&t, le, uint16(0))
	binary.Write(&t, le, uint32(14))
	// IFD1 at 14: two entries, then the thumbnail at 14+2+24+4 = 44
	binary.Write(&t, le, uint16(2))
	entry := func(tag uint16, v uint32) {
		binary.Write(&t, le, tag)
		binary.Write(&t, le, uint16(4))
		binary.Write(&t, le, uint32(1))
		binary.Write(&t, le, v)
	}
	entry(0x0201, 44)
	entry(0x0202, uint32(len(thumb)))
	binary.Write(&t, le, uint32(0))
	t.Write(thumb)
	return segment(0xe1, append([]byte("Exif\x00\x00"), t.Bytes()...))
}

func xmpSegment(x string) []byte {
	return segment(0xe1, append(append([]byte{}, xmpID...), x...))
}

// containerXMP lists files after the primary picture the way a Pixel
// does, as elements.
func containerXMP(items ...[3]string) string {
	var b strings.Builder
	b.WriteString(`<x:xmpmeta xmlns:x="adobe:ns:meta/"><rdf:RDF><rdf:Description><Container:Directory><rdf:Seq>`)
	b.WriteString(`<rdf:li><rdf:Description><rdf:value><rdf:Description><Item:Mime>image/jpeg</Item:Mime><Item:Length>0</Item:Length><Item:DataURI>primary_image</Item:DataURI></rdf:Description></rdf:value></rdf:Description></rdf:li>`)
	for _, it := range items {
		b.WriteString(`<rdf:li><rdf:Description><rdf:value><rdf:Description><Item:Mime>` + it[0] + `</Item:Mime><Item:Length>` + it[1] + `</Item:Length><Item:DataURI>` + it[2] + `</Item:DataURI></rdf:Description></rdf:value></rdf:Description></rdf:li>`)
	}
	b.WriteString(`</rdf:Seq></Container:Directory></rdf:Description></rdf:RDF></x:xmpmeta>`)
	return b.String()
}

// mp4 is the smallest MP4 readVideo describes: a movie header and two
// tracks, one of them sound.
func mp4(seconds float64, sound bool) []byte {
	b := func(kind string, body ...[]byte) []byte {
		var all []byte
		for _, x := range body {
			all = append(all, x...)
		}
		out := make([]byte, 8, 8+len(all))
		binary.BigEndian.PutUint32(out, uint32(8+len(all)))
		copy(out[4:], kind)
		return append(out, all...)
	}
	mvhd := make([]byte, 100)
	binary.BigEndian.PutUint32(mvhd[12:], 1000)
	binary.BigEndian.PutUint32(mvhd[16:], uint32(seconds*1000))
	hdlr := func(kind string) []byte {
		h := make([]byte, 24)
		copy(h[8:], kind)
		return b("hdlr", h)
	}
	moov := [][]byte{b("mvhd", mvhd), b("trak", b("mdia", hdlr("vide")))}
	if sound {
		moov = append(moov, b("trak", b("mdia", hdlr("soun"))))
	}
	return append(b("ftyp", []byte("mp42\x00\x00\x00\x00mp42isom")), b("moov", moov...)...)
}

// officeZip builds an Office file from its parts.
func officeZip(t *testing.T, parts map[string]string) []byte {
	t.Helper()
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	for name, body := range parts {
		w, err := z.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(body))
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// check runs everything on one file and gives back what was found, by
// kind.
func check(t *testing.T, name string, data []byte) *run {
	t.Helper()
	c := newRun(shower{})
	c.inspect(name, name, data, 0)
	return c
}

func (c *run) has(kind string) bool { return len(c.hits[kind]) > 0 }

func (c *run) only(t *testing.T, kinds ...string) {
	t.Helper()
	want := map[string]bool{}
	for _, k := range kinds {
		want[k] = true
		if !c.has(k) {
			t.Errorf("expected %s, found %v", k, c.found())
		}
	}
	for k := range c.hits {
		if !want[k] {
			t.Errorf("found %s, which should not be there: %v", k, c.hits[k])
		}
	}
}

func (c *run) found() []string {
	var out []string
	for k := range c.hits {
		out = append(out, k)
	}
	return out
}
