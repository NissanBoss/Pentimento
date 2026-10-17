package main

// Everything a JPEG keeps besides the picture it shows, wherever it keeps
// it: the EXIF thumbnail, the Multi-Picture index, base64 inside the XMP,
// the files a Google container lists after the end, Samsung's trailer, and
// whatever else is there that none of those account for.

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

type role int

const (
	roleImage    role = iota // another picture, not known to be anything in particular
	rolePreview              // a smaller copy of the picture, for showing it quickly
	roleOriginal             // the picture before an effect was applied
	roleDepth                // a map of distance, or of where the people are
	roleGainMap              // the HDR brightness map
	roleVideo
	roleAudio
	roleEdit  // how the picture was edited, kept so it can be undone
	roleWhere // where the phone was
	roleOther
	// roleDebug is what a camera app leaves for its own engineers: the
	// exposure, focus and white balance it worked out. It is not anything
	// the person took out, and it is not reported.
	roleDebug
)

type embedded struct {
	role  role
	what  string // what it is, in words
	from  string // where in the file it was found
	at    int    // its offset in the file, or -1 when it was inside the XMP
	data  []byte
	extra string // a value worth printing, for small entries
	// private is an extra that says something about the person, masked
	// like any other words from their files.
	private bool
}

func (j *jpegFile) inside(data []byte) []embedded {
	var out []embedded
	taken := map[int]bool{}
	add := func(e embedded) {
		if e.at >= 0 {
			if taken[e.at] {
				return
			}
			taken[e.at] = true
		}
		out = append(out, e)
	}

	if t := j.thumbnail(); t != nil {
		add(embedded{role: rolePreview, what: "the EXIF thumbnail", from: "EXIF", at: -1, data: t})
	}
	for _, m := range j.mpfImages(data) {
		e := embedded{role: roleImage, what: "a second picture", from: "the Multi-Picture index", at: m.at, data: m.inFile}
		switch m.kind {
		case 0x010001, 0x010002:
			e.role, e.what = rolePreview, "a large preview"
		case 0x020002:
			e.role, e.what = roleDepth, "a disparity map"
		}
		classifyByOwnXMP(&e)
		add(e)
	}

	xmp := strings.Join(j.xmp, "\n")
	if b := xmpBinary(xmp, "GImage:Data"); b != nil {
		add(embedded{role: roleOriginal, what: "the photo before Lens Blur blurred it", from: "the XMP, as GImage:Data", at: -1, data: b})
	}
	if b := xmpBinary(xmp, "GDepth:Data"); b != nil {
		add(embedded{role: roleDepth, what: "a depth map", from: "the XMP, as GDepth:Data", at: -1, data: b})
	}
	if b := xmpBinary(xmp, "GDepth:Confidence"); b != nil {
		add(embedded{role: roleDepth, what: "a depth confidence map", from: "the XMP, as GDepth:Confidence", at: -1, data: b})
	}
	if b := xmpBinary(xmp, "GCamera:RelitInputImageData"); b != nil {
		add(embedded{role: roleOriginal, what: "the photo before Portrait Light relit it", from: "the XMP, as GCamera:RelitInputImageData", at: -1, data: b})
	}

	// A Google container lists the files after the end, in order. The
	// first item is the picture itself, with no length.
	if items := containerItems(xmp); len(items) > 1 {
		at := j.end
		for _, it := range items[1:] {
			at += it.padding
			if it.length <= 0 || at+it.length > len(data) {
				break
			}
			e := embedded{what: containerWhat(it), from: "the Google container, as " + it.name(), at: at, data: data[at : at+it.length]}
			e.role = containerRole(it)
			add(e)
			at += it.length
		}
	}
	// Before the container, Pixels named the video by its distance from
	// the end of the file.
	if v, ok := xmpField(xmp, "GCamera:MicroVideoOffset"); ok {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n < len(data)-j.end+1 {
			at := len(data) - n
			add(embedded{role: roleVideo, what: "a video", from: "the XMP, as GCamera:MicroVideoOffset", at: at, data: data[at:]})
		}
	}
	for _, e := range samsungTrailer(data, j.end) {
		add(e)
	}

	// Anything after the end that nothing above explains.
	covered := make([]bool, len(data)-j.end)
	for _, e := range out {
		if e.at >= j.end {
			for k := e.at; k < e.at+len(e.data) && k < len(data); k++ {
				covered[k-j.end] = true
			}
		}
	}
	if s, ok := samsungSpan(data, j.end); ok {
		for k := s; k < len(data); k++ {
			covered[k-j.end] = true
		}
	}
	for start := 0; start < len(covered); {
		if covered[start] {
			start++
			continue
		}
		end := start
		for end < len(covered) && !covered[end] {
			end++
		}
		if piece := data[j.end+start : j.end+end]; !allZero(piece) && len(piece) > 16 {
			add(unexplained(piece, j.end+start))
		}
		start = end
	}
	return out
}

