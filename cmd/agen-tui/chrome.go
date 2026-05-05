package main

import (
	"path/filepath"
	"strings"

	"github.com/pimrutgers/agen-tui/internal/theme"
)

// header returns the repo-name + branch line at the top of the git pane.
func headerLine(repoRoot, branch string) string {
	name := filepath.Base(repoRoot)
	return theme.Bold.Render(" "+name) + "  " + theme.Muted.Render("("+branch+")")
}

// divider returns a full-width muted horizontal rule.
func divider(width int) string {
	w := width
	if w < 1 {
		w = 40
	}
	return theme.Muted.Render(strings.Repeat("─", w))
}

// labelDivider returns a horizontal rule with a centred-ish label,
// like `─── input ─────────────────`.
func labelDivider(width int, label string) string {
	w := width
	if w < 1 {
		w = 40
	}
	l := " " + label + " "
	dashes := w - len(l)
	if dashes < 6 {
		dashes = 6
	}
	return theme.Muted.Render(strings.Repeat("─", 3) + l + strings.Repeat("─", dashes-3))
}
