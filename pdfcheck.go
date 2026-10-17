package main

// Pictures in a PDF.
//
// The reader of objects and drawing instructions is Baggage's, from this
// same series, where it finds text under black boxes; that is left to it,
// and to x-ray. What is looked at here is the pictures: a box drawn over a
// picture, which leaves the picture whole underneath; a picture shown
// through a clip, which keeps all of it; an area marked for redaction and
// never redacted; a box drawn as an annotation, which lifts off; and every
// JPEG in the file, which is a JPEG like any other, thumbnail and all.

import (
	"fmt"
	"image"
	"strings"
)

func (c *run) pdf(where string, data []byte, depth int) {
	doc, err := Parse(data)
	if err != nil {
		c.gap(where, "could not be read as a PDF: "+err.Error())
		return
	}
	seen := map[Ref]bool{}
	unread := 0
	for i, pg := range doc.Pages() {
		n := i + 1
		content, err := doc.content(pg)
		if err != nil && len(content) == 0 {
			unread++
			continue
		}
		p := readPage(doc, content, doc.resources(pg))
		if len(p.Skipped) > 0 {
			unread++
		}
		c.pdfPictures(where, n, doc, p, seen, depth)
		c.pdfAnnotations(where, n, doc, pg, p)
	}
	if unread > 0 {
		c.gap(where, fmt.Sprintf("%s could not be followed all the way, so what is drawn there was not all checked", plural(unread, "page", "pages")))
	}
}

func (c *run) pdfPictures(where string, n int, doc *Document, p *pdfPage, seen map[Ref]bool, depth int) {
	for _, im := range p.Fills {
		if im.Kind != coverImage || im.Image == nil {
			continue
		}
		whole := area(im.Box)
		if whole < 1 {
			continue
		}
		shown := intersect(im.Box, im.Clip)
		name := pdfPictureName(im)
		if s := area(shown) / whole; s < 0.97 && s > 0 {
			c.hit("pdf-crop", where, fmt.Sprintf("On page %d, %s is shown with %s of it cut off by a clip.", n, name, percent(1-s, 1)))
		}
		visible := area(shown)
		var pixels image.Image
		looked := false
		for _, f := range p.Fills {
			if f.Seq <= im.Seq || visible < 1 {
				continue
			}
			over := area(intersect(f.Box, shown)) / visible
			// A box the size of the whole picture or more is a page or a
			// panel behind something else drawn later, as often as it is a
			// cover; a box over part of a picture is what hides a part.
			if over < 0.002 || over > 0.95 {
				continue
			}
			if !looked {
				pixels, looked = pdfImage(doc, im.Image), true
			}
			if pixels != nil && blankUnder(pixels, im.Box, intersect(f.Box, shown)) {
				continue
			}
			what := "a filled box"
			if f.Kind == coverImage {
				what = "another picture"
			}
			c.hit("pdf-covered", where, fmt.Sprintf("On page %d, %s is drawn over %s of %s.", n, what, percent(over, 1), name))
		}
		if seen[im.Ref] && im.Ref != (Ref{}) {
			continue
		}
		seen[im.Ref] = true
		// A DCTDecode stream is a JPEG file, header, EXIF and all, put in
		// the PDF as it was. Only a stream with that one filter is.
		if filters := filterList(im.Image.Dict["Filter"]); len(filters) == 1 && filters[0] == "DCTDecode" {
			c.inspect(fmt.Sprintf("%s > page %d, %s", where, n, name), "picture.jpg", im.Image.Raw, depth+1)
		}
	}
}

func pdfPictureName(im filled) string {
	w, _ := im.Image.Dict["Width"].(float64)
	h, _ := im.Image.Dict["Height"].(float64)
	if w > 0 && h > 0 {
		return fmt.Sprintf("a picture of %.0fx%.0f", w, h)
	}
	return "a picture"
}

// pdfAnnotations looks at what is laid on top of the page rather than
// drawn on it. An annotation is a separate object a reader draws over the
// page; deleting it, or switching annotations off, shows what is under.
func (c *run) pdfAnnotations(where string, n int, doc *Document, pg Dict, p *pdfPage) {
	annots, _ := doc.Resolve(pg["Annots"]).(Array)
	for _, a := range annots {
		d, ok := doc.Resolve(a).(Dict)
		if !ok {
			continue
		}
		sub, _ := doc.Resolve(d["Subtype"]).(Name)
		box, ok := annotRect(doc, d["Rect"])
		if !ok {
			continue
		}
		switch sub {
		case "Redact":
			// Marked, and never applied: applying is what takes the
			// content out, and until then the mark is only a mark.
			c.hit("pdf-redact", where, fmt.Sprintf("On page %d, an area of %.0fx%.0f points is marked for redaction, and the redaction was never applied.", n, box.width(), box.height()))
		case "Square", "Circle", "Polygon", "Ink", "FreeText", "Stamp":
			_, filledIn := d["IC"]
			if sub == "Ink" || sub == "Stamp" {
				filledIn = true // a scribble or a stamp is drawn over things
			}
			if !filledIn || !coversSomething(box, p) {
				continue
			}
			c.hit("pdf-annotation", where, fmt.Sprintf("On page %d, %s drawn as an annotation covers %.0fx%.0f points of what is on the page.", n, annotName(sub), box.width(), box.height()))
		}
	}
}

