package main

// Boxes nearly the colour of the page.
//
// seeThrough finds a box by its region: everything close in colour that
// touches, ending where the colour jumps. A black box at ninety per cent
// on a page in dark mode is three levels from black where the page is
// thirty, and a white one on a white page is a few levels brighter than
// the paper; both run into the page and become one region with it. The
// bench had none of them found.
//
// What they do have is an edge no page has: a straight line along which
// the colour steps by the same few levels, for as long as the box is
// wide. Two such lines one above the other, stepping opposite ways, with
// the same two ends, and sides that step the same way down, are a box.

import "image"

type stepRun struct {
	at, from, to int // the line, and where along it the run starts and ends
	step         int
}

type faintBox struct {
	r    image.Rectangle
	step int
}

// faintBoxes gives boxes whose edges step by less than seeThrough looks
// for, with what is inside them left to letsThrough.
func faintBoxes(lum []uint8, w, h int, lossless bool) []faintBox {
	var edges []stepRun
	var runs []stepRun
	for y := 2; y < h-1; y++ {
		runs = steps(runs[:0], w, func(x int) (int, int, int) {
			i := y*w + x
			return int(lum[i]) - int(lum[i-w]), int(lum[i-w]) - int(lum[i-2*w]), int(lum[i+w]) - int(lum[i])
		}, lossless)
		for _, r := range joinRuns(runs) {
			r.at = y
			edges = append(edges, r)
		}
	}
	var out []faintBox
	for i, t := range edges {
		for _, b := range edges[i+1:] {
			if b.at-t.at < 12 || !opposite(t.step, b.step) {
				continue
			}
			lo, hi := max(t.from, b.from), min(t.to, b.to)
			if hi-lo < max(t.to-t.from, b.to-b.from)*8/10 {
				continue
			}
			r := image.Rect(min(t.from, b.from), t.at, max(t.to, b.to), b.at)
			if r, ok := sides(lum, w, r, image.Rect(lo, t.at, hi, b.at), t.step, lossless); ok && squareCorners(lum, w, h, r, t.step, lossless) && !ruled(lum, w, h, r, t.step) {
				out = append(out, faintBox{r, t.step})
				break
			}
		}
	}
	return out
}

// joinRuns joins the runs of one line that step alike and are near each
// other. In a JPEG the edge of a box comes and goes in the noise of the
// text beside it, and a box six hundred wide came out as two runs, one
// stepping by twenty one and the other by twenty four, neither of them
// starting where the box did.
func joinRuns(runs []stepRun) []stepRun {
	var out []stepRun
	for _, r := range runs {
		if n := len(out); n > 0 {
			last := &out[n-1]
			if r.from-last.to <= 48 && abs(r.step-last.step) <= max(4, abs(last.step)/4) {
				last.to = r.to
				continue
			}
		}
		out = append(out, r)
	}
	return out
}

// steps finds the runs along one line where the colour steps across it
// by a steady few levels, and does not step on the lines either side,
// which is what keeps a gradient out. at(k) gives the step across the
// line at k, and the steps across the lines before and after it.
func steps(out []stepRun, n int, at func(k int) (int, int, int), lossless bool) []stepRun {
	// Text that crosses the edge breaks it for a few pixels.
	const gap = 12
	tol := 2
	if !lossless {
		tol = 4
	}
	cur := stepRun{from: -1}
	last := -1
	flush := func() {
		if cur.from >= 0 && last-cur.from >= 39 {
			cur.to = last + 1
			out = append(out, cur)
		}
		cur.from = -1
	}
	for k := 1; k < n-1; k++ {
		d, before, after := at(k)
		edge := abs(d) >= 3 && abs(d) < 48 && abs(before) <= abs(d)/3 && abs(after) <= abs(d)/3
		if cur.from >= 0 {
			if edge && abs(d-cur.step) <= max(tol, abs(cur.step)/5) {
				last = k
				continue
			}
			// The first pixel or two of a run is a corner, and a JPEG blurs
			// corners: a box stepping by twenty seven started at twenty two,
			// and the rest of its edge did not match.
			if edge && last-cur.from < 4 {
				cur, last = stepRun{from: k, step: d}, k
				continue
			}
			if k-last <= gap {
				continue
			}
			flush()
		}
		if edge {
			cur, last = stepRun{from: k, step: d}, k
		}
	}
	flush()
	return out
}

