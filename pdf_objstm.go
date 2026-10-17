package main

// Objects packed inside a stream.
//
// From PDF 1.5 onwards a file may put most of its objects inside a
// compressed stream instead of writing them out one by one. What is left in
// the clear is the streams themselves, because a stream cannot be packed
// inside another one, and everything that is a dictionary goes inside: the
// catalogue, the page tree, the resources every page draws from, the
// annotations, the form fields, the list of attached files.
//
// A reader that scans the bytes for "12 0 obj" finds none of it. On the tax
// form this was tested against, eighty-five objects are in the clear and
// nine hundred and ninety three are inside seventeen of these, and the
// second number is where the page tree and the attachments live.

import (
	"fmt"
)

// unpack expands every object stream in the document, putting what it finds
// alongside the objects that were already in the clear.
//
// An object already read from the file wins over one found in here. The
// packed copy belongs to whichever save wrote that stream, and the loose
// copy is the one a later save appended, so preferring the loose one keeps
// the newest definition without needing the index to say so.
func (d *Document) unpack() {
	var carriers []Ref
	for ref, obj := range d.Objects {
		if s, ok := obj.(*Stream); ok && s.Dict["Type"] == Name("ObjStm") {
			carriers = append(carriers, ref)
		}
	}
	if len(carriers) == 0 {
		return
	}
	sortRefs(carriers)

	opened, found := 0, 0
	for _, ref := range carriers {
		stream := d.Objects[ref].(*Stream)
		got, err := d.insideObjStm(ref, stream)
		if err != nil {
			d.note("object stream %d could not be opened: %v", ref.Num, err)
			continue
		}
		opened++
		for num, value := range got {
			at := Ref{num, 0}
			d.Seen[at] = append(d.Seen[at], sighting{Value: value, Offset: -1, Stm: ref.Num})
			if _, already := d.Objects[at]; already {
				continue
			}
			d.Live[at] = len(d.Seen[at]) - 1
			d.Objects[at] = value
			d.Packed[at] = ref.Num
			found++
		}
	}
	if found > 0 {
		d.Unpacked = found
		d.Carriers = opened
	}
}

// insideObjStm reads the pairs at the front of the stream and then each
// object they point at.
func (d *Document) insideObjStm(ref Ref, s *Stream) (map[int]any, error) {
	body, err := d.Decoded(s)
	if err != nil {
		return nil, err
	}
	count, ok := asInt(d.Resolve(s.Dict["N"]))
	if !ok {
		return nil, fmt.Errorf("it does not say how many objects it holds")
	}
	first, ok := asInt(d.Resolve(s.Dict["First"]))
	if !ok {
		return nil, fmt.Errorf("it does not say where its objects start")
	}
	if count < 0 || count > 1<<20 {
		return nil, fmt.Errorf("it claims to hold %d objects", count)
	}
	if first < 0 || first > len(body) {
		return nil, fmt.Errorf("it says its objects start at %d and it is %d bytes long", first, len(body))
	}

	// The front of the stream is pairs of numbers: an object number and
	// where that object starts, counted from First.
	head := &lexer{data: body[:first], pos: 0}
	type where struct{ num, at int }
	var places []where
	for i := 0; i < count; i++ {
		head.skipSpace()
		num, err1 := head.readNumber()
		head.skipSpace()
		off, err2 := head.readNumber()
		if err1 != nil || err2 != nil {
			return nil, fmt.Errorf("the table at the front runs out after %d of %d objects", i, count)
		}
		places = append(places, where{int(num), int(off)})
	}

	out := map[int]any{}
	for _, p := range places {
		at := first + p.at
		if at < 0 || at >= len(body) {
			d.note("object %d is said to be at %d inside object stream %d, which is %d bytes long",
				p.num, at, ref.Num, len(body))
			continue
		}
		lex := &lexer{data: body, pos: at}
		value, err := lex.readObject(d)
		if err != nil {
			d.note("object %d inside object stream %d could not be read: %v", p.num, ref.Num, err)
			continue
		}
		out[p.num] = value
	}
	return out, nil
}

func sortRefs(refs []Ref) {
	for i := 1; i < len(refs); i++ {
		for j := i; j > 0 && refs[j].Num < refs[j-1].Num; j-- {
			refs[j], refs[j-1] = refs[j-1], refs[j]
		}
	}
}
