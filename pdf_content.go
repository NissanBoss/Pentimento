package main

// Reading a page's drawing instructions, far enough to know what ended up
// on top of what.
//
// The whole detection rests on one thing: order. A filled rectangle drawn
// after some text covers it. The same rectangle drawn before the text is
// just a background, and the text sits on top of it in plain view. Tools
// that only look for "dark rectangle near text" flag every highlighted
// heading in the document; this one does not, because it knows which was
// drawn first.

import (
	"math"
	"strings"
)

// matrix is the usual PDF triple: a b c d e f.
type matrix [6]float64

var identity = matrix{1, 0, 0, 1, 0, 0}

// mul returns m applied first, then n.
func (m matrix) mul(n matrix) matrix {
	return matrix{
		m[0]*n[0] + m[1]*n[2],
		m[0]*n[1] + m[1]*n[3],
		m[2]*n[0] + m[3]*n[2],
		m[2]*n[1] + m[3]*n[3],
		m[4]*n[0] + m[5]*n[2] + n[4],
		m[4]*n[1] + m[5]*n[3] + n[5],
	}
}

func (m matrix) apply(x, y float64) (float64, float64) {
	return m[0]*x + m[2]*y + m[4], m[1]*x + m[3]*y + m[5]
}

type pdfRect struct{ X0, Y0, X1, Y1 float64 }

func (r pdfRect) width() float64  { return r.X1 - r.X0 }
func (r pdfRect) height() float64 { return r.Y1 - r.Y0 }

// coverKind says what was painted over the text, because the advice
// differs. A rectangle is usually somebody's idea of redaction; an image
// pasted on top is usually a screenshot patch.
type coverKind string

const (
	coverBox   coverKind = "box"
	coverImage coverKind = "image"
)

// A painted shape, with the order it was painted in.
type filled struct {
	Seq       int
	Kind      coverKind
	Box       pdfRect
	Luminance float64 // 0 is black, 1 is white; meaningless for an image
	// For an image: the stream, its object, and the clip it was drawn in.
	Image *Stream
	Ref   Ref
	Clip  pdfRect
}

// A run of text, with where it started and the order it was painted in.
type textRun struct {
	Seq   int
	X, Y  float64
	Size  float64
	Text  string
	Width float64 // estimated, see widthOf
}

type pdfPage struct {
	Fills []filled
	Texts []textRun

	// Skipped records drawing this reader could not follow. A page with
	// unread drawing on it is not a page known to be clean, and saying so
	// is the whole difference between this and a tool that guesses.
	Skipped []string
}

type graphicsState struct {
	ctm       matrix
	luminance float64
	// clip is the rectangle drawing is limited to, in page space: what a
	// cropped picture is cut down to when shown.
	clip pdfRect
}

// reader walks content streams, including the ones nested inside form
// XObjects, keeping one running sequence number across all of them so that
// "painted after" still means the same thing at any depth.
type reader struct {
	doc  *Document
	page *pdfPage
	seq  int
	busy map[Ref]bool // forms currently being drawn, to catch a cycle
}

// readPage reads a page's content, following the forms it draws.
func readPage(doc *Document, content []byte, resources Dict) *pdfPage {
	r := &reader{doc: doc, page: &pdfPage{}, busy: map[Ref]bool{}}
	r.walk(content, resources, graphicsState{ctm: identity, luminance: 0, clip: everywhere}, 0)
	return r.page
}

func (r *reader) note(s string) {
	for _, existing := range r.page.Skipped {
		if existing == s {
			return // one mention of each kind is enough
		}
	}
	r.page.Skipped = append(r.page.Skipped, s)
}

