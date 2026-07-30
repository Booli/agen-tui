package main

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/pimrutgers/agen-tui/internal/config"
	"github.com/pimrutgers/agen-tui/internal/filetree"
	"github.com/pimrutgers/agen-tui/internal/fuzzy"
	"github.com/pimrutgers/agen-tui/internal/icons"
	"github.com/pimrutgers/agen-tui/internal/theme"
)

// renderIcon styles a Nerd Font icon. Ignored (gitignored) entries render
// muted; otherwise the device-icon brand color is applied when present.
func renderIcon(ic icons.Icon, muted bool) string {
	switch {
	case muted:
		return theme.Muted.Render(ic.Glyph)
	case ic.Color != "":
		return lipgloss.NewStyle().Foreground(lipgloss.Color(ic.Color)).Render(ic.Glyph)
	}
	return ic.Glyph
}

// gitMark maps a Node.Symbol() code to a small status glyph and its color,
// snacks.nvim-style (dots/arrows instead of letters), used when icons are on.
// Returns ("", _) for a clean node. The style also tints the file name.
func gitMark(sym string) (string, lipgloss.Style) {
	switch sym {
	case "?":
		return "?", theme.Untracked
	case "M":
		return "○", theme.Modified // hollow dot
	case "A":
		return "●", theme.Staged // filled dot
	case "D":
		return "✕", theme.Deleted // cross
	case "!":
		return "!", theme.Conflict
	case "R":
		return "→", theme.Renamed // arrow
	case "~":
		return "~", theme.Muted
	default:
		return "", lipgloss.NewStyle()
	}
}

type treeKeyMap struct {
	Up     key.Binding
	Down   key.Binding
	Toggle key.Binding // expand/collapse dir, or view file content
	Edit   key.Binding // open file in main tmux pane
}

func newTreeKeys() treeKeyMap {
	return treeKeyMap{
		Up:     key.NewBinding(key.WithKeys("k", "up")),
		Down:   key.NewBinding(key.WithKeys("j", "down")),
		Toggle: key.NewBinding(key.WithKeys("enter")),
		Edit:   key.NewBinding(key.WithKeys("o")),
	}
}

// visItem is a single rendered row.
type visItem struct {
	node     *filetree.Node
	depth    int
	expanded bool
}

type treeView struct {
	root     *filetree.Node
	expanded map[string]bool
	visible  []visItem
	cursor   int
	offset   int
	width    int
	height   int
	repoRoot string
	keys     treeKeyMap

	// showIcons enables Nerd Font decorations (folder/file icons, chevrons,
	// git-status glyphs). When false the tree uses plain arrows + letters.
	showIcons bool

	// collapseFolders starts every folder collapsed on first load instead of
	// auto-expanding top-level dirs and the ancestors of changed files.
	collapseFolders bool

	// fuzzy search state. When query != "" the view switches from a
	// tree to a flat ranked list of matching files (preserves the
	// repo-wide search story without trying to maintain a partial tree).
	searching bool
	query     string
	matches   []*filetree.Node
}

func newTreeView(cfg config.Config) treeView {
	return treeView{
		expanded:        map[string]bool{},
		keys:            newTreeKeys(),
		showIcons:       cfg.ShowIcons,
		collapseFolders: cfg.CollapseFolders,
	}
}

func (v treeView) Searching() bool     { return v.searching }
func (v treeView) SearchQuery() string { return v.query }

// SetData refreshes the tree from a new filetree root. On the first
// load (when expanded is empty), top-level dirs and ancestors of any
// touched file are auto-expanded — unless collapseFolders is set, in which
// case every folder starts collapsed.
func (v treeView) SetData(root *filetree.Node, repoRoot string) treeView {
	firstLoad := root != nil && len(v.expanded) == 0
	v.root = root
	v.repoRoot = repoRoot
	if firstLoad && !v.collapseFolders {
		for _, c := range root.Children {
			if c.IsDir {
				v.expanded[c.Path] = true
			}
		}
		expandTouchedAncestors(root, v.expanded)
	}
	v.visible = buildVisible(v.root, v.expanded)
	if v.query != "" {
		v.matches = fuzzyMatchTree(v.root, v.query)
	}
	if v.cursor >= v.rowCount() {
		v.cursor = max0(v.rowCount() - 1)
	}
	return v.clamp()
}

// rowCount is the number of selectable rows in the current display
// mode (tree or flat-search).
func (v treeView) rowCount() int {
	if v.query != "" {
		return len(v.matches)
	}
	return len(v.visible)
}

func (v treeView) SetSize(width, height int) treeView {
	v.width = width
	v.height = height
	return v.clamp()
}

