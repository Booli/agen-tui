package main

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/pimrutgers/agen-tui/internal/fuzzy"
	"github.com/pimrutgers/agen-tui/internal/session"
	"github.com/pimrutgers/agen-tui/internal/theme"
	"github.com/pimrutgers/agen-tui/internal/ui"
)

type toolsKeyMap struct {
	Up     key.Binding
	Down   key.Binding
	Open   key.Binding
	Filter key.Binding
	Close  key.Binding
}

func newToolsKeys() toolsKeyMap {
	return toolsKeyMap{
		Up:     key.NewBinding(key.WithKeys("k", "up")),
		Down:   key.NewBinding(key.WithKeys("j", "down")),
		Open:   key.NewBinding(key.WithKeys("enter", " ")),
		Filter: key.NewBinding(key.WithKeys("f")),
		Close:  key.NewBinding(key.WithKeys("esc")),
	}
}

// toolsView holds the tool-call timeline plus an optional detail
// overlay. The list is always present; when detail != nil it's an
// inline scrollable pane (powered by bubbles/viewport).
type toolsView struct {
	tools    []session.ToolCall // chronological
	filtered []session.ToolCall // newest-first, post-filter
	filter   session.FilterMode
	cursor   int
	offset   int
	anchor   string // tool_use id under cursor — preserved across refreshes
	detail   *toolDetailView

	// fuzzy search state. Active when searching == true; the input
	// captures keys, and rebuildFilter applies fuzzy.Filter on top of
	// the existing FilterMode (category) filter. matchIdx maps a row
	// index in `filtered` → match positions for highlighting.
	searching bool
	query     string
	matchIdx  map[string][]int // tool_use ID → matched rune positions in summary

	width  int
	height int
	keys   toolsKeyMap
}

func newToolsView() toolsView {
	return toolsView{keys: newToolsKeys()}
}

// SetData replaces the full tool list (chronological) and rebuilds
// the filtered view. Re-anchors the cursor to the previously-selected
// tool ID where possible.
func (v toolsView) SetData(fresh []session.ToolCall) toolsView {
	v.tools = fresh
	return v.rebuildFilter()
}

func (v toolsView) SetSize(width, height int) toolsView {
	v.width = width
	v.height = height
	if v.detail != nil {
		v.detail.SetSize(width, height)
	}
	return v.clamp()
}

func (v toolsView) Update(msg tea.Msg) (toolsView, tea.Cmd) {
	// Detail overlay swallows everything when active.
	if v.detail != nil {
		var cmd tea.Cmd
		var d *toolDetailView
		d, cmd = v.detail.Update(msg)
		v.detail = d
		return v, cmd
	}

	if k, ok := msg.(tea.KeyMsg); ok {
		// Search-input mode swallows printable keys; only esc/enter/backspace
		// have special meaning.
		if v.searching {
			return v.updateSearching(k)
		}
		switch k.String() {
		case "/":
			v.searching = true
			return v, nil
		}
		switch {
		case key.Matches(k, v.keys.Up):
			if v.cursor > 0 {
				v.cursor--
				v = v.clamp()
			}
		case key.Matches(k, v.keys.Down):
			if v.cursor < len(v.filtered)-1 {
				v.cursor++
				v = v.clamp()
			}
		case key.Matches(k, v.keys.Filter):
			v.filter = (v.filter + 1) % session.FilterCount
			v = v.rebuildFilter()
		case key.Matches(k, v.keys.Open):
			if v.cursor < len(v.filtered) {
				d := newToolDetailView(v.filtered[v.cursor], v.width, v.height)
				v.detail = &d
			}
		}
	}
	return v, nil
}

