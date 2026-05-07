package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/pimrutgers/agen-tui/internal/theme"
	"github.com/pimrutgers/agen-tui/internal/tunnel"
)

// viewTunnelStrip renders a single always-visible row above the claude
// strip listing each tunnel's status, port, and host (host is hidden
// when every tunnel shares the same one). Empty when no tunnels.
func (m model) viewTunnelStrip() string {
	if len(m.tunnels) == 0 {
		return ""
	}
	showHost := false
	for i, e := range m.tunnels {
		if i > 0 && e.Host != m.tunnels[0].Host {
			showHost = true
			break
		}
	}
	parts := make([]string, 0, len(m.tunnels))
	for _, e := range m.tunnels {
		s := tunnel.ProbeStatus(&e)
		// ProbeStatus needs the spec parsed; rely on registry-stored
		// spec being canonical, so a fresh ParseSpec succeeds.
		_, _ = tunnel.ParseSpec(e.Spec)
		port := portFromSpec(e.Spec)
		token := port
		if showHost {
			token = port + theme.Muted.Render("@"+e.Host)
		}
		switch s {
		case tunnel.StatusUp:
			parts = append(parts, theme.Staged.Render(token+" ↑"))
		case tunnel.StatusStarting:
			parts = append(parts, theme.Muted.Render(token+" …"))
		case tunnel.StatusPortBusy:
			parts = append(parts, theme.Untracked.Render(token+" busy"))
		case tunnel.StatusError:
			parts = append(parts, theme.Untracked.Render(token+" ✗"))
		case tunnel.StatusDead:
			parts = append(parts, theme.Untracked.Render(token+" ✗"))
		default:
			parts = append(parts, theme.Muted.Render(token+" ·"))
		}
	}
	hostHint := ""
	if !showHost {
		hostHint = theme.Muted.Render("[" + m.tunnels[0].Host + "] ")
	}
	label := theme.Muted.Render(" tunnels ")
	return label + hostHint + strings.Join(parts, theme.Muted.Render("  ")) + "\n"
}

// portFromSpec extracts the local-port portion of a canonical
// "LOCAL:RHOST:REMOTE" spec for compact display.
func portFromSpec(spec string) string {
	if i := strings.IndexByte(spec, ':'); i > 0 {
		return spec[:i]
	}
	return spec
}

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

func fmtK(n int64) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	if n < 1_000_000 {
		return fmt.Sprintf("%.0fk", float64(n)/1000)
	}
	return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
}
