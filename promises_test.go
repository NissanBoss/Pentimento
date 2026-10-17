package main

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The README makes promises. Promises in a README rot; promises with a test
// behind them break the build first.

// It only reads. The files it is given are somebody's, and what it finds
// inside them is said, never taken out and saved: there is no way for it
// to write the old screenshot to disk, because there is no way for it to
// write anything.
func TestNeverWrites(t *testing.T) {
	banned := []string{
		"os.WriteFile", "os.Create", "os.OpenFile", "os.Remove", "os.RemoveAll",
		"os.Rename", "os.Truncate", "os.Chmod", "os.Chown", "os.Mkdir",
		"os.MkdirAll", "os.Symlink", "os.Link", "os.Chtimes", "os.CreateTemp",
		"ioutil.WriteFile",
	}
	scanSource(t, banned, "this tool must not write anything")
}

// No network code at all: not a promise not to send your files anywhere,
// but no way to.
func TestNeverOpensTheNetwork(t *testing.T) {
	banned := []string{`"net`, "net.Dial", "http.Get", "http.Post", "syscall.Socket"}
	scanSource(t, banned, "this tool must have no way to send anything anywhere")
}

func TestNeverRunsAnotherProgram(t *testing.T) {
	scanSource(t, []string{`"os/exec"`, "syscall.Exec", "syscall.ForkExec", "StartProcess"},
		"this tool must not run another program")
}

func scanSource(t *testing.T, banned []string, why string) {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	looked := 0
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		looked++
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, bad := range banned {
			if bytes.Contains(src, []byte(bad)) {
				t.Errorf("%s contains %q: %s", name, bad, why)
			}
		}
	}
	if looked < 15 {
		t.Fatalf("only %d source files were read, so this test proved nothing", looked)
	}
}

// The long dash is not used anywhere in this project, in code, comments,
// documentation or anything the program prints. Written as a number here
// so this file passes its own test.
func TestNoLongDash(t *testing.T) {
	dash := []byte(string(rune(0x2014)))
	looked := 0
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != "." && (strings.HasPrefix(d.Name(), ".") && d.Name() != ".github" || d.Name() == "dist") {
				return fs.SkipDir
			}
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if bytes.IndexByte(body, 0) >= 0 {
			return nil
		}
		looked++
		if bytes.Contains(body, dash) {
			t.Errorf("%s has a long dash in it", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if looked < 15 {
		t.Fatalf("only %d files were looked at", looked)
	}
}

// The home folder is written as ~ unless --reveal is given.
func TestTheHomeFolderIsNotShown(t *testing.T) {
	home := filepath.Join(t.TempDir(), "marisol")
	p := filepath.Join(home, "Pictures", "shot.png")
	if got := (shower{home: home}).path(p); got != "~/Pictures/shot.png" {
		t.Errorf("shown as %s", got)
	}
	if got := (shower{home: home, reveal: true}).path(p); got != p {
		t.Errorf("--reveal showed %s", got)
	}
	if got := (shower{home: home}).path(home + "-backup/x.png"); got != home+"-backup/x.png" && got != filepath.Join(home+"-backup", "x.png") {
		t.Errorf("a folder beside the home folder came out as %s", got)
	}
}

// Words that came out of somebody's file are masked unless --reveal is
// given, and keep their shape.
func TestWordsFromFilesAreMasked(t *testing.T) {
	if got := (shower{}).text("Q3 layoffs (draft)"); got != "Q* l*****s (d***t)" {
		t.Errorf("masked as %q", got)
	}
	if got := (shower{reveal: true}).text("Q3 layoffs (draft)"); got != "Q3 layoffs (draft)" {
		t.Errorf("--reveal gave %q", got)
	}
}

// What it could not read is said, never counted as clean.
func TestAFileItCannotReadIsSaid(t *testing.T) {
	c := check(t, "broken.png", append(append([]byte{}, pngMagic...), 0, 0, 0, 13, 'I', 'H'))
	if len(c.gaps) == 0 {
		t.Error("a broken PNG produced no gap")
	}
	r := c.report()
	if len(r.Gaps) == 0 {
		t.Error("the report did not say what was not read")
	}
	for _, f := range r.Findings {
		if f.Tag == "CLEAR   " {
			t.Error("a report with something unread called itself clear")
		}
	}
}
