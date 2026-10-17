package main

// The chain of cross-reference sections, which is the history of the file.
//
// A PDF can be saved by appending. The editor writes the objects that
// changed, a new index covering just those, and a pointer back to the index
// before it. The old objects are not removed, because nothing in the format
// removes anything: they sit in the file above the new ones and stop being
// referred to.
//
// So a PDF that has been edited contains every version of itself. Following
// the chain of Prev pointers gives you those versions, and that is the only
// way to find out what a document said before somebody changed it.
//
// Parse elsewhere in this program does not use any of this to find objects:
// it scans for them, because a broken index is common and a file with one is
// still worth reading. What the chain is for is knowing how many times the
// file was saved and which definition of an object belongs to which save.

import (
	"bytes"
	"fmt"
	"strconv"
)

type slot struct {
	// Kind is 'n' for an object at Offset, 'c' for one packed inside the
	// object stream numbered Stm, and 'f' for a slot that was freed.
	Kind   byte
	Offset int
	Stm    int
	Index  int
	Gen    int
}

// Revision is one saved state of the file: the index that describes it and
// the objects that index puts in the file for the first time or afresh.
type Revision struct {
	// StartXref is where this revision's index begins, which is also how
	// far into the file this version of the document ended.
	StartXref int
	Trailer   Dict
	Slots     map[int]slot
	// Stream is true when the index was a compressed stream rather than a
	// table of text, which is what a PDF written this century uses.
	Stream bool
}

// history walks the chain from the last index backwards and returns the
// revisions oldest first, which is the order somebody reads them in.
func history(data []byte, doc *Document) ([]Revision, []string) {
	var trouble []string
	start, ok := lastStartXref(data)
	if !ok {
		return nil, []string{"the file does not end with a startxref, so its index could not be found"}
	}

	var found []Revision
	seen := map[int]bool{}
	at := start
	for len(found) < 64 {
		if at < 0 || at >= len(data) || seen[at] {
			if seen[at] {
				trouble = append(trouble, fmt.Sprintf(
					"the chain of previous indexes loops back to offset %d, so it was not followed further", at))
			}
			break
		}
		seen[at] = true

		rev, why := readXrefAt(data, at, doc)
		if why != "" {
			trouble = append(trouble, why)
			break
		}
		found = append(found, rev)

		// A hybrid file keeps a table for old readers and a stream beside
		// it holding the objects the table cannot describe. Both belong to
		// this same save.
		if extra, ok := asInt(doc.Resolve(rev.Trailer["XRefStm"])); ok && !seen[extra] {
			seen[extra] = true
			if side, why := readXrefAt(data, extra, doc); why == "" {
				for num, s := range side.Slots {
					if _, already := found[len(found)-1].Slots[num]; !already {
						found[len(found)-1].Slots[num] = s
					}
				}
			}
		}

		prev, ok := asInt(doc.Resolve(rev.Trailer["Prev"]))
		if !ok {
			break
		}
		at = prev
	}

	// Reverse, so the first revision in the slice is the first one written.
	for i, j := 0, len(found)-1; i < j; i, j = i+1, j-1 {
		found[i], found[j] = found[j], found[i]
	}
	return found, trouble
}

func lastStartXref(data []byte) (int, bool) {
	// The pointer is near the end, and a file with rubbish appended still
	// has to be read, so the search is bounded rather than anchored.
	tail := data
	from := 0
	if len(data) > 2048 {
		from = len(data) - 2048
		tail = data[from:]
	}
	at := bytes.LastIndex(tail, []byte("startxref"))
	if at < 0 {
		return 0, false
	}
	lex := &lexer{data: data, pos: from + at + len("startxref")}
	lex.skipSpace()
	n, err := lex.readNumber()
	if err != nil {
		return 0, false
	}
	return int(n), true
}

func readXrefAt(data []byte, at int, doc *Document) (Revision, string) {
	if at < 0 || at >= len(data) {
		return Revision{}, fmt.Sprintf("an index is said to be at offset %d and the file is %d bytes", at, len(data))
	}
	lex := &lexer{data: data, pos: at}
	lex.skipSpace()
	if lex.peekWord() == "xref" {
		return readXrefTable(data, at, doc)
	}
	return readXrefStream(data, at, doc)
}

// readXrefTable reads the plain text kind: subsection headers followed by
// twenty byte entries.
func readXrefTable(data []byte, at int, doc *Document) (Revision, string) {
	rev := Revision{StartXref: at, Slots: map[int]slot{}}
	lex := &lexer{data: data, pos: at}
	lex.skipSpace()
	lex.readWord() // xref

	for {
		lex.skipSpace()
		if lex.peekWord() == "trailer" {
			lex.readWord()
			value, err := lex.readObject(doc)
			if err != nil {
				return rev, fmt.Sprintf("the trailer at offset %d could not be read: %v", at, err)
			}
			if d, ok := value.(Dict); ok {
				rev.Trailer = d
			}
			return rev, ""
		}
		first, err := lex.readNumber()
		if err != nil {
			return rev, fmt.Sprintf("the index at offset %d stops making sense at byte %d", at, lex.pos)
		}
		lex.skipSpace()
		count, err := lex.readNumber()
		if err != nil {
			return rev, fmt.Sprintf("the index at offset %d gives a subsection with no count", at)
		}
		if count < 0 || count > 1<<22 {
			return rev, fmt.Sprintf("the index at offset %d claims a subsection of %d entries", at, int(count))
		}
		for i := 0; i < int(count); i++ {
			lex.skipSpace()
			offset, err1 := lex.readNumber()
			lex.skipSpace()
			gen, err2 := lex.readNumber()
			lex.skipSpace()
			kind := lex.readWord()
			if err1 != nil || err2 != nil || (kind != "n" && kind != "f") {
				return rev, fmt.Sprintf("entry %d of the index at offset %d is not an entry", i, at)
			}
			num := int(first) + i
			if kind == "f" {
				rev.Slots[num] = slot{Kind: 'f', Gen: int(gen)}
				continue
			}
			rev.Slots[num] = slot{Kind: 'n', Offset: int(offset), Gen: int(gen)}
		}
	}
}

