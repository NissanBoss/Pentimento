package main

import (
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"strings"
	"testing"
)

// A JPEG cropped by an editor that kept the old EXIF block: the thumbnail
// still shows the whole frame.
func TestAThumbnailFromBeforeTheCrop(t *testing.T) {
	scene := photo(1200, 900, 5)
	thumb := encodeJPEG(t, shrunk(scene, 160, 120), 80)
	main := encodeJPEG(t, cropped(scene, image.Rect(300, 200, 1000, 800)), 90)
	c := check(t, "cropped.jpg", withSegments(main, exifWithThumbnail(thumb)))
	c.only(t, "preview-differs")
	if !strings.Contains(c.hits["preview-differs"][0].what, "more than the picture") {
		t.Errorf("the crop was not described as one: %v", c.hits["preview-differs"])
	}
	// With the size the camera wrote, which the editor left as it was.
	c = check(t, "cropped.jpg", withSegments(main, exifWithThumbnail(thumb, image.Pt(1200, 900))))
	if more := c.hits["preview-differs"][0].more; len(more) == 0 || !strings.Contains(more[0], "made it 1200x900, and it is 700x600") {
		t.Errorf("the EXIF size was not said: %v", c.hits["preview-differs"])
	}
}

// A black box drawn over a name on a photo of a letter, and the thumbnail
// left as it was.
func TestAThumbnailFromBeforeTheBox(t *testing.T) {
	scene := photo(1200, 900, 6)
	letter := page(500, 300, 6)
	draw.Draw(scene, image.Rect(350, 250, 850, 550), letter, image.Point{}, draw.Src)
	thumb := encodeJPEG(t, shrunk(scene, 320, 240), 85)
	box := image.Rect(420, 300, 780, 420)
	main := encodeJPEG(t, boxed(scene, box, color.NRGBA{0, 0, 0, 255}, 1), 90)
	c := check(t, "boxed.jpg", withSegments(main, exifWithThumbnail(thumb)))
	c.only(t, "preview-differs")
	if c.has("preview-differs") && !strings.Contains(c.hits["preview-differs"][0].what, "one place") {
		t.Errorf("the box was not found as one place: %v", c.hits["preview-differs"])
	}
}

// The ordinary case, a camera's thumbnail of its own photo, says nothing.
// Most JPEGs anybody has are this, and a tool that flags them is a tool
// people stop running.
func TestAThumbnailThatMatches(t *testing.T) {
	scene := photo(1200, 900, 7)
	main := encodeJPEG(t, scene, 90)
	thumb := encodeJPEG(t, shrunk(scene, 160, 120), 70)
	check(t, "camera.jpg", withSegments(main, exifWithThumbnail(thumb))).only(t)
}

// A camera that letterboxes a wide photo into a 4:3 thumbnail puts black
// bars round it, and the bars are not a crop.
func TestALetterboxedThumbnail(t *testing.T) {
	scene := photo(1600, 900, 8)
	main := encodeJPEG(t, scene, 90)
	thumb := image.NewNRGBA(image.Rect(0, 0, 160, 120))
	small := shrunk(scene, 160, 90)
	for y := 0; y < 90; y++ {
		for x := 0; x < 160; x++ {
			thumb.Set(x, y+15, small.At(x, y))
		}
	}
	check(t, "wide.jpg", withSegments(main, exifWithThumbnail(encodeJPEG(t, thumb, 80)))).only(t)
}

// A Pixel portrait: the photo before the blur, a depth map and a motion
// video after the end, listed in the container.
func TestAPortraitWithItsOriginal(t *testing.T) {
	scene := photo(800, 600, 9)
	original := encodeJPEG(t, scene, 85)
	depth := encodeJPEG(t, shrunk(scene, 200, 150), 60)
	video := mp4(1.5, true)
	main := encodeJPEG(t, shrunk(scene, 800, 600), 90)
	xmp := containerXMP(
		[3]string{"image/jpeg", fmt.Sprint(len(original)), "android/original_image"},
		[3]string{"image/jpeg", fmt.Sprint(len(depth)), "android/depthmap"},
		[3]string{"video/mp4", fmt.Sprint(len(video)), "android/motionphoto"},
	)
	data := withSegments(main, xmpSegment(xmp))
	data = append(data, original...)
	data = append(data, depth...)
	data = append(data, video...)
	c := check(t, "PXL_PORTRAIT.jpg", data)
	c.only(t, "original-inside", "depth", "motion")
	if line := c.hits["motion"][0].what; !strings.Contains(line, "1.5-second video with sound") {
		t.Errorf("the video was described as %q", line)
	}
}

