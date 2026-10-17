package main

// The end of an earlier JPEG, decoded.
//
// When the Snipping Tool saved a crop over the old file, what was left
// behind the new JPEG's end marker is the end of the old JPEG's scan: the
// compressed blocks of its bottom rows, with no header. The header is
// what the new picture overwrote, but the new picture was written by the
// same encoder with the same settings, and its tables are the old one's:
// on both pairs of Snipping Tool files published with CVE-2023-28303 the
// quantisation and Huffman tables are byte for byte the same. So the old
// blocks are decoded with the new picture's tables, from a point in the
// stream where a block begins.
//
// One thing cannot come back. A JPEG stores each block's average as the
// difference from the block before it, and the blocks before the tail are
// gone, so the tail's averages are all off by one amount, the same for
// every block of a colour. The picture comes back exact but for its overall
// brightness and tint, which is as readable as the original.

import (
	"bytes"
	"encoding/binary"
	"errors"
)

var errScan = errors.New("not this scan")

type jpegHuff struct {
	maxcode [18]int32
	valptr  [17]int32
	mincode [17]int32
	vals    []byte
}

func newJPEGHuff(counts []byte, vals []byte) *jpegHuff {
	h := &jpegHuff{vals: vals}
	code, k := int32(0), int32(0)
	for l := 1; l <= 16; l++ {
		n := int32(counts[l-1])
		if n == 0 {
			h.maxcode[l] = -1
		} else {
			h.valptr[l] = k
			h.mincode[l] = code
			code += n
			k += n
			h.maxcode[l] = code - 1
		}
		code <<= 1
	}
	h.maxcode[17] = 1 << 30
	return h
}

// scanBits reads the entropy-coded data with the stuffed zeros already out.
type scanBits struct {
	b   []byte
	pos int // in bits
}

func (s *scanBits) bit() (int32, bool) {
	if s.pos >= len(s.b)*8 {
		return 0, false
	}
	v := int32(s.b[s.pos>>3]>>(7-uint(s.pos&7))) & 1
	s.pos++
	return v, true
}

func (s *scanBits) bits(n int) (int32, bool) {
	var v int32
	for i := 0; i < n; i++ {
		b, ok := s.bit()
		if !ok {
			return 0, false
		}
		v = v<<1 | b
	}
	return v, true
}

func (h *jpegHuff) decode(s *scanBits) (byte, bool) {
	var code int32
	for l := 1; l <= 16; l++ {
		b, ok := s.bit()
		if !ok {
			return 0, false
		}
		code = code<<1 | b
		if h.maxcode[l] >= 0 && code <= h.maxcode[l] {
			i := h.valptr[l] + code - h.mincode[l]
			if int(i) >= len(h.vals) {
				return 0, false
			}
			return h.vals[i], true
		}
	}
	return 0, false
}

func extend(v int32, n int) int32 {
	if n == 0 {
		return 0
	}
	if v < 1<<(n-1) {
		return v - (1 << n) + 1
	}
	return v
}

// scanSetup is what the front picture says about how its scan is made.
type scanSetup struct {
	comps []scanComp
	mcuW  int // pixels across one MCU
	dcQ   int // the luma DC quantiser, to turn averages into levels
}

type scanComp struct {
	blocks int // in one MCU
	dc, ac *jpegHuff
}

