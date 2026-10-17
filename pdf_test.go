package main

import (
	"fmt"
	"image"
	"image/color"
	"strings"
	"testing"
)

// onePagePDF writes a PDF of one page with one JPEG on it, drawn by the
// content given, with an index that points at every object.
func onePagePDF(t *testing.T, content string, annots string) []byte {
	t.Helper()
	jpg := encodeJPEG(t, photo(200, 100, 60), 85)
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 600 800] /Contents 4 0 R /Resources << /XObject << /Im1 5 0 R >> >>" + annots + " >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content),
		fmt.Sprintf("<< /Type /XObject /Subtype /Image /Width 200 /Height 100 /ColorSpace /DeviceRGB /BitsPerComponent 8 /Filter /DCTDecode /Length %d >>\nstream\n%s\nendstream", len(jpg), jpg),
	}
	var b strings.Builder
	b.WriteString("%PDF-1.7\n")
	offsets := make([]int, len(objects))
	for i, o := range objects {
		offsets[i] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, o)
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(objects)+1)
	for _, off := range offsets {
		fmt.Fprintf(&b, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, xref)
	return []byte(b.String())
}

func TestABoxOverAPictureInAPDF(t *testing.T) {
	drawn := "q 400 0 0 200 100 500 cm /Im1 Do Q 0 g 180 560 120 40 re f"
	c := check(t, "letter.pdf", onePagePDF(t, drawn, ""))
	c.only(t, "pdf-covered")
	// The same box drawn first is a background, and covers nothing.
	check(t, "letter.pdf", onePagePDF(t, "0 g 180 560 120 40 re f q 400 0 0 200 100 500 cm /Im1 Do Q", "")).only(t)
	// An outline is a frame.
	check(t, "letter.pdf", onePagePDF(t, "q 400 0 0 200 100 500 cm /Im1 Do Q 0 G 180 560 120 40 re S", "")).only(t)
}

func TestAClippedPictureInAPDF(t *testing.T) {
	drawn := "q 100 500 250 200 re W n 400 0 0 200 100 500 cm /Im1 Do Q"
	c := check(t, "slides.pdf", onePagePDF(t, drawn, ""))
	c.only(t, "pdf-crop")
	if line := c.hits["pdf-crop"][0].what; !strings.Contains(line, "38%") {
		t.Errorf("described as %q", line)
	}
}

func TestAnnotationsThatCoverAPDF(t *testing.T) {
	drawn := "q 400 0 0 200 100 500 cm /Im1 Do Q"
	redact := " /Annots [<< /Type /Annot /Subtype /Redact /Rect [150 550 300 600] >>]"
	check(t, "court.pdf", onePagePDF(t, drawn, redact)).only(t, "pdf-redact")
	square := " /Annots [<< /Type /Annot /Subtype /Square /Rect [150 550 300 600] /IC [0 0 0] >>]"
	check(t, "court.pdf", onePagePDF(t, drawn, square)).only(t, "pdf-annotation")
	// A comment box in an empty margin covers nothing.
	margin := " /Annots [<< /Type /Annot /Subtype /Square /Rect [10 10 60 40] /IC [1 1 0] >>]"
	check(t, "court.pdf", onePagePDF(t, drawn, margin)).only(t)
}

// The JPEG in a PDF is a file of its own, and is checked like one.
func TestAJPEGInsideAPDF(t *testing.T) {
	old := photo(900, 700, 61)
	jpg := overwritten(encodeJPEG(t, cropped(old, old.Rect.Inset(250)), 90), encodeJPEG(t, old, 90))
	doc := onePagePDF(t, "q 400 0 0 200 100 500 cm /Im1 Do Q", "")
	doc = []byte(strings.Replace(string(doc), string(encodeJPEG(t, photo(200, 100, 60), 85)), string(jpg), 1))
	// The index is wrong now, which the reader is expected to cope with.
	c := check(t, "scan.pdf", doc)
	c.only(t, "jpeg-tail")
	if where := c.hits["jpeg-tail"][0].where; !strings.Contains(where, "page 1") {
		t.Errorf("found at %q", where)
	}
}

// A redaction that was applied takes the pixels out and draws a box over
// the hole; that is a cover done right, and says nothing.
func TestAnAppliedRedactionOverAPicture(t *testing.T) {
	blanked := boxed(photo(200, 100, 60), image.Rect(34, 44, 106, 76), color.NRGBA{255, 255, 255, 255}, 1)
	doc := onePagePDF(t, "q 400 0 0 200 100 500 cm /Im1 Do Q 0 g 180 560 120 40 re f", "")
	doc = []byte(strings.Replace(string(doc), string(encodeJPEG(t, photo(200, 100, 60), 85)), string(encodeJPEG(t, blanked, 85)), 1))
	check(t, "redacted.pdf", doc).only(t)
}
