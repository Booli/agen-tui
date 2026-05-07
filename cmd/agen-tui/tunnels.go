package main

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/pimrutgers/agen-tui/internal/theme"
	"github.com/pimrutgers/agen-tui/internal/tunnel"
)

// tunnelsView lists tunnels from the on-disk registry and lets the user
// add (a), delete (d), or toggle (space/enter) entries inline. Adding
// opens a one-line input — esc cancels, enter confirms.
//
// In remote mode (defaultHost set) the input accepts just "PORT" or
// "LOCAL:HOST:REMOTE" and uses defaultHost. In local mode the input
// requires "HOST PORT" so each forward is bound to a specific server.
type tunnelsView struct {
	entries     []tunnel.Entry
	defaultHost string
	cursor      int

	adding   bool
	input    string
	inputErr string

	width, height int
}

func newTunnelsView() tunnelsView { return tunnelsView{} }

// SetDefaultHost is called by the model so the view knows which host
// to use when the user types just a port (remote-mode shorthand).
func (v tunnelsView) SetDefaultHost(host string) tunnelsView {
	v.defaultHost = host
	return v
}

func (v tunnelsView) SetData(entries []tunnel.Entry) tunnelsView {
	v.entries = entries
	if v.cursor >= len(entries) {
		v.cursor = len(entries) - 1
	}
	if v.cursor < 0 {
		v.cursor = 0
	}
	return v
}

func (v tunnelsView) SetSize(w, h int) tunnelsView {
	v.width, v.height = w, h
	return v
}

// Adding reports whether the view is currently in port-input mode.
func (v tunnelsView) Adding() bool { return v.adding }

func (v tunnelsView) Update(msg tea.Msg) (tunnelsView, tea.Cmd) {
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		return v, nil
	}
	if v.adding {
		return v.updateAdding(k)
	}
	switch k.String() {
	case "a":
		v.adding = true
		v.inputErr = ""
		return v, nil
	case "j", "down":
		if v.cursor < len(v.entries)-1 {
			v.cursor++
		}
	case "k", "up":
		if v.cursor > 0 {
			v.cursor--
		}
	case "d", "x":
		if v.cursor < len(v.entries) {
			return v, removeTunnelCmd(v.entries[v.cursor].ID)
		}
	case " ", "enter":
		if v.cursor < len(v.entries) {
			return v, toggleTunnelCmd(v.entries[v.cursor].ID)
		}
	}
	return v, nil
}

func (v tunnelsView) updateAdding(k tea.KeyMsg) (tunnelsView, tea.Cmd) {
	switch k.String() {
	case "esc":
		v.adding = false
		v.input = ""
		v.inputErr = ""
		return v, nil
	case "enter":
		host, spec, err := v.parseInput()
		if err != nil {
			v.inputErr = err.Error()
			return v, nil
		}
		v.adding = false
		v.input = ""
		v.inputErr = ""
		return v, addTunnelCmd(host, spec.String())
	case "backspace":
		if len(v.input) > 0 {
			v.input = v.input[:len(v.input)-1]
		}
		return v, nil
	}
	if len(k.Runes) == 1 {
		v.input += string(k.Runes)
	}
	return v, nil
}

func (v tunnelsView) View() string {
	var lines []string

	if len(v.entries) == 0 && !v.adding {
		lines = append(lines, theme.Muted.Render("  no tunnels — press a to add"))
	}

	for i, e := range v.entries {
		s := tunnel.ProbeStatus(&e)
		cursor := "  "
		if i == v.cursor && !v.adding {
			cursor = " " + theme.Cyan.Bold(true).Render("▌")
		}
		host := theme.Muted.Render(e.Host)
		row := cursor + tunnelStatusBadge(s) + "  " + host + "  " + e.Spec
		if e.LastError != "" {
			row += "  " + theme.Untracked.Render("("+e.LastError+")")
		}
		lines = append(lines, row)
	}

	if v.adding {
		label := "host port:"
		hint := "  enter: confirm  esc: cancel  (HOST PORT  or  HOST LOCAL:RHOST:REMOTE)"
		if v.defaultHost != "" {
			label = "port:"
			hint = "  enter: confirm  esc: cancel  (PORT  or  LOCAL:RHOST:REMOTE — host=" + v.defaultHost + ")"
		}
		caret := theme.Cyan.Render("│")
		prompt := " " + theme.Cyan.Bold(true).Render(label) + " " + v.input + caret
		lines = append(lines, "")
		lines = append(lines, prompt)
		lines = append(lines, theme.Muted.Render(hint))
		if v.inputErr != "" {
			lines = append(lines, " "+theme.Untracked.Render(v.inputErr))
		}
	}

	for len(lines) < v.height {
		lines = append(lines, "")
	}
	if len(lines) > v.height {
		lines = lines[:v.height]
	}
	return strings.Join(lines, "\n")
}

// parseInput resolves the input field into a (host, spec) pair.
// Accepts "HOST PORT[:RHOST:RPORT]" or, when defaultHost is set, just
// "PORT[:RHOST:RPORT]" as shorthand.
func (v tunnelsView) parseInput() (string, tunnel.Spec, error) {
	in := strings.TrimSpace(v.input)
	if in == "" {
		return "", tunnel.Spec{}, fmt.Errorf("empty input")
	}
	if i := strings.IndexByte(in, ' '); i > 0 {
		host := strings.TrimSpace(in[:i])
		spec, err := tunnel.ParseSpec(strings.TrimSpace(in[i+1:]))
		if err != nil {
			return "", tunnel.Spec{}, err
		}
		return host, spec, nil
	}
	if v.defaultHost == "" {
		return "", tunnel.Spec{}, fmt.Errorf("local mode: type \"HOST PORT\" (e.g. myserver 5137)")
	}
	spec, err := tunnel.ParseSpec(in)
	if err != nil {
		return "", tunnel.Spec{}, err
	}
	return v.defaultHost, spec, nil
}

func tunnelStatusBadge(s tunnel.Status) string {
	switch s {
	case tunnel.StatusUp:
		return theme.Staged.Render("● up      ")
	case tunnel.StatusStarting:
		return theme.Muted.Render("○ starting")
	case tunnel.StatusPortBusy:
		return theme.Untracked.Render("● busy    ")
	case tunnel.StatusError:
		return theme.Untracked.Render("● error   ")
	case tunnel.StatusDead:
		return theme.Untracked.Render("● dead    ")
	default:
		return theme.Muted.Render("○ stopped ")
	}
}
