package main

// Comparing a picture with a copy of it kept inside the same file.
//
// The copy can be the same frame, smaller (a thumbnail); a bigger frame
// that the picture was cropped out of (a preview made before the crop);
// letterboxed, with bars where the shapes differ (cameras do this); or a
// different picture altogether. So the picture is first found inside the
// copy, at whatever size and place fits best, and only then compared, a
// few pixels at a time, for the parts that are not the same.

import (
	"image"
	"math"
	"sort"
)

type comparison struct {
	// rotated is the copy matching once turned, which is an editor that
	// rotated the picture and not the preview, and hides nothing.
	rotated bool
	// unrelated is a copy that is not this picture at all.
	unrelated bool
	score     float64 // how well the picture matched where it was placed, -1 to 1
	// outside is the share of the copy that is not in the picture: what a
	// crop took off.
	outside float64
	// changed are the parts of the picture that differ from the copy, in
	// the picture's own pixels.
	changed []image.Rectangle
	share   float64 // of the picture, how much differs
	// reshaped is a picture straightened or warped since the copy was
	// made, which the parts that differ are not looked for in.
	reshaped bool
}

const (
	searchSide = 64
	fineSide   = 512
)

func compareCopies(main, copied image.Image) comparison {
	mb := main.Bounds()
	M := greyOf(main, fineSide)
	C := greyOf(copied, fineSide)
	if M.w < 8 || M.h < 8 || C.w < 8 || C.h < 8 {
		return comparison{score: 1}
	}
	cs := greyOf(copied, searchSide)
	ms := greyOf(main, searchSide*2)
	place, score := findInside(ms, cs)
	// A page of text shrunk to sixty four pixels is grey mush, and a strip
	// cropped out of it was not found in its own thumbnail: jpegtran's crop
	// of a letter, which keeps the thumbnail. A poor fit is looked for
	// again on a finer grid, in finer steps of size.
	if score < 0.85 {
		cs2, ms2 := greyOf(copied, 96), greyOf(main, 192)
		if p, s := findInsideBy(ms2, cs2, 0.0125); s > score {
			place, score, cs = p, s, cs2
		}
	}
	if score < 0.6 {
		// Turned a quarter, half or three quarters, at the same size.
		t := ms
		for k := 0; k < 3; k++ {
			t = t.turned()
			if p, s := findInside(t, cs); s > 0.9 && p.full(cs) {
				return comparison{rotated: true, score: s}
			}
		}
	}
	if score < 0.3 {
		return comparison{unrelated: true, score: score}
	}
	// The same place on finer copies, nudged to the best fit on each. One
	// jump from the coarse grid to the fine one left a cropped ballot two
	// per cent off, and every line of text on it came out as changed.
	fine, from := place, cs
	for _, side := range []int{128, fineSide} {
		g := C
		if side < fineSide {
			g = greyOf(copied, side)
		}
		f := float64(g.w) / float64(from.w)
		fine = placement{x: int(math.Round(float64(fine.x) * f)), y: int(math.Round(float64(fine.y) * f)),
			w: int(math.Round(float64(fine.w) * f)), h: int(math.Round(float64(fine.h) * f))}
		reach := 3
		if side < fineSide {
			reach = 5
		}
		fine = refine(M, g, fine, reach)
		from = g
	}
	// The match is scored leaving out the part that differs most. A black
	// box over a tenth of a photo pulls the plain correlation down to a
	// half, and the photo then looked like another picture altogether,
	// which is the one thing the box must not do to the result.
	plain := ncc(M.resized(fine.w, fine.h), C, fine)
	score = trimmedNCC(M.resized(fine.w, fine.h), C, fine)
	// Text shrunk to a thumbnail matches itself badly: a strip cropped
	// from a letter scored 0.56 in the right place. So the line for "not
	// this picture" is low, and a crop found with a poor match is believed
	// only when the shape changed, which is what a crop does and a
	// misplaced match of the same frame does not.
	if score < 0.35 {
		return comparison{unrelated: true, score: score}
	}
	r := comparison{score: score}
	r.outside = outsideShare(C, fine)
	sameShape := math.Abs(float64(M.w)/float64(M.h)-float64(C.w)/float64(C.h)) < 0.03*float64(C.w)/float64(C.h)
	if score < 0.8 && sameShape {
		r.outside = 0
	}
	// A Pixel corrects the distortion of its lens after making the
	// thumbnail, and the correction trims a few per cent off every edge:
	// the thumbnail of a third of the Pixel photos in the calibration
	// showed four or five per cent more than the photo, in the same shape.
	// A crop by hand that keeps the shape and takes less than a tenth is
	// rarer than that, and hides little.
	if sameShape && r.outside < 0.10 {
		r.outside = 0
	}
	var shifted int
	r.changed, r.share, shifted = changedParts(M, C, fine, mb)
	r.changed = awayFromEdges(r.changed, mb)
	if lensMoved(r.changed, mb) {
		r.changed, r.share = nil, 0
	}
	if len(r.changed) == 0 {
		r.share = 0
	}
	// Below this the picture was straightened or warped since the copy
	// was made, and the parts that differ are every edge in it, not what
	// an edit covered: a ballot photographed at an angle and squared up
	// came out as nineteen changes. The crop is still measured. A box
	// lowers the plain score too; what tells the two apart is that a box
	// leaves flat parts where the copy has something, and straightening
	// leaves many parts where the edges are merely somewhere else.
	if plain < 0.85 && shifted > 5 {
		r.changed, r.share, r.reshaped = nil, 0, true
	}
	return r
}