func (r *reader) walk(content []byte, resources Dict, gs graphicsState, depth int) {
	if depth > 12 {
		r.note("drawing nested more than twelve deep was not followed")
		return
	}
	lex := &lexer{data: content}

	var stack []graphicsState
	var operands []any
	var pending []pdfRect // rectangles built but not yet painted
	clipping := false     // W was given, so the path ends by cutting the clip

	// Text state, valid between BT and ET.
	var textMatrix, lineMatrix matrix
	fontSize := 0.0
	leading := 0.0

	num := func(i int) float64 {
		// Operands are counted from the end, since operators take their
		// arguments from the right.
		if i >= len(operands) {
			return 0
		}
		v, _ := operands[len(operands)-1-i].(float64)
		return v
	}

	showText := func(s string) {
		if strings.TrimSpace(s) == "" {
			return
		}
		full := textMatrix.mul(gs.ctm)
		r.seq++
		size := fontSize * scaleOf(full)
		r.page.Texts = append(r.page.Texts, textRun{
			Seq: r.seq, X: full[4], Y: full[5], Size: size,
			Text: s, Width: widthOf(s, size),
		})
	}

	for {
		lex.skipSpace()
		if lex.pos >= len(lex.data) {
			break
		}
		v, err := lex.readObject(nil)
		if err != nil {
			r.note("part of a content stream would not parse")
			break
		}
		op, isOp := v.(keyword)
		if !isOp {
			operands = append(operands, v)
			if len(operands) > 64 {
				operands = operands[len(operands)-64:]
			}
			continue
		}

		switch op {
		case "q":
			stack = append(stack, gs)
		case "Q":
			if n := len(stack); n > 0 {
				gs = stack[n-1]
				stack = stack[:n-1]
			}
		case "cm":
			m := matrix{num(5), num(4), num(3), num(2), num(1), num(0)}
			gs.ctm = m.mul(gs.ctm)

		case "g":
			gs.luminance = clamp(num(0))
		case "rg":
			gs.luminance = clamp(0.299*num(2) + 0.587*num(1) + 0.114*num(0))
		case "k":
			gs.luminance = cmykLuminance(num(3), num(2), num(1), num(0))
		case "sc", "scn":
			// Without resolving the colour space this is a guess, so take
			// the safer one and assume it could be dark.
			if len(operands) > 0 {
				gs.luminance = clamp(num(0))
			}

		case "re":
			// A rectangle is only a shape so far. Whether it ends up
			// painted, outlined or thrown away is decided by the operator
			// that closes the path, several tokens later.
			x, y, w, h := num(3), num(2), num(1), num(0)
			x0, y0 := gs.ctm.apply(x, y)
			x1, y1 := gs.ctm.apply(x+w, y+h)
			pending = append(pending, normalise(pdfRect{x0, y0, x1, y1}))

		case "W", "W*":
			clipping = true

		case "f", "F", "f*", "b", "b*", "B", "B*":
			if clipping {
				gs.clip = intersect(gs.clip, bounds(pending))
				clipping = false
			}
			// Painted. These are the only ones that can hide anything.
			for _, box := range pending {
				r.seq++
				r.page.Fills = append(r.page.Fills, filled{
					Seq: r.seq, Kind: coverBox, Box: box, Luminance: gs.luminance,
				})
			}
			pending = pending[:0]

		case "S", "s", "n":
			// Outlined or discarded. A table border is drawn this way, and
			// counting those was what made the first version light up every
			// form it was shown. "W n" is how a clip is set.
			if clipping && len(pending) > 0 {
				gs.clip = intersect(gs.clip, bounds(pending))
			}
			clipping = false
			pending = pending[:0]

		case "BT", "ET":
			textMatrix, lineMatrix = identity, identity

		case "Tf":
			fontSize = num(0)
		case "TL":
			leading = num(0)
		case "Td":
			lineMatrix = matrix{1, 0, 0, 1, num(1), num(0)}.mul(lineMatrix)
			textMatrix = lineMatrix
		case "TD":
			leading = -num(0)
			lineMatrix = matrix{1, 0, 0, 1, num(1), num(0)}.mul(lineMatrix)
			textMatrix = lineMatrix
		case "Tm":
			lineMatrix = matrix{num(5), num(4), num(3), num(2), num(1), num(0)}
			textMatrix = lineMatrix
		case "T*":
			lineMatrix = matrix{1, 0, 0, 1, 0, -leading}.mul(lineMatrix)
			textMatrix = lineMatrix

		case "Tj":
			if len(operands) > 0 {
				if s, ok := operands[len(operands)-1].([]byte); ok {
					showText(string(s))
				}
			}
		case "'", "\"":
			lineMatrix = matrix{1, 0, 0, 1, 0, -leading}.mul(lineMatrix)
			textMatrix = lineMatrix
			if len(operands) > 0 {
				if s, ok := operands[len(operands)-1].([]byte); ok {
					showText(string(s))
				}
			}
		case "TJ":
			if len(operands) > 0 {
				if arr, ok := operands[len(operands)-1].(Array); ok {
					var b strings.Builder
					for _, item := range arr {
						if s, ok := item.([]byte); ok {
							b.Write(s)
						}
					}
					showText(b.String())
				}
			}

		case "Do":
			if len(operands) > 0 {
				if name, ok := operands[len(operands)-1].(Name); ok {
					r.draw(name, resources, gs, depth)
				}
			}
		}
		operands = operands[:0]
	}
}

