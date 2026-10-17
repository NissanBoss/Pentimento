package main

// A DEFLATE decoder that can start in the middle of a stream.
//
// What a cropping tool leaves behind the end of a PNG is the tail of the
// old picture's compressed data, cut at an arbitrary byte. compress/flate
// cannot start there: it needs the beginning, and it needs the 32 KB of
// output that every back reference may point into. This one starts at any
// bit, and treats everything before that bit as unknown. A copy from
// before the start copies unknowns, and those wash out as the stream goes
// on, because later references point at what was decoded here.

import "errors"

// unknown marks a byte of output whose value came from before the start.
const unknown = -1

const window = 32768

var errInflate = errors.New("not deflate")

type bitReader struct {
	in  []byte
	pos int // the next byte to load
	acc uint64
	n   uint // bits in acc
}

func (b *bitReader) need(n uint) bool {
	for b.n < n {
		if b.pos >= len(b.in) {
			return false
		}
		b.acc |= uint64(b.in[b.pos]) << b.n
		b.pos++
		b.n += 8
	}
	return true
}

func (b *bitReader) bits(n uint) (int, bool) {
	if !b.need(n) {
		return 0, false
	}
	v := int(b.acc & (1<<n - 1))
	b.acc >>= n
	b.n -= n
	return v, true
}

// consumed is how many bits have been read.
func (b *bitReader) consumed() int {
	return b.pos*8 - int(b.n)
}

// huffman is a canonical code, decoded a bit at a time. Slower than a
// table, but the streams here are a few hundred kilobytes and most starts
// tried fail within a few dozen symbols.
type huffman struct {
	count  [16]int // codes of each length
	symbol []int   // symbols in code order
}

func newHuffman(lengths []int, partialOK bool) (*huffman, error) {
	h := &huffman{}
	for _, l := range lengths {
		h.count[l]++
	}
	h.count[0] = 0
	left := 1
	for l := 1; l < 16; l++ {
		left <<= 1
		left -= h.count[l]
		if left < 0 {
			return nil, errInflate // more codes than the lengths allow
		}
	}
	// A code with room left over is legal only where zlib writes one: the
	// distance code with a single distance in it. Accepting it anywhere
	// else lets random bits pass for a block header far too often.
	if left > 0 && !partialOK {
		return nil, errInflate
	}
	var offs [16]int
	for l := 1; l < 15; l++ {
		offs[l+1] = offs[l] + h.count[l]
	}
	h.symbol = make([]int, len(lengths))
	for s, l := range lengths {
		if l != 0 {
			h.symbol[offs[l]] = s
			offs[l]++
		}
	}
	return h, nil
}

func (h *huffman) decode(b *bitReader) (int, error) {
	code, first, index := 0, 0, 0
	for l := 1; l < 16; l++ {
		bit, ok := b.bits(1)
		if !ok {
			return 0, errInflate
		}
		code |= bit
		count := h.count[l]
		if code-count < first {
			return h.symbol[index+(code-first)], nil
		}
		index += count
		first += count
		first <<= 1
		code <<= 1
	}
	return 0, errInflate
}

var (
	lengthBase  = [...]int{3, 4, 5, 6, 7, 8, 9, 10, 11, 13, 15, 17, 19, 23, 27, 31, 35, 43, 51, 59, 67, 83, 99, 115, 131, 163, 195, 227, 258}
	lengthExtra = [...]uint{0, 0, 0, 0, 0, 0, 0, 0, 1, 1, 1, 1, 2, 2, 2, 2, 3, 3, 3, 3, 4, 4, 4, 4, 5, 5, 5, 5, 0}
	distBase    = [...]int{1, 2, 3, 4, 5, 7, 9, 13, 17, 25, 33, 49, 65, 97, 129, 193, 257, 385, 513, 769, 1025, 1537, 2049, 3073, 4097, 6145, 8193, 12289, 16385, 24577}
	distExtra   = [...]uint{0, 0, 0, 0, 1, 1, 2, 2, 3, 3, 4, 4, 5, 5, 6, 6, 7, 7, 8, 8, 9, 9, 10, 10, 11, 11, 12, 12, 13, 13}
	codeOrder   = [...]int{16, 17, 18, 0, 8, 7, 9, 6, 10, 5, 11, 4, 12, 3, 13, 2, 14, 1, 15}
)

var fixedLit, fixedDist = func() (*huffman, *huffman) {
	var l [288]int
	for i := range l {
		switch {
		case i < 144:
			l[i] = 8
		case i < 256:
			l[i] = 9
		case i < 280:
			l[i] = 7
		default:
			l[i] = 8
		}
	}
	var d [30]int
	for i := range d {
		d[i] = 5
	}
	lit, _ := newHuffman(l[:], false)
	// Thirty codes of five bits leave two unused: incomplete by design.
	dist, _ := newHuffman(d[:], true)
	return lit, dist
}()

// inflated is what came out of a stream started in the middle.
type inflated struct {
	out []int16 // the bytes, or unknown
	// end is the bit after the final block, so the caller can check that
	// the stream ended where the data does.
	end    int
	blocks int
}