// setupFrom reads the front picture's tables. Only a baseline JPEG with
// one scan of all its colours is something the tail can be read with.
func setupFrom(front []byte) (*scanSetup, bool) {
	dcT, acT := map[byte]*jpegHuff{}, map[byte]*jpegHuff{}
	type comp struct{ id, h, v, tq byte }
	var comps []comp
	q := map[byte]int{}
	hmax := byte(1)
	i := 2
	for i+4 <= len(front) && front[i] == 0xff {
		m := front[i+1]
		n := int(binary.BigEndian.Uint16(front[i+2:]))
		if i+2+n > len(front) {
			return nil, false
		}
		body := front[i+4 : i+2+n]
		switch m {
		case 0xc2, 0xc3, 0xc5, 0xc6, 0xc7, 0xc9, 0xca, 0xcb, 0xcd, 0xce, 0xcf:
			return nil, false // progressive, lossless or arithmetic
		case 0xdd:
			// A restart interval would make the tail easy, and exact; the
			// Snipping Tool does not write one, and it is not handled.
			if len(body) >= 2 && binary.BigEndian.Uint16(body) != 0 {
				return nil, false
			}
		case 0xdb:
			for k := 0; k < len(body); {
				prec, id := body[k]>>4, body[k]&15
				if prec == 0 && k+65 <= len(body) {
					q[id] = int(body[k+1])
					k += 65
				} else if k+129 <= len(body) {
					q[id] = int(binary.BigEndian.Uint16(body[k+1:]))
					k += 129
				} else {
					break
				}
			}
		case 0xc4:
			for k := 0; k+17 <= len(body); {
				class, id := body[k]>>4, body[k]&15
				counts := body[k+1 : k+17]
				total := 0
				for _, c := range counts {
					total += int(c)
				}
				if k+17+total > len(body) {
					return nil, false
				}
				t := newJPEGHuff(counts, body[k+17:k+17+total])
				if class == 0 {
					dcT[id] = t
				} else {
					acT[id] = t
				}
				k += 17 + total
			}
		case 0xc0, 0xc1:
			if len(body) < 6 {
				return nil, false
			}
			nc := int(body[5])
			for c := 0; c < nc && 6+3*c+2 < len(body); c++ {
				e := body[6+3*c:]
				comps = append(comps, comp{e[0], e[1] >> 4, e[1] & 15, e[2]})
				hmax = max(hmax, e[1]>>4)
			}
		case 0xda:
			if len(body) < 1 || int(body[0]) != len(comps) {
				return nil, false
			}
			s := &scanSetup{mcuW: 8 * int(hmax)}
			for c := 0; c < len(comps); c++ {
				if 1+2*c+1 >= len(body) {
					return nil, false
				}
				id, sel := body[1+2*c], body[2+2*c]
				var cc *comp
				for k := range comps {
					if comps[k].id == id {
						cc = &comps[k]
					}
				}
				if cc == nil || dcT[sel>>4] == nil || acT[sel&15] == nil {
					return nil, false
				}
				blocks := int(cc.h) * int(cc.v)
				if len(comps) == 1 {
					blocks = 1
					s.mcuW = 8
				}
				s.comps = append(s.comps, scanComp{blocks, dcT[sel>>4], acT[sel&15]})
				if c == 0 {
					s.dcQ = q[cc.tq]
				}
			}
			return s, s.dcQ > 0
		}
		i += 2 + n
	}
	return nil, false
}

// unstuffed takes the 0x00 out after every 0xFF and stops at the end
// marker, or at any marker other than a restart.
func unstuffed(tail []byte) ([]byte, bool) {
	out := make([]byte, 0, len(tail))
	for i := 0; i < len(tail); i++ {
		c := tail[i]
		if c != 0xff {
			out = append(out, c)
			continue
		}
		if i+1 >= len(tail) {
			return out, false
		}
		switch next := tail[i+1]; {
		case next == 0x00:
			out = append(out, 0xff)
			i++
		case next == 0xd9:
			return out, true
		default:
			return out, false // a restart, or something else this does not follow
		}
	}
	return out, false
}

