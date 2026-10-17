package main

// What a cover leaves in the pixels themselves.
//
// Two kinds of cover keep part of what they cover. A box that is not
// quite opaque is a dimmer copy of what is under it, and stretching the
// contrast inside it brings that back. A mosaic keeps the average colour
// of every block, and the averages of text in a known font can be matched
// (Depix did it in 2020). A solid box keeps nothing, and is not reported.
// A blur is not looked for: telling a blur somebody applied from a part
// of a photo that is out of focus is not something this can do well
// enough to say.

import (
	"fmt"
	"image"
	"sort"
)

func (c *run) pixels(where string, data []byte) {
	img, err := decodePicture(data)
	if err != nil {
		c.gap(where, "the picture could not be decoded, so its pixels were not looked at")
		return
	}
	c.pixelsOf(where, img)
}

func (c *run) pixelsOf(where string, img image.Image) {
	b := img.Bounds()
	if b.Dx() < 8 || b.Dy() < 4 || b.Dx()*b.Dy() > 80<<20 {
		return
	}
	px := packed(img)
	lossless := !isYCbCr(img)
	for _, m := range mosaics(px, b.Dx(), b.Dy(), lossless) {
		c.hit("mosaic", where, fmt.Sprintf("%dx%d at %d,%d, in blocks of %d pixels, %d across and %d down.",
			m.r.Dx(), m.r.Dy(), b.Min.X+m.r.Min.X, b.Min.Y+m.r.Min.Y, m.block, m.r.Dx()/m.block, max(1, m.r.Dy()/m.block)))
	}
	for _, t := range seeThrough(px, b.Dx(), b.Dy(), lossless) {
		c.hit("shows-through", where, fmt.Sprintf("%dx%d at %d,%d: what is under it differs by %d levels out of 255, so stretching the contrast about %dx shows it.",
			t.r.Dx(), t.r.Dy(), b.Min.X+t.r.Min.X, b.Min.Y+t.r.Min.Y, t.spread, max(2, 255/max(1, t.spread))))
	}
}

func isYCbCr(img image.Image) bool {
	_, ok := img.(*image.YCbCr)
	return ok
}

// packed is the picture as one 24-bit colour per pixel, row by row.
func packed(img image.Image) []uint32 {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	out := make([]uint32, w*h)
	switch p := img.(type) {
	case *image.NRGBA:
		for y := 0; y < h; y++ {
			row := p.Pix[y*p.Stride:]
			for x := 0; x < w; x++ {
				out[y*w+x] = uint32(row[4*x])<<16 | uint32(row[4*x+1])<<8 | uint32(row[4*x+2])
			}
		}
		return out
	case *image.RGBA:
		for y := 0; y < h; y++ {
			row := p.Pix[y*p.Stride:]
			for x := 0; x < w; x++ {
				out[y*w+x] = uint32(row[4*x])<<16 | uint32(row[4*x+1])<<8 | uint32(row[4*x+2])
			}
		}
		return out
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r, g, bl, _ := img.At(b.Min.X+x, b.Min.Y+y).RGBA()
			out[y*w+x] = (r>>8)<<16 | (g>>8)<<8 | bl>>8
		}
	}
	return out
}

func luma(c uint32) int {
	return (299*int(c>>16&0xff) + 587*int(c>>8&0xff) + 114*int(c&0xff)) / 1000
}

func near(a, b uint32, tol int) bool {
	if tol == 0 {
		return a == b
	}
	for s := 0; s < 24; s += 8 {
		d := int(a>>s&0xff) - int(b>>s&0xff)
		if d > tol || d < -tol {
			return false
		}
	}
	return true
}

type mosaic struct {
	r     image.Rectangle
	block int
}

