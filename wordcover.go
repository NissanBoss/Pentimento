package main

// Shapes over pictures in Word and LibreOffice documents.
//
// A slide puts every shape at a place on the slide, and comparing two is
// arithmetic. A document puts things in the flow of the text: a picture
// can sit in a line like a letter, and a shape floats at an offset from
// the paragraph, the column, the line or the page it is anchored to. So
// two things are compared only when their places are measured from the
// same thing: both anchored in one paragraph, a picture in the line of
// that paragraph and a shape floating from it, or both from the page.
// When you draw a box over a picture in Word or Writer, the box is
// anchored to the paragraph the picture is in, which is the case that
// matters, and anything else is left alone rather than guessed at.

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"strconv"
	"strings"
)

type placed struct {
	box     rect
	frame   string // what the place is measured from
	z       int    // drawing order, higher in front
	picture bool
	filled  bool
	target  string // the picture's relationship id or file
}

// wordCovers reads document.xml paragraph by paragraph.
func wordCovers(body []byte) [][]placed {
	d := xml.NewDecoder(bytes.NewReader(body))
	var paras [][]placed
	var cur []placed
	var it *placed
	depth, inPara, inFallback, inLine := 0, 0, 0, 0
	inAnchor, inInline := false, false
	field := ""
	var hFrom, vFrom string
	var hOff, vOff, cx, cy float64
	order := 0
	for {
		t, err := d.Token()
		if err != nil {
			break
		}
		switch e := t.(type) {
		case xml.StartElement:
			depth++
			switch e.Name.Local {
			case "p":
				if inPara == 0 && e.Name.Space != "" && strings.Contains(e.Name.Space, "wordprocessingml") {
					inPara = depth
					cur = nil
				}
			case "Fallback":
				// The same drawing again, in VML, for older readers.
				if inFallback == 0 {
					inFallback = depth
				}
			case "anchor", "inline":
				if inFallback > 0 || inPara == 0 {
					continue
				}
				order++
				it = &placed{z: order}
				inAnchor, inInline = e.Name.Local == "anchor", e.Name.Local == "inline"
				hFrom, vFrom, hOff, vOff, cx, cy = "", "", 0, 0, 0, 0
				if inAnchor {
					if h, err := strconv.Atoi(attr(e, "relativeHeight")); err == nil {
						it.z = h
					}
					if attr(e, "behindDoc") == "1" {
						it.z = -1
					}
				} else {
					// In the text layer: behind anything floating in front of
					// the text.
					it.z = 0
				}
			case "positionH":
				hFrom = attr(e, "relativeFrom")
			case "positionV":
				vFrom = attr(e, "relativeFrom")
			case "posOffset":
				field = "off"
			case "extent":
				if it != nil && cx == 0 {
					cx, cy = num(attr(e, "cx")), num(attr(e, "cy"))
				}
			case "pic":
				if it != nil {
					it.picture = true
				}
			case "blip":
				if it != nil {
					it.target = attr(e, "embed")
				}
			case "ln":
				inLine = depth
			case "solidFill":
				if it != nil && !it.picture && inLine == 0 {
					it.filled = true
				}
			case "noFill":
				if it != nil && inLine == 0 {
					it.filled = false
				}
			}
		case xml.CharData:
			if field == "off" {
				v := num(strings.TrimSpace(string(e)))
				if vFrom == "" {
					hOff = v
				} else {
					vOff = v
				}
				field = ""
			}
		case xml.EndElement:
			switch e.Name.Local {
			case "anchor", "inline":
				if it != nil && (inAnchor || inInline) && inFallback == 0 {
					it.box = rect{hOff, vOff, hOff + cx, vOff + cy}
					switch {
					case inInline:
						// In the line of the paragraph: taken to start where
						// the paragraph does, which is where a picture on a
						// line of its own sits.
						it.frame = "line"
					case hFrom == "page" && vFrom == "page":
						it.frame = "page"
					case (hFrom == "column" || hFrom == "character") && (vFrom == "paragraph" || vFrom == "line"):
						it.frame = "line"
					}
					cur = append(cur, *it)
				}
				it, inAnchor, inInline = nil, false, false
			case "ln":
				inLine = 0
			}
			if depth == inFallback {
				inFallback = 0
			}
			if depth == inPara {
				inPara = 0
				if len(cur) > 1 {
					paras = append(paras, cur)
				}
				cur = nil
			}
			depth--
		}
	}
	return paras
}

