package main

// What a finding is, and how it reaches the screen. The layout is
// Birthmark's and Outtakes', so the three read the same.

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

type Severity int

const (
	Note Severity = iota
	// Carries is something the file holds that the picture does not show,
	// without it being what somebody meant to take out: a depth map, the
	// country the phone was in, a video from before the shot.
	Carries
	// Weak is a cover that keeps part of what it covers. A mosaic keeps the
	// average of every block, and averages of known text can be matched.
	Weak
	// Kept is what was cropped, covered or blurred, still in the file. From
	// Weak up the exit code says something was found.
	Kept
)

type Finding struct {
	Tag    string
	Weight Severity
	Title  string
	Lines  []string
	Advice string
}

type Report struct {
	Title    string
	Summary  []string
	Findings []Finding
	// Gaps are what could not be read, printed first, because a clean
	// report about half the files is half a report.
	Gaps []string
}

func (r *Report) add(f Finding) {
	r.Findings = append(r.Findings, f)
}

func (r *Report) worst() Severity {
	worst := Note
	for _, f := range r.Findings {
		worst = max(worst, f.Weight)
	}
	return worst
}

const (
	indent = "              "
	wrap   = 62
)

func (r *Report) render(out io.Writer) {
	fmt.Fprintf(out, "\n  %s\n  %s\n\n", r.Title, strings.Repeat("=", min(runes(r.Title), wrap+14)))
	for _, line := range r.Summary {
		for i, l := range wrapText(line, wrap+12) {
			if i > 0 {
				l = "  " + l
			}
			fmt.Fprintf(out, "  %s\n", l)
		}
	}
	if len(r.Summary) > 0 {
		fmt.Fprintln(out)
	}
	for _, g := range r.Gaps {
		block(out, "UNREAD  ", "Something here could not be read", nil, g)
	}
	// Loudest first, keeping the order within a level, so somebody who
	// reads the first screen and stops has read the part that mattered.
	sorted := make([]Finding, len(r.Findings))
	copy(sorted, r.Findings)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Weight > sorted[j].Weight })
	said := map[string]bool{}
	for _, f := range sorted {
		advice := f.Advice
		if said[advice] {
			advice = ""
		}
		said[f.Advice] = true
		block(out, f.Tag, f.Title, f.Lines, advice)
	}
	fmt.Fprintln(out, "    Nothing was changed, nothing was taken out, and nothing was sent anywhere.")
	fmt.Fprintln(out)
}

func block(out io.Writer, tag, title string, lines []string, advice string) {
	head := wrapText(title, wrap+10)
	if len(head) == 0 {
		head = []string{""}
	}
	fmt.Fprintf(out, "    [%s] %s\n", tag, head[0])
	for _, line := range head[1:] {
		fmt.Fprintf(out, "%s%s\n", indent, line)
	}
	for _, line := range lines {
		if line = strings.TrimRight(line, " "); line == "" {
			fmt.Fprintln(out)
			continue
		}
		fmt.Fprintf(out, "%s%s\n", indent, line)
	}
	if advice != "" {
		if len(lines) > 0 {
			fmt.Fprintln(out)
		}
		for _, line := range wrapText(advice, wrap) {
			fmt.Fprintf(out, "%s%s\n", indent, line)
		}
	}
	fmt.Fprintln(out)
}

// wrapText counts letters rather than bytes, so a line with a mask or an
// accent in it wraps where it looks like it should.
func wrapText(s string, width int) []string {
	words := strings.Fields(s)
	if len(words) == 0 {
		return nil
	}
	var lines []string
	line := words[0]
	for _, w := range words[1:] {
		if runes(line)+1+runes(w) > width {
			lines = append(lines, line)
			line = w
			continue
		}
		line += " " + w
	}
	return append(lines, line)
}

func runes(s string) int { return len([]rune(s)) }

func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%s %s", commas(n), many)
}

func commas(n int) string {
	s := fmt.Sprint(n)
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	if neg {
		return "-" + s
	}
	return s
}

// atMost keeps a list to a screen.
func atMost(lines []string, n int) []string {
	if len(lines) <= n {
		return lines
	}
	out := append([]string{}, lines[:n]...)
	return append(out, fmt.Sprintf("... and %d more", len(lines)-n))
}

func size(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	case n >= 10<<20:
		return fmt.Sprintf("%.0f MB", float64(n)/(1<<20))
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.0f KB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d bytes", n)
}

func percent(part, whole float64) string {
	if whole <= 0 {
		return "0%"
	}
	p := 100 * part / whole
	if p > 0 && p < 1 {
		return "under 1%"
	}
	return fmt.Sprintf("%.0f%%", p)
}