func (v treeView) Update(msg tea.Msg) (treeView, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		if v.searching {
			return v.updateSearching(k), nil
		}
		switch k.String() {
		case "/":
			v.searching = true
			return v, nil
		case "esc":
			if v.query != "" {
				v.query = ""
				v.matches = nil
				v.cursor = 0
				v.offset = 0
				return v.clamp(), nil
			}
		}
		switch {
		case key.Matches(k, v.keys.Up):
			if v.cursor > 0 {
				v.cursor--
				v = v.clamp()
			}
		case key.Matches(k, v.keys.Down):
			if v.cursor < v.rowCount()-1 {
				v.cursor++
				v = v.clamp()
			}
		case key.Matches(k, v.keys.Toggle):
			n := v.cursorNode()
			if n == nil {
				return v, nil
			}
			if n.IsDir {
				v.expanded[n.Path] = !v.expanded[n.Path]
				v.visible = buildVisible(v.root, v.expanded)
				v = v.clamp()
			} else {
				return v, openFileView(n.Path, v.repoRoot)
			}
		case key.Matches(k, v.keys.Edit):
			n := v.cursorNode()
			if n != nil && !n.IsDir {
				return v, openInPane(v.repoRoot, n.Path)
			}
		}
	}
	return v, nil
}

// cursorNode returns the file/dir under the cursor in either display mode.
func (v treeView) cursorNode() *filetree.Node {
	if v.query != "" {
		if v.cursor < 0 || v.cursor >= len(v.matches) {
			return nil
		}
		return v.matches[v.cursor]
	}
	if v.cursor < 0 || v.cursor >= len(v.visible) {
		return nil
	}
	return v.visible[v.cursor].node
}

// updateSearching captures keys while the / input is active.
func (v treeView) updateSearching(k tea.KeyMsg) treeView {
	switch k.String() {
	case "esc":
		v.searching = false
		v.query = ""
		v.matches = nil
		v.cursor = 0
		v.offset = 0
		return v.clamp()
	case "enter":
		v.searching = false
		return v
	case "down", "up", "pgdown", "pgup":
		v.searching = false
		switch k.String() {
		case "down":
			if v.cursor < v.rowCount()-1 {
				v.cursor++
			}
		case "up":
			if v.cursor > 0 {
				v.cursor--
			}
		}
		return v.clamp()
	case "backspace":
		if r := []rune(v.query); len(r) > 0 {
			v.query = string(r[:len(r)-1])
			if v.query == "" {
				v.matches = nil
			} else {
				v.matches = fuzzyMatchTree(v.root, v.query)
			}
			v.cursor = 0
			v.offset = 0
			return v.clamp()
		}
		return v
	}
	if len(k.Runes) == 1 {
		v.query += string(k.Runes)
		v.matches = fuzzyMatchTree(v.root, v.query)
		v.cursor = 0
		v.offset = 0
		return v.clamp()
	}
	return v
}

// fuzzyMatchTree flattens every file leaf and ranks them against query.
// Directories are never search results — the user is looking for a file.
func fuzzyMatchTree(root *filetree.Node, query string) []*filetree.Node {
	if root == nil {
		return nil
	}
	var leaves []*filetree.Node
	var walk func(*filetree.Node)
	walk = func(n *filetree.Node) {
		for _, c := range n.Children {
			if c.IsDir {
				walk(c)
			} else {
				leaves = append(leaves, c)
			}
		}
	}
	walk(root)
	ranked := fuzzy.Filter(query, leaves, func(n *filetree.Node) string { return n.Path })
	out := make([]*filetree.Node, len(ranked))
	for i, r := range ranked {
		out[i] = r.Item
	}
	return out
}