// classifyByOwnXMP reads the XMP of a picture inside the picture, which is
// where Apple and Ultra HDR say what a secondary picture is for.
func classifyByOwnXMP(e *embedded) {
	if len(e.data) < 4 || e.data[0] != 0xff || e.data[1] != 0xd8 {
		return
	}
	inner := readJPEG(e.data)
	x := strings.Join(inner.xmp, "\n")
	switch {
	case strings.Contains(x, "hdrgm:Version") || strings.Contains(x, "aux:hdrgainmap"):
		e.role, e.what = roleGainMap, "an HDR gain map"
	case strings.Contains(x, "aux:portraiteffectsmatte"):
		e.role, e.what = roleDepth, "an outline of the people in it"
	case strings.Contains(x, "semanticskinmatte"):
		e.role, e.what = roleDepth, "a map of the skin of the people in it"
	case strings.Contains(x, "semantichairmatte"):
		e.role, e.what = roleDepth, "a map of their hair"
	case strings.Contains(x, "semanticteethmatte"):
		e.role, e.what = roleDepth, "a map of their teeth"
	case strings.Contains(x, "semanticglassesmatte"):
		e.role, e.what = roleDepth, "a map of their glasses"
	case strings.Contains(x, "semanticskymatte"):
		e.role, e.what = roleDepth, "a map of the sky"
	case strings.Contains(x, "aux:depth"), strings.Contains(x, "aux:disparity"):
		e.role, e.what = roleDepth, "a depth map"
	}
}

func containerRole(it containerItem) role {
	n := strings.ToLower(it.semantic + " " + it.uri)
	switch {
	case strings.HasPrefix(it.mime, "video/") || strings.Contains(n, "motionphoto"):
		return roleVideo
	case strings.HasPrefix(it.mime, "audio/"):
		return roleAudio
	case strings.Contains(n, "gainmap"):
		return roleGainMap
	case strings.Contains(n, "original"):
		return roleOriginal
	case strings.Contains(n, "depth"), strings.Contains(n, "confidence"):
		return roleDepth
	}
	return roleImage
}

func containerWhat(it containerItem) string {
	switch containerRole(it) {
	case roleVideo:
		return "a video"
	case roleAudio:
		return "a sound recording"
	case roleGainMap:
		return "an HDR gain map"
	case roleOriginal:
		return "the photo before Portrait blurred it"
	case roleDepth:
		if strings.Contains(strings.ToLower(it.uri+it.semantic), "confidence") {
			return "a depth confidence map"
		}
		return "a depth map"
	}
	return "a second picture"
}

// Samsung's trailer ends the file with "SEFT", after a directory that
// counts back to each entry from where the directory starts. Read the way
// ExifTool's Samsung module reads it, which is the only description of
// the format there is.
func samsungTrailer(data []byte, end int) []embedded {
	dirAt, count, ok := samsungDir(data, end)
	if !ok {
		return nil
	}
	var out []embedded
	for k := 0; k < count; k++ {
		e := data[dirAt+12+12*k:]
		kind := binary.LittleEndian.Uint16(e[2:])
		back := int(binary.LittleEndian.Uint32(e[4:]))
		n := int(binary.LittleEndian.Uint32(e[8:]))
		at := dirAt - back
		if back > dirAt || n > back || n < 8 || at < end {
			continue
		}
		block := data[at : at+n]
		nameLen := int(binary.LittleEndian.Uint32(block[4:]))
		if 8+nameLen > len(block) {
			continue
		}
		name := string(block[8 : 8+nameLen])
		body := block[8+nameLen:]
		out = append(out, samsungEntry(kind, name, body, at+8+nameLen))
	}
	return out
}

func samsungDir(data []byte, end int) (int, int, bool) {
	if len(data)-end < 16 || !bytes.HasSuffix(data, []byte("SEFT")) {
		return 0, 0, false
	}
	n := int(binary.LittleEndian.Uint32(data[len(data)-8:]))
	dirAt := len(data) - 8 - n
	if n < 12 || dirAt < end || string(data[dirAt:dirAt+4]) != "SEFH" {
		return 0, 0, false
	}
	count := int(binary.LittleEndian.Uint32(data[dirAt+8:]))
	if 12+12*count > n {
		return 0, 0, false
	}
	return dirAt, count, true
}

// samsungSpan is where the trailer starts, so the bytes in it are not
// called unexplained. The first entry is the furthest back.
func samsungSpan(data []byte, end int) (int, bool) {
	dirAt, count, ok := samsungDir(data, end)
	if !ok {
		return 0, false
	}
	start := dirAt
	for k := 0; k < count; k++ {
		back := int(binary.LittleEndian.Uint32(data[dirAt+12+12*k+4:]))
		if back <= dirAt-end {
			start = min(start, dirAt-back)
		}
	}
	return start, true
}