// Google's older Lens Blur kept the original in the XMP itself.
func TestLensBlurInTheXMP(t *testing.T) {
	scene := photo(400, 300, 10)
	original := base64.StdEncoding.EncodeToString(encodeJPEG(t, scene, 80))
	xmp := `<x:xmpmeta><rdf:RDF><rdf:Description GImage:Mime="image/jpeg" GImage:Data="` + original + `"/></rdf:RDF></x:xmpmeta>`
	check(t, "lensblur.jpg", withSegments(encodeJPEG(t, scene, 90), xmpSegment(xmp))).only(t, "original-inside")
}

// Samsung's trailer, with the country of the network and, after a
// screenshot, the app that was open.
func samsungTrailerBytes(entries ...[3]any) []byte {
	var blocks, dir []byte
	type placed struct {
		kind     uint16
		at, size int
	}
	var ps []placed
	for _, e := range entries {
		kind, name, body := e[0].(uint16), e[1].(string), e[2].(string)
		block := make([]byte, 8)
		binary.LittleEndian.PutUint16(block[2:], kind)
		binary.LittleEndian.PutUint32(block[4:], uint32(len(name)))
		block = append(append(block, name...), body...)
		ps = append(ps, placed{kind, len(blocks), len(block)})
		blocks = append(blocks, block...)
	}
	dir = append(dir, "SEFH"...)
	dir = binary.LittleEndian.AppendUint32(dir, 101)
	dir = binary.LittleEndian.AppendUint32(dir, uint32(len(ps)))
	for _, p := range ps {
		dir = binary.LittleEndian.AppendUint16(dir, 0)
		dir = binary.LittleEndian.AppendUint16(dir, p.kind)
		dir = binary.LittleEndian.AppendUint32(dir, uint32(len(blocks)-p.at))
		dir = binary.LittleEndian.AppendUint32(dir, uint32(p.size))
	}
	out := append(blocks, dir...)
	out = binary.LittleEndian.AppendUint32(out, uint32(len(dir)))
	return append(out, "SEFT"...)
}

func TestSamsungsTrailer(t *testing.T) {
	jpg := encodeJPEG(t, photo(400, 300, 11), 85)
	data := append(append([]byte{}, jpg...), samsungTrailerBytes(
		[3]any{uint16(0x0aa1), "MCC_Data", "214"},
		[3]any{uint16(0x0a01), "Image_UTC_Data", "1700000000000"},
	)...)
	c := check(t, "20231114_101010.jpg", data)
	c.only(t, "phone-trailer")
	if !strings.Contains(c.hits["phone-trailer"][0].what, "214, Spain") {
		t.Errorf("the country was given as %v", c.hits["phone-trailer"])
	}

	app := base64.StdEncoding.EncodeToString([]byte(`{"comp":"org.example.chat/.MainActivity"}`))
	shot := append(encodePNG(t, page(300, 200, 12)), samsungTrailerBytes(
		[3]any{uint16(0x0c51), "Samsung_Capture_Info", "Screenshot"},
		[3]any{uint16(0x0da1), "Captured_App_Info", app},
	)...)
	c = check(t, "Screenshot_20231114.png", shot)
	c.only(t, "phone-trailer")
	if line := c.hits["phone-trailer"][0].what; !strings.Contains(line, "o*g.e*****e.c**t") {
		t.Errorf("the app was not masked: %q", line)
	}
}

// The end of an earlier JPEG behind this one, as the Snipping Tool left it.
func TestTheEndOfAnEarlierJPEG(t *testing.T) {
	older := encodeJPEG(t, photo(900, 700, 13), 92)
	newer := encodeJPEG(t, photo(300, 200, 14), 80)
	check(t, "snip.jpg", overwritten(newer, older)).only(t, "jpeg-tail")
}

// A letter cropped with jpegtran, which keeps every marker: a strip of
// text, and a thumbnail of the whole page.
func TestAThumbnailOfALetterBeforeTheCrop(t *testing.T) {
	letter := page(900, 600, 50)
	thumb := encodeJPEG(t, shrunk(letter, 160, 107), 75)
	main := encodeJPEG(t, cropped(letter, image.Rect(0, 40, 600, 240)), 92)
	c := check(t, "letter.jpg", withSegments(main, exifWithThumbnail(thumb)))
	c.only(t, "preview-differs")
}