// readXrefStream reads the compressed kind, which is an ordinary object
// whose stream holds fixed width rows described by W.
func readXrefStream(data []byte, at int, doc *Document) (Revision, string) {
	rev := Revision{StartXref: at, Slots: map[int]slot{}, Stream: true}

	j := bytes.Index(data[at:], []byte("obj"))
	if j < 0 || j > 64 {
		return rev, fmt.Sprintf("there is neither a table nor a stream at offset %d, where an index should be", at)
	}
	lex := &lexer{data: data, pos: at + j + 3}
	value, err := lex.readObject(doc)
	if err != nil {
		return rev, fmt.Sprintf("the index stream at offset %d could not be read: %v", at, err)
	}
	stream, ok := value.(*Stream)
	if !ok {
		return rev, fmt.Sprintf("the object at offset %d is an index and is not a stream", at)
	}
	rev.Trailer = stream.Dict

	body, err := doc.Decoded(stream)
	if err != nil {
		return rev, fmt.Sprintf("the index stream at offset %d would not decode: %v", at, err)
	}

	widths, ok := doc.Resolve(stream.Dict["W"]).(Array)
	if !ok || len(widths) < 3 {
		return rev, fmt.Sprintf("the index stream at offset %d does not say how wide its columns are", at)
	}
	var w [8]int
	total := 0
	for i, item := range widths {
		if i >= len(w) {
			break
		}
		n, _ := asInt(doc.Resolve(item))
		if n < 0 || n > 8 {
			return rev, fmt.Sprintf("the index stream at offset %d gives column %d a width of %d", at, i, n)
		}
		w[i] = n
		total += n
	}
	if total == 0 {
		return rev, fmt.Sprintf("the index stream at offset %d has columns of no width at all", at)
	}

	// Index says which object numbers the rows are for. Without one the
	// rows start at zero and run for Size entries.
	type run struct{ first, count int }
	var runs []run
	if list, ok := doc.Resolve(stream.Dict["Index"]).(Array); ok {
		for i := 0; i+1 < len(list); i += 2 {
			first, _ := asInt(doc.Resolve(list[i]))
			count, _ := asInt(doc.Resolve(list[i+1]))
			runs = append(runs, run{first, count})
		}
	}
	if len(runs) == 0 {
		size, _ := asInt(doc.Resolve(stream.Dict["Size"]))
		runs = append(runs, run{0, size})
	}

	pos := 0
	field := func(width int) int {
		n := 0
		for i := 0; i < width; i++ {
			if pos < len(body) {
				n = n<<8 | int(body[pos])
			}
			pos++
		}
		return n
	}
	for _, r := range runs {
		if r.count < 0 || r.count > 1<<22 {
			return rev, fmt.Sprintf("the index stream at offset %d claims a run of %d entries", at, r.count)
		}
		for i := 0; i < r.count; i++ {
			if pos+total > len(body) {
				// A short stream is worth what was read rather than
				// nothing, and the caller is told how far it got.
				return rev, fmt.Sprintf(
					"the index stream at offset %d describes more entries than it holds bytes for, so %d of them were read",
					at, len(rev.Slots))
			}
			kind := 1
			if w[0] > 0 {
				kind = field(w[0])
			}
			second := field(w[1])
			third := field(w[2])
			for k := 3; k < len(widths) && k < len(w); k++ {
				field(w[k]) // columns past the third are not defined by the spec
			}
			num := r.first + i
			switch kind {
			case 0:
				rev.Slots[num] = slot{Kind: 'f', Gen: third}
			case 1:
				rev.Slots[num] = slot{Kind: 'n', Offset: second, Gen: third}
			case 2:
				rev.Slots[num] = slot{Kind: 'c', Stm: second, Index: third}
			}
		}
	}
	return rev, ""
}

// linearized says whether the file was laid out for reading over a slow
// link, which puts a second index near the front so the first page can be
// shown before the rest arrives.
//
// This matters because that second index is not a second save. A reader
// counting indexes would tell somebody their file has a version in it that
// it does not have, and a warning that fires on ordinary files is a warning
// people learn to ignore.
func linearized(data []byte) bool {
	head := data
	if len(head) > 2048 {
		head = head[:2048]
	}
	return bytes.Contains(head, []byte("/Linearized"))
}

// saves is how many times the file was written, which is the chain of
// indexes less the one linearisation adds.
func saves(data []byte, revisions []Revision) int {
	n := len(revisions)
	if n > 1 && linearized(data) {
		n--
	}
	return n
}

// eofsIn counts the end of file markers, which is the cheapest way to say
// how many times a file was saved and works even when the chain is broken.
func eofsIn(data []byte) int {
	return bytes.Count(data, []byte("%%EOF"))
}

// numberAt reads the object number of the definition sitting at an offset,
// so a slot can be checked against what is really there.
func numberAt(data []byte, at int) (int, bool) {
	if at < 0 || at >= len(data) {
		return 0, false
	}
	lex := &lexer{data: data, pos: at}
	lex.skipSpace()
	word := lex.readWord()
	n, err := strconv.Atoi(word)
	if err != nil {
		return 0, false
	}
	return n, true
}
