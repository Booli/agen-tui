package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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
	files    []git.FileStatus
	cursor   int
	offset   int
	width    int
	height   int // visible rows excluding header/divider/divider/footer
	repoRoot string
	keys     flatKeyMap
}

func newFlatView() flatView { return flatView{keys: newFlatKeys()} }

// SetData updates files (called from model on refreshMsg).
func (v flatView) SetData(files []git.FileStatus, repoRoot string) flatView {
	v.files = files
	v.repoRoot = repoRoot
	if v.cursor >= len(files) {
		v.cursor = max0(len(files) - 1)
	}
	return v.clamp()
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
		switch {
		case key.Matches(k, v.keys.Up):
			if v.cursor > 0 {
				v.cursor--
				v = v.clamp()
			}
		case key.Matches(k, v.keys.Down):
			if v.cursor < len(v.files)-1 {
				v.cursor++
				v = v.clamp()
			}
		case key.Matches(k, v.keys.Open):
			if v.cursor < len(v.files) {
				f := v.files[v.cursor]
				return v, openFileDetail(f.Path, f.IsUntracked(), v.repoRoot)
			}
		case key.Matches(k, v.keys.Edit):
			if v.cursor < len(v.files) {
				f := v.files[v.cursor]
				return v, openInPane(v.repoRoot, f.Path)
			}
		}
	}
	return v, nil
}

func (v flatView) clamp() flatView {
	avail := v.height
	if avail < 1 {
		avail = 1
	}
	if v.cursor < 0 {
		v.cursor = 0
	}
	if v.cursor >= len(v.files) {
		v.cursor = max0(len(v.files) - 1)
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

	if len(v.files) == 0 {
		lines = append(lines, theme.Muted.Render("  clean"))
	}

	avail := v.height
	if avail < 1 {
		avail = 1
	}
	end := v.offset + avail
	if end > len(v.files) {
		end = len(v.files)
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
		f := v.files[i]
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
