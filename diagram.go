package main

// The drawing kept inside an exported picture.
//
// draw.io and Excalidraw both put a copy of the drawing inside the PNG or
// SVG they export, so the picture can be opened and edited again. The
// copy is not the picture: it is the drawing, with whatever the picture
// leaves out. draw.io keeps every page and every hidden layer; Excalidraw
// keeps the elements it marked deleted, when they are passed to the
// export, and the whole of every image that was cropped on the canvas.

import (
	"bytes"
	"compress/flate"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
)

// drawioPages reads an mxfile: one <diagram> per page, each holding its
// model either as XML or compressed (base64 of raw deflate of the
// percent-encoded XML).
type drawioPage struct {
	name   string
	labels []string
	hidden []string // labels of cells that are not visible, or on hidden layers
	cells  int
}

func drawioPages(mxfile string) ([]drawioPage, bool) {
	var f struct {
		Diagrams []struct {
			Name  string `xml:"name,attr"`
			Inner string `xml:",innerxml"`
		} `xml:"diagram"`
	}
	if err := xml.Unmarshal([]byte(mxfile), &f); err != nil || len(f.Diagrams) == 0 {
		// An old export holds a bare model, one page with no name.
		if strings.Contains(mxfile, "<mxGraphModel") {
			p := drawioModel(mxfile)
			return []drawioPage{p}, true
		}
		return nil, false
	}
	var pages []drawioPage
	for _, d := range f.Diagrams {
		model := strings.TrimSpace(d.Inner)
		if !strings.HasPrefix(model, "<") {
			model = inflateDrawio(model)
		}
		p := drawioModel(model)
		p.name = d.Name
		pages = append(pages, p)
	}
	return pages, true
}

func inflateDrawio(s string) string {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(s))
	if err != nil {
		return ""
	}
	out, err := io.ReadAll(io.LimitReader(flate.NewReader(bytes.NewReader(raw)), 64<<20))
	if err != nil {
		return ""
	}
	return percentDecode(string(out))
}

func drawioModel(model string) drawioPage {
	var m struct {
		Cells []struct {
			ID      string `xml:"id,attr"`
			Parent  string `xml:"parent,attr"`
			Value   string `xml:"value,attr"`
			Visible string `xml:"visible,attr"`
			Label   string `xml:"label,attr"`
		} `xml:"root>mxCell"`
		Objects []struct {
			Label string `xml:"label,attr"`
			Cell  struct {
				ID      string `xml:"id,attr"`
				Parent  string `xml:"parent,attr"`
				Visible string `xml:"visible,attr"`
			} `xml:"mxCell"`
			ID string `xml:"id,attr"`
		} `xml:"root>object"`
	}
	var p drawioPage
	if err := xml.Unmarshal([]byte(model), &m); err != nil {
		return p
	}
	type cell struct{ parent, label, visible string }
	cells := map[string]cell{}
	var order []string
	for _, c := range m.Cells {
		cells[c.ID] = cell{c.Parent, c.Value, c.Visible}
		order = append(order, c.ID)
	}
	for _, o := range m.Objects {
		cells[o.ID] = cell{o.Cell.Parent, o.Label, o.Cell.Visible}
		order = append(order, o.ID)
	}
	hiddenUp := func(id string) bool {
		for k := 0; k < 64 && id != ""; k++ {
			c, ok := cells[id]
			if !ok {
				return false
			}
			if c.visible == "0" {
				return true
			}
			id = c.parent
		}
		return false
	}
	for _, id := range order {
		c := cells[id]
		p.cells++
		label := plainLabel(c.label)
		if hiddenUp(id) {
			if label != "" {
				p.hidden = append(p.hidden, label)
			} else {
				p.hidden = append(p.hidden, "")
			}
			continue
		}
		if label != "" {
			p.labels = append(p.labels, label)
		}
	}
	return p
}

// plainLabel takes the HTML out of a draw.io label.
func plainLabel(s string) string {
	if s == "" {
		return ""
	}
	var b strings.Builder
	in := false
	for _, r := range s {
		switch {
		case r == '<':
			in = true
			b.WriteByte(' ')
		case r == '>':
			in = false
		case !in:
			b.WriteRune(r)
		}
	}
	return strings.Join(strings.Fields(xmlUnescape(b.String())), " ")
}

