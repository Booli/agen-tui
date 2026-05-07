package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/pimrutgers/agen-tui/internal/fuzzy"
	"github.com/pimrutgers/agen-tui/internal/git"
	"github.com/pimrutgers/agen-tui/internal/theme"
)

type flatKeyMap struct {
	Up   key.Binding
	Down key.Binding
	Open key.Binding // open diff overlay
	Edit key.Binding // open file in main tmux pane
}

func newFlatKeys() flatKeyMap {
	return flatKeyMap{
		Up:   key.NewBinding(key.WithKeys("k", "up")),
		Down: key.NewBinding(key.WithKeys("j", "down")),
		Open: key.NewBinding(key.WithKeys("enter", " ")),
		Edit: key.NewBinding(key.WithKeys("o")),
	}
}

// flatView renders the porcelain-style git status list with a cursor.
// Pressing Open returns commands that ask the parent to open a file
// detail overlay.
type flatView struct {
	files    []git.FileStatus // raw set from refresh
	filtered []git.FileStatus // post-fuzzy list shown in the body
	cursor   int
	offset   int
	width    int
	height   int // visible rows excluding header/divider/divider/footer
	repoRoot string
	keys     flatKeyMap

	// fuzzy search state
	searching bool
	query     string
	matchIdx  map[string][]int // path → matched rune positions
}

func newFlatView() flatView { return flatView{keys: newFlatKeys()} }

// SetData updates files (called from model on refreshMsg).
func (v flatView) SetData(files []git.FileStatus, repoRoot string) flatView {
	v.files = files
	v.repoRoot = repoRoot
	v = v.rebuildFilter()
	if v.cursor >= len(v.filtered) {
		v.cursor = max0(len(v.filtered) - 1)
	}
	return v.clamp()
}

// Searching reports whether the / input is active. App.go uses this
// to keep digit/letter keys out of the global hotkey switch.
func (v flatView) Searching() bool   { return v.searching }
func (v flatView) SearchQuery() string { return v.query }

// rebuildFilter applies the current fuzzy query to v.files.
func (v flatView) rebuildFilter() flatView {
	if v.query == "" {
		v.filtered = v.files
		v.matchIdx = nil
		return v
	}
	ranked := fuzzy.Filter(v.query, v.files, func(f git.FileStatus) string { return f.Path })
	out := make([]git.FileStatus, len(ranked))
	idx := make(map[string][]int, len(ranked))
	for i, r := range ranked {
		out[i] = r.Item
		if len(r.Indexes) > 0 {
			idx[r.Item.Path] = r.Indexes
		}
	}
	v.filtered = out
	v.matchIdx = idx
	return v
}

// SetSize updates layout dimensions.
func (v flatView) SetSize(width, height int) flatView {
	v.width = width
	v.height = height
	return v.clamp()
}

// Update handles key input. Returns (view, cmd). The cmd may include
// an openFileDetailMsg + diff fetch when the user opens a row.
func (v flatView) Update(msg tea.Msg) (flatView, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		if v.searching {
			return v.updateSearching(k), nil
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
		case key.Matches(k, v.keys.Open):
			if v.cursor < len(v.filtered) {
				f := v.filtered[v.cursor]
				return v, openFileDetail(f.Path, f.IsUntracked(), v.repoRoot)
			}
		case key.Matches(k, v.keys.Edit):
			if v.cursor < len(v.filtered) {
				f := v.filtered[v.cursor]
				return v, openInPane(v.repoRoot, f.Path)
			}
		}
	}
	return v, nil
}

// updateSearching captures keys while the / input is active.
func (v flatView) updateSearching(k tea.KeyMsg) flatView {
	switch k.String() {
	case "esc":
		v.searching = false
		v.query = ""
		v = v.rebuildFilter()
		return v.clamp()
	case "enter":
		v.searching = false
		return v
	case "backspace":
		if r := []rune(v.query); len(r) > 0 {
			v.query = string(r[:len(r)-1])
			v = v.rebuildFilter()
			return v.clamp()
		}
		return v
	}
	if len(k.Runes) == 1 {
		v.query += string(k.Runes)
		v = v.rebuildFilter()
		// Reset cursor to top on each keystroke so the best match is selected.
		v.cursor = 0
		v.offset = 0
		return v.clamp()
	}
	return v
}

func (v flatView) clamp() flatView {
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
	return v
}

// View returns the body lines (without header/footer).
func (v flatView) View() string {
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
		hint := "  clean"
		if v.query != "" {
			hint = "  no files match " + theme.Cyan.Render("/"+v.query)
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

	// Reserve a fixed slot on the right for the line-change tag so all
	// rows align. The widest tag we render is "+NNNN/-NNNN" — pick a
	// pragmatic 10-col slot.
	const tagSlot = 10
	const prefixCols = 5 // cursor + space + symbol + two spaces

	pathWidth := v.width - prefixCols - tagSlot - 1
	if pathWidth < 10 {
		pathWidth = 10
	}

	for i := v.offset; i < end; i++ {
		f := v.filtered[i]
		var style lipgloss.Style
		switch f.Symbol() {
		case "A":
			style = theme.Staged
		case "D":
			style = theme.Deleted
		case "?":
			style = theme.Untracked
		case "!":
			style = theme.Conflict
		default:
			if f.IsStaged() {
				style = theme.Staged
			} else {
				style = theme.Modified
			}
		}
		cursor := " "
		if i == v.cursor {
			cursor = theme.Cyan.Bold(true).Render("▌")
		}
		short := git.ShortPath(f.Path, pathWidth)

		// Build the right-side tag (visible-width known).
		var tag, tagStyled string
		if f.Added > 0 || f.Deleted > 0 {
			tag = fmt.Sprintf("+%d/-%d", f.Added, f.Deleted)
			tagStyled = theme.DiffAdd.Render(fmt.Sprintf("+%d", f.Added)) +
				theme.Muted.Render("/") +
				theme.DiffDel.Render(fmt.Sprintf("-%d", f.Deleted))
		}

		// Compose: cursor + space + sym + "  " + short + padding + tag
		usedCols := prefixCols + len(short)
		pad := v.width - usedCols - len(tag)
		if pad < 1 {
			pad = 1
		}
		entry := cursor + " " + style.Render(f.Symbol()+"  "+short) +
			strings.Repeat(" ", pad) + tagStyled
		lines = append(lines, entry)
	}

	return strings.Join(lines, "\n")
}

// Summary returns the bottom counter row (~M ~A ~D ~?).
func (v flatView) Summary() string {
	var nMod, nAdded, nDel, nUntracked int
	for _, f := range v.files {
		switch f.Symbol() {
		case "A":
			nAdded++
		case "D":
			nDel++
		case "?":
			nUntracked++
		default:
			nMod++
		}
	}
	sum := " "
	if nMod > 0 {
		sum += theme.Modified.Render(fmt.Sprintf("~%d", nMod)) + " "
	}
	if nAdded > 0 {
		sum += theme.Added.Render(fmt.Sprintf("+%d", nAdded)) + " "
	}
	if nDel > 0 {
		sum += theme.Deleted.Render(fmt.Sprintf("-%d", nDel)) + " "
	}
	if nUntracked > 0 {
		sum += theme.Untracked.Render(fmt.Sprintf("?%d", nUntracked)) + " "
	}
	if sum == " " {
		sum += theme.Muted.Render("clean")
	}
	return sum
}

func max0(n int) int {
	if n < 0 {
		return 0
	}
	return n
}