type placement struct{ x, y, w, h int }

func (p placement) full(g grid) bool {
	return p.x == 0 && p.y == 0 && p.w == g.w && p.h == g.h
}

// findInside looks for the picture m inside the copy c, at every size from
// as big as fits down to a third of that, and every place.
func findInside(m, c grid) (placement, float64) {
	return findInsideBy(m, c, 0.025)
}

func findInsideBy(m, c grid, step float64) (placement, float64) {
	return findInsideDown(m, c, step, 0.33)
}

func findInsideDown(m, c grid, step, least float64) (placement, float64) {
	aspect := float64(m.w) / float64(m.h)
	fitW, fitH := float64(c.w), float64(c.w)/aspect
	if fitH > float64(c.h) {
		fitH, fitW = float64(c.h), float64(c.h)*aspect
	}
	best, bestScore := placement{}, -2.0
	// Same frame, stretched to the copy. Cameras round a thumbnail's size,
	// so a shape a few per cent off is still the same frame.
	if r := aspect / (float64(c.w) / float64(c.h)); r > 0.97 && r < 1.03 {
		p := placement{0, 0, c.w, c.h}
		if s := ncc(m.resized(c.w, c.h), c, p); s > 0.92 {
			return p, s
		} else {
			best, bestScore = p, s
		}
	}
	for s := 1.0; s >= least; s -= step {
		w, h := int(math.Round(fitW*s)), int(math.Round(fitH*s))
		if w < 6 || h < 6 {
			break
		}
		small := m.resized(w, h)
		for y := 0; y+h <= c.h; y++ {
			for x := 0; x+w <= c.w; x++ {
				p := placement{x, y, w, h}
				if sc := ncc(small, c, p); sc > bestScore {
					best, bestScore = p, sc
				}
			}
		}
	}
	return best, bestScore
}

// refine tries the sizes and places next to a placement on finer grids.
func refine(m, c grid, p placement, reach int) placement {
	// Scaling up from the coarse grid can round a pixel past the edge.
	p.w, p.h = min(p.w, c.w), min(p.h, c.h)
	p.x, p.y = clampInt(p.x, 0, c.w-p.w), clampInt(p.y, 0, c.h-p.h)
	best, bestScore := p, ncc(m.resized(p.w, p.h), c, p)
	for dw := -reach; dw <= reach; dw++ {
		w := p.w + dw
		h := int(math.Round(float64(w) * float64(m.h) / float64(m.w)))
		if w < 8 || h < 8 || w > c.w || h > c.h {
			continue
		}
		small := m.resized(w, h)
		for dy := -reach; dy <= reach; dy++ {
			for dx := -reach; dx <= reach; dx++ {
				q := placement{p.x + dx, p.y + dy, w, h}
				if q.x < 0 || q.y < 0 || q.x+w > c.w || q.y+h > c.h {
					continue
				}
				if s := ncc(small, c, q); s > bestScore {
					best, bestScore = q, s
				}
			}
		}
	}
	return best
}

// ncc is the normalised correlation of the picture with the part of the
// copy it is placed on: 1 when one is the other with the brightness and
// contrast changed.
func ncc(small, c grid, p placement) float64 {
	var sa, sb, saa, sbb, sab float64
	n := float64(p.w * p.h)
	for y := 0; y < p.h; y++ {
		row := (p.y+y)*c.w + p.x
		for x := 0; x < p.w; x++ {
			a := float64(small.v[y*p.w+x])
			b := float64(c.v[row+x])
			sa += a
			sb += b
			saa += a * a
			sbb += b * b
			sab += a * b
		}
	}
	va := saa - sa*sa/n
	vb := sbb - sb*sb/n
	if va < 1e-6 || vb < 1e-6 {
		// Something flat matches anything flat, and says nothing.
		return 0
	}
	return (sab - sa*sb/n) / math.Sqrt(va*vb)
}