// updateSearching captures keys while the / input field is active.
// enter commits the query (exits search mode but keeps the filter), esc
// clears query and exits, backspace pops a rune. Any other printable
// key extends the query and re-runs the fuzzy filter.
func (v toolsView) updateSearching(k tea.KeyMsg) (toolsView, tea.Cmd) {
	switch k.String() {
	case "esc":
		v.searching = false
		v.query = ""
		return v.rebuildFilter(), nil
	case "enter":
		v.searching = false
		return v, nil
	case "backspace":
		if r := []rune(v.query); len(r) > 0 {
			v.query = string(r[:len(r)-1])
			return v.rebuildFilter(), nil
		}
		return v, nil
	}
	if len(k.Runes) == 1 {
		v.query += string(k.Runes)
		return v.rebuildFilter(), nil
	}
	return v, nil
}

// Searching reports whether the / input field is currently active.
// The parent uses this to suppress global hotkeys while typing.
func (v toolsView) Searching() bool { return v.searching }

// SearchQuery returns the current fuzzy query (for footer display).
func (v toolsView) SearchQuery() string { return v.query }

func (v toolsView) rebuildFilter() toolsView {
	out := make([]session.ToolCall, 0, len(v.tools))
	for i := len(v.tools) - 1; i >= 0; i-- {
		t := v.tools[i]
		if v.filter.Keep(t) {
			out = append(out, t)
		}
	}

	// Layer the fuzzy query on top of the category filter. Empty query
	// is a no-op and leaves order intact (chronological reverse).
	v.matchIdx = nil
	if v.query != "" {
		ranked := fuzzy.Filter(v.query, out, func(t session.ToolCall) string {
			// Search across name + summary so "edit app.go" matches an
			// Edit tool call on app.go.
			return t.Name + " " + t.Summary
		})
		out = make([]session.ToolCall, len(ranked))
		idx := make(map[string][]int, len(ranked))
		for i, r := range ranked {
			out[i] = r.Item
			// Indexes are positions in (Name + " " + Summary); shift to
			// summary-only by subtracting len(Name)+1 when applicable.
			if r.Indexes != nil {
				offset := len([]rune(r.Item.Name)) + 1
				summaryIdx := make([]int, 0, len(r.Indexes))
				for _, p := range r.Indexes {
					if p >= offset {
						summaryIdx = append(summaryIdx, p-offset)
					}
				}
				if len(summaryIdx) > 0 {
					idx[r.Item.ID] = summaryIdx
				}
			}
		}
		v.matchIdx = idx
	}
	v.filtered = out

	if v.anchor != "" {
		for i, t := range v.filtered {
			if t.ID == v.anchor {
				v.cursor = i
				return v.clamp()
			}
		}
		v.cursor = 0
		v.anchor = ""
	}
	return v.clamp()
}

func (v toolsView) clamp() toolsView {
	avail := v.height
	if avail < 1 {
		avail = 1
	}
	if v.cursor < 0 {
		v.cursor = 0
	}
	if v.cursor >= len(v.filtered) {
		v.cursor = max0(len(v.filtered) - 1)
	}
	if v.cursor < v.offset {
		v.offset = v.cursor
	}
	if v.cursor >= v.offset+avail {
		v.offset = v.cursor - avail + 1
	}
	if v.offset < 0 {
		v.offset = 0
	}
	if len(v.filtered) > 0 && v.cursor < len(v.filtered) {
		v.anchor = v.filtered[v.cursor].ID
	}
	return v
}

func (v toolsView) View() string {
	if v.detail != nil {
		return v.detail.View()
	}

	var lines []string

	// Search prompt at the top when active or when a query is held.
	if v.searching || v.query != "" {
		caret := ""
		if v.searching {
			caret = theme.Cyan.Render("│")
		}
		prompt := " " + theme.Cyan.Bold(true).Render("/") + v.query + caret
		lines = append(lines, prompt)
	}

	if len(v.filtered) == 0 {
		hint := "  no tool calls yet"
		switch {
		case v.query != "":
			hint = "  no tool calls match " + theme.Cyan.Render("/"+v.query)
		case len(v.tools) > 0:
			hint = "  filter hides all calls (press f)"
		}
		lines = append(lines, theme.Muted.Render(hint))
	}

	avail := v.height - len(lines)
	if avail < 1 {
		avail = 1
	}
	end := v.offset + avail
	if end > len(v.filtered) {
		end = len(v.filtered)
	}
	for i := v.offset; i < end; i++ {
		lines = append(lines, v.renderRow(v.filtered[i], i == v.cursor))
	}
	return strings.Join(lines, "\n")
}