// squareCorners is whether the box has the corners of one drawn over a
// picture. A button greyed out because it cannot be used is a dimmer
// rectangle with dimmer text in it, which is what a box that lets things
// through is too; two of them came out of a few hundred real screenshots.
// Buttons have rounded corners, and there the corner pixels are the page.
// Three corners of four will do, as text can run through one. A JPEG
// blurs the corner pixel itself, so there the two looked at are one and
// two pixels in, which a corner rounded by six or more still misses.
func squareCorners(lum []uint8, w, h int, r image.Rectangle, step int, lossless bool) bool {
	first, tol := 0, max(2, abs(step)/3)
	if !lossless {
		first, tol = 1, max(4, abs(step)/2)
	}
	good := 0
	corner := func(x, y, dx, dy int) {
		out := image.Pt(x-dx, y-dy)
		if out.X < 0 || out.Y < 0 || out.X >= w || out.Y >= h {
			good++
			return
		}
		o := int(lum[out.Y*w+out.X])
		for k := first; k <= first+1; k++ {
			if abs(int(lum[(y+k*dy)*w+x+k*dx])-o-step) > tol {
				return
			}
		}
		good++
	}
	corner(r.Min.X, r.Min.Y, 1, 1)
	corner(r.Max.X-1, r.Min.Y, -1, 1)
	corner(r.Min.X, r.Max.Y-1, 1, -1)
	corner(r.Max.X-1, r.Max.Y-1, -1, -1)
	return good >= 3
}

// ruled is whether the edges of r are lines rather than the edge of a
// box. A box is a step that holds: a few pixels in, it is still the box,
// and a few pixels out, still the page. Three real screenshots had a
// background ruled in squares, and the rows of squares came out as boxes
// with a line along each edge, where four pixels either side of the edge
// were the same colour.
func ruled(lum []uint8, w, h int, r image.Rectangle, step int) bool {
	holds, looked := 0, 0
	probe := func(x, y, dx, dy int) {
		// x, y is the first pixel outside; dx, dy points away from the box.
		ix, iy := x-5*dx, y-5*dy
		ox, oy := x+4*dx, y+4*dy
		if ox < 0 || oy < 0 || ox >= w || oy >= h {
			return
		}
		looked++
		if abs(int(lum[iy*w+ix])-int(lum[oy*w+ox])-step) <= max(2, abs(step)/2) {
			holds++
		}
	}
	for x := r.Min.X + 5; x < r.Max.X-5; x += max(1, r.Dx()/16) {
		probe(x, r.Min.Y-1, 0, -1)
		probe(x, r.Max.Y, 0, 1)
	}
	for y := r.Min.Y + 5; y < r.Max.Y-5; y += max(1, r.Dy()/16) {
		probe(r.Min.X-1, y, -1, 0)
		probe(r.Max.X, y, 1, 0)
	}
	return looked > 0 && holds*2 < looked
}

func opposite(a, b int) bool {
	return a*b < 0 && abs(a+b) <= max(3, abs(a)/3)
}

// sides finds the left and right edges of a box, which step like its
// top: the left the same way, the right the other way, on most rows. The
// ends of the top and bottom runs are near where the sides are, in outer,
// and inner is the part both runs cover; a JPEG blurs corners, and a box
// drawn from three had its runs start at nineteen and twenty six. A side
// at the edge of the picture cannot be seen, and is let by, but not both.
func sides(lum []uint8, w int, outer, inner image.Rectangle, step int, lossless bool) (image.Rectangle, bool) {
	tol := max(2, abs(step)/5)
	if !lossless {
		tol = max(4, abs(step)/4)
	}
	score := func(x, sign int) int {
		if x == 0 || x == w {
			// Just enough to pass, so a side that can be seen wins.
			return (outer.Dy()*7 + 9) / 10
		}
		good := 0
		for y := outer.Min.Y; y < outer.Max.Y; y++ {
			d := sign * (int(lum[y*w+x]) - int(lum[y*w+x-1]))
			if abs(d-step) <= tol {
				good++
			}
		}
		return good
	}
	find := func(from, to, sign int) (int, bool) {
		best, at := 0, 0
		for x := max(from, 0); x <= min(to, w); x++ {
			if s := score(x, sign); s > best {
				best, at = s, x
			}
		}
		return at, best*10 >= outer.Dy()*7
	}
	x0, ok0 := find(outer.Min.X-24, inner.Min.X+2, 1)
	x1, ok1 := find(inner.Max.X-2, outer.Max.X+24, -1)
	// Both sides at the edges of the picture is a band, not a box: the
	// rows of a background ruled in squares, in three real screenshots.
	seen := x0 > 0 || x1 < w
	return image.Rect(x0, outer.Min.Y, x1, outer.Max.Y), ok0 && ok1 && seen && x1-x0 > inner.Dx()/2
}
