package main

// The real files: what other people's phones, editors and test suites
// wrote, listed in testdata/samples.tsv and downloaded by
// testdata/fetch.sh. Nothing of them is in this repository. Without
// PENTIMENTO_SAMPLES pointing at the folder they were downloaded to, this
// is skipped; samples.yml runs it on every push.

import (
	"bufio"
	"bytes"
	"image"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type sample struct {
	name, url string
	expect    []string
	original  string
}

func readSamples(t *testing.T) []sample {
	f, err := os.Open(filepath.Join("testdata", "samples.tsv"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out []sample
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := s.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		cols := strings.Split(line, "\t")
		if len(cols) < 3 {
			t.Fatalf("a line of samples.tsv has %d columns: %q", len(cols), line)
		}
		x := sample{name: cols[0], url: cols[1]}
		if cols[2] != "-" {
			x.expect = strings.Split(cols[2], ",")
		}
		if len(cols) > 3 {
			x.original = cols[3]
		}
		out = append(out, x)
	}
	return out
}

func TestSamples(t *testing.T) {
	dir := os.Getenv("PENTIMENTO_SAMPLES")
	if dir == "" {
		t.Skip("PENTIMENTO_SAMPLES is not set; see testdata/fetch.sh")
	}
	for _, x := range readSamples(t) {
		t.Run(x.name, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join(dir, x.name))
			if err != nil {
				t.Fatalf("not downloaded: %v", err)
			}
			c := newRun(shower{})
			c.inspect(x.name, x.name, data, 0)
			if len(c.gaps) > 0 {
				t.Errorf("could not read all of it: %v", c.gaps)
			}
			c.only(t, x.expect...)
			if x.original != "" && strings.HasSuffix(x.name, ".jpg") {
				sameBlocks(t, data, filepath.Join(dir, x.original))
			} else if x.original != "" {
				exactBytes(t, data, filepath.Join(dir, x.original))
			}
		})
	}
}

// exactBytes checks every byte of the old picture said to come back
// against the picture as it was before the crop. One wrong byte fails it:
// a recovery that guesses would be worse than one that says less.
func exactBytes(t *testing.T, cropped []byte, originalPath string) {
	t.Helper()
	raw, err := os.ReadFile(originalPath)
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	orig := image.NewNRGBA(img.Bounds())
	draw.Draw(orig, orig.Bounds(), img, img.Bounds().Min, draw.Src)
	p := readPNG(cropped)
	stream, ok := oldChunks(cropped[p.end:])
	if !ok {
		t.Fatal("no old chunks")
	}
	var got *inflated
	for bit := 0; bit < len(stream)*8; bit++ {
		if g, err := inflateFrom(stream, bit, 256<<20, nil); err == nil && len(stream)*8-g.end >= 32 && len(stream)*8-g.end <= 39 {
			got = g
			break
		}
	}
	if got == nil {
		t.Fatal("did not decode")
	}
	w := orig.Rect.Dx()
	recon, _ := unfiltered(got.out, p, w)
	known, wrong := 0, 0
	for r, row := range recon {
		y := orig.Rect.Dy() - len(recon) + r
		truth := orig.Pix[y*orig.Stride : y*orig.Stride+4*w]
		for i, v := range row {
			if v == unknown {
				continue
			}
			known++
			if byte(v) != truth[i] {
				wrong++
			}
		}
	}
	if known == 0 || wrong > 0 {
		t.Errorf("%d bytes came back, %d of them wrong", known, wrong)
	}
	t.Logf("%d bytes of the old picture came back, all of them right", known)
}

// TestMadeByTools reads what testdata/tools.sh made with ImageMagick,
// ExifTool, jpegtran and Pillow. The kinds to find are in each name,
// before the two underscores; clean is nothing.
func TestMadeByTools(t *testing.T) {
	dir := os.Getenv("PENTIMENTO_TOOLS")
	if dir == "" {
		t.Skip("PENTIMENTO_TOOLS is not set; see testdata/tools.sh")
	}
	files, _ := filepath.Glob(filepath.Join(dir, "*__*"))
	if len(files) < 7 {
		t.Fatalf("only %d files made", len(files))
	}
	for _, f := range files {
		name := filepath.Base(f)
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			c := newRun(shower{})
			c.inspect(name, name, data, 0)
			var want []string
			if k := strings.SplitN(name, "__", 2)[0]; k != "clean" {
				want = strings.Split(k, "+")
			}
			c.only(t, want...)
		})
	}
}

// sameBlocks checks the end of an earlier JPEG decoded from behind a crop
// against the JPEG as it was before: the average of every MCU must follow
// the original's, as it does when every block came back but for the one
// amount of brightness that went with the overwritten part.
func sameBlocks(t *testing.T, cropped []byte, originalPath string) {
	t.Helper()
	j := readJPEG(cropped)
	o, ok := decodeTail(cropped[:j.end], tailOfOld(cropped, j.end), j.width)
	if !ok || o.perRow == 0 {
		t.Fatal("the tail did not decode")
	}
	raw, err := os.ReadFile(originalPath)
	if err != nil {
		t.Fatal(err)
	}
	img, err := decodePicture(raw)
	if err != nil {
		t.Fatal(err)
	}
	r := tailMatch(o, img)
	if want := (img.Bounds().Dx() + 15) / 16; o.perRow != want {
		t.Errorf("%d MCUs across, and the original has %d", o.perRow, want)
	}
	if r < 0.99 {
		t.Errorf("the decoded tail follows the original at %.4f", r)
	}
	t.Logf("%d MCUs decoded, following the original at %.4f", o.mcus, r)
}
