package main

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/pimrutgers/agen-tui/internal/theme"
	"path/filepath"
	"strings"
)

// titleRow renders the repo name + current branch on its own row.
// Splitting the title from the tabs prevents long repo names from
// pushing the tab strip off-screen.
func titleRow(repoRoot, branch string) string {
	name := filepath.Base(repoRoot)
	if name == "" || name == "." {
		name = "—"
	}
	if branch == "" {
		branch = "?"
	}
	return " " + theme.Cyan.Bold(true).Render(name) + "  " + theme.Muted.Render("("+branch+")")
}

// tabsDivider merges the tab strip into the body divider so tabs look
// like notches on a horizontal rule. Active tab is bracketed in ┤ ├
// and bold cyan; inactive tabs flow with the rule. Right-padded to width.
func tabsDivider(mode viewMode, width int) string {
	labels := []string{"1 flat", "2 tree", "3 tools", "4 tunnels"}
	var sb strings.Builder
	sb.WriteString(theme.Muted.Render("──"))
	for i, label := range labels {
		// Same total width for active/inactive (label + 4 visible cells)
		// so labels don't shift when the active tab changes.
		if viewMode(i) == mode {
			sb.WriteString(theme.Muted.Render("┤ "))
			sb.WriteString(theme.Cyan.Bold(true).Render(label))
			sb.WriteString(theme.Muted.Render(" ├"))
		} else {
			sb.WriteString(theme.Muted.Render("─ "))
			sb.WriteString(theme.Muted.Render(label))
			sb.WriteString(theme.Muted.Render(" ─"))
		}
	}
	rendered := sb.String()
	rest := width - lipgloss.Width(rendered)
	if rest < 0 {
		rest = 0
	}
	return rendered + theme.Muted.Render(strings.Repeat("─", rest))
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
