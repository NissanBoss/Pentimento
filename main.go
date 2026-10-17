// Pentimento says whether what you cropped, covered or blurred in a
// picture is really gone: whether the old picture is still behind the end
// of a screenshot, the photo from before the blur still inside a portrait,
// the uncropped original still inside a slide, or what is under a box
// still showing through it.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strings"
)

var version = "unreleased"

const (
	exitClean   = 0
	exitFound   = 1 // something that was taken out is still in a file
	exitBroken  = 2
	exitNothing = 3 // nothing could be read, which is not the same as nothing found
)

// The biggest file read. A picture is rarely a tenth of this; a video
// that somebody named .jpg could be anything.
const tooBig = 1 << 30

func main() {
	reveal := flag.Bool("reveal", false, "show the home folder in paths, and the words found in files, instead of masking them")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Usage = usage
	flag.Parse()
	if *showVersion {
		fmt.Println("Pentimento " + version)
		return
	}
	if flag.NArg() == 0 {
		usage()
		os.Exit(exitBroken)
	}
	home, _ := os.UserHomeDir()
	show := shower{home: home, reveal: *reveal}
	// One file at a time, and a ceiling the garbage collector works to
	// stay under: a photo of fifty megapixels is most of a gigabyte while
	// its copies are compared, and this must never be what slows a machine
	// to a halt.
	debug.SetMemoryLimit(2 << 30)

	files, err := filesIn(flag.Args(), show)
	if err != nil {
		fmt.Fprintf(os.Stderr, "pentimento: %v\n", err)
		os.Exit(exitBroken)
	}
	c := newRun(show)
	for _, f := range files {
		c.file(f)
	}
	r := c.report()
	r.render(os.Stdout)
	switch {
	case c.read == 0:
		os.Exit(exitNothing)
	case r.worst() >= Weak:
		os.Exit(exitFound)
	}
	os.Exit(exitClean)
}

// A run is everything found at once, gathered by kind so that ten
// screenshots with the same problem make one finding with ten lines.
type run struct {
	show   shower
	hits   map[string][]hit
	gaps   []string
	unread map[string][]string
	read   int
	// heicRead says that a HEIC was read, for the line that says what
	// reading one does not cover.
	heicRead bool
	checked  map[string]int // what was looked at, for the summary
}

type hit struct {
	where string   // the path as shown, with the part inside a file after " > "
	what  string   // the line about it
	more  []string // lines under it
}

func newRun(show shower) *run {
	return &run{show: show, hits: map[string][]hit{}, unread: map[string][]string{}, checked: map[string]int{}}
}

func (c *run) hit(kind, where, what string, more ...string) {
	c.hits[kind] = append(c.hits[kind], hit{where, what, more})
}

func (c *run) gap(where, why string) {
	c.gaps = append(c.gaps, where+": "+why)
}

func (c *run) file(path string) {
	where := c.show.path(path)
	info, err := os.Stat(path)
	if err != nil {
		c.gap(where, "could not be opened")
		return
	}
	if info.Size() > tooBig {
		c.gap(where, fmt.Sprintf("%s is more than this reads", size(info.Size())))
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		c.gap(where, "could not be read")
		return
	}
	c.inspect(where, filepath.Base(path), data, 0)
}

// inspect looks at one file, or one file inside another. depth stops a
// document that holds a picture that holds a document from going on
// forever.
func (c *run) inspect(where, name string, data []byte, depth int) {
	if depth > 3 {
		return
	}
	kind := sniff(data[:min(len(data), 64)], name)
	if what, ok := unreadKinds[kind]; ok {
		c.unread[what] = append(c.unread[what], where)
		return
	}
	switch kind {
	case "png":
		c.png(where, data, depth)
	case "jpeg":
		c.jpeg(where, data)
	case "gif":
		c.gif(where, data)
	case "heic":
		c.heic(where, data)
	case "office":
		c.office(where, data, depth)
	case "pdf":
		c.pdf(where, data, depth)
	case "text-drawing":
		c.textDrawing(where, data, depth)
	default:
		if depth == 0 && worthOpening[strings.ToLower(filepath.Ext(name))] {
			c.gap(where, "is not the kind of file its name says, and not one this reads")
		}
		return
	}
	// Files inside files are looked at, and not counted: the summary
	// says what was given, not how many pictures were in the documents.
	if depth == 0 {
		c.read++
		c.checked[kind]++
	}
}

