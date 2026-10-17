package main

// Undoing a predictor.
//
// Flate on its own compresses badly on data laid out in rows of similar
// numbers, so the format lets a stream say that each row was first
// rewritten as its difference from the row above. The compressor gets
// something far more repetitive and the reader has to add it all back.
//
// Every cross-reference stream in the wild uses one, so without this the
// index of a modern PDF inflates to numbers that look plausible and are
// not the offsets of anything.

import "fmt"

type predictor struct {
	Kind    int
	Colours int
	Bits    int
	Columns int
}

func predictorFrom(d *Document, parms any) (predictor, bool) {
	// The parameters can be one dictionary or, when a stream has several
	// filters, an array with one entry per filter.
	switch p := d.Resolve(parms).(type) {
	case Dict:
		return readPredictor(d, p), true
	case Array:
		for _, item := range p {
			if inner, ok := d.Resolve(item).(Dict); ok {
				if got := readPredictor(d, inner); got.Kind > 1 {
					return got, true
				}
			}
		}
	}
	return predictor{}, false
}

func readPredictor(d *Document, p Dict) predictor {
	get := func(key Name, fallback int) int {
		if n, ok := asInt(d.Resolve(p[key])); ok {
			return n
		}
		return fallback
	}
	return predictor{
		Kind:    get("Predictor", 1),
		Colours: get("Colors", 1),
		Bits:    get("BitsPerComponent", 8),
		Columns: get("Columns", 1),
	}
}

// undo puts the rows back the way they were before the encoder subtracted
// them from each other.
func (p predictor) undo(data []byte) ([]byte, error) {
	switch {
	case p.Kind <= 1:
		return data, nil
	case p.Colours < 1 || p.Bits < 1 || p.Columns < 1:
		return nil, fmt.Errorf("a predictor with %d colours at %d bits over %d columns is not a shape this reads",
			p.Colours, p.Bits, p.Columns)
	}

	// A pixel cannot be less than a byte for the purpose of looking left,
	// which is what the format says and what every encoder assumes.
	step := (p.Colours*p.Bits + 7) / 8
	row := (p.Colours*p.Bits*p.Columns + 7) / 8

	if p.Kind == 2 {
		return p.tiff(data, step, row)
	}
	return p.png(data, step, row)
}

// tiff is the older scheme: every value is the difference from the one to
// its left, and there is no per row marker.
func (p predictor) tiff(data []byte, step, row int) ([]byte, error) {
	if p.Bits != 8 {
		// The sub byte cases exist and nothing writes them into a PDF this
		// program will meet. Saying so beats getting them quietly wrong.
		return nil, fmt.Errorf("a TIFF predictor at %d bits is not one this reads", p.Bits)
	}
	out := append([]byte(nil), data...)
	for start := 0; start+row <= len(out); start += row {
		for i := step; i < row; i++ {
			out[start+i] += out[start+i-step]
		}
	}
	return out, nil
}

// png is the scheme PNG uses, where every row starts with a byte saying
// which of five ways it was encoded.
func (p predictor) png(data []byte, step, row int) ([]byte, error) {
	out := make([]byte, 0, len(data))
	previous := make([]byte, row)
	current := make([]byte, row)

	for at := 0; at < len(data); at += row + 1 {
		how := data[at]
		line := data[at+1:]
		if len(line) > row {
			line = line[:row]
		}
		if len(line) == 0 {
			break
		}
		copy(current, line)
		for i := len(line); i < row; i++ {
			current[i] = 0 // a short last row is padded rather than refused
		}

		for i := range current {
			var left, up, upLeft byte
			if i >= step {
				left, upLeft = current[i-step], previous[i-step]
			}
			up = previous[i]
			switch how {
			case 0: // none
			case 1:
				current[i] += left
			case 2:
				current[i] += up
			case 3:
				current[i] += byte((int(left) + int(up)) / 2)
			case 4:
				current[i] += paethByte(left, up, upLeft)
			default:
				return nil, fmt.Errorf("row at offset %d says it was encoded by method %d, and there are five", at, how)
			}
		}
		out = append(out, current[:len(line)]...)
		previous, current = current, previous
	}
	return out, nil
}

// paethByte picks whichever of the three neighbours the value is closest to.
func paethByte(left, up, upLeft byte) byte {
	p := int(left) + int(up) - int(upLeft)
	pa, pb, pc := abs(p-int(left)), abs(p-int(up)), abs(p-int(upLeft))
	switch {
	case pa <= pb && pa <= pc:
		return left
	case pb <= pc:
		return up
	}
	return upLeft
}