// mosaics finds parts of the picture made of square blocks of one colour.
//
// In each row, a mosaic changes colour only on a grid: every place where
// one pixel differs from the next is a whole number of blocks from the
// others. Neighbouring blocks of the same colour run together, which is
// most of them over text, where every block between words is the paper's
// colour; so the gaps are multiples of the block, not the block. Rows
// that agree on the block and the grid are stacked into regions, and a
// region is a mosaic when its rows also change only every block. JPEG
// blurs the edges of every block, so there a colour is a few levels either
// way, and only blocks of eight and up are looked for.
func mosaics(px []uint32, w, h int, lossless bool) []mosaic {
	tol, minBlock := 0, 3
	if !lossless {
		tol, minBlock = 6, 8
	}
	const maxBlock = 64
	type key struct{ block, phase int }
	type span struct{ x0, x1, y0, y1 int }
	open := map[key][]span{}
	var done []mosaic
	var edges []int
	for y := 0; y <= h; y++ {
		found := map[key][]span{}
		if y < h {
			line := px[y*w : (y+1)*w]
			edges = edges[:0]
			for x := 1; x < w; x++ {
				if !near(line[x], line[x-1], tol) {
					edges = append(edges, x)
				}
			}
			for block := minBlock; block <= maxBlock; block++ {
				for i := 0; i < len(edges); {
					j, exact := i, 0
					for j+1 < len(edges) {
						gap := edges[j+1] - edges[j]
						if gap%block != 0 || gap > 4*block {
							break
						}
						if gap == block {
							exact++
						}
						j++
					}
					// Four edges on the grid, two of them a block apart:
					// three gaps at random falling on multiples of the same
					// number is rare, and text that is not a mosaic has its
					// edges everywhere.
					if j-i >= 3 && exact >= 2 {
						k := key{block, edges[i] % block}
						found[k] = append(found[k], span{edges[i], edges[j], y, y + 1})
					}
					i = max(j, i+1)
				}
			}
		}
		// Carry each open region on, or close it.
		next := map[key][]span{}
		for k, spans := range open {
			for _, s := range spans {
				joined := false
				// A row carries a region on only where a mosaic would: the same
				// as the row above, or a whole number of blocks below the top.
				// Without this the region of a test mosaic ran on into the text
				// under it, wherever a line of it happened to fit the grid.
				for n, f := range found[k] {
					lo, hi := max(s.x0, f.x0), min(s.x1, f.x1)
					if lo < hi && (sameRow(px, w, y, lo, hi, tol) || (y-s.y0)%k.block == 0) {
						// Two regions can reach the same row; they become one, and
						// keep the top of the older, which the second used to
						// overwrite.
						found[k][n] = span{min(s.x0, f.x0), max(s.x1, f.x1), min(s.y0, f.y0), y + 1}
						joined = true
						break
					}
				}
				// A band of blocks that are nearly all the paper's colour has no
				// edges to find, and must not end the region: it stays open
				// two blocks' worth of rows.
				if !joined && y < h && y-s.y1 < 2*k.block && ((y-s.y0)%k.block == 0 || sameRow(px, w, y, s.x0+k.block, s.x1-k.block, tol)) {
					next[k] = append(next[k], s)
					continue
				}
				if !joined {
					r := image.Rect(max(0, s.x0), s.y0, min(w, s.x1), s.y1)
					if m, ok := checkMosaic(px, w, r, k.block, tol); ok {
						done = append(done, m)
					}
				}
			}
		}
		for k, f := range found {
			next[k] = append(next[k], f...)
		}
		open = next
	}
	return mergeMosaics(done)
}

