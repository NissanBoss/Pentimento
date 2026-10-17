package main

// Walking a document's pages and asking each one whether anything was
// buried under something else.

import (
	"fmt"
	"sort"
)

type pageResult struct {
	Number  int
	Hidden  []hidden
	Skipped []string
	Trouble string // why this page could not be read at all, if it could not
}

// Pages returns the page objects in reading order. It follows the page tree
// when there is one, and falls back to object order when there is not,
// because a file with a damaged tree is still worth checking.
func (d *Document) Pages() []Dict {
	if pages := d.pagesFromTree(); len(pages) > 0 {
		return pages
	}
	var refs []Ref
	for ref, obj := range d.Objects {
		if dict, ok := obj.(Dict); ok && dict["Type"] == Name("Page") {
			refs = append(refs, ref)
		}
	}
	sort.Slice(refs, func(i, j int) bool { return refs[i].Num < refs[j].Num })
	var out []Dict
	for _, r := range refs {
		out = append(out, d.Objects[r].(Dict))
	}
	if len(out) > 0 {
		d.note("the page tree could not be followed, so pages are numbered by object order")
	}
	return out
}

func (d *Document) pagesFromTree() []Dict {
	var root Dict
	for _, obj := range d.Objects {
		if dict, ok := obj.(Dict); ok && dict["Type"] == Name("Catalog") {
			root = dict
			break
		}
	}
	if root == nil {
		return nil
	}
	var out []Dict
	var walk func(node any, depth int)
	walk = func(node any, depth int) {
		if depth > 32 {
			return // a page tree this deep is a loop
		}
		dict, ok := d.Resolve(node).(Dict)
		if !ok {
			return
		}
		switch dict["Type"] {
		case Name("Page"):
			out = append(out, dict)
		default:
			kids, _ := d.Resolve(dict["Kids"]).(Array)
			for _, kid := range kids {
				walk(kid, depth+1)
			}
		}
	}
	walk(root["Pages"], 0)
	return out
}

// content gathers a page's drawing instructions. A page may hold several
// streams, and the spec says to treat them as one, joined in order.
func (d *Document) content(pg Dict) ([]byte, error) {
	var streams []*Stream
	switch c := d.Resolve(pg["Contents"]).(type) {
	case *Stream:
		streams = append(streams, c)
	case Array:
		for _, item := range c {
			if s, ok := d.Resolve(item).(*Stream); ok {
				streams = append(streams, s)
			}
		}
	default:
		return nil, fmt.Errorf("this page has no content stream this reader understands")
	}

	var joined []byte
	for _, s := range streams {
		data, err := d.Decoded(s)
		if err != nil {
			return joined, err
		}
		joined = append(joined, data...)
		joined = append(joined, '\n')
	}
	return joined, nil
}

// Inspect reads every page and reports what is buried.
func (d *Document) Inspect() []pageResult {
	var out []pageResult
	for i, pg := range d.Pages() {
		result := pageResult{Number: i + 1}
		content, err := d.content(pg)
		if err != nil && len(content) == 0 {
			result.Trouble = err.Error()
			out = append(out, result)
			continue
		}
		if err != nil {
			result.Skipped = append(result.Skipped, err.Error())
		}
		p := readPage(d, content, d.resources(pg))
		result.Hidden = p.findHidden()
		result.Skipped = append(result.Skipped, p.Skipped...)
		out = append(out, result)
	}
	return out
}

// resources finds the resource dictionary a page draws from. It can sit on
// the page itself or on any ancestor in the page tree, and a reader that
// only looked at the page would fail to resolve half the forms in a
// document produced by a word processor.
func (d *Document) resources(pg Dict) Dict {
	node := pg
	for range 32 {
		if res, ok := d.Resolve(node["Resources"]).(Dict); ok {
			return res
		}
		parent, ok := d.Resolve(node["Parent"]).(Dict)
		if !ok {
			break
		}
		node = parent
	}
	return nil
}
