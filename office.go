package main

// Pictures inside Word, PowerPoint, Excel and LibreOffice files.
//
// An office document is a zip of XML parts, and a picture in it is a file
// in the zip that the XML points at. Everything done to the picture in the
// application is said in the XML and not done to the file: a crop is four
// numbers, a box over it is another shape, and an artistic effect such as
// Blur or Remove Background keeps the picture as it was beside the one
// with the effect, so the effect can be changed later.

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"path"
	"sort"
	"strconv"
	"strings"
)

type officeFile struct {
	z     *zip.Reader
	parts map[string]*zip.File
}

func (o *officeFile) read(name string) []byte {
	f := o.parts[name]
	if f == nil || f.UncompressedSize64 > 512<<20 {
		return nil
	}
	r, err := f.Open()
	if err != nil {
		return nil
	}
	defer r.Close()
	b, _ := io.ReadAll(io.LimitReader(r, 512<<20))
	return b
}

// rels maps the relationship ids of one part to the parts they point at.
func (o *officeFile) rels(part string) map[string]string {
	dir, file := path.Split(part)
	body := o.read(dir + "_rels/" + file + ".rels")
	var r struct {
		Rels []struct {
			ID     string `xml:"Id,attr"`
			Target string `xml:"Target,attr"`
			Mode   string `xml:"TargetMode,attr"`
		} `xml:"Relationship"`
	}
	xml.Unmarshal(body, &r)
	out := map[string]string{}
	for _, x := range r.Rels {
		if x.Mode == "External" {
			continue
		}
		t := x.Target
		if strings.HasPrefix(t, "/") {
			t = strings.TrimPrefix(t, "/")
		} else {
			t = path.Clean(dir + t)
		}
		out[x.ID] = t
	}
	return out
}

func (c *run) office(where string, data []byte, depth int) {
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		c.gap(where, "is not a zip this could open")
		return
	}
	o := &officeFile{z: z, parts: map[string]*zip.File{}}
	for _, f := range z.File {
		o.parts[f.Name] = f
	}
	if o.parts["content.xml"] != nil {
		c.openDocument(where, o, depth)
		return
	}
	// Slide layouts and masters are left out. They come with the template,
	// and a template's background photo under its decorative bars, or its
	// logo cropped to fit, came out as four boxes over a picture in the
	// first real deck tried; nobody hid anything there. Their pictures are
	// still opened below, with every other picture in the file.
	var parts []string
	for name := range o.parts {
		switch {
		case name == "word/document.xml",
			strings.HasPrefix(name, "word/header") && strings.HasSuffix(name, ".xml"),
			strings.HasPrefix(name, "word/footer") && strings.HasSuffix(name, ".xml"),
			strings.HasPrefix(name, "ppt/slides/slide") && strings.HasSuffix(name, ".xml"),
			strings.HasPrefix(name, "xl/drawings/drawing") && strings.HasSuffix(name, ".xml"):
			parts = append(parts, name)
		}
	}
	sort.Slice(parts, func(i, j int) bool { return naturalLess(parts[i], parts[j]) })
	looked := map[string]bool{}
	for _, part := range parts {
		body := o.read(part)
		rels := o.rels(part)
		place := partName(part)
		for _, p := range drawingPictures(body) {
			target := rels[p.embed]
			if target == "" {
				continue
			}
			media := o.read(target)
			if p.hidden() > 0.01 {
				line := fmt.Sprintf("%s %s is shown with %s of it cropped away", place, mediaName(target, media), percent(p.hidden(), 1))
				c.hit("office-crop", where, line+".")
			}
			if p.original != "" {
				if orig := rels[p.original]; orig != "" {
					c.hit("office-effect", where, fmt.Sprintf("%s %s has Office's %s on it, and the picture before it is in the file as %s.",
						place, mediaName(target, media), p.effect, mediaName(orig, o.read(orig))))
				}
			}
			for _, cover := range p.coveredBy {
				c.hit("office-covered", where, fmt.Sprintf("%s %s has a filled shape over %s of it.", place, mediaName(target, media), percent(cover, 1)))
			}
			if !looked[target] && media != nil {
				looked[target] = true
				c.inspect(where+" > "+target, path.Base(target), media, depth+1)
			}
		}
	}
	// Pictures in the file that no part shows are still in it. A picture
	// deleted from a slide is usually gone from the zip on save, but not
	// always, and a picture that is in the zip goes wherever the file goes.
	for name, f := range o.parts {
		if looked[name] || !isMedia(name) {
			continue
		}
		if f.UncompressedSize64 > 0 {
			looked[name] = true
			c.inspect(where+" > "+name, path.Base(name), o.read(name), depth+1)
		}
	}
}

func isMedia(name string) bool {
	switch strings.ToLower(path.Ext(name)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".svg":
		return strings.Contains(name, "media/") || strings.HasPrefix(name, "Pictures/")
	}
	return false
}

