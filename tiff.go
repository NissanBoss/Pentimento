package main

// Just enough TIFF to read the IFDs that EXIF and MPF are made of.

import (
	"encoding/binary"
	"errors"
)

type tiff struct {
	data  []byte
	order binary.ByteOrder
	ifd0  ifd
	ifd1  ifd
}

type ifd map[uint16]tiffEntry

type tiffEntry struct {
	kind  uint16
	count uint32
	value []byte // the four bytes in the entry, or what they point to
	order binary.ByteOrder
}

var errTIFF = errors.New("not TIFF")

func parseTIFF(data []byte) (*tiff, error) {
	if len(data) < 8 {
		return nil, errTIFF
	}
	t := &tiff{data: data}
	switch string(data[:2]) {
	case "II":
		t.order = binary.LittleEndian
	case "MM":
		t.order = binary.BigEndian
	default:
		return nil, errTIFF
	}
	first := int(t.order.Uint32(data[4:]))
	var next int
	t.ifd0, next = t.readIFD(first)
	if t.ifd0 == nil {
		return nil, errTIFF
	}
	if next > 0 && next != first {
		t.ifd1, _ = t.readIFD(next)
	}
	return t, nil
}

var typeSize = map[uint16]int64{1: 1, 2: 1, 3: 2, 4: 4, 5: 8, 6: 1, 7: 1, 8: 2, 9: 4, 10: 8, 11: 4, 12: 8}

func (t *tiff) readIFD(at int) (ifd, int) {
	if at < 8 || at+2 > len(t.data) {
		return nil, 0
	}
	n := int(t.order.Uint16(t.data[at:]))
	if at+2+12*n+4 > len(t.data) {
		return nil, 0
	}
	out := ifd{}
	for k := 0; k < n; k++ {
		e := t.data[at+2+12*k:]
		tag := t.order.Uint16(e)
		kind := t.order.Uint16(e[2:])
		count := t.order.Uint32(e[4:])
		total := typeSize[kind] * int64(count)
		var value []byte
		if total <= 4 {
			value = e[8 : 8+total]
		} else {
			off := int64(t.order.Uint32(e[8:]))
			if off+total > int64(len(t.data)) {
				continue
			}
			value = t.data[off : off+total]
		}
		out[tag] = tiffEntry{kind, count, value, t.order}
	}
	return out, int(t.order.Uint32(t.data[at+2+12*n:]))
}

func (t *tiff) bytes(e tiffEntry) []byte { return e.value }

// uint reads a SHORT or a LONG.
func (d ifd) uint(tag uint16) (uint32, bool) {
	e, ok := d[tag]
	if !ok {
		return 0, false
	}
	switch {
	case e.kind == 3 && len(e.value) >= 2:
		return uint32(e.order.Uint16(e.value)), true
	case e.kind == 4 && len(e.value) >= 4:
		return e.order.Uint32(e.value), true
	}
	return 0, false
}