// checkMosaic decides whether a stack of equal runs is a mosaic: blocks
// at least a block tall, rows that change only where a block ends, and
// enough different colours that it is not a pattern of two, which is what
// a QR code, a checkerboard or a dashed line is.
func checkMosaic(px []uint32, w int, r image.Rectangle, block, tol int) (mosaic, bool) {
	if r.Dy() < block {
		return mosaic{}, false
	}
	// The edges of a region pick up a block's width of whatever is beside
	// the mosaic, where a line of text happened to fit the grid. Columns
	// of blocks that change between rows inside a band are not mosaic, and
	// come off the sides.
	offGrid := func(x0 int) bool {
		for y := r.Min.Y + 1; y < r.Max.Y; y++ {
			if (y-r.Min.Y)%block == 0 {
				continue
			}
			for x := x0; x < min(x0+block, r.Max.X); x++ {
				if !near(px[y*w+x], px[(y-1)*w+x], tol) {
					return true
				}
			}
		}
		return false
	}
	for r.Dx() >= block && offGrid(r.Min.X) {
		r.Min.X += block
	}
	for r.Dx() >= block && offGrid(r.Max.X-block) {
		r.Max.X -= block
	}
	if r.Dx() < 3*block {
		return mosaic{}, false
	}
	// Where the rows change, from the top of the stack.
	var changes []int
	for y := r.Min.Y + 1; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x += block {
			if !near(px[y*w+x], px[(y-1)*w+x], tol) {
				changes = append(changes, y-r.Min.Y)
				break
			}
		}
	}
	for _, c := range changes {
		if c%block != 0 {
			return mosaic{}, false
		}
	}
	// Two rows of blocks at least. One row of equal blocks in different
	// colours is a tab bar or a row of swatches as often as it is a mosaic,
	// and two real screenshots of eighty nine had one.
	bands := r.Dy() / block
	if bands < 2 {
		return mosaic{}, false
	}
	if bands > 1 && len(changes) < (bands-1)/2 {
		// A gradient bar is equal runs too, and the same all the way down.
		return mosaic{}, false
	}
	colours := map[uint32]bool{}
	for y := r.Min.Y; y < r.Max.Y; y += block {
		for x := r.Min.X; x < r.Max.X; x += block {
			colours[px[y*w+x]&0xf0f0f0] = true
		}
	}
	// JPEG compresses in blocks of eight, and a smooth sky compressed hard
	// is flat blocks of eight on that same grid, a mosaic in every way but
	// one: neighbouring blocks differ by a few levels, where the blocks of
	// a mosaic over text differ by tens. Two Pixel photos of six had skies
	// like that.
	if tol > 0 && block%8 == 0 && r.Min.X%8 == 0 && r.Min.Y%8 == 0 && medianStep(px, w, r, block) < 16 {
		return mosaic{}, false
	}
	// Six blocks across and twelve in all: a word under a mosaic is at
	// least that, and smaller patches of squares in real screenshots were
	// icons, four blocks wide, every one of them.
	across := r.Dx() / block
	// In a photo, patches of six or seven blocks of eight came out of JPEG
	// itself twice in nine hundred, both under seventy pixels wide in
	// photos four thousand wide; the one real mosaic in the same photos,
	// a karaoke screen showing its own pixelated preview, was five hundred.
	if tol > 0 && r.Dx() < max(64, w/60) {
		return mosaic{}, false
	}
	if len(colours) < 4 || across < 6 || across*bands < 12 {
		return mosaic{}, false
	}
	return mosaic{r: r, block: block}, true
}

// mergeMosaics joins pieces of one mosaic that a row of a different colour
// at its edge split, and drops the ones inside another.
func mergeMosaics(ms []mosaic) []mosaic {
	// Joining two can bring a third within reach, so it goes round until
	// nothing more joins.
	for {
		n := len(ms)
		ms = mergeOnce(ms)
		if len(ms) == n {
			return ms
		}
	}
}

func mergeOnce(ms []mosaic) []mosaic {
	sort.Slice(ms, func(i, j int) bool { return ms[i].r.Dx()*ms[i].r.Dy() > ms[j].r.Dx()*ms[j].r.Dy() })
	var out []mosaic
	for _, m := range ms {
		merged := false
		for k := range out {
			// Three blocks of slack across: a space between two words
			// in a pixelated line is a block or two of one colour, and cut the
			// line in a Depix test picture into three.
			grown := image.Rect(out[k].r.Min.X-3*out[k].block, out[k].r.Min.Y-out[k].block, out[k].r.Max.X+3*out[k].block, out[k].r.Max.Y+out[k].block)
			if out[k].block == m.block && grown.Overlaps(m.r) {
				out[k].r = out[k].r.Union(m.r)
				merged = true
				break
			}
		}
		if !merged {
			out = append(out, m)
		}
	}
	return out
}

type cover struct {
	r      image.Rectangle
	spread int // how far apart the levels inside it are
}

