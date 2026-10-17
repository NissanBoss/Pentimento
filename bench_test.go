package main

// The bench made by testdata/bench.sh: how often a box that lets things
// through, and a mosaic, are found, by opacity, colour, block size, page
// and format. It fails only on a clean page with anything found on it;
// the rates are for reading, and for the README.

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestBench(t *testing.T) {
	dir := os.Getenv("PENTIMENTO_BENCH")
	if dir == "" {
		t.Skip("PENTIMENTO_BENCH is not set; see testdata/bench.sh")
	}
	files, _ := filepath.Glob(filepath.Join(dir, "*__*"))
	type tally struct{ found, of int }
	by := map[string]*tally{}
	count := func(key string, hit bool) {
		if by[key] == nil {
			by[key] = &tally{}
		}
		by[key].of++
		if hit {
			by[key].found++
		}
	}
	for _, f := range files {
		name := filepath.Base(f)
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		c := newRun(shower{})
		c.inspect(name, name, data, 0)
		kind, rest, _ := strings.Cut(strings.TrimSuffix(name, filepath.Ext(name)), "__")
		format := strings.TrimPrefix(filepath.Ext(name), ".")
		parts := strings.Split(rest, "_")
		switch kind {
		case "clean":
			if len(c.hits) > 0 {
				t.Errorf("%s: found %v on a clean page", name, c.found())
			}
		case "box":
			hit := c.has("shows-through")
			colour, op, look := parts[0], parts[1], parts[2]
			count("box "+format+" opacity "+op, hit)
			count("box "+format+" "+colour, hit)
			count("box "+format+" page "+look, hit)
		case "mosaic":
			hit := c.has("mosaic")
			block, look := parts[0], parts[1]
			count(fmt.Sprintf("mosaic %s block %02s", format, block), hit)
			count("mosaic "+format+" page "+look, hit)
		}
	}
	var keys []string
	for k := range by {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var lines []string
	for _, k := range keys {
		lines = append(lines, fmt.Sprintf("%-28s %3d of %3d", k, by[k].found, by[k].of))
	}
	t.Log("\n" + strings.Join(lines, "\n"))
}
