package main

import (
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"strings"
	"testing"
)

// pixelated averages blocks of a part of the picture, as GIMP's Pixelize
// and every other mosaic filter does.
func pixelated(img *image.NRGBA, r image.Rectangle, block int) *image.NRGBA {
	out := cropped(img, img.Bounds())
	for by := r.Min.Y; by < r.Max.Y; by += block {
		for bx := r.Min.X; bx < r.Max.X; bx += block {
			var sr, sg, sb, n int
			for y := by; y < min(by+block, r.Max.Y); y++ {
				for x := bx; x < min(bx+block, r.Max.X); x++ {
					c := img.NRGBAAt(x, y)
					sr, sg, sb, n = sr+int(c.R), sg+int(c.G), sb+int(c.B), n+1
				}
			}
			avg := color.NRGBA{uint8(sr / n), uint8(sg / n), uint8(sb / n), 255}
			for y := by; y < min(by+block, r.Max.Y); y++ {
				for x := bx; x < min(bx+block, r.Max.X); x++ {
					out.SetNRGBA(x, y, avg)
				}
			}
		}
	}
	return out
}

func TestAMosaicOverText(t *testing.T) {
	shot := pixelated(page(600, 300, 40), image.Rect(80, 60, 400, 120), 8)
	c := check(t, "shot.png", encodePNG(t, shot))
	c.only(t, "mosaic")
	if line := c.hits["mosaic"][0].what; !strings.Contains(line, "blocks of 8") {
		t.Errorf("described as %q", line)
	}
}

// Things made of equal squares that are not mosaics: a QR code is two
// colours, and so is a dashed line; a single row of swatches is a tab bar
// as often as anything else.
func TestSquaresThatAreNotAMosaic(t *testing.T) {
	img := page(400, 300, 41)
	for by := 0; by < 21; by++ {
		for bx := 0; bx < 21; bx++ {
			c := color.NRGBA{255, 255, 255, 255}
			if (bx*7+by*13)%3 == 0 {
				c = color.NRGBA{0, 0, 0, 255}
			}
			for y := 0; y < 6; y++ {
				for x := 0; x < 6; x++ {
					img.SetNRGBA(250+bx*6+x, 40+by*6+y, c)
				}
			}
		}
	}
	for k := 0; k < 12; k++ {
		for y := 200; y < 216; y++ {
			for x := 0; x < 16; x++ {
				img.SetNRGBA(10+k*16+x, y, color.NRGBA{uint8(k * 20), uint8(255 - k*20), 128, 255})
			}
		}
	}
	check(t, "ui.png", encodePNG(t, img)).only(t)
}

func TestABoxThatLetsTheTextThrough(t *testing.T) {
	text := page(600, 300, 42)
	box := image.Rect(60, 50, 360, 140)
	c := check(t, "shot.png", encodePNG(t, boxed(text, box, color.NRGBA{0, 0, 0, 255}, 0.92)))
	c.only(t, "shows-through")
	if line := c.hits["shows-through"][0].what; !strings.Contains(line, "300x90 at 60,50") {
		t.Errorf("described as %q", line)
	}
	// The same box, solid.
	check(t, "shot.png", encodePNG(t, boxed(text, box, color.NRGBA{0, 0, 0, 255}, 1))).only(t)
	// An outline with the same colour inside and out is a frame, not a
	// cover, which is what Android's Recents button looked like.
	framed := cropped(text, text.Bounds())
	for y := 40; y < 150; y++ {
		for x := 50; x < 370; x++ {
			if x < 58 || x >= 362 || y < 48 || y >= 142 {
				framed.SetNRGBA(x, y, color.NRGBA{20, 20, 20, 255})
			}
		}
	}
	check(t, "frame.png", encodePNG(t, framed)).only(t)
}

// A HEIC portrait: the photo, its depth map and a matte of the people in
// it, as Apple names them. The boxes are put together here because no
// public portrait HEIC was found to test against; the README says so.
func TestAPortraitHEIC(t *testing.T) {
	bx := func(kind string, body ...[]byte) []byte {
		var all []byte
		for _, x := range body {
			all = append(all, x...)
		}
		out := binary.BigEndian.AppendUint32(nil, uint32(8+len(all)))
		return append(append(out, kind...), all...)
	}
	full := func(v byte) []byte { return []byte{v, 0, 0, 0} }
	infe := func(id uint16, kind string) []byte {
		b := append(full(2), byte(id>>8), byte(id), 0, 0)
		return bx("infe", append(append(b, kind...), 0))
	}
	u16 := func(v uint16) []byte { return binary.BigEndian.AppendUint16(nil, v) }
	auxC := func(urn string) []byte { return bx("auxC", append(full(0), append([]byte(urn), 0)...)) }
	ispe := func(w, h uint32) []byte {
		b := append(full(0), binary.BigEndian.AppendUint32(nil, w)...)
		return bx("ispe", append(b, binary.BigEndian.AppendUint32(nil, h)...))
	}
	iinf := bx("iinf", full(0), u16(3), infe(1, "hvc1"), infe(2, "hvc1"), infe(3, "hvc1"))
	iref := bx("iref", full(0), bx("auxl", u16(2), u16(1), u16(1)), bx("auxl", u16(3), u16(1), u16(1)))
	ipco := bx("ipco", ispe(4032, 3024), ispe(768, 576), auxC("urn:com:apple:photo:2018:aux:portraiteffectsmatte"), auxC("urn:mpeg:hevc:2015:auxid:2"))
	ipma := bx("ipma", full(0), binary.BigEndian.AppendUint32(nil, 3),
		u16(1), []byte{1, 1},
		u16(2), []byte{2, 2, 3},
		u16(3), []byte{2, 2, 4})
	meta := bx("meta", full(0), bx("pitm", full(0), u16(1)), iinf, iref, bx("iprp", ipco, ipma))
	heic := append(bx("ftyp", []byte("heic\x00\x00\x00\x00mif1heic")), meta...)
	c := check(t, "IMG_0001.HEIC", heic)
	c.only(t, "depth")
	h := c.hits["depth"][0]
	all := strings.Join(append([]string{h.what}, h.more...), " ")
	if !strings.Contains(all, "outline of the people") || !strings.Contains(all, "depth map, 768x576") {
		t.Errorf("described as %q", all)
	}
}

// The same, saved as a JPEG afterwards, as a screenshot pasted into a chat
// often is: with blocks on JPEG's own grid of eight and off it.
func TestAMosaicSavedAsJPEG(t *testing.T) {
	for _, block := range []int{8, 12} {
		shot := pixelated(page(600, 300, 43), image.Rect(80, 64, 400, 128), block)
		t.Run(fmt.Sprint(block), func(t *testing.T) {
			check(t, "shot.jpg", encodeJPEG(t, shot, 92)).only(t, "mosaic")
		})
	}
}
