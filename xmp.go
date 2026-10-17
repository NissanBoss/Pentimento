package main

// Reading the few XMP fields that say what a phone put inside a photo.
//
// XMP is RDF written as XML, and the same field turns up as an attribute
// (GImage:Mime="image/jpeg") in one phone's files and as an element
// (<GImage:Mime>image/jpeg</GImage:Mime>) in another's. A real XML parser
// would need the namespaces declared right, and phones do not always; so
// this looks for the prefixed name in both shapes, which is what every
// file seen so far needs.

import (
	"encoding/base64"
	"strconv"
	"strings"
)

func xmpField(x, name string) (string, bool) {
	if i := strings.Index(x, name+`="`); i >= 0 {
		rest := x[i+len(name)+2:]
		if j := strings.IndexByte(rest, '"'); j >= 0 {
			return xmlUnescape(rest[:j]), true
		}
	}
	if i := strings.Index(x, "<"+name+">"); i >= 0 {
		rest := x[i+len(name)+2:]
		if j := strings.Index(rest, "</"+name+">"); j >= 0 {
			return xmlUnescape(strings.TrimSpace(rest[:j])), true
		}
	}
	return "", false
}

func xmlUnescape(s string) string {
	if !strings.Contains(s, "&") {
		return s
	}
	return strings.NewReplacer("&lt;", "<", "&gt;", ">", "&quot;", `"`, "&apos;", "'", "&#xA;", "\n", "&#10;", "\n", "&amp;", "&").Replace(s)
}

// xmpBinary is a field that holds a whole file in base64, the way Google
// stored the original of a Lens Blur photo in GImage:Data.
func xmpBinary(x, name string) []byte {
	v, ok := xmpField(x, name)
	if !ok || len(v) < 16 {
		return nil
	}
	v = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == ' ' || r == '\t' {
			return -1
		}
		return r
	}, v)
	b, err := base64.StdEncoding.DecodeString(v)
	if err != nil {
		b, err = base64.RawStdEncoding.DecodeString(strings.TrimRight(v, "="))
		if err != nil {
			return nil
		}
	}
	return b
}

// containerItem is one entry of a Google container directory: the file
// that follows the primary picture, in order, after its end.
type containerItem struct {
	mime, semantic, uri string
	length, padding     int
}

func (i containerItem) name() string {
	switch {
	case i.semantic != "":
		return i.semantic
	case i.uri != "":
		return i.uri
	}
	return i.mime
}

func containerItems(x string) []containerItem {
	start := strings.Index(x, "Container:Directory")
	if start < 0 {
		return nil
	}
	dir := x[start:]
	if end := strings.Index(dir[1:], "Container:Directory"); end >= 0 {
		dir = dir[:end+1]
	}
	var items []containerItem
	for _, piece := range strings.Split(dir, "<rdf:li")[1:] {
		var it containerItem
		it.mime, _ = xmpField(piece, "Item:Mime")
		it.semantic, _ = xmpField(piece, "Item:Semantic")
		it.uri, _ = xmpField(piece, "Item:DataURI")
		if v, ok := xmpField(piece, "Item:Length"); ok {
			it.length, _ = strconv.Atoi(v)
		}
		if v, ok := xmpField(piece, "Item:Padding"); ok {
			it.padding, _ = strconv.Atoi(v)
		}
		if it.mime != "" {
			items = append(items, it)
		}
	}
	return items
}
