package main

// Which files are looked at, what they are, and how their paths are shown.

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// The kind of a file is decided by its first bytes, not its name: a
// screenshot saved as .jpg by an app that wrote a PNG is still a PNG, and
// still carries what a PNG can.
func sniff(head []byte, name string) string {
	ext := strings.ToLower(filepath.Ext(name))
	switch {
	case bytes.HasPrefix(head, pngMagic):
		return "png"
	case bytes.HasPrefix(head, []byte{0xff, 0xd8, 0xff}):
		return "jpeg"
	case bytes.HasPrefix(head, []byte("GIF87a")), bytes.HasPrefix(head, []byte("GIF89a")):
		return "gif"
	case len(head) >= 12 && string(head[4:8]) == "ftyp":
		brand := string(head[8:12])
		switch brand {
		case "heic", "heix", "heim", "heis", "mif1", "msf1", "hevc", "hevx":
			return "heic"
		case "avif", "avis":
			return "avif"
		}
		return "video"
	case bytes.HasPrefix(head, []byte("PK\x03\x04")):
		switch ext {
		case ".docx", ".docm", ".pptx", ".pptm", ".xlsx", ".xlsm", ".odt", ".odp", ".ods", ".odg":
			return "office"
		}
		return ""
	case ext == ".svg", bytes.Contains(head, []byte("<svg")), bytes.HasPrefix(head, []byte("<?xml")):
		return "text-drawing"
	case bytes.HasPrefix(head, []byte("RIFF")) && len(head) >= 12 && string(head[8:12]) == "WEBP":
		return "webp"
	case bytes.HasPrefix(head, []byte("II*\x00")), bytes.HasPrefix(head, []byte("MM\x00*")):
		return "tiff"
	case bytes.HasPrefix(head, []byte("8BPS")):
		return "psd"
	case bytes.HasPrefix(head, []byte("%PDF")):
		return "pdf"
	case bytes.HasPrefix(head, []byte{0xd0, 0xcf, 0x11, 0xe0, 0xa1, 0xb1, 0x1a, 0xe1}):
		// An Office file saved with a password is an OLE container with
		// the real one encrypted inside it.
		return "locked"
	}
	return ""
}

// unreadKinds are found and named, not skipped in silence: a folder of
// WebP files with nothing found in it has not been checked.
var unreadKinds = map[string]string{
	"webp":   "WebP images",
	"avif":   "AVIF images",
	"tiff":   "TIFF images, which can keep a Photoshop file's layers",
	"psd":    "Photoshop files, which are layers by design",
	"pdf":    "PDFs, which Baggage and x-ray read",
	"locked": "documents locked with a password, or in the old binary formats",
}

// The extensions worth opening at all when walking a folder. Anything else
// is left alone without being read, so pointing this at a home folder does
// not mean reading every file in it.
var worthOpening = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".jfif": true, ".gif": true,
	".heic": true, ".heif": true, ".hif": true, ".avif": true, ".webp": true,
	".tif": true, ".tiff": true, ".psd": true, ".pdf": true,
	".svg":  true,
	".docx": true, ".docm": true, ".pptx": true, ".pptm": true, ".xlsx": true, ".xlsm": true,
	".odt": true, ".odp": true, ".ods": true, ".odg": true,
}

// filesIn lists what is under the paths given. A file named on the
// command line is always opened, whatever it is called.
// The error names the path the way the report would, home folder as ~:
// the system's own message spells out the whole path, user name and all.
func filesIn(paths []string, show shower) ([]string, error) {
	var out []string
	for _, root := range paths {
		abs, err := filepath.Abs(root)
		if err != nil {
			return nil, err
		}
		info, err := os.Stat(abs)
		if err != nil {
			return nil, fmt.Errorf("%s: no such file or folder, or it cannot be opened", show.path(abs))
		}
		if !info.IsDir() {
			out = append(out, abs)
			continue
		}
		err = filepath.WalkDir(abs, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				if p != abs && (d.Name() == ".git" || d.Name() == "node_modules") {
					return fs.SkipDir
				}
				return nil
			}
			if d.Type().IsRegular() && worthOpening[strings.ToLower(filepath.Ext(p))] {
				out = append(out, p)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Strings(out)
	return out, nil
}

// shower writes paths the way the report shows them: the home folder as
// ~, which keeps the one part of a path that names a person out of a
// report that might get pasted somewhere. --reveal shows it.
type shower struct {
	home   string
	reveal bool
}

func (s shower) path(p string) string {
	if s.reveal || s.home == "" {
		return p
	}
	rel, err := filepath.Rel(s.home, p)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return p
	}
	return "~/" + filepath.ToSlash(rel)
}

// text is for words that came out of somebody's file: the labels in a
// diagram, the name of a page. Masked down to their first and last
// letters unless --reveal is given, which is enough for the owner to
// recognise their own and not enough for anybody else to read.
func (s shower) text(t string) string {
	t = strings.Join(strings.Fields(t), " ")
	if runes(t) > 60 {
		t = string([]rune(t)[:57]) + "..."
	}
	if s.reveal {
		return t
	}
	return maskText(t)
}
