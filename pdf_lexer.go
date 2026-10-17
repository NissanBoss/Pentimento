package main

// Reading PDF syntax: numbers, names, strings, arrays, dictionaries and the
// streams hanging off them.

import (
	"bytes"
	"compress/flate"
	"errors"
	"fmt"
	"io"
	"strconv"
)

type lexer struct {
	data []byte
	pos  int
}

func (l *lexer) skipSpace() {
	for l.pos < len(l.data) {
		c := l.data[l.pos]
		if isSpace(c) {
			l.pos++
			continue
		}
		if c == '%' { // comment to end of line
			for l.pos < len(l.data) && l.data[l.pos] != '\n' && l.data[l.pos] != '\r' {
				l.pos++
			}
			continue
		}
		return
	}
}

func (l *lexer) peekWord() string {
	save := l.pos
	defer func() { l.pos = save }()
	l.skipSpace()
	start := l.pos
	for l.pos < len(l.data) && !isSpace(l.data[l.pos]) && !isDelim(l.data[l.pos]) {
		l.pos++
	}
	return string(l.data[start:l.pos])
}

func (l *lexer) readWord() string {
	l.skipSpace()
	start := l.pos
	for l.pos < len(l.data) && !isSpace(l.data[l.pos]) && !isDelim(l.data[l.pos]) {
		l.pos++
	}
	return string(l.data[start:l.pos])
}

// readObject reads one value. doc may be nil when reading a content stream,
// where there are no indirect references to resolve.
func (l *lexer) readObject(doc *Document) (any, error) {
	l.skipSpace()
	if l.pos >= len(l.data) {
		return nil, io.ErrUnexpectedEOF
	}

	switch c := l.data[l.pos]; {
	case c == '/':
		return l.readName(), nil

	case c == '(':
		return l.readLiteralString()

	case c == '<':
		if l.pos+1 < len(l.data) && l.data[l.pos+1] == '<' {
			return l.readDict(doc)
		}
		return l.readHexString()

	case c == '[':
		l.pos++
		var arr Array
		for {
			l.skipSpace()
			if l.pos >= len(l.data) {
				return nil, errors.New("array never closed")
			}
			if l.data[l.pos] == ']' {
				l.pos++
				return arr, nil
			}
			v, err := l.readObject(doc)
			if err != nil {
				return nil, err
			}
			arr = append(arr, v)
		}

	case c == ']' || c == '>' || c == '}' || c == ')':
		return nil, fmt.Errorf("unexpected %q", c)

	case isDigit(c) || c == '+' || c == '-' || c == '.':
		return l.readNumberOrRef()
	}

	switch w := l.readWord(); w {
	case "true":
		return true, nil
	case "false":
		return false, nil
	case "null":
		return nil, nil
	case "":
		l.pos++ // an unreadable byte; step over it rather than spin
		return nil, errors.New("unreadable byte")
	default:
		return keyword(w), nil
	}
}

// keyword is an operator or bare word, which only appears inside content
// streams. Object bodies never contain one.
type keyword string

func (l *lexer) readName() Name {
	l.pos++ // the slash
	var out []byte
	for l.pos < len(l.data) {
		c := l.data[l.pos]
		if isSpace(c) || isDelim(c) {
			break
		}
		if c == '#' && l.pos+2 < len(l.data) {
			if v, err := strconv.ParseUint(string(l.data[l.pos+1:l.pos+3]), 16, 8); err == nil {
				out = append(out, byte(v))
				l.pos += 3
				continue
			}
		}
		out = append(out, c)
		l.pos++
	}
	return Name(out)
}

func (l *lexer) readLiteralString() ([]byte, error) {
	l.pos++ // the paren
	var out []byte
	depth := 1
	for l.pos < len(l.data) {
		c := l.data[l.pos]
		switch c {
		case '\\':
			l.pos++
			if l.pos >= len(l.data) {
				return out, errors.New("string ended inside an escape")
			}
			switch e := l.data[l.pos]; e {
			case 'n':
				out = append(out, '\n')
			case 'r':
				out = append(out, '\r')
			case 't':
				out = append(out, '\t')
			case 'b':
				out = append(out, '\b')
			case 'f':
				out = append(out, '\f')
			case '\n':
				// a line continuation contributes nothing
			default:
				out = append(out, e)
			}
			l.pos++
		case '(':
			depth++
			out = append(out, c)
			l.pos++
		case ')':
			depth--
			l.pos++
			if depth == 0 {
				return out, nil
			}
			out = append(out, c)
		default:
			out = append(out, c)
			l.pos++
		}
	}
	return out, errors.New("string never closed")
}

func (l *lexer) readHexString() ([]byte, error) {
	l.pos++ // the angle bracket
	var digits []byte
	for l.pos < len(l.data) && l.data[l.pos] != '>' {
		c := l.data[l.pos]
		if !isSpace(c) {
			digits = append(digits, c)
		}
		l.pos++
	}
	if l.pos < len(l.data) {
		l.pos++ // closing bracket
	}
	if len(digits)%2 == 1 {
		digits = append(digits, '0') // an odd digit count pads with zero
	}
	out := make([]byte, 0, len(digits)/2)
	for i := 0; i+1 < len(digits); i += 2 {
		v, err := strconv.ParseUint(string(digits[i:i+2]), 16, 8)
		if err != nil {
			return out, nil // stop at the first thing that is not hex
		}
		out = append(out, byte(v))
	}
	return out, nil
}