// FilterLabel returns the current filter's name (used by the footer).
func (v toolsView) FilterLabel() string { return v.filter.Name() }

// HasOverlay returns true if a detail overlay is currently active.
func (v toolsView) HasOverlay() bool { return v.detail != nil }

// CloseOverlay dismisses any active detail overlay.
func (v toolsView) CloseOverlay() toolsView {
	v.detail = nil
	return v
}

// Reset jumps the cursor back to the newest tool call and drops the
// previously-anchored selection. Called when entering the tools view
// so users always land on a known position.
func (v toolsView) Reset() toolsView {
	v.cursor = 0
	v.offset = 0
	v.anchor = ""
	return v.clamp()
}

// nameStyleFor maps a tool name to a foreground colour by category.
func nameStyleFor(name string) lipgloss.Style {
	switch name {
	case "Write", "Edit", "MultiEdit", "NotebookEdit":
		return theme.Staged // green = mutates files
	case "Read", "Grep", "Glob":
		return theme.Cyan // cyan = reads files
	case "Bash":
		return theme.Muted // dim = noise
	case "Agent", "Skill":
		return theme.Renamed // blue = sub-agent / skill
	case "WebFetch", "WebSearch":
		return theme.Modified // orange = network
	}
	return lipgloss.NewStyle()
}

func (v toolsView) renderRow(t session.ToolCall, selected bool) string {
	cursor := " "
	if selected {
		cursor = theme.Cyan.Bold(true).Render("▌")
	}

	var icon string
	var iconStyle lipgloss.Style
	switch {
	case !t.Done:
		icon, iconStyle = "·", theme.Muted
	case t.Error:
		icon, iconStyle = "✗", theme.Untracked
	default:
		icon, iconStyle = "✓", theme.Staged
	}

	ts := "     "
	if !t.Time.IsZero() {
		ts = t.Time.Local().Format("15:04")
	}

	const nameW = 7
	name := strings.ToLower(t.Name)
	if len(name) > nameW {
		name = name[:nameW-1] + "…"
	} else {
		name = name + strings.Repeat(" ", nameW-len(name))
	}
	nameStyled := nameStyleFor(t.Name).Render(name)

	// optional +A/-D tag
	var tag, tagStyled string
	if t.Added > 0 || t.Deleted > 0 {
		switch {
		case t.Deleted == 0:
			tag = "+" + ui.Itoa(t.Added)
			tagStyled = theme.DiffAdd.Render(tag)
		case t.Added == 0:
			tag = "-" + ui.Itoa(t.Deleted)
			tagStyled = theme.DiffDel.Render(tag)
		default:
			tag = "+" + ui.Itoa(t.Added) + "/-" + ui.Itoa(t.Deleted)
			tagStyled = theme.DiffAdd.Render("+"+ui.Itoa(t.Added)) +
				theme.Muted.Render("/") +
				theme.DiffDel.Render("-"+ui.Itoa(t.Deleted))
		}
	}

	prefixCols := 1 + 1 + len(ts) + 1 + 1 + 1 + nameW + 1
	tagCols := 0
	if tag != "" {
		tagCols = 1 + len(tag)
	}
	sumWidth := v.width - prefixCols - tagCols
	if sumWidth < 4 {
		sumWidth = 4
	}
	summary := ui.TruncRunes(t.Summary, sumWidth)
	summaryStyle := lipgloss.NewStyle()
	if selected {
		summaryStyle = lipgloss.NewStyle().Bold(true)
	}

	line := cursor + " " +
		theme.Muted.Render(ts) + " " +
		iconStyle.Render(icon) + " " +
		nameStyled + " " +
		highlightMatches(summary, v.matchIdx[t.ID], summaryStyle)
	if tag != "" {
		line += " " + tagStyled
	}
	return line
}

