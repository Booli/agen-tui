package main

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/pimrutgers/agen-tui/internal/theme"
	"github.com/pimrutgers/agen-tui/internal/ui"
)

// fileDetailView is the overlay shown when the user opens a file from
// flat or tree view. In diff mode it shows git diff; in view mode it
// shows syntax-highlighted file content.
type fileDetailView struct {
	path      string
	untracked bool
	diff      string
	content   string
	isView    bool // true = file-content view, false = diff view
	loading   bool
	err       error
	vp        viewport.Model
	width     int
	height    int
	keys      struct {
		Close  key.Binding
		Up     key.Binding
		Down   key.Binding
		Top    key.Binding
		Bottom key.Binding
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
	v.keys.Top = key.NewBinding(key.WithKeys("g", "home"))
	v.keys.Bottom = key.NewBinding(key.WithKeys("G", "end"))
	v.vp = viewport.New(width, height)
	v.vp.SetContent(v.body())
	return v
}

func newFileViewDetail(path string, width, height int) fileDetailView {
	v := newFileDetailView(path, false, width, height)
	v.isView = true
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

// SetContent applies the result of doFileRead. Loading is cleared.
func (v fileDetailView) SetContent(path, content string, err error) fileDetailView {
	if path != v.path {
		return v
	}
	v.content = content
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
		case key.Matches(k, v.keys.Top):
			v.vp.GotoTop()
			return v, nil, true
		case key.Matches(k, v.keys.Bottom):
			v.vp.GotoBottom()
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

	if v.isView {
		header := " " + theme.Bold.Render(v.path)
		body := []string{header, divider(v.width)}
		switch {
		case v.loading:
			body = append(body, theme.Muted.Render(" loading…"))
		case v.err != nil:
			body = append(body, theme.Untracked.Render(" "+v.err.Error()))
		default:
			body = append(body, ui.NumberedLines(strings.Split(v.content, "\n"))...)
		}
		return strings.Join(body, "\n")
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
		body = append(body, ui.RenderUnifiedDiff(v.diff, v.path, colW)...)
	}
	return strings.Join(body, "\n")
}