// readNumberOrRef handles the one piece of PDF syntax that needs lookahead:
// "12 0 R" is a reference, while "12 0" is two numbers that happen to be
// next to each other.
func (l *lexer) readNumberOrRef() (any, error) {
	save := l.pos
	first, err := l.readNumber()
	if err != nil {
		return nil, err
	}
	if first != float64(int(first)) || first < 0 {
		return first, nil
	}

	mark := l.pos
	l.skipSpace()
	if l.pos < len(l.data) && isDigit(l.data[l.pos]) {
		second, err := l.readNumber()
		if err == nil && second == float64(int(second)) && second >= 0 {
			if l.peekWord() == "R" {
				l.readWord()
				return Ref{int(first), int(second)}, nil
			}
		}
	}
	l.pos = mark
	_ = save
	return first, nil
}

func (l *lexer) readNumber() (float64, error) {
	l.skipSpace()
	start := l.pos
	for l.pos < len(l.data) {
		c := l.data[l.pos]
		if isDigit(c) || c == '.' || c == '+' || c == '-' {
			l.pos++
			continue
		}
		break
	}
	if start == l.pos {
		return 0, errors.New("not a number")
	}
	text := string(l.data[start:l.pos])
	v, err := strconv.ParseFloat(text, 64)
	if err != nil {
		// PDFs in the wild contain things like "--5" and "4.". Take what
		// parses rather than refusing the whole file over it.
		if v2, err2 := strconv.ParseFloat(salvageNumber(text), 64); err2 == nil {
			return v2, nil
		}
		return 0, err
	}
	return v, nil
}

func salvageNumber(s string) string {
	var out []byte
	seenDot := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '-' && len(out) == 0:
			out = append(out, c)
		case c == '.' && !seenDot:
			seenDot = true
			out = append(out, c)
		case isDigit(c):
			out = append(out, c)
		}
	}
	if len(out) == 0 || string(out) == "-" {
		return "0"
	}
	return string(out)
}

func (l *lexer) readDict(doc *Document) (any, error) {
	l.pos += 2 // <<
	d := Dict{}
	for {
		l.skipSpace()
		if l.pos >= len(l.data) {
			return nil, errors.New("dictionary never closed")
		}
		if l.data[l.pos] == '>' {
			l.pos++
			if l.pos < len(l.data) && l.data[l.pos] == '>' {
				l.pos++
			}
			break
		}
		if l.data[l.pos] != '/' {
			// A malformed key. Step over it rather than abandon the file.
			l.pos++
			continue
		}
		key := l.readName()
		v, err := l.readObject(doc)
		if err != nil {
			return d, err
		}
		d[key] = v
	}

	// A dictionary followed by "stream" owns the bytes that come next.
	if l.peekWord() == "stream" {
		return l.readStream(d, doc)
	}
	return d, nil
}

func (l *lexer) readStream(d Dict, doc *Document) (any, error) {
	l.readWord() // stream
	// The keyword is followed by CRLF or LF, and nothing else.
	if l.pos < len(l.data) && l.data[l.pos] == '\r' {
		l.pos++
	}
	if l.pos < len(l.data) && l.data[l.pos] == '\n' {
		l.pos++
	}
	start := l.pos

	// Trust /Length when it is a plain number and lands somewhere sane.
	// Otherwise find the end, because a wrong Length is common in files
	// that have been edited by something careless.
	end := -1
	if n, ok := asInt(resolveMaybe(doc, d["Length"])); ok && n >= 0 && start+n <= len(l.data) {
		after := l.data[start+n:]
		trimmed := bytes.TrimLeft(after, " \r\n\t")
		if bytes.HasPrefix(trimmed, []byte("endstream")) {
			end = start + n
		}
	}
	if end < 0 {
		j := bytes.Index(l.data[start:], []byte("endstream"))
		if j < 0 {
			return nil, errors.New("stream never ended")
		}
		end = start + j
		for end > start && (l.data[end-1] == '\n' || l.data[end-1] == '\r') {
			end--
		}
	}

	raw := l.data[start:end]
	l.pos = end
	if j := bytes.Index(l.data[l.pos:], []byte("endstream")); j >= 0 {
		l.pos += j + len("endstream")
	}
	return &Stream{Dict: d, Raw: raw}, nil
}

func resolveMaybe(doc *Document, v any) any {
	if doc == nil {
		return v
	}
	return doc.Resolve(v)
}

func asInt(v any) (int, bool) {
	f, ok := v.(float64)
	if !ok {
		return 0, false
	}
	return int(f), true
}

func rawInflate(data []byte) ([]byte, error) {
	r := flate.NewReader(bytes.NewReader(data))
	defer r.Close()
	out, err := io.ReadAll(r)
	if err != nil && len(out) == 0 {
		return nil, err
	}
	return out, nil
}