func samsungEntry(kind uint16, name string, body []byte, at int) embedded {
	e := embedded{role: roleOther, what: name, from: "Samsung's trailer, as " + name, at: at, data: body}
	switch {
	case kind == 0x0a30 || strings.HasPrefix(name, "MotionPhoto_Data"):
		e.role, e.what = roleVideo, "a video"
	case kind == 0x0100:
		e.role, e.what = roleAudio, "a sound recording"
	case kind == 0x0001 && strings.HasPrefix(name, "DualShot"):
		e.role, e.what = roleOriginal, "the photo before Live Focus blurred it"
	case kind == 0x0001:
		e.role, e.what = roleImage, "a second picture"
	case kind == 0x0a20:
		e.role, e.what = roleImage, "the other camera's picture"
	case kind == 0x0201:
		e.role, e.what = roleVideo, "the video a surround shot was made from"
	case kind == 0x0ab1 || kind == 0x0b41:
		e.role, e.what = roleDepth, "a depth map"
	case kind == 0x0aa1:
		e.role, e.what = roleWhere, "the country of the mobile network the phone was on"
		e.extra = strings.TrimSpace(string(body))
		if c := mccCountry[e.extra]; c != "" {
			e.extra += ", " + c
		}
	case name == "Captured_App_Info":
		// base64 of {"comp":"package/.Activity"}: the app on screen when a
		// screenshot was taken, whatever the crop left of it.
		e.role, e.what, e.private = roleWhere, "the app the screenshot was taken in", true
		if raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(body))); err == nil {
			var app struct {
				Comp string `json:"comp"`
			}
			if json.Unmarshal(raw, &app) == nil && app.Comp != "" {
				e.extra = app.Comp
			}
		}
	case kind == 0x0ba1:
		e.role = roleEdit
		switch name {
		case "PhotoEditor_Re_Edit_Data":
			e.what = "the edits made in Samsung's gallery, kept so they can be undone"
		case "Original_Path_Hash_Key":
			e.what = "a key to the copy of the original that the gallery keeps on the phone"
		default:
			if strings.HasPrefix(name, "deco_") {
				e.what = "what was drawn on the photo in the gallery, kept as its own layer"
			}
		}
	}
	return e
}

// The mobile country codes seen in Samsung trailers, and the commonest
// others. An unknown code is printed as the number.
var mccCountry = map[string]string{
	"202": "Greece", "204": "Netherlands", "206": "Belgium", "208": "France", "214": "Spain",
	"216": "Hungary", "222": "Italy", "226": "Romania", "228": "Switzerland", "230": "Czechia",
	"232": "Austria", "234": "United Kingdom", "235": "United Kingdom", "238": "Denmark",
	"240": "Sweden", "242": "Norway", "244": "Finland", "250": "Russia", "255": "Ukraine",
	"260": "Poland", "262": "Germany", "268": "Portugal", "272": "Ireland", "286": "Turkey",
	"302": "Canada", "310": "United States", "311": "United States", "312": "United States",
	"334": "Mexico", "404": "India", "405": "India", "440": "Japan", "450": "South Korea",
	"454": "Hong Kong", "460": "China", "466": "Taiwan", "505": "Australia", "510": "Indonesia",
	"515": "Philippines", "520": "Thailand", "525": "Singapore", "530": "New Zealand",
	"602": "Egypt", "655": "South Africa", "714": "Panama", "716": "Peru", "722": "Argentina",
	"724": "Brazil", "730": "Chile", "732": "Colombia", "734": "Venezuela", "740": "Ecuador",
	"744": "Paraguay", "748": "Uruguay", "736": "Bolivia",
}

// unexplained says what a stretch of bytes after the end is when nothing
// in the file accounts for it.
func unexplained(b []byte, at int) embedded {
	e := embedded{role: roleOther, from: "after the end, accounted for by nothing in the file", at: at, data: b}
	switch {
	case bytes.HasPrefix(b, []byte("aecDebug")), bytes.HasPrefix(b, []byte("afDebug")), bytes.HasPrefix(b, []byte("awbDebug")):
		// Pixels from the 4a to the 6 end every photo with these, some
		// tens of kilobytes: all nine such photos from Wikimedia Commons
		// that this was first run on had them.
		e.role, e.what = roleDebug, "the camera app's own tuning data"
	case len(b) > 12 && string(b[4:8]) == "ftyp":
		e.role, e.what = roleVideo, "a video"
	case bytes.HasPrefix(b, []byte{0xff, 0xd8, 0xff}):
		e.role, e.what = roleImage, "another JPEG"
	case bytes.HasSuffix(bytes.TrimRight(b, "\x00"), []byte{0xff, 0xd9}) && !bytes.Contains(b[:min(len(b), 64)], []byte{0xff, 0xd8}):
		// The end of a JPEG's compressed picture and its end marker,
		// without a beginning: what is left of a bigger JPEG that was
		// written over, as the Snipping Tool did.
		e.role, e.what = roleOther, "the end of an earlier JPEG"
	default:
		if what := knownFile(b); what != "" {
			e.what = what
		} else {
			e.what = fmt.Sprintf("%s of data this cannot name", size(int64(len(b))))
		}
	}
	return e
}