func (v treeView) clamp() treeView {
	avail := v.height
	if avail < 1 {
		avail = 1
	}
	rows := v.rowCount()
	if v.cursor < 0 {
		v.cursor = 0
	}
	if v.cursor >= rows {
		v.cursor = max0(rows - 1)
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
	return v
}

func (v treeView) View() string {
	var lines []string

	if v.searching || v.query != "" {
		caret := ""
		if v.searching {
			caret = theme.Cyan.Render("│")
		}
		prompt := " " + theme.Cyan.Bold(true).Render("/") + v.query + caret
		lines = append(lines, prompt)
	}

	if v.query != "" {
		if len(v.matches) == 0 {
			lines = append(lines, theme.Muted.Render("  no files match "+theme.Cyan.Render("/"+v.query)))
		}
		avail := v.height - len(lines)
		if avail < 1 {
			avail = 1
		}
		end := v.offset + avail
		if end > len(v.matches) {
			end = len(v.matches)
		}
		for i := v.offset; i < end; i++ {
			lines = append(lines, v.renderMatchLine(v.matches[i], i == v.cursor))
		}
		return strings.Join(lines, "\n")
	}

	avail := v.height - len(lines)
	if avail < 1 {
		avail = 1
	}
	if len(v.visible) == 0 {
		lines = append(lines, theme.Muted.Render("  clean"))
	}
	end := v.offset + avail
	if end > len(v.visible) {
		end = len(v.visible)
	}
	for i, item := range v.visible[v.offset:end] {
		lines = append(lines, v.renderLine(item, v.offset+i == v.cursor))
	}
	return strings.Join(lines, "\n")
}

// renderMatchLine renders a flat-search result row: full path, no indent.
func (v treeView) renderMatchLine(n *filetree.Node, selected bool) string {
	cursor := " "
	if selected {
		cursor = theme.Cyan.Render("❯")
	}
	sym := n.Symbol()
	glyph, style := gitMark(sym)

	// prefix = cursor + space + (file icon + space, only with icons on).
	prefix := cursor + " "
	prefixCols := 2
	symbol := sym
	if v.showIcons {
		prefix += renderIcon(icons.FileIcon(n.Name), n.Ignored) + " "
		prefixCols += 2
		symbol = glyph
	} else {
		prefix += " "
		prefixCols++
	}

	suffixCols := 0
	if symbol != "" {
		suffixCols = 2
	}
	maxPath := v.width - prefixCols - suffixCols
	if maxPath < 8 {
		maxPath = 8
	}
	path := n.Path
	if len(path) > maxPath {
		path = "…" + path[len(path)-maxPath+1:]
	}
	line := prefix + style.Render(path)
	if symbol != "" {
		line += " " + style.Render(symbol)
	}
	return line
}

func (v treeView) renderLine(item visItem, selected bool) string {
	n := item.node

	cursor := " "
	if selected {
		cursor = theme.Cyan.Render("❯")
	}

	indent := strings.Repeat("  ", item.depth)
	sym := n.Symbol()
	glyph, style := gitMark(sym)

	// iconSeg is everything between the indent and the file name. With icons
	// off it's the classic arrow column; with icons on it's a chevron plus a
	// device-icon (snacks.nvim-style). iconCols is its display width.
	var iconSeg string
	var iconCols int
	symbol := sym // trailing status mark: a letter without icons, a glyph with
	if v.showIcons {
		chevron := " "
		var ic icons.Icon
		if n.IsDir {
			if item.expanded {
				chevron = "" // nf-fa-chevron_down
				ic = icons.FolderOpen
			} else {
				chevron = "" // nf-fa-chevron_right
				ic = icons.FolderClosed
			}
		} else {
			ic = icons.FileIcon(n.Name)
		}
		iconSeg = theme.Muted.Render(chevron) + " " + renderIcon(ic, n.Ignored) + " "
		iconCols = 4 // chevron + space + icon + space
		symbol = glyph
	} else {
		arrow := " "
		if n.IsDir {
			if item.expanded {
				arrow = "▼"
			} else {
				arrow = "▶"
			}
		}
		iconSeg = arrow + " "
		iconCols = 2 // arrow + space
	}

	prefixLen := 2 + len(indent) + iconCols
	suffixLen := 0
	if symbol != "" {
		suffixLen = 2
	}
	nameWidth := v.width - prefixLen - suffixLen
	if nameWidth < 4 {
		nameWidth = 4
	}

	name := n.Name
	if len(name) > nameWidth {
		name = name[:nameWidth-1] + "…"
	}

	line := cursor + " " + indent + iconSeg + style.Render(name)
	if symbol != "" {
		line += " " + style.Render(symbol)
	}
	return line
}

func buildVisible(root *filetree.Node, expanded map[string]bool) []visItem {
	var out []visItem
	if root != nil {
		flatten(root, 0, expanded, &out)
	}
	return out
}

func flatten(n *filetree.Node, depth int, expanded map[string]bool, out *[]visItem) {
	for _, child := range n.Children {
		exp := child.IsDir && expanded[child.Path]
		*out = append(*out, visItem{child, depth, exp})
		if exp {
			flatten(child, depth+1, expanded, out)
		}
	}
}

// expandTouchedAncestors walks the tree and marks every ancestor
// directory of any file with non-empty git status as expanded.
func expandTouchedAncestors(root *filetree.Node, expanded map[string]bool) {
	if root == nil {
		return
	}
	var walk func(n *filetree.Node)
	walk = func(n *filetree.Node) {
		for _, c := range n.Children {
			if !c.IsDir && c.XY != "" {
				p := c.Path
				for {
					i := strings.LastIndexByte(p, '/')
					if i < 0 {
						break
					}
					p = p[:i]
					expanded[p] = true
				}
			}
			if c.IsDir {
				walk(c)
			}
		}
	}
	walk(root)
}