// outsideShare is how much of the copy lies outside the picture and has
// something in it. Bars a camera put round a thumbnail are flat, and are
// not something a crop took away.
func outsideShare(c grid, p placement) float64 {
	// How flat the outside is, is judged a few pixels away from the
	// picture: JPEG rings at the edge of a bar, and that ringing made a
	// camera's letterboxed thumbnail look like a crop.
	const ring = 3
	var outside, far []float64
	for y := 0; y < c.h; y++ {
		for x := 0; x < c.w; x++ {
			if x >= p.x && x < p.x+p.w && y >= p.y && y < p.y+p.h {
				continue
			}
			v := float64(c.at(x, y))
			outside = append(outside, v)
			if x < p.x-ring || x >= p.x+p.w+ring || y < p.y-ring || y >= p.y+p.h+ring {
				far = append(far, v)
			}
		}
	}
	if len(outside)*50 < len(c.v) {
		return 0 // two per cent is the rounding of a thumbnail's size
	}
	if len(far) > 0 {
		sort.Float64s(far)
		mid := far[len(far)/2]
		flat := 0
		for _, v := range far {
			if math.Abs(v-mid) <= 6 {
				flat++
			}
		}
		// All of it, near enough: the margin of a letter is paper too, and
		// the two lines of text left in it are what the crop took off.
		if flat*100 >= len(far)*99 {
			return 0
		}
	}
	return float64(len(outside)) / float64(len(c.v))
}

// changedParts compares the picture with the copy where it sits, block by
// block, and gathers the blocks where the copy shows something the picture
// does not into rectangles in the picture's own pixels.
//
// Brightness is no use for this. Phones make the thumbnail from a frame
// tone-mapped on its own, so the sky in one is lighter than in the other
// and a straight comparison flagged a third of the first thirty real
// photos tried. What an edit that hides something changes is the shape
// of what is there: a box or a blur leaves a flat patch where the copy
// has edges, and a patch painted over has different edges from the copy.
// So each block is compared by its own correlation, which does not care
// how light or dark either is, and a block where the copy is flat says
// nothing either way and is skipped. A block where the picture has edges
// and the copy does not is something added, a caption or a watermark,
// which hides nothing, and is skipped too.
func changedParts(m, c grid, p placement, frame image.Rectangle) (parts []image.Rectangle, share float64, shifted int) {
	// Both are softened first. A thumbnail is saved at a low quality, and
	// the edges of its eight-pixel blocks are edges the picture does not
	// have: on a real Pixel thumbnail they flagged a classroom with nothing
	// changed in it.
	small := m.resized(p.w, p.h).softened()
	cp := c.part(p).softened()
	const blk = 6
	bw, bh := p.w/blk, p.h/blk
	if bw < 2 || bh < 2 {
		return nil, 0, 0
	}
	// A block is flagged flat (the picture has nothing where the copy has
	// something: a box, a blur) or unlike (both have something, and it is
	// not the same thing).
	const flat, unlikeBlock = 1, 2
	flagged := make([]int8, bw*bh)
	for by := 0; by < bh; by++ {
		for bx := 0; bx < bw; bx++ {
			var sa, sb, saa, sbb, sab float64
			for y := by * blk; y < (by+1)*blk; y++ {
				for x := bx * blk; x < (bx+1)*blk; x++ {
					a := float64(small.v[y*p.w+x])
					b := float64(cp.v[y*p.w+x])
					sa += a
					sb += b
					saa += a * a
					sbb += b * b
					sab += a * b
				}
			}
			n := float64(blk * blk)
			va := math.Max(0, saa/n-(sa/n)*(sa/n))
			vb := math.Max(0, sbb/n-(sb/n)*(sb/n))
			sdPicture, sdCopy := math.Sqrt(va), math.Sqrt(vb)
			if sdCopy < copyFlat {
				continue
			}
			switch {
			case sdPicture < pictureFlat && sdCopy >= copyEdges:
				flagged[by*bw+bx] = flat
			case sdPicture >= pictureFlat && sdCopy >= copyEdges:
				r := (sab/n - (sa/n)*(sb/n)) / (sdPicture * sdCopy)
				if r < unlike {
					flagged[by*bw+bx] = unlikeBlock
				}
			}
		}
	}
	// Gather neighbouring blocks, and keep the groups of three or more:
	// one block on its own is the compression of one edge more often than
	// it is anything somebody did.
	seen := make([]bool, len(flagged))
	kept := 0
	sx := float64(frame.Dx()) / float64(p.w)
	sy := float64(frame.Dy()) / float64(p.h)
	for i := range flagged {
		if flagged[i] == 0 || seen[i] {
			continue
		}
		stack := []int{i}
		seen[i] = true
		x0, y0, x1, y1, size := bw, bh, -1, -1, 0
		anyFlat := false
		for len(stack) > 0 {
			k := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			bx, by := k%bw, k/bw
			size++
			anyFlat = anyFlat || flagged[k] == flat
			x0, y0, x1, y1 = min(x0, bx), min(y0, by), max(x1, bx), max(y1, by)
			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					nx, ny := bx+dx, by+dy
					if nx < 0 || ny < 0 || nx >= bw || ny >= bh {
						continue
					}
					j := ny*bw + nx
					if flagged[j] != 0 && !seen[j] {
						seen[j] = true
						stack = append(stack, j)
					}
				}
			}
		}
		// Only a part with something flat in it counts: the picture has
		// nothing where the copy has something, which is what a box or a
		// blur leaves. A part that is only unlike, the same things in both
		// but a few pixels apart, is lens correction moving them, and in
		// the calibration it was every one of the eighteen Pixel photos
		// that differed from their own thumbnails in the middle. Something
		// painted over with something plausible is missed for this, and
		// the README says so. The unlike parts are still counted, for the
		// straightened picture.
		if !anyFlat {
			if size >= minPart {
				shifted++
			}
			continue
		}
		if size < minPart || !stillDiffers(small, cp, x0*blk, y0*blk, (x1+1)*blk, (y1+1)*blk) {
			continue
		}
		kept += size
		parts = append(parts, image.Rect(
			frame.Min.X+int(float64(x0*blk)*sx), frame.Min.Y+int(float64(y0*blk)*sy),
			frame.Min.X+int(math.Ceil(float64((x1+1)*blk)*sx)), frame.Min.Y+int(math.Ceil(float64((y1+1)*blk)*sy))))
	}
	sort.Slice(parts, func(i, j int) bool {
		return parts[i].Dx()*parts[i].Dy() > parts[j].Dx()*parts[j].Dy()
	})
	return parts, float64(kept) / float64(bw*bh), shifted
}