// decodeMCUs decodes from a bit to the end and gives the luma average of
// each MCU, as a running sum of differences from an unknown start. It
// fails unless the stream ends exactly where the data does, padded with
// ones as JPEG pads.
func decodeMCUs(s *scanSetup, data []byte, start int) ([]int32, error) {
	r := &scanBits{b: data, pos: start}
	pred := make([]int32, len(s.comps))
	var out []int32
	end := len(data) * 8
	for {
		left := end - r.pos
		if left < 8 {
			// What is left must be the padding: all ones.
			for r.pos < end {
				b, _ := r.bit()
				if b != 1 {
					return nil, errScan
				}
			}
			if len(out) < 8 {
				return nil, errScan
			}
			return out, nil
		}
		var luma int32
		for ci, c := range s.comps {
			for b := 0; b < c.blocks; b++ {
				t, ok := c.dc.decode(r)
				if !ok || t > 11 {
					return nil, errScan
				}
				v, ok := r.bits(int(t))
				if !ok {
					return nil, errScan
				}
				pred[ci] += extend(v, int(t))
				if ci == 0 {
					luma += pred[ci]
				}
				for k := 1; k < 64; {
					rs, ok := c.ac.decode(r)
					if !ok {
						return nil, errScan
					}
					run, size := int(rs>>4), int(rs&15)
					if size == 0 {
						if run == 15 {
							k += 16
							continue
						}
						break
					}
					k += run
					if k > 63 {
						return nil, errScan
					}
					if _, ok := r.bits(size); !ok {
						return nil, errScan
					}
					k++
				}
			}
		}
		out = append(out, luma/int32(s.comps[0].blocks))
	}
}

type oldJPEG struct {
	mcus      int
	perRow    int // MCUs across, once worked out
	rows      int // MCU rows, whole ones
	width     int // pixels across, as many as the MCUs cover
	rowPixels int // pixel rows the MCU rows make
	sure      bool
	luma      []int32 // the averages, for the tests
}

// decodeTail tries every bit from the start of the tail for one where the
// blocks decode cleanly to the very end. Huffman codes fall back into step
// by themselves, so a start a few bits wrong decodes into the right blocks
// after a few of them: the first MCU row is not counted.
func decodeTail(front, tail []byte, frontWidth int) (*oldJPEG, bool) {
	s, ok := setupFrom(front)
	if !ok {
		return nil, false
	}
	data, ended := unstuffed(tail)
	if !ended || len(data) < 64 {
		return nil, false
	}
	var luma []int32
	for bit := 0; bit < min(len(data)*8, 1<<16); bit++ {
		if l, err := decodeMCUs(s, data, bit); err == nil {
			luma = l
			break
		}
	}
	if luma == nil {
		return nil, false
	}
	o := &oldJPEG{mcus: len(luma), luma: luma}
	o.perRow, o.sure = mcusPerRow(luma, (frontWidth+s.mcuW-1)/s.mcuW)
	if o.perRow == 0 {
		return o, true
	}
	o.rows = len(luma)/o.perRow - 1
	o.width = o.perRow * s.mcuW
	mcuH := s.mcuW
	if s.comps[0].blocks == 2 {
		mcuH = 8 // 4:2:2, two blocks across and one down
	}
	o.rowPixels = o.rows * mcuH
	return o, true
}

// mcusPerRow finds how many MCUs make a row of the old picture: the number
// at which each MCU is most like the one above it. The old picture was at
// least as wide as the crop of it.
func mcusPerRow(luma []int32, least int) (int, bool) {
	scores := map[int]float64{}
	best := 0
	for n := max(1, least); n <= min(len(luma)/3, 4096); n++ {
		var d float64
		for i := n; i < len(luma); i++ {
			x := luma[i] - luma[i-n]
			if x < 0 {
				x = -x
			}
			d += float64(x)
		}
		d /= float64(len(luma) - n)
		scores[n] = d
		if best == 0 || d < scores[best] {
			best = n
		}
	}
	if best == 0 {
		return 0, false
	}
	// Twice the width is the row above the row above, and nearly as alike;
	// it is not a rival width. Sure is no other width coming close.
	for n, d := range scores {
		if n != best && n%best != 0 && d < scores[best]/0.8 {
			return best, false
		}
	}
	return best, true
}

// splitTail separates the JPEG's own end from the rest, for the caller.
func tailOfOld(data []byte, end int) []byte {
	rest := data[end:]
	if i := bytes.LastIndex(rest, []byte{0xff, 0xd9}); i >= 0 {
		return rest[:i+2]
	}
	return nil
}
