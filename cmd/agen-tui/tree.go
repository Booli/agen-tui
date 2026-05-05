package main

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/pimrutgers/agen-tui/internal/filetree"
	"github.com/pimrutgers/agen-tui/internal/theme"
)

type treeKeyMap struct {
	Up     key.Binding
	Down   key.Binding
	Toggle key.Binding // expand/collapse dir, or open file diff
	Edit   key.Binding // open file in main tmux pane
}

func newTreeKeys() treeKeyMap {
	return treeKeyMap{
		Up:     key.NewBinding(key.WithKeys("k", "up")),
		Down:   key.NewBinding(key.WithKeys("j", "down")),
		Toggle: key.NewBinding(key.WithKeys("enter", " ")),
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
}

func newTreeView() treeView {
	return treeView{
		expanded: map[string]bool{},
		keys:     newTreeKeys(),
	}
}

// SetData refreshes the tree from a new filetree root. On the first
// load (when expanded is empty), top-level dirs and ancestors of any
// touched file are auto-expanded.
func (v treeView) SetData(root *filetree.Node, repoRoot string) treeView {
	firstLoad := root != nil && len(v.expanded) == 0
	v.root = root
	v.repoRoot = repoRoot
	if firstLoad {
		for _, c := range root.Children {
			if c.IsDir {
				v.expanded[c.Path] = true
			}
		}
		expandTouchedAncestors(root, v.expanded)
	}
	v.visible = buildVisible(v.root, v.expanded)
	if v.cursor >= len(v.visible) {
		v.cursor = max0(len(v.visible) - 1)
	}
	return v.clamp()
}

func (v treeView) SetSize(width, height int) treeView {
	v.width = width
	v.height = height
	return v.clamp()
}

func (v treeView) Update(msg tea.Msg) (treeView, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch {
		case key.Matches(k, v.keys.Up):
			if v.cursor > 0 {
				v.cursor--
				v = v.clamp()
			}
		case key.Matches(k, v.keys.Down):
			if v.cursor < len(v.visible)-1 {
				v.cursor++
				v = v.clamp()
			}
		case key.Matches(k, v.keys.Toggle):
			if v.cursor < len(v.visible) {
				n := v.visible[v.cursor].node
				if n.IsDir {
					v.expanded[n.Path] = !v.expanded[n.Path]
					v.visible = buildVisible(v.root, v.expanded)
					v = v.clamp()
				} else {
					return v, openFileDetail(n.Path, n.XY == "??", v.repoRoot)
				}
			}
		case key.Matches(k, v.keys.Edit):
			if v.cursor < len(v.visible) {
				n := v.visible[v.cursor].node
				if !n.IsDir {
					return v, openInPane(v.repoRoot, n.Path)
				}
			}
		}
	}
	return v, nil
}

func (v treeView) clamp() treeView {
	avail := v.height
	if avail < 1 {
		avail = 1
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

	avail := v.height
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

func (v treeView) renderLine(item visItem, selected bool) string {
	n := item.node

	cursor := " "
	if selected {
		cursor = theme.Cyan.Render("❯")
	}

	icon := " "
	if n.IsDir {
		if item.expanded {
			icon = "▼"
		} else {
			icon = "▶"
		}
	}

	indent := strings.Repeat("  ", item.depth)
	sym := n.Symbol()

	prefixLen := 2 + len(indent) + 2
	suffixLen := 0
	if sym != "" {
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

	var style lipgloss.Style
	switch sym {
	case "?":
		style = theme.Untracked
	case "M":
		style = theme.Modified
	case "A":
		style = theme.Staged
	case "D":
		style = theme.Deleted
	case "!":
		style = theme.Conflict
	case "R":
		style = theme.Renamed
	default:
		style = lipgloss.NewStyle()
	}

	line := cursor + " " + indent + icon + " " + style.Render(name)
	if sym != "" {
		line += " " + style.Render(sym)
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