// The lines for changedParts, in grey levels of 0 to 255 at the copy's
// size. Set on real photos and their thumbnails, see the calibration in
// the README.
const (
	copyFlat    = 3.0 // a copy block with less than this says nothing
	copyEdges   = 8.0 // a copy block with this much has something in it
	pictureFlat = 1.5 // a picture block with less than this has nothing in it
	unlike      = 0.2 // below this the two blocks show different things
	minPart     = 3   // blocks in a group before it counts
)

// stillDiffers looks at a group of changed blocks as one patch. Taken
// together, blocks that each looked different because of noise turn out
// to be the same thing; a part that was covered or painted over is
// different as a whole.
func stillDiffers(m, c grid, x0, y0, x1, y1 int) bool {
	var sa, sb, saa, sbb, sab float64
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			a, b := float64(m.at(x, y)), float64(c.at(x, y))
			sa += a
			sb += b
			saa += a * a
			sbb += b * b
			sab += a * b
		}
	}
	n := float64((x1 - x0) * (y1 - y0))
	sdM := math.Sqrt(math.Max(0, saa/n-(sa/n)*(sa/n)))
	sdC := math.Sqrt(math.Max(0, sbb/n-(sb/n)*(sb/n)))
	if sdM < pictureFlat*2 && sdC >= copyEdges {
		return true
	}
	if sdM < 1e-3 || sdC < 1e-3 {
		return false
	}
	return (sab/n-(sa/n)*(sb/n))/(sdM*sdC) < 0.5
}

// part is the piece of a grid under a placement.
func (g grid) part(p placement) grid {
	out := grid{w: p.w, h: p.h, v: make([]float32, p.w*p.h)}
	for y := 0; y < p.h; y++ {
		copy(out.v[y*p.w:(y+1)*p.w], g.v[(p.y+y)*g.w+p.x:(p.y+y)*g.w+p.x+p.w])
	}
	return out
}