// seeThrough finds boxes that let what is under them through.
//
// A box is found as a region of gently changing pixels with straight
// edges and a sharp step all round it. Inside, a box drawn at ninety per
// cent over black text on white is levels 0 and 25 where the text and
// the paper were: a spread of a few levels, in strokes. A solid box has
// no spread at all, and a panel of a program's window with text in it has
// a spread far bigger than any cover lets through.
func seeThrough(px []uint32, w, h int, lossless bool) []cover {
	const join = 48 // neighbours closer than this are one region
	lum := make([]uint8, len(px))
	for i, c := range px {
		lum[i] = uint8(luma(c))
	}
	minSpread, maxSpread := 3, join
	if !lossless {
		// A photo has real rectangles with faint text in them, a printed
		// card in a shop at forty six levels in the first camera photos
		// tried, so in a JPEG only a cover of eighty five per cent and up
		// is looked for.
		minSpread, maxSpread = 14, 36
	}
	labels := regions(lum, w, h, join)
	type box struct {
		x0, y0, x1, y1, n int
	}
	// Most regions of a photo are a few pixels of noise. They are counted
	// first, so only the big ones get a box; a map of every region made a
	// twelve megapixel photo take seconds.
	counts := make([]int32, len(labels))
	for _, l := range labels {
		counts[l]++
	}
	boxes := map[int32]*box{}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			l := labels[y*w+x]
			if counts[l] < 400 {
				continue
			}
			bx := boxes[l]
			if bx == nil {
				bx = &box{x, y, x, y, 0}
				boxes[l] = bx
			}
			bx.x0, bx.y0, bx.x1, bx.y1 = min(bx.x0, x), min(bx.y0, y), max(bx.x1, x), max(bx.y1, y)
			bx.n++
		}
	}
	var out []cover
	for l, bx := range boxes {
		// A stroke of text that crosses the edge of a box is close enough
		// to the box's colour to join it, and sticks out of it. The box
		// is the run of rows and of columns that are nearly all region.
		bx.x0, bx.y0, bx.x1, bx.y1 = core(labels, w, l, bx.x0, bx.y0, bx.x1, bx.y1)
		bw, bh := bx.x1-bx.x0+1, bx.y1-bx.y0+1
		if bw < 16 || bh < 8 || bw*bh < 400 || bw*bh > w*h/3 {
			continue
		}
		// In a photo, a small bright rectangle with faint marks in it is a
		// window, a sign or a label far more often than a cover: all
		// thirteen found in nine hundred phone photos were, every one of
		// them narrower than a fiftieth of the photo.
		if !lossless && (bw < max(40, w/50) || bh < 16) {
			continue
		}
		inside := 0
		for y := bx.y0; y <= bx.y1; y++ {
			for x := bx.x0; x <= bx.x1; x++ {
				if labels[y*w+x] == l {
					inside++
				}
			}
		}
		if float64(inside) < 0.98*float64(bw*bh) {
			continue
		}
		if !steppedEdges(lum, labels, w, h, l, bx.x0, bx.y0, bx.x1, bx.y1) {
			continue
		}
		// Inside, away from the edge, where a JPEG rings.
		var vals []int
		for y := bx.y0 + 2; y <= bx.y1-2; y++ {
			for x := bx.x0 + 2; x <= bx.x1-2; x++ {
				vals = append(vals, int(lum[y*w+x]))
			}
		}
		if len(vals) < 100 {
			continue
		}
		sorted := append([]int(nil), vals...)
		sort.Ints(sorted)
		lo, hi := sorted[len(sorted)/200], sorted[len(sorted)-1-len(sorted)/200]
		spread := hi - lo
		if spread < minSpread || spread > maxSpread {
			continue
		}
		mode := modeOf(sorted)
		if framed(lum, w, h, bx.x0, bx.y0, bx.x1, bx.y1, mode) {
			continue
		}
		if !strokes(lum, w, bx.x0+2, bx.y0+2, bx.x1-2, bx.y1-2, mode, max(2, spread/3)) {
			continue
		}
		out = append(out, cover{image.Rect(bx.x0, bx.y0, bx.x1+1, bx.y1+1), spread})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].r.Min.Y != out[j].r.Min.Y {
			return out[i].r.Min.Y < out[j].r.Min.Y
		}
		return out[i].r.Min.X < out[j].r.Min.X
	})
	return out
}

