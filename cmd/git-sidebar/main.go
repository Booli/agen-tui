package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/pimrutgers/agen-tui/internal/git"
	"github.com/pimrutgers/agen-tui/internal/theme"
)

const refreshInterval = 2 * time.Second

// ── messages ──────────────────────────────────────────────────────────────────

type tickMsg time.Time
type statusMsg struct {
	root   string
	branch string
	files  []git.FileStatus
	err    error
}

// ── model ─────────────────────────────────────────────────────────────────────

type model struct {
	root   string
	branch string
	files  []git.FileStatus
	err    error
	width  int
	height int
}

func initialModel() model {
	cwd, _ := os.Getwd()
	return model{root: cwd}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(fetchStatus(m.root), tick())
}

// ── update ────────────────────────────────────────────────────────────────────

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "q" || msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tickMsg:
		return m, tea.Batch(fetchStatus(m.root), tick())
	case statusMsg:
		m.root, m.branch, m.files, m.err = msg.root, msg.branch, msg.files, msg.err
	}
	return m, nil
}

// ── view ──────────────────────────────────────────────────────────────────────

func (m model) View() string {
	if m.err != nil {
		return theme.Muted.Render("  not a git repo\n")
	}

	pathWidth := m.width - 10 // leave room for symbol + diff counts
	if pathWidth < 10 {
		pathWidth = 10
	}

	repoName := filepath.Base(m.root)
	header := theme.Bold.Render(" "+repoName) + "  " + theme.Muted.Render("("+m.branch+")")
	divider := theme.Muted.Render(repeat("─", m.width))

	lines := []string{header, divider}

	if len(m.files) == 0 {
		lines = append(lines, theme.Muted.Render("  clean"))
	}

	var nModified, nAdded, nDeleted, nUntracked int

	for _, f := range m.files {
		var fileStyle lipgloss.Style
		switch f.Symbol() {
		case "A":
			fileStyle = theme.Staged
			nAdded++
		case "D":
			fileStyle = theme.Deleted
			nDeleted++
		case "?":
			fileStyle = theme.Untracked
			nUntracked++
		case "!":
			fileStyle = theme.Conflict
			nModified++
		default:
			if f.IsStaged() {
				fileStyle = theme.Staged
			} else {
				fileStyle = theme.Modified
			}
			nModified++
		}

		short := git.ShortPath(f.Path, pathWidth)
		entry := "  " + fileStyle.Render(f.Symbol()+"  "+short)

		if f.Added > 0 || f.Deleted > 0 {
			entry += "  " + theme.DiffAdd.Render(fmt.Sprintf("+%d", f.Added)) +
				theme.Muted.Render("/") +
				theme.DiffDel.Render(fmt.Sprintf("-%d", f.Deleted))
		}
		lines = append(lines, entry)
	}

	// summary footer
	lines = append(lines, divider)
	summary := " "
	if nModified > 0 {
		summary += theme.Modified.Render(fmt.Sprintf("~%d", nModified)) + " "
	}
	if nAdded > 0 {
		summary += theme.Added.Render(fmt.Sprintf("+%d", nAdded)) + " "
	}
	if nDeleted > 0 {
		summary += theme.Deleted.Render(fmt.Sprintf("-%d", nDeleted)) + " "
	}
	if nUntracked > 0 {
		summary += theme.Untracked.Render(fmt.Sprintf("?%d", nUntracked)) + " "
	}
	if summary == " " {
		summary += theme.Muted.Render("clean")
	}
	lines = append(lines, summary)

	out := ""
	for _, l := range lines {
		out += l + "\n"
	}
	return out
}

// ── commands ──────────────────────────────────────────────────────────────────

func fetchStatus(cwd string) tea.Cmd {
	return func() tea.Msg {
		root := git.Root(cwd)
		if root == "" {
			return statusMsg{err: fmt.Errorf("not a git repo")}
		}
		branch := git.Branch(root)
		files, err := git.Status(root)
		return statusMsg{root: root, branch: branch, files: files, err: err}
	}
}

func tick() tea.Cmd {
	return tea.Tick(refreshInterval, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func repeat(s string, n int) string {
	out := ""
	for i := 0; i < n; i++ {
		out += s
	}
	return out
}

// ── main ──────────────────────────────────────────────────────────────────────

func main() {
	p := tea.NewProgram(initialModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
