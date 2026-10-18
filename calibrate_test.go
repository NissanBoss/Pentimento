package main

// The calibration: hundreds of real photos and screenshots, downloaded by
// testdata/calibrate.sh on GitHub's machines, and counted. It is where the
// numbers in the README come from, and where a rule that fires on
// ordinary files shows up as a count that should be near nothing: a
// mosaic in a sky, a box in a toolbar, a thumbnail that differs because a
// phone tone-maps its thumbnails on their own. Each photo is also looked
// at as if its EXIF were gone, which is how it arrives after a chat app
// has had it, and those counts are the "stripped" ones. Only the counts
// are ever printed; the files are other people's.

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestCalibration(t *testing.T) {
	dir := os.Getenv("PENTIMENTO_CALIBRATION")
	if dir == "" {
		t.Skip("PENTIMENTO_CALIBRATION is not set; see testdata/calibrate.sh")
	}
	groups := []string{"pixel", "samsung", "iphone", "screenshots", "cameras"}
	var table []string
	for _, g := range groups {
		files, _ := filepath.Glob(filepath.Join(dir, g, "*"))
		if len(files) == 0 {
			// calibrate.yml downloads one group per job.
			continue
		}
		sort.Strings(files)
		counts := map[string]int{}
		read, gaps, jpegs, screens := 0, 0, 0, 0
		var loud []string
		for _, f := range files {
			data, err := os.ReadFile(f)
			if err != nil || len(data) == 0 {
				continue
			}
			c := newRun(shower{})
			c.inspect(filepath.Base(f), filepath.Base(f), data, 0)
			if c.read == 0 {
				continue
			}
			read++
			if len(c.gaps) > 0 {
				gaps++
			}
			if len(data) > 2 && data[0] == 0xFF && data[1] == 0xD8 {
				if img, err := decodePicture(data); err == nil {
					jpegs++
					b := img.Bounds()
					px := packed(img)
					if screenLike(px, b.Dx(), b.Dy()) {
						screens++
					}
					if g != "screenshots" {
						bare := newRun(shower{})
						bare.pixelsOf(filepath.Base(f), img, false, false)
						zero, one, three := flatShares(px, b.Dx(), b.Dy())
						for k, hits := range bare.hits {
							counts["stripped "+k]++
							for _, h := range hits {
								loud = append(loud, fmt.Sprintf("%s/%s: stripped %s: %s [flat %.0f%% %.0f%% %.0f%%]", g, filepath.Base(f), k, h.what, 100*zero, 100*one, 100*three))
							}
						}
					}
				}
			}
			for k, hits := range c.hits {
				counts[k]++
				if kindsOfHit[k].weight >= Weak {
					for _, h := range hits {
						line := fmt.Sprintf("%s/%s: %s: %s", g, filepath.Base(f), k, h.what)
						if k == "preview-differs" {
							line += " [" + editTrail(data) + "]"
						}
						loud = append(loud, line)
					}
				}
			}
		}
		var kinds []string
		for k := range counts {
			kinds = append(kinds, k)
		}
		sort.Strings(kinds)
		var parts []string
		for _, k := range kinds {
			parts = append(parts, fmt.Sprintf("%s %d (%.1f%%)", k, counts[k], 100*float64(counts[k])/float64(max(1, read))))
		}
		// How many JPEGs are taken for pictures of a screen, which decides
		// the rules they are held to.
		table = append(table, fmt.Sprintf("%-12s %4d read, %d with a gap, %d of %d JPEGs taken for screens: %s", g, read, gaps, screens, jpegs, strings.Join(parts, ", ")))
		// Every loud finding, by file number, so each one can be looked at
		// by hand: a rule is only kept when the ones it finds are real.
		for _, l := range loud {
			t.Log(l)
		}
	}
	t.Log("\n" + strings.Join(table, "\n"))
}

// editTrail is what the EXIF says about how a photo got to be what it
// is: the size the camera made it, the size it is, the program that saved
// it last, and whether it was saved later than it was taken. A thumbnail
// that shows more, in a photo at its camera's size saved by its camera's
// own software at the time it was taken, is the camera's doing, and the
// rule that found it has to learn that.
func editTrail(data []byte) string {
	j := readJPEG(data)
	t, err := parseTIFF(j.exif)
	if err != nil {
		return "no EXIF"
	}
	e := t.sub(t.ifd0, 0x8769)
	made := "?"
	if m, ok := j.madeAs(); ok {
		made = fmt.Sprintf("%dx%d", m.X, m.Y)
	}
	later := e.text(0x9003) != "" && t.ifd0.text(0x0132) != "" && e.text(0x9003) != t.ifd0.text(0x0132)
	return fmt.Sprintf("made %s, is %dx%d, saved by %q, later %v", made, j.width, j.height, t.ifd0.text(0x0131), later)
}