// covered gives, for each picture covered in a paragraph, the share of it
// under shapes drawn in front of it.
func covered(group []placed) []struct {
	target string
	share  float64
} {
	var out []struct {
		target string
		share  float64
	}
	for _, p := range group {
		if !p.picture || p.frame == "" || p.box.area() <= 0 {
			continue
		}
		best := 0.0
		for _, s := range group {
			if s.picture || !s.filled || s.frame != p.frame || s.z <= p.z {
				continue
			}
			best = max(best, p.box.overlap(s.box)/p.box.area())
		}
		if best > 0.002 && best <= 0.95 {
			out = append(out, struct {
				target string
				share  float64
			}{p.target, best})
		}
	}
	return out
}

// writerCovers reads an OpenDocument text's content.xml the same way:
// frames and shapes anchored in one paragraph, or both to the page, with
// their places in svg:x and svg:y and their order in draw:z-index.
func writerCovers(body []byte, noFill map[string]bool) [][]placed {
	d := xml.NewDecoder(bytes.NewReader(body))
	var paras [][]placed
	var page []placed
	var cur []placed
	depth, inPara, order := 0, 0, 0
	var frame *placed
	frameDepth := 0
	for {
		t, err := d.Token()
		if err != nil {
			break
		}
		switch e := t.(type) {
		case xml.StartElement:
			depth++
			name := e.Name.Local
			switch {
			case name == "p" || name == "h":
				if inPara == 0 {
					inPara, cur = depth, nil
				}
			case name == "frame" || name == "rect" || name == "custom-shape" || name == "ellipse":
				order++
				p := placed{z: order}
				if z, err := strconv.Atoi(attr(e, "z-index")); err == nil {
					p.z = z
				}
				x, y := odfLength(attr(e, "x")), odfLength(attr(e, "y"))
				p.box = rect{x, y, x + odfLength(attr(e, "width")), y + odfLength(attr(e, "height"))}
				switch attr(e, "anchor-type") {
				case "page":
					p.frame = "page"
				case "paragraph", "char":
					p.frame = "line"
				}
				if name != "frame" {
					p.filled = !noFill[attr(e, "style-name")]
					appendPlaced(&page, &cur, p, inPara)
					continue
				}
				frame, frameDepth = &p, depth
			case name == "image" && frame != nil:
				frame.picture = true
				frame.target = attr(e, "href")
			}
		case xml.EndElement:
			if frame != nil && depth == frameDepth {
				if frame.picture {
					appendPlaced(&page, &cur, *frame, inPara)
				}
				frame = nil
			}
			if depth == inPara {
				inPara = 0
				if len(cur) > 1 {
					paras = append(paras, cur)
				}
				cur = nil
			}
			depth--
		}
	}
	if len(page) > 1 {
		paras = append(paras, page)
	}
	return paras
}

func appendPlaced(page, cur *[]placed, p placed, inPara int) {
	if p.frame == "page" {
		*page = append(*page, p)
	} else if inPara > 0 {
		*cur = append(*cur, p)
	}
}

// odfLength reads 2.5cm, 1in, 10mm or 12pt as points.
func odfLength(s string) float64 {
	units := map[string]float64{"cm": 72 / 2.54, "mm": 72 / 25.4, "in": 72, "pt": 1, "pc": 12}
	for u, f := range units {
		if strings.HasSuffix(s, u) {
			return num(strings.TrimSuffix(s, u)) * f
		}
	}
	return num(s)
}

func coverLine(share float64, target string) string {
	name := target
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	if name == "" {
		name = "a picture"
	} else {
		name = "the picture " + name
	}
	return fmt.Sprintf("In the document, %s has a filled shape over %s of it.", name, percent(share, 1))
}