// percentDecode undoes encodeURIComponent, which is how draw.io writes the
// model before compressing it. net/url would do it, and this program has
// no business importing anything with net in its name.
func percentDecode(s string) string {
	if !strings.Contains(s, "%") {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '%' && i+2 < len(s) {
			if v, ok := hexByte(s[i+1], s[i+2]); ok {
				b.WriteByte(v)
				i += 2
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func hexByte(a, b byte) (byte, bool) {
	h := func(c byte) (byte, bool) {
		switch {
		case c >= '0' && c <= '9':
			return c - '0', true
		case c >= 'a' && c <= 'f':
			return c - 'a' + 10, true
		case c >= 'A' && c <= 'F':
			return c - 'A' + 10, true
		}
		return 0, false
	}
	x, ok1 := h(a)
	y, ok2 := h(b)
	return x<<4 | y, ok1 && ok2
}

func (c *run) drawio(where, mxfile string) {
	pages, ok := drawioPages(mxfile)
	if !ok {
		c.gap(where, "carries a draw.io diagram that could not be read")
		return
	}
	if len(pages) > 1 {
		var names []string
		for _, p := range pages {
			n := c.show.text(p.name)
			if n == "" {
				n = "a page with no name"
			}
			names = append(names, fmt.Sprintf("%s (%s)", n, plural(len(p.labels), "label", "labels")))
		}
		c.hit("drawing-pages", where, fmt.Sprintf("The copy inside holds %d pages: %s.", len(pages), strings.Join(names, ", ")))
	}
	for _, p := range pages {
		if len(p.hidden) == 0 {
			continue
		}
		var shown []string
		for _, l := range p.hidden {
			if l != "" {
				shown = append(shown, `"`+c.show.text(l)+`"`)
			}
		}
		line := fmt.Sprintf("%s hidden from view", plural(len(p.hidden), "shape is", "shapes are"))
		if p.name != "" && len(pages) > 1 {
			line += " on " + c.show.text(p.name)
		}
		line += ", in the copy of the diagram but not in the picture"
		if len(shown) > 0 {
			line += ", among them " + strings.Join(atMost(shown, 4), ", ")
		}
		c.hit("drawing-outside", where, line+".")
	}
}

// The wrapper Excalidraw puts its scene in, inside a PNG or an SVG.
type excalidrawWrapper struct {
	Version    any    `json:"version"` // a string in the wrapper, a number in a bare scene
	Encoding   string `json:"encoding"`
	Compressed bool   `json:"compressed"`
	Encoded    string `json:"encoded"`
}

type excalidrawScene struct {
	Elements []struct {
		ID        string  `json:"id"`
		Type      string  `json:"type"`
		IsDeleted bool    `json:"isDeleted"`
		Text      string  `json:"text"`
		FileID    string  `json:"fileId"`
		Opacity   float64 `json:"opacity"`
		Crop      *struct {
			Width         float64 `json:"width"`
			Height        float64 `json:"height"`
			NaturalWidth  float64 `json:"naturalWidth"`
			NaturalHeight float64 `json:"naturalHeight"`
		} `json:"crop"`
	} `json:"elements"`
	Files map[string]struct {
		MimeType string `json:"mimeType"`
		DataURL  string `json:"dataURL"`
	} `json:"files"`
}

// excalidrawScene decodes the wrapper. "bstring" is a JavaScript string in
// which every character is one byte, which JSON writes with \u escapes.
func readExcalidraw(wrapper []byte) (*excalidrawScene, bool) {
	var w excalidrawWrapper
	if err := json.Unmarshal(wrapper, &w); err != nil {
		return nil, false
	}
	// The first version of the format had no wrapper: the scene itself.
	if w.Encoded == "" {
		var s excalidrawScene
		if err := json.Unmarshal(wrapper, &s); err != nil || s.Elements == nil {
			return nil, false
		}
		return &s, true
	}
	raw := []byte(w.Encoded)
	if w.Encoding == "bstring" {
		r := []rune(w.Encoded)
		raw = make([]byte, len(r))
		for i, x := range r {
			raw[i] = byte(x)
		}
	}
	if w.Compressed {
		out, err := unzlib(raw, 256<<20)
		if err != nil {
			return nil, false
		}
		raw = out
	}
	var s excalidrawScene
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, false
	}
	return &s, true
}

func (c *run) excalidraw(where string, wrapper []byte, depth int) {
	s, ok := readExcalidraw(wrapper)
	if !ok {
		c.gap(where, "carries an Excalidraw scene that could not be read")
		return
	}
	used := map[string]bool{}
	var deleted []string
	deletedCount := 0
	for _, e := range s.Elements {
		if e.IsDeleted {
			deletedCount++
			if strings.TrimSpace(e.Text) != "" {
				deleted = append(deleted, `"`+c.show.text(e.Text)+`"`)
			}
			continue
		}
		if e.FileID != "" {
			used[e.FileID] = true
		}
		if e.Type == "image" && e.Crop != nil && e.Crop.NaturalWidth > 0 && e.Crop.NaturalHeight > 0 {
			shown := (e.Crop.Width * e.Crop.Height) / (e.Crop.NaturalWidth * e.Crop.NaturalHeight)
			if shown < 0.97 {
				c.hit("drawing-crop", where, fmt.Sprintf("An image of %.0fx%.0f is shown cropped to %s of it.", e.Crop.NaturalWidth, e.Crop.NaturalHeight, percent(shown, 1)))
			}
		}
	}
	if deletedCount > 0 {
		line := fmt.Sprintf("%s deleted from the drawing %s still in it", plural(deletedCount, "element", "elements"), map[bool]string{true: "is", false: "are"}[deletedCount == 1])
		if len(deleted) > 0 {
			line += ", text among them: " + strings.Join(atMost(deleted, 4), ", ")
		}
		c.hit("drawing-deleted", where, line+".")
	}
	removed := 0
	for id, f := range s.Files {
		data := dataURL(f.DataURL)
		if !used[id] {
			removed++
		}
		if data != nil {
			c.inspect(where+" > image "+shortID(id), "image", data, depth+1)
		}
	}
	if removed > 0 {
		c.hit("drawing-deleted", where, fmt.Sprintf("%s that no element in the drawing shows %s kept in it.", plural(removed, "image", "images"), map[bool]string{true: "is", false: "are"}[removed == 1]))
	}
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

// dataURL decodes data:image/png;base64,...
func dataURL(s string) []byte {
	i := strings.Index(s, ";base64,")
	if !strings.HasPrefix(s, "data:") || i < 0 {
		return nil
	}
	b, err := base64.StdEncoding.DecodeString(s[i+8:])
	if err != nil {
		return nil
	}
	return b
}

// drawingInPNG looks in a PNG's text chunks for either kind of copy.
func (c *run) drawingInPNG(where string, p *pngFile, depth int) {
	for _, t := range p.texts {
		switch {
		case t.key == "mxfile" || t.key == "mxGraphModel":
			c.drawio(where, percentDecode(t.value))
		case t.key == "application/vnd.excalidraw+json":
			c.excalidraw(where, []byte(t.value), depth)
		}
	}
}

// textDrawing reads an SVG, for a draw.io copy in its content attribute,
// an Excalidraw scene in its payload comment, and pictures embedded in it.
func (c *run) textDrawing(where string, data []byte, depth int) {
	s := string(data)
	if !strings.Contains(s[:min(len(s), 4096)], "<svg") {
		return
	}
	if i := strings.Index(s, "<svg"); i >= 0 {
		head := s[i:]
		if j := strings.IndexByte(head, '>'); j >= 0 {
			if v, ok := xmpField(head[:j], "content"); ok && strings.Contains(v, "mxfile") {
				c.drawio(where, v)
			}
		}
	}
	if i := strings.Index(s, "<!-- payload-start -->"); i >= 0 {
		rest := s[i+len("<!-- payload-start -->"):]
		if j := strings.Index(rest, "<!-- payload-end -->"); j >= 0 {
			payload := strings.TrimSpace(rest[:j])
			// The base64 is of a byte string, one byte per character, so the
			// JSON inside is Latin-1 and not UTF-8. Read as UTF-8, every byte
			// of the compressed scene above 127 turned into a replacement
			// character, and not one of thirty public drawings would open.
			if raw, err := base64.StdEncoding.DecodeString(payload); err == nil {
				c.excalidraw(where, []byte(latin1(raw)), depth)
			}
		}
	}
	// Pictures written into the SVG as data URLs are files of their own,
	// and get every check a file gets.
	n := 0
	for rest := s; ; {
		i := strings.Index(rest, "data:image/")
		if i < 0 {
			break
		}
		rest = rest[i:]
		end := strings.IndexAny(rest, `"')`)
		if end < 0 {
			break
		}
		if b := dataURL(rest[:end]); b != nil {
			n++
			c.inspect(fmt.Sprintf("%s > picture %d", where, n), "picture", b, depth+1)
		}
		rest = rest[end:]
	}
}
