package main

import (
	"bytes"
	"compress/flate"
	"image"
	"math/rand"
	"testing"
)

// The bug itself, done the way the Markup editor did it: the cropped
// picture written over the start of the old file, and the file left as
// long as it was.
func overwritten(newer, older []byte) []byte {
	out := append([]byte{}, older...)
	copy(out, newer)
	return out
}

func TestTheOldPictureBehindTheEnd(t *testing.T) {
	// A photo, not a page of text: a flat background repeats itself, and
	// the compressor writes each repeat as a copy of an earlier row, which
	// behind the end is a copy of a row that is gone. On a page of text
	// made here nothing at all came back, as on one of the real Windows 11
	// screenshots; that is said in the README.
	old := photo(640, 900, 1)
	oldPNG := encodePNG(t, old)
	crop := encodePNG(t, cropped(old, image.Rect(40, 60, 400, 260)))
	if len(crop) >= len(oldPNG) {
		t.Fatal("the crop came out bigger than the picture, so nothing would be left behind")
	}
	data := overwritten(crop, oldPNG)
	p := readPNG(data)
	l := leftoverPNG(p, data)
	if l == nil || l.png == nil {
		t.Fatalf("the old picture behind the end was not recognised: %+v", l)
	}
	o := l.png
	if o.width != 640 {
		t.Fatalf("width %d, the old picture was 640 wide: %+v colour %d", o.width, *o, p.colour)
	}
	// Go's encoder filters every row of a photo against the row above,
	// and the row above the first one left is gone, so here nothing comes
	// back exactly; the real files in samples_test.go, from the Pixel and
	// the Snipping Tool, bring rows back whole. What is checked here is the
	// width, and that nothing is ever claimed that is not so.
	if o.rows < 700 {
		t.Fatalf("%d rows found behind the end, and about 780 are there", o.rows)
	}
	// Every byte said to have come back has to be the old picture's own.
	// A recovery that guesses is worse than none.
	stream, _ := oldChunks(data[p.end:])
	var got *inflated
	for bit := 0; bit < len(stream)*8; bit++ {
		if g, err := inflateFrom(stream, bit, 64<<20, nil); err == nil && len(stream)*8-g.end >= 32 && len(stream)*8-g.end <= 39 {
			got = g
			break
		}
	}
	recon, _ := unfiltered(got.out, p, 640)
	bottom := len(recon)
	for r, row := range recon {
		y := old.Rect.Dy() - bottom + r
		truth := old.Pix[y*old.Stride : y*old.Stride+len(row)]
		for i, v := range row {
			if v != unknown && byte(v) != truth[(i/3)*4+i%3] {
				t.Fatalf("row %d byte %d came back as %d, and was %d", y, i, v, truth[(i/3)*4+i%3])
			}
		}
	}
	check(t, "shot.png", data).only(t, "crop-tail")
}

// The same picture saved properly, as a new file, has nothing behind it.
func TestACropSavedProperlyIsClean(t *testing.T) {
	crop := encodePNG(t, cropped(page(640, 900, 2), image.Rect(0, 0, 300, 200)))
	check(t, "shot.png", crop).only(t)
}

// Zeros after the end are padding some writers add, and a zip appended on
// purpose is a different thing from a crop gone wrong.
func TestWhatElseCanFollowTheEnd(t *testing.T) {
	clean := encodePNG(t, page(200, 100, 3))
	check(t, "pad.png", append(append([]byte{}, clean...), make([]byte, 512)...)).only(t)
	zipped := officeZip(t, map[string]string{"a.txt": "hello"})
	check(t, "zip.png", append(append([]byte{}, clean...), zipped...)).only(t, "tail-other")
}

// Starting in the middle of a DEFLATE stream at a block boundary must give
// exactly the end of what the whole stream gives.
func TestInflatingFromTheMiddle(t *testing.T) {
	r := rand.New(rand.NewSource(4))
	var plain bytes.Buffer
	for plain.Len() < 600000 {
		word := []byte("the quick brown fox jumps over ")
		if r.Intn(3) == 0 {
			word = []byte{byte(r.Intn(256)), byte(r.Intn(256))}
		}
		plain.Write(word)
	}
	var packed bytes.Buffer
	w, _ := flate.NewWriter(&packed, 6)
	w.Write(plain.Bytes())
	w.Close()
	whole, err := inflateFrom(packed.Bytes(), 0, 64<<20, nil)
	if err != nil || !bytes.Equal(int16sToBytes(whole.out), plain.Bytes()) {
		t.Fatalf("decoding from the start does not give the input back: %v", err)
	}
	// Start from the second block, wherever it is.
	half := packed.Bytes()[packed.Len()/2:]
	for bit := 0; bit < len(half)*8; bit++ {
		g, err := inflateFrom(half, bit, 64<<20, nil)
		if err != nil || len(half)*8-g.end > 7 {
			continue
		}
		tail := plain.Bytes()[plain.Len()-len(g.out):]
		for i, v := range g.out {
			if v != unknown && byte(v) != tail[i] {
				t.Fatalf("byte %d of the tail came out wrong", i)
			}
		}
		return
	}
	t.Fatal("no block start found in the second half of the stream")
}

func int16sToBytes(v []int16) []byte {
	out := make([]byte, len(v))
	for i, x := range v {
		out[i] = byte(x)
	}
	return out
}

func TestTheEndOfAnEarlierGIF(t *testing.T) {
	gif := []byte("GIF89a\x02\x00\x02\x00\x00\x00\x00,\x00\x00\x00\x00\x02\x00\x02\x00\x00\x02\x02D\x01\x00;")
	tail := append(bytes.Repeat([]byte{0x41, 0x7f, 0x13}, 40), 0x00, 0x3b)
	check(t, "anim.gif", append(append([]byte{}, gif...), tail...)).only(t, "gif-tail")
	check(t, "anim.gif", gif).only(t)
}
