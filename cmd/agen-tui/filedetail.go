package main

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/pimrutgers/agen-tui/internal/theme"
	"github.com/pimrutgers/agen-tui/internal/ui"
)

// fileDetailView is the diff overlay shown when the user opens a file
// from flat or tree view. It owns its own viewport and keeps fetching
// the diff once on construction. The model dismisses it on closeOverlayMsg.
type fileDetailView struct {
	path      string
	untracked bool
	diff      string
	loading   bool
	err       error
	vp        viewport.Model
	width     int
	height    int
	keys      struct {
		Close key.Binding
		Up    key.Binding
		Down  key.Binding
	}
}

func newFileDetailView(path string, untracked bool, width, height int) fileDetailView {
	v := fileDetailView{
		path:      path,
		untracked: untracked,
		loading:   true,
		width:     width,
		height:    height,
	}
	v.keys.Close = key.NewBinding(key.WithKeys("esc", "enter", " "))
	v.keys.Up = key.NewBinding(key.WithKeys("k", "up"))
	v.keys.Down = key.NewBinding(key.WithKeys("j", "down"))
	v.vp = viewport.New(width, height)
	v.vp.SetContent(v.body())
	return v
}

func (v fileDetailView) SetSize(width, height int) fileDetailView {
	v.width = width
	v.height = height
	v.vp.Width = width
	v.vp.Height = height
	v.vp.SetContent(v.body())
	return v
}

// SetDiff applies the result of doFileDiff. Loading is cleared.
func (v fileDetailView) SetDiff(path, diff string, err error) fileDetailView {
	if path != v.path {
		return v // user moved on
	}
	v.diff = diff
	v.err = err
	v.loading = false
	v.vp.SetContent(v.body())
	return v
}

func (v fileDetailView) Update(msg tea.Msg) (fileDetailView, tea.Cmd, bool) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch {
		case key.Matches(k, v.keys.Close):
			return v, closeOverlay(), true
		case key.Matches(k, v.keys.Up):
			v.vp.LineUp(1)
			return v, nil, true
		case key.Matches(k, v.keys.Down):
			v.vp.LineDown(1)
			return v, nil, true
		}
	}
	var cmd tea.Cmd
	v.vp, cmd = v.vp.Update(msg)
	return v, cmd, true
}

func (v fileDetailView) View() string {
	return v.vp.View()
}

func (v fileDetailView) body() string {
	colW := v.width - 2
	if colW < 10 {
		colW = 10
	}

	var statusLabel string
	if v.untracked {
		statusLabel = theme.Untracked.Render("untracked")
	} else {
		statusLabel = theme.Modified.Render("modified")
	}
	header := " " + theme.Bold.Render(v.path) + "  " +
		theme.Muted.Render("(") + statusLabel + theme.Muted.Render(")")

	body := []string{header, divider(v.width)}

	switch {
	case v.loading:
		body = append(body, theme.Muted.Render(" loading…"))
	case v.err != nil:
		body = append(body, theme.Untracked.Render(" "+v.err.Error()))
	case strings.TrimSpace(v.diff) == "":
		body = append(body, theme.Muted.Render(" (no changes)"))
	case v.untracked:
		body = append(body,
			ui.NumberedHead(strings.Split(v.diff, "\n"), 60, colW-5)...)
	default:
		body = append(body, ui.RenderUnifiedDiff(v.diff, colW)...)
	}
	return strings.Join(body, "\n")
}