func partName(part string) string {
	base := strings.TrimSuffix(path.Base(part), ".xml")
	switch {
	case strings.HasPrefix(part, "ppt/slides/slide"):
		return "On slide " + strings.TrimPrefix(base, "slide") + ", the picture"
	case strings.HasPrefix(part, "xl/drawings/"):
		return "In the drawing layer of a sheet, the picture"
	case strings.HasPrefix(base, "header"):
		return "In a header, the picture"
	case strings.HasPrefix(base, "footer"):
		return "In a footer, the picture"
	}
	return "In the document, the picture"
}

func mediaName(target string, media []byte) string {
	s := path.Base(target)
	if w, h, ok := pictureSize(media); ok {
		s += fmt.Sprintf(" (%dx%d)", w, h)
	}
	return s
}

// naturalLess sorts slide2 before slide10.
func naturalLess(a, b string) bool {
	na, nb := trailingNumber(a), trailingNumber(b)
	if na >= 0 && nb >= 0 && strings.TrimRight(strings.TrimSuffix(a, ".xml"), "0123456789") == strings.TrimRight(strings.TrimSuffix(b, ".xml"), "0123456789") {
		return na < nb
	}
	return a < b
}

func trailingNumber(s string) int {
	s = strings.TrimSuffix(s, ".xml")
	i := len(s)
	for i > 0 && s[i-1] >= '0' && s[i-1] <= '9' {
		i--
	}
	n, err := strconv.Atoi(s[i:])
	if err != nil {
		return -1
	}
	return n
}

// officePicture is one picture placed in a part.
type officePicture struct {
	embed                    string
	left, top, right, bottom float64 // the crop, as shares of the picture
	original                 string  // the picture kept under an artistic effect
	effect                   string
	box                      rect
	coveredBy                []float64
}

func (p officePicture) hidden() float64 {
	pos := func(v float64) float64 { return max(0, v) }
	w := 1 - pos(p.left) - pos(p.right)
	h := 1 - pos(p.top) - pos(p.bottom)
	if w <= 0 || h <= 0 {
		return 1
	}
	return 1 - w*h
}

type rect struct{ x0, y0, x1, y1 float64 }

func (r rect) area() float64 { return max(0, r.x1-r.x0) * max(0, r.y1-r.y0) }

func (r rect) overlap(s rect) float64 {
	return rect{max(r.x0, s.x0), max(r.y0, s.y0), min(r.x1, s.x1), min(r.y1, s.y1)}.area()
}

// drawingPictures walks the XML of a part with a token reader rather than
// into structs: pictures sit at different depths in Word, PowerPoint and
// Excel, and what matters is the order, because a shape that comes after
// a picture is drawn on top of it.
func drawingPictures(body []byte) []officePicture {
	d := xml.NewDecoder(bytes.NewReader(body))
	var pics []officePicture
	type shape struct {
		box    rect
		filled bool
		text   bool
	}
	var shapes []struct {
		shape
		after int // how many pictures came before it
	}
	var cur *officePicture
	var sp *shape
	depthPic, depthSp, depthLayer := 0, 0, 0
	// A solid fill inside a line is the colour of the outline, which
	// covers nothing.
	inFill, inLine := false, false
	level := 0
	for {
		t, err := d.Token()
		if err != nil {
			break
		}
		switch e := t.(type) {
		case xml.StartElement:
			level++
			name := e.Name.Local
			switch {
			case name == "pic" && cur == nil:
				pics = append(pics, officePicture{})
				cur = &pics[len(pics)-1]
				depthPic = level
			case name == "sp" && cur == nil && sp == nil:
				sp = &shape{}
				depthSp = level
			case name == "blip" && cur != nil && cur.embed == "":
				cur.embed = attr(e, "embed")
			case name == "imgLayer" && cur != nil:
				cur.original = attr(e, "embed")
				depthLayer = level
			case cur != nil && depthLayer > 0 && level == depthLayer+2:
				cur.effect = effectName(name)
			case name == "srcRect" && cur != nil:
				cur.left = permille(attr(e, "l"))
				cur.top = permille(attr(e, "t"))
				cur.right = permille(attr(e, "r"))
				cur.bottom = permille(attr(e, "b"))
			case name == "off" || name == "ext":
				var r *rect
				switch {
				case cur != nil:
					r = &cur.box
				case sp != nil:
					r = &sp.box
				}
				if r != nil {
					if name == "off" {
						r.x0, r.y0 = num(attr(e, "x")), num(attr(e, "y"))
					} else {
						r.x1, r.y1 = r.x0+num(attr(e, "cx")), r.y0+num(attr(e, "cy"))
					}
				}
			case name == "ln" && sp != nil:
				inLine = true
			case name == "solidFill" && sp != nil && !inLine:
				inFill = true
				sp.filled = true
			case name == "alpha" && sp != nil && inFill:
				if v := num(attr(e, "val")); v < 60000 {
					sp.filled = false // see-through enough not to be a cover
				}
			case name == "noFill" && sp != nil:
				sp.filled = false
			case name == "t" && sp != nil:
				sp.text = true
			}
		case xml.EndElement:
			name := e.Name.Local
			switch {
			case name == "pic" && cur != nil && level == depthPic:
				cur = nil
			case name == "sp" && sp != nil && level == depthSp:
				shapes = append(shapes, struct {
					shape
					after int
				}{*sp, len(pics)})
				sp = nil
			case name == "imgLayer":
				depthLayer = 0
			case name == "solidFill":
				inFill = false
			case name == "ln":
				inLine = false
			}
			level--
		}
	}
	for i := range pics {
		p := &pics[i]
		if p.box.area() <= 0 {
			continue
		}
		for _, s := range shapes {
			// Only a shape drawn after the picture is on top of it, and a
			// shape with words in it is a caption or a callout, not a box
			// put there to hide something.
			if s.after <= i || !s.filled || s.text {
				continue
			}
			if share := p.box.overlap(s.box) / p.box.area(); share > 0.002 {
				p.coveredBy = append(p.coveredBy, share)
			}
		}
	}
	return pics
}