// softened is a three by three average.
func (g grid) softened() grid {
	out := grid{w: g.w, h: g.h, v: make([]float32, len(g.v))}
	for y := 0; y < g.h; y++ {
		for x := 0; x < g.w; x++ {
			var s, n float32
			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					xx, yy := x+dx, y+dy
					if xx >= 0 && yy >= 0 && xx < g.w && yy < g.h {
						s += g.at(xx, yy)
						n++
					}
				}
			}
			out.v[y*g.w+x] = s / n
		}
	}
	return out
}

// lensMoved is the pattern that distortion correction leaves: every part
// that differs is near the edge of the frame, and they are near more than
// one edge. A Pixel's ultra-wide photo of a classroom differed from its
// own thumbnail in five places along three edges and nowhere else. An
// edit covers something where it is, and two covers that both hug the
// frame on different sides, with nothing else changed, are far rarer
// than a lens.
func lensMoved(parts []image.Rectangle, frame image.Rectangle) bool {
	if len(parts) < 2 {
		return false
	}
	mx, my := frame.Dx()/10, frame.Dy()/10
	inner := image.Rect(frame.Min.X+mx, frame.Min.Y+my, frame.Max.X-mx, frame.Max.Y-my)
	sides := map[string]bool{}
	for _, p := range parts {
		if p.In(image.Rect(inner.Min.X, frame.Min.Y, inner.Max.X, frame.Max.Y)) && p.In(image.Rect(frame.Min.X, inner.Min.Y, frame.Max.X, inner.Max.Y)) {
			return false // inside the margin on both axes: in the middle
		}
		switch {
		case p.Min.X < inner.Min.X:
			sides["left"] = true
		case p.Max.X > inner.Max.X:
			sides["right"] = true
		}
		switch {
		case p.Min.Y < inner.Min.Y:
			sides["top"] = true
		case p.Max.Y > inner.Max.Y:
			sides["bottom"] = true
		}
	}
	return len(sides) >= 2
}

// trimmedNCC is the correlation over all but the fifteen per cent of
// pixels that fit worst, after the copy is mapped onto the picture with a
// straight line.
func trimmedNCC(small, c grid, p placement) float64 {
	n := p.w * p.h
	a := make([]float64, n)
	b := make([]float64, n)
	var sa, sb, saa, sab float64
	for y := 0; y < p.h; y++ {
		for x := 0; x < p.w; x++ {
			i := y*p.w + x
			a[i] = float64(small.v[i])
			b[i] = float64(c.at(p.x+x, p.y+y))
			sa += a[i]
			sb += b[i]
			saa += a[i] * a[i]
			sab += a[i] * b[i]
		}
	}
	fn := float64(n)
	gain := 1.0
	if d := saa - sa*sa/fn; d > 1e-6 {
		gain = (sab - sa*sb/fn) / d
	}
	offset := (sb - gain*sa) / fn
	res := make([]float64, n)
	for i := range res {
		res[i] = math.Abs(b[i] - (gain*a[i] + offset))
	}
	sorted := append([]float64(nil), res...)
	sort.Float64s(sorted)
	cut := sorted[n*85/100]
	var ka, kb, kaa, kbb, kab, kn float64
	for i := range res {
		if res[i] > cut {
			continue
		}
		ka += a[i]
		kb += b[i]
		kaa += a[i] * a[i]
		kbb += b[i] * b[i]
		kab += a[i] * b[i]
		kn++
	}
	va := kaa - ka*ka/kn
	vb := kbb - kb*kb/kn
	if va < 1e-6 || vb < 1e-6 {
		return 0
	}
	return (kab - ka*kb/kn) / math.Sqrt(va*vb)
}

// awayFromEdges drops the parts that touch the outer band of the frame.
// Lens correction moves the edges of a photo against its thumbnail by
// different amounts in different places, which no placement undoes: in
// the calibration, twenty three Pixel photos differed from their own
// thumbnail in one place each, every one of them against an edge, and
// none of them was an edit. A cover at the very edge of a picture is
// missed for this, and the README says so.
func awayFromEdges(parts []image.Rectangle, frame image.Rectangle) []image.Rectangle {
	mx, my := frame.Dx()*6/100, frame.Dy()*6/100
	inner := image.Rect(frame.Min.X+mx, frame.Min.Y+my, frame.Max.X-mx, frame.Max.Y-my)
	var out []image.Rectangle
	for _, p := range parts {
		if p.In(inner) {
			out = append(out, p)
		}
	}
	return out
}