// highlightMatches renders s with rune-positions in idx styled in
// bold-cyan and the rest in base. idx may include positions beyond
// the (truncated) length of s; those are silently dropped.
func highlightMatches(s string, idx []int, base lipgloss.Style) string {
	if len(idx) == 0 {
		return base.Render(s)
	}
	hit := make(map[int]bool, len(idx))
	for _, i := range idx {
		hit[i] = true
	}
	hl := theme.Cyan.Bold(true).Underline(true)
	var b strings.Builder
	for i, r := range s {
		if hit[i] {
			b.WriteString(hl.Render(string(r)))
		} else {
			b.WriteString(base.Render(string(r)))
		}
	}
	return b.String()
}

// ── tool detail overlay ─────────────────────────────────────────────────────

type toolDetailView struct {
	tool session.ToolCall
	vp   viewport.Model
	keys struct {
		Close  key.Binding
		Up     key.Binding
		Down   key.Binding
		Top    key.Binding
		Bottom key.Binding
	}
}

func newToolDetailView(t session.ToolCall, width, height int) toolDetailView {
	d := toolDetailView{tool: t}
	d.keys.Close = key.NewBinding(key.WithKeys("esc", "enter", " "))
	d.keys.Up = key.NewBinding(key.WithKeys("k", "up"))
	d.keys.Down = key.NewBinding(key.WithKeys("j", "down"))
	d.keys.Top = key.NewBinding(key.WithKeys("g", "home"))
	d.keys.Bottom = key.NewBinding(key.WithKeys("G", "end"))
	d.vp = viewport.New(width, height)
	d.vp.SetContent(d.body(width))
	return d
}

func (d *toolDetailView) SetSize(width, height int) {
	d.vp.Width = width
	d.vp.Height = height
	d.vp.SetContent(d.body(width))
}

// Update returns nil when the overlay was dismissed (caller should set
// the parent's detail pointer to nil).
func (d *toolDetailView) Update(msg tea.Msg) (*toolDetailView, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch {
		case key.Matches(k, d.keys.Close):
			return nil, nil
		case key.Matches(k, d.keys.Up):
			d.vp.LineUp(1)
			return d, nil
		case key.Matches(k, d.keys.Down):
			d.vp.LineDown(1)
			return d, nil
		case key.Matches(k, d.keys.Top):
			d.vp.GotoTop()
			return d, nil
		case key.Matches(k, d.keys.Bottom):
			d.vp.GotoBottom()
			return d, nil
		}
	}
	var cmd tea.Cmd
	d.vp, cmd = d.vp.Update(msg)
	return d, cmd
}

func (d *toolDetailView) View() string {
	return d.vp.View()
}

func (d *toolDetailView) body(width int) string {
	t := d.tool
	colW := width - 2
	if colW < 10 {
		colW = 10
	}

	var icon string
	var iconStyle lipgloss.Style
	switch {
	case !t.Done:
		icon, iconStyle = "·", theme.Muted
	case t.Error:
		icon, iconStyle = "✗", theme.Untracked
	default:
		icon, iconStyle = "✓", theme.Staged
	}
	ts := "        "
	if !t.Time.IsZero() {
		ts = t.Time.Local().Format("15:04:05")
	}
	header := " " + iconStyle.Render(icon) + " " +
		nameStyleFor(t.Name).Bold(true).Render(strings.ToLower(t.Name)) +
		"  " + theme.Muted.Render(ts)

	label := func(s string) string { return labelDivider(width, s) }
	body := []string{header}
	body = append(body, renderDetail(t, width, label)...)
	return strings.Join(body, "\n")
}
