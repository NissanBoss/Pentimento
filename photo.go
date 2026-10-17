package main

// What a JPEG keeps, said in the report.

import (
	"fmt"
	"image"
	"strings"
)

func (c *run) jpeg(where string, data []byte) {
	j := readJPEG(data)
	if j.broken != "" {
		c.gap(where, j.broken)
	}
	var picture image.Image
	decoded := false
	pic := func() image.Image {
		if !decoded {
			decoded = true
			var err error
			if picture, err = decodePicture(data[:j.end]); err != nil {
				c.gap(where, "the picture could not be decoded, so the copies inside it were not compared with it")
			}
		}
		return picture
	}
	var maps []string
	var phone []embedded
	for _, e := range j.inside(data) {
		switch e.role {
		case rolePreview, roleImage:
			if p := pic(); p != nil {
				c.copyOf(where, p, e, cutFrom(j, p))
			}
		case roleOriginal:
			line := capital(e.what) + ", " + describe(e) + "."
			var more []string
			if p := pic(); p != nil {
				if img, err := decodePicture(e.data); err == nil {
					r := compareCopies(p, img)
					switch {
					case r.unrelated:
						more = append(more, "It is not the same picture as the one shown.")
					case r.share > 0.01:
						more = append(more, fmt.Sprintf("It differs from the picture shown over %s of it.", percent(r.share, 1)))
					}
				}
			}
			c.hit("original-inside", where, line, more...)
		case roleDepth:
			maps = append(maps, e.what+", "+describe(e))
		case roleVideo:
			c.hit("motion", where, videoLine(e))
		case roleAudio:
			c.hit("motion", where, fmt.Sprintf("A sound recording of %s, %s.", size(int64(len(e.data))), e.from))
		case roleWhere, roleEdit:
			phone = append(phone, e)
		case roleOther:
			if e.what == "the end of an earlier JPEG" {
				c.jpegTail(where, data, j, e)
			} else if !strings.HasPrefix(e.from, "Samsung") {
				c.hit("tail-other", where, fmt.Sprintf("%s after the end: %s.", size(int64(len(e.data))), e.what))
			}
		}
	}
	if len(maps) > 0 {
		for i := range maps {
			maps[i] = capital(maps[i]) + "."
		}
		c.hit("depth", where, maps[0], maps[1:]...)
	}
	c.phoneTrailer(where, phone)
	if p := pic(); p != nil {
		c.pixelsOf(where, p, false, j.fromCamera())
	}
}

// copyOf compares the picture with a copy of it and says what differs.
// cut, when it is not empty, is what the EXIF says about the picture
// having been cut down, which backs up a copy that shows more.
func (c *run) copyOf(where string, p image.Image, e embedded, cut string) {
	img, err := decodePicture(e.data)
	if err != nil {
		if e.role == rolePreview {
			c.gap(where, e.what+" could not be decoded, so it was not compared with the picture")
		}
		return
	}
	r := compareCopies(p, img)
	name := capital(e.what)
	ib := img.Bounds()
	switch {
	case r.rotated:
		return
	case r.unrelated:
		// A second picture that is not this one is often something else a
		// phone keeps, a gain map that came out unlabelled; only a preview
		// is meant to be this picture, so only a preview that is not is
		// a finding.
		if e.role == rolePreview {
			c.hit("preview-differs", where, fmt.Sprintf("%s (%dx%d) is a different picture altogether.", name, ib.Dx(), ib.Dy()),
				"Whatever the file showed before it was saved over is what its preview still shows.")
		}
		return
	}
	var lines []string
	if r.outside > 0.03 {
		lines = append(lines, fmt.Sprintf("%s (%dx%d) shows %s more than the picture: what a crop or a zoom took off.", name, ib.Dx(), ib.Dy(), percent(r.outside, 1)))
		if cut != "" {
			lines = append(lines, cut)
		}
	}
	if len(r.changed) > 0 {
		big := r.changed[0]
		s := fmt.Sprintf("%s (%dx%d) differs from the picture", name, ib.Dx(), ib.Dy())
		if len(lines) > 0 {
			s = "It also differs from the picture"
		}
		if len(r.changed) == 1 {
			s += fmt.Sprintf(" in one place, %dx%d at %d,%d.", big.Dx(), big.Dy(), big.Min.X, big.Min.Y)
		} else {
			s += fmt.Sprintf(" in %d places, the biggest %dx%d at %d,%d.", len(r.changed), big.Dx(), big.Dy(), big.Min.X, big.Min.Y)
		}
		lines = append(lines, s)
	}
	if len(lines) > 0 && r.reshaped {
		lines = append(lines, "The picture was also straightened or warped after the copy was made, so other changes were not looked for.")
	}
	if len(lines) > 0 {
		c.hit("preview-differs", where, lines[0], lines[1:]...)
	}
}