// regions labels the picture into regions of neighbours that differ by
// less than join, with a union-find over the grid.
func regions(lum []uint8, w, h, join int) []int32 {
	parent := make([]int32, len(lum))
	for i := range parent {
		parent[i] = int32(i)
	}
	var find func(int32) int32
	find = func(i int32) int32 {
		for parent[i] != i {
			parent[i] = parent[parent[i]]
			i = parent[i]
		}
		return i
	}
	// By rank, so the trees stay shallow: without it, finding the root
	// was most of the time spent on a photo.
	rank := make([]uint8, len(lum))
	union := func(a, b int32) {
		ra, rb := find(a), find(b)
		switch {
		case ra == rb:
		case rank[ra] < rank[rb]:
			parent[ra] = rb
		case rank[ra] > rank[rb]:
			parent[rb] = ra
		default:
			parent[rb] = ra
			rank[ra]++
		}
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := int32(y*w + x)
			v := int(lum[i])
			if x+1 < w && abs(v-int(lum[i+1])) < join {
				union(i, i+1)
			}
			if y+1 < h && abs(v-int(lum[int(i)+w])) < join {
				union(i, int32(int(i)+w))
			}
		}
	}
	for i := range parent {
		parent[i] = find(int32(i))
	}
	return parent
}

// steppedEdges asks whether the region is a rectangle with a sharp step
// along its sides: of the sides not at the edge of the picture, eight
// pixels in ten just outside differ sharply from the one just inside. Not
// all of them, because a line of text runs on past the edge of a box that
// covers part of it, and dark text beside a dark box is no step.
func steppedEdges(lum []uint8, labels []int32, w, h int, l int32, x0, y0, x1, y1 int) bool {
	sides, good := 0, 0
	check := func(n int, inside, outside func(k int) int) {
		steps := 0
		for k := 0; k < n; k++ {
			if abs(int(lum[inside(k)])-int(lum[outside(k)])) >= 48 {
				steps++
			}
		}
		sides++
		if steps*10 >= n*8 {
			good++
		}
	}
	if y0 > 0 {
		check(x1-x0+1, func(k int) int { return y0*w + x0 + k }, func(k int) int { return (y0-1)*w + x0 + k })
	}
	if y1 < h-1 {
		check(x1-x0+1, func(k int) int { return y1*w + x0 + k }, func(k int) int { return (y1+1)*w + x0 + k })
	}
	if x0 > 0 {
		check(y1-y0+1, func(k int) int { return (y0+k)*w + x0 }, func(k int) int { return (y0+k)*w + x0 - 1 })
	}
	if x1 < w-1 {
		check(y1-y0+1, func(k int) int { return (y0+k)*w + x1 }, func(k int) int { return (y0+k)*w + x1 + 1 })
	}
	return sides >= 2 && good == sides
}

// strokes asks whether what differs from the box's own colour inside it is
// shaped like something: three or more separate marks of a few pixels
// each, covering a small part of the box. Noise is single pixels
// everywhere; a gradient is everything.
func strokes(lum []uint8, w, x0, y0, x1, y1, mode, by int) bool {
	bw, bh := x1-x0+1, y1-y0+1
	if bw < 4 || bh < 4 {
		return false
	}
	off := make([]bool, bw*bh)
	count := 0
	for y := 0; y < bh; y++ {
		for x := 0; x < bw; x++ {
			if abs(int(lum[(y0+y)*w+x0+x])-mode) >= by {
				off[y*bw+x] = true
				count++
			}
		}
	}
	share := float64(count) / float64(bw*bh)
	if share < 0.005 || share > 0.6 {
		return false
	}
	marks := 0
	seen := make([]bool, len(off))
	for i := range off {
		if !off[i] || seen[i] {
			continue
		}
		size := 0
		stack := []int{i}
		seen[i] = true
		for len(stack) > 0 {
			k := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			size++
			x, y := k%bw, k/bw
			for _, d := range [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
				nx, ny := x+d[0], y+d[1]
				if nx >= 0 && ny >= 0 && nx < bw && ny < bh {
					j := ny*bw + nx
					if off[j] && !seen[j] {
						seen[j] = true
						stack = append(stack, j)
					}
				}
			}
		}
		if size >= 4 {
			marks++
		}
	}
	return marks >= 3
}

