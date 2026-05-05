// Package ui contains rendering primitives shared between views: text
// wrapping/truncation, line numbering, head/tail trimming, and the line
// diff used for inline edit previews. Nothing in this package knows
// about model state.
package ui

import (
	"strings"

	"github.com/pimrutgers/agen-tui/internal/theme"
)

// WrapLines hard-wraps each line in s to width display columns (assuming
// 1 column per rune). Empty input lines are preserved as empty entries.
func WrapLines(s string, width int) []string {
	if width < 4 {
		width = 4
	}
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if line == "" {
			out = append(out, "")
			continue
		}
		rs := []rune(line)
		for len(rs) > width {
			out = append(out, string(rs[:width]))
			rs = rs[width:]
		}
		out = append(out, string(rs))
	}
	return out
}

// TruncRunes truncates s to at most n display columns (assuming 1
// column per rune). Adds a single-column ellipsis when shortened.
func TruncRunes(s string, n int) string {
	if n <= 0 {
		return ""
	}
	rs := []rune(s)
	if len(rs) <= n {
		return s
	}
	return string(rs[:n-1]) + "…"
}

// PadLeft right-aligns s in a field of n bytes. Does not truncate.
func PadLeft(s string, n int) string {
	if len(s) >= n {
		return s
	}
	return strings.Repeat(" ", n-len(s)) + s
}

// Itoa avoids pulling strconv into hot rendering paths.
func Itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := false
	if i < 0 {
		neg = true
		i = -i
	}
	var buf [20]byte
	n := len(buf)
	for i > 0 {
		n--
		buf[n] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		n--
		buf[n] = '-'
	}
	return string(buf[n:])
}

// NumberedHead returns the first n lines prefixed with right-aligned
// line numbers (in muted style), wrapping each line to width. When
// lines exceeds n, a muted "… N more lines" trailer is appended.
func NumberedHead(lines []string, n, width int) []string {
	limit := n
	if limit > len(lines) {
		limit = len(lines)
	}
	var out []string
	for i := 0; i < limit; i++ {
		num := theme.Muted.Render(PadLeft(Itoa(i+1), 4))
		for j, w := range WrapLines(lines[i], width) {
			if j == 0 {
				out = append(out, " "+num+"│ "+w)
			} else {
				out = append(out, "     │ "+w)
			}
		}
	}
	if len(lines) > n {
		out = append(out, " "+theme.Muted.Render("… "+Itoa(len(lines)-n)+" more lines"))
	}
	return out
}

// HeadTailLines keeps the first `head` and last `tail` lines of s with
// a muted "… N omitted …" separator between them. Each kept line is
// wrapped to width and indented by a single space.
func HeadTailLines(s string, head, tail, width int) []string {
	lines := strings.Split(s, "\n")
	var keep []string
	if len(lines) <= head+tail {
		keep = lines
	} else {
		keep = append([]string{}, lines[:head]...)
		omitted := len(lines) - head - tail
		keep = append(keep, "")
		keep = append(keep, theme.Muted.Render("… "+Itoa(omitted)+" lines omitted …"))
		keep = append(keep, "")
		keep = append(keep, lines[len(lines)-tail:]...)
	}
	var out []string
	for _, line := range keep {
		for _, w := range WrapLines(line, width) {
			out = append(out, " "+w)
		}
	}
	return out
}
