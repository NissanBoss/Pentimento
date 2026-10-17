package main

// The calibration: hundreds of real photos and screenshots, downloaded by
// testdata/calibrate.sh on GitHub's machines, and counted. It is where the
// numbers in the README come from, and where a rule that fires on
// ordinary files shows up as a count that should be near nothing: a
// mosaic in a sky, a box in a toolbar, a thumbnail that differs because a
// phone tone-maps its thumbnails on their own. Only the counts are ever
// printed; the files are other people's.

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
		sort.Strings(files)
		counts := map[string]int{}
		read, gaps := 0, 0
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
			for k, hits := range c.hits {
				counts[k]++
				if kindsOfHit[k].weight >= Weak {
					for _, h := range hits {
						loud = append(loud, fmt.Sprintf("%s/%s: %s: %s", g, filepath.Base(f), k, h.what))
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
		table = append(table, fmt.Sprintf("%-12s %4d read, %d with a gap: %s", g, read, gaps, strings.Join(parts, ", ")))
		// Every loud finding, by file number, so each one can be looked at
		// by hand: a rule is only kept when the ones it finds are real.
		for _, l := range loud {
			t.Log(l)
		}
	}
	t.Log("\n" + strings.Join(table, "\n"))
}