func attr(e xml.StartElement, local string) string {
	for _, a := range e.Attr {
		if a.Name.Local == local {
			return a.Value
		}
	}
	return ""
}

func num(s string) float64 {
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

// permille reads a crop edge, written in thousandths of a per cent.
func permille(s string) float64 { return num(s) / 100000 }

func effectName(local string) string {
	names := map[string]string{
		"backgroundRemoval":         "Remove Background",
		"artisticBlur":              "Blur effect",
		"artisticPixelate":          "Mosaic Bubbles effect",
		"artisticMarker":            "Marker effect",
		"artisticPencilSketch":      "Pencil Sketch effect",
		"artisticGlowEdges":         "Glow Edges effect",
		"artisticPaintStrokes":      "Paint Strokes effect",
		"artisticPlasticWrap":       "Plastic Wrap effect",
		"artisticWatercolorSponge":  "Watercolor Sponge effect",
		"artisticCutout":            "Cutout effect",
		"artisticFilmGrain":         "Film Grain effect",
		"artisticGlass":             "Glass effect",
		"artisticLightScreen":       "Light Screen effect",
		"artisticMosiaicBubbles":    "Mosaic Bubbles effect",
		"artisticPastelsSmooth":     "Pastels Smooth effect",
		"artisticPhotocopy":         "Photocopy effect",
		"artisticTexturizer":        "Texturizer effect",
		"artisticCement":            "Cement effect",
		"artisticChalkSketch":       "Chalk Sketch effect",
		"artisticCrisscrossEtching": "Crisscross Etching effect",
		"artisticLineDrawing":       "Line Drawing effect",
		"artisticPaintBrush":        "Paint Brush effect",
		"artisticPencilGrayscale":   "Pencil Grayscale effect",
	}
	if n := names[local]; n != "" {
		return n
	}
	return "picture effect " + local
}

// openDocument reads the LibreOffice formats, where a crop is a clip in
// the frame's graphic style.
func (c *run) openDocument(where string, o *officeFile, depth int) {
	clipped := map[string]bool{}
	for _, part := range []string{"styles.xml", "content.xml"} {
		d := xml.NewDecoder(bytes.NewReader(o.read(part)))
		style := ""
		for {
			t, err := d.Token()
			if err != nil {
				break
			}
			if e, ok := t.(xml.StartElement); ok {
				switch e.Name.Local {
				case "style":
					style = attr(e, "name")
				case "graphic-properties":
					if clip := attr(e, "clip"); clip != "" && clipCuts(clip) {
						clipped[style] = true
					}
				}
			}
		}
	}
	d := xml.NewDecoder(bytes.NewReader(o.read("content.xml")))
	frameStyle := ""
	looked := map[string]bool{}
	for {
		t, err := d.Token()
		if err != nil {
			break
		}
		e, ok := t.(xml.StartElement)
		if !ok {
			continue
		}
		switch e.Name.Local {
		case "frame":
			frameStyle = attr(e, "style-name")
		case "image":
			href := attr(e, "href")
			media := o.read(href)
			if clipped[frameStyle] {
				c.hit("office-crop", where, fmt.Sprintf("The picture %s is shown cropped.", mediaName(href, media)))
			}
			if media != nil && !looked[href] {
				looked[href] = true
				c.inspect(where+" > "+href, path.Base(href), media, depth+1)
			}
		}
	}
}

// clipCuts says whether a clip of rect(top, right, bottom, left) takes
// anything off. Every edge at zero, in any unit, is no crop.
func clipCuts(clip string) bool {
	inner := strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(clip), "rect("), ")")
	for _, f := range strings.FieldsFunc(inner, func(r rune) bool { return r == ',' || r == ' ' }) {
		v := strings.TrimRight(f, "abcdefghijklmnopqrstuvwxyz%")
		if n, err := strconv.ParseFloat(v, 64); err == nil && n > 0 {
			return true
		}
	}
	return false
}