// cutFrom says so when the picture is smaller than the EXIF says the
// camera made it, either way round.
func cutFrom(j *jpegFile, p image.Image) string {
	made, ok := j.madeAs()
	is := p.Bounds().Size()
	if !ok || made == is || made == image.Pt(is.Y, is.X) {
		return ""
	}
	if is.X*is.Y >= made.X*made.Y {
		return ""
	}
	return fmt.Sprintf("Its EXIF says the camera made it %dx%d, and it is %dx%d now.", made.X, made.Y, is.X, is.Y)
}

func describe(e embedded) string {
	s := size(int64(len(e.data)))
	if w, h, ok := pictureSize(e.data); ok {
		s = fmt.Sprintf("%dx%d, %s", w, h, s)
	}
	return s + ", in " + e.from
}

func videoLine(e embedded) string {
	v, ok := readVideo(e.data)
	if !ok {
		return fmt.Sprintf("A video of %s, in %s.", size(int64(len(e.data))), e.from)
	}
	s := "A"
	if v.seconds > 0 {
		s += fmt.Sprintf(" %.1f-second", v.seconds)
	}
	s += " video"
	if v.sound {
		s += " with sound"
	} else {
		s += ", without sound"
	}
	if v.width > 0 {
		s += fmt.Sprintf(", %dx%d", v.width, v.height)
	}
	return s + fmt.Sprintf(", %s, in %s.", size(int64(len(e.data))), e.from)
}

func capital(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	if r[0] >= 'a' && r[0] <= 'z' {
		r[0] -= 'a' - 'A'
	}
	return string(r)
}

// phoneTrailer says what a Samsung trailer holds about where the phone was
// and what was done with it. Entries of other kinds are left for the
// checks that read them.
func (c *run) phoneTrailer(where string, entries []embedded) {
	var lines []string
	for _, e := range entries {
		if e.role != roleWhere && e.role != roleEdit {
			continue
		}
		line := capital(e.what)
		if e.extra != "" {
			v := e.extra
			if e.private {
				v = c.show.text(v)
			}
			line += ": " + v
		}
		lines = append(lines, line+".")
	}
	if len(lines) > 0 {
		c.hit("phone-trailer", where, lines[0], lines[1:]...)
	}
}

// jpegTail says what is left of an earlier JPEG behind this one, decoded
// with this one's tables when they are the same encoder's.
func (c *run) jpegTail(where string, data []byte, j *jpegFile, e embedded) {
	o, ok := decodeTail(data[:j.end], e.data, j.width)
	if !ok || o.perRow == 0 {
		c.hit("jpeg-tail", where, fmt.Sprintf("%s of an earlier JPEG's compressed picture and its end marker, after the end of this one.", size(int64(len(e.data)))),
			"Its blocks do not decode with this picture's tables, so how much of it is there was not worked out.")
		return
	}
	line := fmt.Sprintf("The bottom %s rows of an earlier picture about %s pixels wide, behind this one of %dx%d.",
		commas(o.rowPixels), commas(o.width), j.width, j.height)
	more := "They decode with this picture's own tables, exact but for their overall brightness and tint, which went with the part the new picture overwrote."
	if !o.sure {
		more += " Other widths fit the rows almost as well, so the width is the likeliest one, not a certain one."
	}
	c.hit("jpeg-tail", where, line, more)
}
