package main

// ISO base media boxes, the structure under MP4 and HEIC alike: enough to
// say how long a video is and whether it has sound, and what pictures a
// HEIC keeps besides the one it shows.

import (
	"encoding/binary"
	"strings"
)

type box struct {
	kind string
	body []byte
}

// boxes splits a run of boxes. A box that claims more than there is ends
// the run rather than the program.
func boxes(b []byte) []box {
	var out []box
	for len(b) >= 8 {
		n := int64(binary.BigEndian.Uint32(b))
		kind := string(b[4:8])
		head := int64(8)
		switch n {
		case 1:
			if len(b) < 16 {
				return out
			}
			n = int64(binary.BigEndian.Uint64(b[8:]))
			head = 16
		case 0:
			n = int64(len(b))
		}
		if n < head || n > int64(len(b)) {
			return out
		}
		out = append(out, box{kind, b[head:n]})
		b = b[n:]
	}
	return out
}

func child(b []byte, path ...string) []byte {
	for _, want := range path {
		found := false
		for _, x := range boxes(b) {
			if x.kind == want {
				b, found = x.body, true
				break
			}
		}
		if !found {
			return nil
		}
	}
	return b
}

type video struct {
	seconds       float64
	sound         bool
	width, height int
}

// readVideo reads an MP4 far enough to describe it. ok is false when the
// bytes are not one.
func readVideo(b []byte) (video, bool) {
	var v video
	moov := child(b, "moov")
	if moov == nil {
		return v, false
	}
	if mvhd := child(moov, "mvhd"); len(mvhd) >= 20 {
		var scale, dur uint64
		if mvhd[0] == 1 && len(mvhd) >= 32 {
			scale = uint64(binary.BigEndian.Uint32(mvhd[20:]))
			dur = binary.BigEndian.Uint64(mvhd[24:])
		} else {
			scale = uint64(binary.BigEndian.Uint32(mvhd[12:]))
			dur = uint64(binary.BigEndian.Uint32(mvhd[16:]))
		}
		if scale > 0 {
			v.seconds = float64(dur) / float64(scale)
		}
	}
	for _, t := range boxes(moov) {
		if t.kind != "trak" {
			continue
		}
		hdlr := child(t.body, "mdia", "hdlr")
		if len(hdlr) < 12 {
			continue
		}
		switch string(hdlr[8:12]) {
		case "soun":
			v.sound = true
		case "vide":
			if tkhd := child(t.body, "tkhd"); len(tkhd) >= 84 {
				off := 76
				if tkhd[0] == 1 {
					off = 88
				}
				if len(tkhd) >= off+8 {
					v.width = int(binary.BigEndian.Uint32(tkhd[off:]) >> 16)
					v.height = int(binary.BigEndian.Uint32(tkhd[off+4:]) >> 16)
				}
			}
		}
	}
	return v, true
}

// heifItem is one picture or block of data a HEIC keeps.
type heifItem struct {
	id            uint32
	kind          string // hvc1, grid, Exif, mime...
	aux           string // the auxiliary type, for depth maps and mattes
	thumbOf       bool   // referenced as a thumbnail
	auxOf         bool   // referenced as an auxiliary image
	width, height int
	primary       bool
}

// readHEIF lists the items. Pixels are not decoded: HEVC is not something
// the standard library reads, and that is said in the report.
func readHEIF(b []byte) ([]heifItem, bool) {
	meta := child(b, "meta")
	if len(meta) < 4 {
		return nil, false
	}
	meta = meta[4:] // a full box: version and flags first
	items := map[uint32]*heifItem{}
	var order []uint32
	var primary uint32
	if pitm := child(meta, "pitm"); len(pitm) >= 6 {
		if pitm[0] == 0 {
			primary = uint32(binary.BigEndian.Uint16(pitm[4:]))
		} else if len(pitm) >= 8 {
			primary = binary.BigEndian.Uint32(pitm[4:])
		}
	}
	if iinf := child(meta, "iinf"); len(iinf) >= 6 {
		skip := 6
		if iinf[0] != 0 {
			skip = 8
		}
		if len(iinf) >= skip {
			for _, e := range boxes(iinf[skip:]) {
				if e.kind != "infe" || len(e.body) < 12 {
					continue
				}
				v := e.body[0]
				var id uint32
				var kind string
				switch {
				case v >= 3 && len(e.body) >= 14:
					id = binary.BigEndian.Uint32(e.body[4:])
					kind = string(e.body[10:14])
				case v == 2:
					id = uint32(binary.BigEndian.Uint16(e.body[4:]))
					kind = string(e.body[8:12])
				default:
					continue
				}
				if items[id] == nil {
					order = append(order, id)
				}
				items[id] = &heifItem{id: id, kind: kind, primary: id == primary}
			}
		}
	}
	if iref := child(meta, "iref"); len(iref) >= 4 {
		wide := iref[0] != 0
		for _, r := range boxes(iref[4:]) {
			body := r.body
			read := func() (uint32, bool) {
				if wide {
					if len(body) < 4 {
						return 0, false
					}
					v := binary.BigEndian.Uint32(body)
					body = body[4:]
					return v, true
				}
				if len(body) < 2 {
					return 0, false
				}
				v := uint32(binary.BigEndian.Uint16(body))
				body = body[2:]
				return v, true
			}
			from, ok := read()
			if !ok || len(body) < 2 {
				continue
			}
			body = body[2:] // the count of items it points at
			if it := items[from]; it != nil {
				switch r.kind {
				case "thmb":
					it.thumbOf = true
				case "auxl":
					it.auxOf = true
				}
			}
		}
	}
	// Properties are listed once and then associated with items by index.
	ipco := child(meta, "iprp", "ipco")
	props := boxes(ipco)
	if ipma := child(meta, "iprp", "ipma"); len(ipma) >= 8 {
		v, flags := ipma[0], ipma[3]
		n := int(binary.BigEndian.Uint32(ipma[4:]))
		b := ipma[8:]
		for k := 0; k < n && len(b) > 0; k++ {
			var id uint32
			if v < 1 {
				if len(b) < 3 {
					break
				}
				id = uint32(binary.BigEndian.Uint16(b))
				b = b[2:]
			} else {
				if len(b) < 5 {
					break
				}
				id = binary.BigEndian.Uint32(b)
				b = b[4:]
			}
			count := int(b[0])
			b = b[1:]
			for a := 0; a < count; a++ {
				var index int
				if flags&1 != 0 {
					if len(b) < 2 {
						break
					}
					index = int(binary.BigEndian.Uint16(b) & 0x7fff)
					b = b[2:]
				} else {
					if len(b) < 1 {
						break
					}
					index = int(b[0] & 0x7f)
					b = b[1:]
				}
				it := items[id]
				if it == nil || index == 0 || index > len(props) {
					continue
				}
				p := props[index-1]
				switch p.kind {
				case "ispe":
					if len(p.body) >= 12 {
						it.width = int(binary.BigEndian.Uint32(p.body[4:]))
						it.height = int(binary.BigEndian.Uint32(p.body[8:]))
					}
				case "auxC":
					if len(p.body) > 4 {
						urn := string(p.body[4:])
						if i := strings.IndexByte(urn, 0); i >= 0 {
							urn = urn[:i]
						}
						it.aux = urn
					}
				}
			}
		}
	}
	var out []heifItem
	for _, id := range order {
		out = append(out, *items[id])
	}
	return out, true
}