func modeOf(sorted []int) int {
	best, bestN, n := sorted[0], 0, 0
	for i := range sorted {
		if i > 0 && sorted[i] == sorted[i-1] {
			n++
		} else {
			n = 1
		}
		if n > bestN {
			best, bestN = sorted[i], n
		}
	}
	return best
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// framed is the inside of an outline: the step at the edge is a line a
// few pixels wide, and past it is the same colour as inside. The square
// of Android's Recents button, white on purple with purple inside, came
// out as a box over something in the first real screenshots tried. A
// cover is a colour of its own, and what is past its edge is not it.
func framed(lum []uint8, w, h, x0, y0, x1, y1, mode int) bool {
	same, looked := 0, 0
	probe := func(x, y int) {
		if x < 0 || y < 0 || x >= w || y >= h {
			return
		}
		looked++
		if abs(int(lum[y*w+x])-mode) < 24 {
			same++
		}
	}
	for d := 3; d <= 16; d++ {
		for x := x0; x <= x1; x += max(1, (x1-x0)/16) {
			probe(x, y0-d)
			probe(x, y1+d)
		}
		for y := y0; y <= y1; y += max(1, (y1-y0)/16) {
			probe(x0-d, y)
			probe(x1+d, y)
		}
	}
	return looked > 0 && same*2 > looked
}

func sameRow(px []uint32, w, y, x0, x1, tol int) bool {
	if y == 0 {
		return false
	}
	for x := max(0, x0); x < min(w, x1); x++ {
		if !near(px[y*w+x], px[(y-1)*w+x], tol) {
			return false
		}
	}
	return true
}

// core trims a region's bounding box to the longest run of rows, and then
// of columns, in which nine pixels in ten belong to it.
func core(labels []int32, w int, l int32, x0, y0, x1, y1 int) (int, int, int, int) {
	run := func(n int, full func(int) bool) (int, int) {
		bestA, bestB, a := 0, -1, -1
		for i := 0; i <= n; i++ {
			if i < n && full(i) {
				if a < 0 {
					a = i
				}
				continue
			}
			if a >= 0 && i-1-a > bestB-bestA {
				bestA, bestB = a, i-1
			}
			a = -1
		}
		return bestA, bestB
	}
	rowFull := func(i int) bool {
		y, n := y0+i, 0
		for x := x0; x <= x1; x++ {
			if labels[y*w+x] == l {
				n++
			}
		}
		return n*10 >= (x1-x0+1)*9
	}
	a, b := run(y1-y0+1, rowFull)
	if b < a {
		return x0, y0, x0 - 1, y0 - 1
	}
	y0, y1 = y0+a, y0+b
	colFull := func(i int) bool {
		x, n := x0+i, 0
		for y := y0; y <= y1; y++ {
			if labels[y*w+x] == l {
				n++
			}
		}
		return n*10 >= (y1-y0+1)*9
	}
	a, b = run(x1-x0+1, colFull)
	if b < a {
		return x0, y0, x0 - 1, y0 - 1
	}
	return x0 + a, y0, x0 + b, y1
}

// medianStep is the median difference in grey between blocks side by side
// that differ at all. Over text most neighbours are both paper, and
// counting those made every mosaic over text look as flat as a sky.
func medianStep(px []uint32, w int, r image.Rectangle, block int) int {
	var steps []int
	for y := r.Min.Y + block/2; y < r.Max.Y; y += block {
		for x := r.Min.X + block/2; x+block < r.Max.X; x += block {
			if d := abs(luma(px[y*w+x]) - luma(px[y*w+x+block])); d > 6 {
				steps = append(steps, d)
			}
		}
	}
	if len(steps) == 0 {
		return 0
	}
	sort.Ints(steps)
	return steps[len(steps)/2]
}