// inflateFrom decodes from bit start to the final block. limit caps the
// output, so a false start that happens to keep decoding cannot eat the
// memory of a machine. buf is reused from one try to the next: finding
// where a block starts means trying thousands of bits, and a fresh buffer
// for each was most of the time spent.
func inflateFrom(in []byte, start int, limit int, buf []int16) (*inflated, error) {
	b := &bitReader{in: in, pos: start / 8}
	if skip := uint(start % 8); skip > 0 {
		if _, ok := b.bits(skip); !ok {
			return nil, errInflate
		}
	}
	// The window before the start is not stored: a copy from further back
	// than what has been decoded comes out unknown.
	out := buf[:0]
	blocks := 0
	for {
		final, ok := b.bits(1)
		if !ok {
			return nil, errInflate
		}
		kind, ok := b.bits(2)
		if !ok {
			return nil, errInflate
		}
		var err error
		switch kind {
		case 0:
			out, err = storedBlock(b, out)
		case 1:
			out, err = codedBlock(b, out, fixedLit, fixedDist, limit)
		case 2:
			var lit, dist *huffman
			if lit, dist, err = dynamicTables(b); err == nil {
				out, err = codedBlock(b, out, lit, dist, limit)
			}
		default:
			err = errInflate
		}
		if err != nil {
			return nil, err
		}
		blocks++
		if final == 1 {
			return &inflated{out: out, end: b.consumed(), blocks: blocks}, nil
		}
	}
}

func storedBlock(b *bitReader, out []int16) ([]int16, error) {
	b.acc >>= b.n % 8
	b.n -= b.n % 8
	n, ok1 := b.bits(16)
	nn, ok2 := b.bits(16)
	if !ok1 || !ok2 || n != ^nn&0xffff {
		return nil, errInflate
	}
	for i := 0; i < n; i++ {
		v, ok := b.bits(8)
		if !ok {
			return nil, errInflate
		}
		out = append(out, int16(v))
	}
	return out, nil
}

func dynamicTables(b *bitReader) (*huffman, *huffman, error) {
	hlit, ok1 := b.bits(5)
	hdist, ok2 := b.bits(5)
	hclen, ok3 := b.bits(4)
	if !ok1 || !ok2 || !ok3 {
		return nil, nil, errInflate
	}
	nlit, ndist := hlit+257, hdist+1
	if nlit > 286 || ndist > 30 {
		return nil, nil, errInflate
	}
	var cl [19]int
	for i := 0; i < hclen+4; i++ {
		v, ok := b.bits(3)
		if !ok {
			return nil, nil, errInflate
		}
		cl[codeOrder[i]] = v
	}
	clh, err := newHuffman(cl[:], false)
	if err != nil {
		return nil, nil, err
	}
	lengths := make([]int, 0, nlit+ndist)
	for len(lengths) < nlit+ndist {
		sym, err := clh.decode(b)
		if err != nil {
			return nil, nil, err
		}
		switch {
		case sym < 16:
			lengths = append(lengths, sym)
		case sym == 16:
			if len(lengths) == 0 {
				return nil, nil, errInflate
			}
			r, ok := b.bits(2)
			if !ok {
				return nil, nil, errInflate
			}
			prev := lengths[len(lengths)-1]
			for i := 0; i < 3+r; i++ {
				lengths = append(lengths, prev)
			}
		case sym == 17:
			r, ok := b.bits(3)
			if !ok {
				return nil, nil, errInflate
			}
			for i := 0; i < 3+r; i++ {
				lengths = append(lengths, 0)
			}
		default:
			r, ok := b.bits(7)
			if !ok {
				return nil, nil, errInflate
			}
			for i := 0; i < 11+r; i++ {
				lengths = append(lengths, 0)
			}
		}
	}
	if len(lengths) > nlit+ndist || lengths[256] == 0 {
		return nil, nil, errInflate // a run past the end, or no way to end the block
	}
	lit, err := newHuffman(lengths[:nlit], false)
	if err != nil {
		return nil, nil, err
	}
	dists := lengths[nlit:]
	used := 0
	for _, l := range dists {
		if l > 0 {
			used++
		}
	}
	dist, err := newHuffman(dists, used <= 1)
	if err != nil {
		return nil, nil, err
	}
	return lit, dist, nil
}

func codedBlock(b *bitReader, out []int16, lit, dist *huffman, limit int) ([]int16, error) {
	for {
		sym, err := lit.decode(b)
		if err != nil {
			return nil, err
		}
		switch {
		case sym < 256:
			out = append(out, int16(sym))
			continue
		case sym == 256:
			return out, nil
		case sym > 285:
			return nil, errInflate
		}
		sym -= 257
		extra, ok := b.bits(lengthExtra[sym])
		if !ok {
			return nil, errInflate
		}
		n := lengthBase[sym] + extra
		ds, err := dist.decode(b)
		if err != nil || ds > 29 {
			return nil, errInflate
		}
		extra, ok = b.bits(distExtra[ds])
		if !ok {
			return nil, errInflate
		}
		d := distBase[ds] + extra
		from := len(out) - d
		for i := 0; i < n; i++ {
			if from+i < 0 {
				out = append(out, unknown)
			} else {
				out = append(out, out[from+i])
			}
		}
		if len(out) > limit {
			return nil, errInflate
		}
	}
}