func annotName(sub Name) string {
	switch sub {
	case "Ink":
		return "a scribble"
	case "Stamp":
		return "a stamp"
	case "FreeText":
		return "a text box"
	}
	return "a filled " + strings.ToLower(string(sub))
}

func annotRect(doc *Document, v any) (pdfRect, bool) {
	arr, ok := doc.Resolve(v).(Array)
	if !ok || len(arr) != 4 {
		return pdfRect{}, false
	}
	var f [4]float64
	for i, x := range arr {
		f[i], ok = doc.Resolve(x).(float64)
		if !ok {
			return pdfRect{}, false
		}
	}
	return normalise(pdfRect{f[0], f[1], f[2], f[3]}), true
}

// coversSomething is an annotation over text or a picture on the page, as
// opposed to one in an empty margin.
func coversSomething(box pdfRect, p *pdfPage) bool {
	for _, f := range p.Fills {
		if f.Kind == coverImage && area(intersect(f.Box, box)) > 0 {
			return true
		}
	}
	for _, t := range p.Texts {
		if t.X >= box.X0 && t.X <= box.X1 && t.Y >= box.Y0 && t.Y <= box.Y1 {
			return true
		}
	}
	return false
}

// pdfImage decodes a picture in a PDF: a JPEG as it is, and Flate as rows
// of 8-bit grey or RGB. Anything else gives nothing, and the cover over it
// is reported without looking under it.
func pdfImage(doc *Document, s *Stream) image.Image {
	filters := filterList(doc.Resolve(s.Dict["Filter"]))
	if len(filters) == 1 && filters[0] == "DCTDecode" {
		img, err := decodePicture(s.Raw)
		if err != nil {
			return nil
		}
		return img
	}
	w, _ := doc.Resolve(s.Dict["Width"]).(float64)
	h, _ := doc.Resolve(s.Dict["Height"]).(float64)
	bpc, _ := doc.Resolve(s.Dict["BitsPerComponent"]).(float64)
	channels := map[Name]int{"DeviceGray": 1, "DeviceRGB": 3, "G": 1, "RGB": 3}[nameOf(doc.Resolve(s.Dict["ColorSpace"]))]
	if bpc != 8 || channels == 0 || w < 1 || h < 1 || w*h > 1<<26 {
		return nil
	}
	raw, err := doc.Decoded(s)
	if err != nil || len(raw) < int(w*h)*channels {
		return nil
	}
	img := image.NewGray(image.Rect(0, 0, int(w), int(h)))
	for i := range img.Pix {
		if channels == 1 {
			img.Pix[i] = raw[i]
		} else {
			p := raw[3*i:]
			img.Pix[i] = uint8((299*int(p[0]) + 587*int(p[1]) + 114*int(p[2])) / 1000)
		}
	}
	return img
}

func nameOf(v any) Name {
	n, _ := v.(Name)
	return n
}

// blankUnder says whether the part of a picture under a box has nothing
// in it: what a redaction that was applied leaves, the pixels taken out
// and the box drawn over the hole. That is a cover done right, and is not
// reported.
func blankUnder(img image.Image, picture, box pdfRect) bool {
	b := img.Bounds()
	// Page space to the picture's pixels; a picture's rows run downwards
	// and the page's upwards.
	fx := float64(b.Dx()) / picture.width()
	fy := float64(b.Dy()) / picture.height()
	x0 := int((box.X0 - picture.X0) * fx)
	x1 := int((box.X1 - picture.X0) * fx)
	y0 := int((picture.Y1 - box.Y1) * fy)
	y1 := int((picture.Y1 - box.Y0) * fy)
	r := image.Rect(x0, y0, x1, y1).Intersect(image.Rect(0, 0, b.Dx(), b.Dy())).Inset(1)
	if r.Empty() {
		return false
	}
	luma := lumaOf(img)
	var s, sq, n float64
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			v := luma(b.Min.X+x, b.Min.Y+y)
			s += v
			sq += v * v
			n++
		}
	}
	m := s / n
	return sq/n-m*m < 9
}