// draw follows a named XObject: a form is walked like any other content,
// and an image is recorded as a shape, because an image pasted over text
// hides it exactly as well as a black rectangle does.
func (r *reader) draw(name Name, resources Dict, gs graphicsState, depth int) {
	if r.doc == nil || resources == nil {
		r.note("something was drawn that could not be looked up")
		return
	}
	catalogue, _ := r.doc.Resolve(resources["XObject"]).(Dict)
	if catalogue == nil {
		r.note("something was drawn that could not be looked up")
		return
	}

	entry := catalogue[name]
	ref, isRef := entry.(Ref)
	stream, ok := r.doc.Resolve(entry).(*Stream)
	if !ok {
		r.note("something was drawn that could not be looked up")
		return
	}

	switch r.doc.Resolve(stream.Dict["Subtype"]) {
	case Name("Image"):
		// An image occupies the unit square, put wherever the current
		// transform puts it.
		x0, y0 := gs.ctm.apply(0, 0)
		x1, y1 := gs.ctm.apply(1, 1)
		r.seq++
		r.page.Fills = append(r.page.Fills, filled{
			Seq: r.seq, Kind: coverImage, Box: normalise(pdfRect{x0, y0, x1, y1}),
			Image: stream, Ref: ref, Clip: gs.clip,
		})
		return

	case Name("Form"):
		if isRef {
			if r.busy[ref] {
				r.note("a form drew itself, so it was not followed round again")
				return
			}
			r.busy[ref] = true
			defer delete(r.busy, ref)
		}
		content, err := r.doc.Decoded(stream)
		if err != nil {
			r.note("a form was drawn but could not be decoded: " + err.Error())
			return
		}
		// A form carries its own transform and may carry its own
		// resources; when it does not, it borrows the ones around it.
		inner := gs
		if m, ok := readMatrix(r.doc, stream.Dict["Matrix"]); ok {
			inner.ctm = m.mul(gs.ctm)
		}
		formResources, _ := r.doc.Resolve(stream.Dict["Resources"]).(Dict)
		if formResources == nil {
			formResources = resources
		}
		r.walk(content, formResources, inner, depth+1)
		return
	}
	r.note("something was drawn that this reader does not handle")
}

func readMatrix(doc *Document, v any) (matrix, bool) {
	arr, ok := doc.Resolve(v).(Array)
	if !ok || len(arr) != 6 {
		return identity, false
	}
	var m matrix
	for i, item := range arr {
		f, ok := doc.Resolve(item).(float64)
		if !ok {
			return identity, false
		}
		m[i] = f
	}
	return m, true
}

// hidden pairs each covering shape with the text it was painted over.
type hidden struct {
	Box   filled
	Under []textRun
}

