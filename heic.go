package main

// What an iPhone's HEIC keeps besides the photo.
//
// The pixels are HEVC, which the standard library does not decode, so a
// HEIC is read for its list of items and not compared with anything. The
// list is enough for what matters here: a Portrait photo keeps a depth
// map and mattes of the people in it, named by Apple's auxiliary types.

import (
	"fmt"
	"strings"
)

// Apple's names for the auxiliary images, from the AVFoundation and Core
// Image documentation. The gain map is not reported: it is the photo's
// own brightness, and nothing the photo does not show.
var appleAux = []struct{ urn, what string }{
	{"aux:portraiteffectsmatte", "an outline of the people in it"},
	{"semanticskinmatte", "a map of the skin of the people in it"},
	{"semantichairmatte", "a map of their hair"},
	{"semanticteethmatte", "a map of their teeth"},
	{"semanticglassesmatte", "a map of their glasses"},
	{"semanticskymatte", "a map of the sky"},
	{"aux:depth", "a depth map"},
	{"aux:disparity", "a depth map"},
	{"urn:mpeg:hevc:2015:auxid:2", "a depth map"},
	{"urn:mpeg:mpegB:cicp:systems:auxiliary:depth", "a depth map"},
}

func (c *run) heic(where string, data []byte) {
	items, ok := readHEIF(data)
	if !ok {
		c.gap(where, "the list of what is in it could not be read")
		return
	}
	c.heicRead = true
	var maps []string
	for _, it := range items {
		if it.aux == "" {
			continue
		}
		for _, a := range appleAux {
			if strings.Contains(it.aux, a.urn) {
				line := capital(a.what)
				if it.width > 0 {
					line += fmt.Sprintf(", %dx%d", it.width, it.height)
				}
				maps = append(maps, line+".")
				break
			}
		}
	}
	if len(maps) > 0 {
		c.hit("depth", where, maps[0], maps[1:]...)
	}
}
