package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/pimrutgers/agen-tui/internal/theme"
	"github.com/pimrutgers/agen-tui/internal/tunnel"
)

// viewSession renders the bottom Claude session strip (4 fixed rows):
// label divider, model + duration + turns, tokens + cache + cost,
// project totals.
func (m model) viewSession() string {
	div := labelDividerRight(m.width, "claude", tunnelStatusRight(m.tunnels))

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
		theme.Muted.Render("  "+dur+"  "+
			fmt.Sprintf("%dp  %dt", s.Prompts, s.ToolCalls))

	inK := fmtK(s.InputTokens + s.CacheReadTokens)
	outK := fmtK(s.OutputTokens)
	line2 := fmt.Sprintf(" in:%s out:%s  cache:%s%%",
		inK, outK,
		theme.Staged.Render(fmt.Sprintf("%.0f", s.CacheHitPct())))

	allIn := fmtK(a.InputTokens + a.CacheReadTokens)
	allOut := fmtK(a.OutputTokens)
	line3 := " " + theme.Muted.Render(fmt.Sprintf(
		"project  in:%s out:%s  %dp  %dt  %d sess",
		allIn, allOut, a.Prompts, a.ToolCalls, a.Sessions))

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

// tunnelStatusRight renders a compact "5137↑ 8080…" fragment for the
// claude divider. Returns "" when there are no tunnels so the divider
// renders without a right segment.
func tunnelStatusRight(tunnels []*tunnel.Tunnel) string {
	if len(tunnels) == 0 {
		return ""
	}
	parts := make([]string, 0, len(tunnels))
	for _, t := range tunnels {
		s, _ := t.Snapshot()
		port := fmt.Sprintf("%d", t.Spec.LocalPort)
		switch s {
		case tunnel.StatusUp:
			parts = append(parts, theme.Staged.Render(port+"↑"))
		case tunnel.StatusStarting:
			parts = append(parts, theme.Muted.Render(port+"…"))
		case tunnel.StatusPortBusy:
			parts = append(parts, theme.Untracked.Render(port+"!busy"))
		case tunnel.StatusError:
			parts = append(parts, theme.Untracked.Render(port+"✗"))
		default:
			parts = append(parts, theme.Muted.Render(port+"·"))
		}
	}
	return theme.Muted.Render("tunnel ") + strings.Join(parts, " ")
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