// findHidden is the whole point. Text counts as hidden when a shape was
// painted after it, over the place it sits.
func (p *pdfPage) findHidden() []hidden {
	var out []hidden
	for _, f := range p.Fills {
		// A shape has to be able to hide something worth hiding. Twenty
		// points is about four characters at normal size; below that a
		// shape is a rule, a table border, or a cell in a chart.
		//
		// The number comes from a real document. A paper with an attention
		// heat map on one page is hundreds of small coloured cells with
		// three letter labels on them, and every one of those read as a
		// redaction until this threshold went in.
		if f.Box.width() < 20 || f.Box.height() < 6 {
			continue
		}
		var under []textRun
		for _, t := range p.Texts {
			if t.Seq > f.Seq {
				continue // drawn afterwards, so it sits on top and shows
			}
			if overlaps(f.Box, t) {
				under = append(under, t)
			}
		}
		if len(under) > 0 {
			out = append(out, hidden{Box: f, Under: under})
		}
	}
	return out
}

// Crossing a run of text is not covering it. A form full of small filled
// cells clips through every long line on the page, and the first version
// reported all of them: three hundred characters declared buried under a
// box fourteen points wide. A cover has to hide most of what it sits on.
func overlaps(box pdfRect, t textRun) bool {
	if t.Y < box.Y0-2 || t.Y > box.Y1+2 {
		return false
	}
	start, end := t.X, t.X+t.Width
	if end < box.X0 || start > box.X1 {
		return false
	}
	if t.Width <= 1 {
		return true // a single glyph, where position is all there is to go on
	}
	covered := math.Min(end, box.X1) - math.Max(start, box.X0)
	return covered/t.Width >= 0.6
}

// widthOf estimates how far a run of text reaches. Doing this exactly means
// reading font metrics out of the file, which is a great deal of work to
// refine a number that only decides whether a box overlaps by a few points.
// Half the point size per character is the usual rule of thumb.
func widthOf(s string, size float64) float64 {
	return float64(len([]rune(s))) * size * 0.5
}

// scaleOf gives roughly how much a matrix magnifies, so a point size can be
// reported in something close to page units.
func scaleOf(m matrix) float64 {
	s := math.Sqrt(math.Abs(m[0]*m[3] - m[1]*m[2]))
	if s == 0 || math.IsNaN(s) || math.IsInf(s, 0) {
		return 1
	}
	return s
}

func normalise(r pdfRect) pdfRect {
	if r.X0 > r.X1 {
		r.X0, r.X1 = r.X1, r.X0
	}
	if r.Y0 > r.Y1 {
		r.Y0, r.Y1 = r.Y1, r.Y0
	}
	return r
}

func clamp(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func cmykLuminance(c, m, y, k float64) float64 {
	r := (1 - clamp(c)) * (1 - clamp(k))
	g := (1 - clamp(m)) * (1 - clamp(k))
	b := (1 - clamp(y)) * (1 - clamp(k))
	return clamp(0.299*r + 0.587*g + 0.114*b)
}

// everywhere is no clip at all.
var everywhere = pdfRect{-1e9, -1e9, 1e9, 1e9}

func intersect(a, b pdfRect) pdfRect {
	r := pdfRect{max(a.X0, b.X0), max(a.Y0, b.Y0), min(a.X1, b.X1), min(a.Y1, b.Y1)}
	if r.X1 < r.X0 || r.Y1 < r.Y0 {
		return pdfRect{r.X0, r.Y0, r.X0, r.Y0}
	}
	return r
}

func bounds(rs []pdfRect) pdfRect {
	if len(rs) == 0 {
		return everywhere
	}
	b := rs[0]
	for _, r := range rs[1:] {
		b = pdfRect{min(b.X0, r.X0), min(b.Y0, r.Y0), max(b.X1, r.X1), max(b.Y1, r.Y1)}
	}
	return b
}

func area(r pdfRect) float64 { return max(0, r.width()) * max(0, r.height()) }
