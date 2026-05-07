package main

import (
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/pimrutgers/agen-tui/internal/theme"
)

// headerWithTabs renders the repo+branch on the left and the three
// mode tabs right-aligned. The active tab is highlighted in cyan.
func headerWithTabs(repoRoot, branch string, mode viewMode, width int) string {
	name := filepath.Base(repoRoot)
	left := " " + theme.Bold.Render(name) + "  " + theme.Muted.Render("("+branch+")")

	labels := []string{"flat", "tree", "tools"}
	parts := make([]string, len(labels))
	for i, label := range labels {
		if viewMode(i) == mode {
			parts[i] = theme.Cyan.Bold(true).Render(label)
		} else {
			parts[i] = theme.Muted.Render(label)
		}
	}
	right := strings.Join(parts, theme.Muted.Render("  "))

	pad := width - lipgloss.Width(left) - lipgloss.Width(right)
	if pad < 2 {
		pad = 2
	}
	return left + strings.Repeat(" ", pad) + right
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
	return labelDividerRight(width, label, "")
}

// labelDividerRight is like labelDivider but embeds a right-aligned
// status fragment, e.g. `─── claude ─── tunnel: 5137 ────`. The right
// fragment may contain styled text — its visible width is computed
// with lipgloss.Width.
func labelDividerRight(width int, label, right string) string {
	w := width
	if w < 1 {
		w = 40
	}
	l := " " + label + " "
	leftDash := 3
	if right == "" {
		dashes := w - len(l)
		if dashes < 6 {
			dashes = 6
		}
		return theme.Muted.Render(strings.Repeat("─", leftDash) + l + strings.Repeat("─", dashes-leftDash))
	}
	rightVis := lipgloss.Width(right)
	// Pattern: "─── label ─── <right> ────"
	rightDash := 4
	mid := w - leftDash - len(l) - 3 - rightVis - 1 - rightDash
	if mid < 3 {
		mid = 3
	}
	return theme.Muted.Render(strings.Repeat("─", leftDash)+l+strings.Repeat("─", mid)+" ") +
		right +
		theme.Muted.Render(" "+strings.Repeat("─", rightDash))
}
