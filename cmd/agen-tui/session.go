package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/pimrutgers/agen-tui/internal/theme"
)

// viewSession renders the bottom Claude session strip (4 fixed rows):
// label divider, model + duration + turns, tokens + cache + cost,
// project totals.
func (m model) viewSession() string {
	div := labelDivider(m.width, "claude")

	if m.sessionErr != nil {
		return div + "\n" +
			theme.Muted.Render("  no active session") + "\n" +
			"\n" +
			"\n"
	}

	s := m.sessionStats
	a := m.allTime

	modelShort := shortModel(s.Model)
	dur := fmtDuration(s.LastTime.Sub(s.StartTime))

	line1 := " " + theme.Cyan.Render(modelShort) +
		theme.Muted.Render("  "+dur+"  "+fmt.Sprintf("%dt", s.Turns))

	inK := fmtK(s.InputTokens + s.CacheReadTokens)
	outK := fmtK(s.OutputTokens)
	line2 := fmt.Sprintf(" in:%s out:%s cache:%s%%  ~$%.2f",
		inK, outK,
		theme.Staged.Render(fmt.Sprintf("%.0f", s.CacheHitPct())),
		s.CostUSD())

	allIn := fmtK(a.InputTokens + a.CacheReadTokens)
	allOut := fmtK(a.OutputTokens)
	line3 := " " + theme.Muted.Render(fmt.Sprintf("project  in:%s out:%s  ~$%.2f",
		allIn, allOut, a.CostUSD()))

	return div + "\n" + line1 + "\n" + line2 + "\n" + line3 + "\n"
}

func shortModel(m string) string {
	return strings.TrimPrefix(m, "claude-")
}

func fmtDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
}

func fmtK(n int64) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	if n < 1_000_000 {
		return fmt.Sprintf("%.0fk", float64(n)/1000)
	}
	return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
}
