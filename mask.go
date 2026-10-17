package main

// Masking, the way Accent does it: first and last letter of every word,
// punctuation kept, so "Q3 layoffs (draft)" comes out as "Q* l*****s
// (d***t)" and still reads as a label.

import "strings"

func maskWord(s string) string {
	r := []rune(s)
	switch {
	case len(r) == 0:
		return ""
	case len(r) <= 2:
		return string(r[0]) + strings.Repeat("*", len(r)-1)
	}
	return string(r[0]) + strings.Repeat("*", len(r)-2) + string(r[len(r)-1])
}

func maskText(s string) string {
	var b strings.Builder
	var word []rune
	flush := func() {
		if len(word) > 0 {
			b.WriteString(maskWord(string(word)))
			word = word[:0]
		}
	}
	for _, r := range s {
		if isWordRune(r) {
			word = append(word, r)
			continue
		}
		flush()
		b.WriteRune(r)
	}
	flush()
	return b.String()
}

func isWordRune(r rune) bool {
	return r == '_' || r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r > 0x7f
}
