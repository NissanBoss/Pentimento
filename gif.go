package main

// Where a GIF really ends. The Snipping Tool's crop bug wrote GIFs the
// same way it wrote PNGs, and what is left after the trailer byte is the
// end of the old picture's compressed frames.

import "fmt"

// gifEnd walks the blocks to the trailer and gives the byte after it, or
// -1 when the blocks stop making sense first.
func gifEnd(b []byte) int {
	if len(b) < 13 {
		return -1
	}
	i := 13
	if b[10]&0x80 != 0 {
		i += 3 << (b[10]&7 + 1)
	}
	subBlocks := func() bool {
		for i < len(b) {
			n := int(b[i])
			i++
			if n == 0 {
				return true
			}
			i += n
		}
		return false
	}
	for i < len(b) {
		switch b[i] {
		case 0x3b:
			return i + 1
		case 0x21:
			i += 2
			if !subBlocks() {
				return -1
			}
		case 0x2c:
			if i+10 > len(b) {
				return -1
			}
			flags := b[i+9]
			i += 10
			if flags&0x80 != 0 {
				i += 3 << (flags&7 + 1)
			}
			i++ // the LZW code size
			if !subBlocks() {
				return -1
			}
		default:
			return -1
		}
	}
	return -1
}

func (c *run) gif(where string, data []byte) {
	end := gifEnd(data)
	if end < 0 {
		c.gap(where, "the blocks stop making sense before the trailer")
		return
	}
	if rest := data[end:]; len(rest) > 16 && !allZero(rest) {
		switch what := knownFile(rest); {
		case what != "":
			c.hit("tail-other", where, fmt.Sprintf("%s after the end: %s.", size(int64(len(rest))), what))
		case rest[len(rest)-1] == 0x3b:
			c.hit("gif-tail", where, fmt.Sprintf("%s of an earlier GIF's compressed frames and its trailer, after the end of this one.", size(int64(len(rest)))))
		default:
			c.hit("tail-other", where, fmt.Sprintf("%s after the end that this cannot name.", size(int64(len(rest)))))
		}
	}
	c.pixels(where, data)
}