func (c *run) report() *Report {
	r := &Report{Title: "Pentimento: whether what you took out of these files is gone"}
	var kinds []string
	for k := range c.checked {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)
	var parts []string
	for _, k := range kinds {
		parts = append(parts, plural(c.checked[k], kindNames[k][0], kindNames[k][1]))
	}
	if len(parts) == 0 {
		r.Summary = append(r.Summary, "Nothing here was a file this reads.")
	} else {
		r.Summary = append(r.Summary, "Looked at "+joinAnd(parts)+".")
		if c.heicRead {
			r.Summary = append(r.Summary, "A HEIC is read for what it keeps beside the photo, not for its pixels, which this does not decode: what is under a box or a mosaic in one is not looked for.")
		}
	}
	r.Gaps = append(r.Gaps, c.gaps...)
	var unread []string
	for what := range c.unread {
		unread = append(unread, what)
	}
	sort.Strings(unread)
	for _, what := range unread {
		files := c.unread[what]
		r.Gaps = append(r.Gaps, fmt.Sprintf("%s: %s, not read, the first of them %s", plural(len(files), "file", "files"), what, files[0]))
	}
	for _, k := range kindOrder {
		hits := c.hits[k]
		if len(hits) == 0 {
			continue
		}
		info := kindsOfHit[k]
		var lines []string
		for i, h := range hits {
			if i > 0 {
				lines = append(lines, "")
			}
			lines = append(lines, h.where)
			for _, m := range append([]string{h.what}, h.more...) {
				for _, l := range wrapText(m, wrap-2) {
					lines = append(lines, "  "+l)
				}
			}
		}
		title := info.one
		if len(hits) > 1 {
			title = fmt.Sprintf(info.many, len(hits))
		}
		r.add(Finding{Tag: info.tag, Weight: info.weight, Title: title, Lines: lines, Advice: info.advice})
	}
	if r.worst() < Weak && c.read > 0 && len(r.Gaps) == 0 {
		r.add(Finding{Tag: "CLEAR   ", Weight: Note,
			Title:  "Nothing that was taken out turned up",
			Advice: "That covers what this knows to look for, listed in its README. A crop or a cover that left no copy in the file and no trace in the pixels is gone as far as anything here can tell."})
	}
	return r
}

var kindNames = map[string][2]string{
	"png":          {"PNG", "PNGs"},
	"jpeg":         {"JPEG", "JPEGs"},
	"gif":          {"GIF", "GIFs"},
	"heic":         {"HEIC photo", "HEIC photos"},
	"office":       {"document", "documents"},
	"pdf":          {"PDF", "PDFs"},
	"text-drawing": {"drawing", "drawings"},
}

func joinAnd(parts []string) string {
	switch len(parts) {
	case 0:
		return ""
	case 1:
		return parts[0]
	}
	return strings.Join(parts[:len(parts)-1], ", ") + " and " + parts[len(parts)-1]
}

func usage() {
	fmt.Fprintln(os.Stderr, `Pentimento - whether what you took out of a picture is gone

  pentimento screenshot.png             one file
  pentimento ~/Desktop/to-post/         a folder, and everything under it
  pentimento --reveal ...               show the home folder and the words found
  pentimento --version

A crop, a box drawn over a name, a blurred background: each looks final
on the screen, and each can leave what it took out inside the file. The
old screenshot behind the end of the cropped one, the photo before the
blur inside the portrait, the whole picture inside the slide that shows a
corner of it, the text that shows through a box that is not quite solid.
This says which of your files still have it, and how much of it.

It only reads. Nothing is written, nothing is taken out of the files to
show you, and nothing is sent anywhere.`)
}
