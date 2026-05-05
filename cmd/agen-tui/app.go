package main

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/pimrutgers/agen-tui/internal/session"
	"github.com/pimrutgers/agen-tui/internal/theme"
)

type viewMode int

const (
	modeFlat viewMode = iota
	modeTree
	modeTools
)

// statsLines is the fixed height of the bottom Claude session strip.
const statsLines = 4

// chromeRows is how many rows a view spends on header + divider +
// divider + footer. flatView uses one extra row for its summary.
func (m model) chromeRows() int {
	if m.mode == modeFlat && m.fileDetail == nil {
		return 5
	}
	return 4
}

type model struct {
	dir string

	// snapshot data
	repoRoot string
	branch   string
	err      error

	// session strip
	sessionStats session.Stats
	allTime      session.Stats
	sessionErr   error

	// sub-views
	flat  flatView
	tree  treeView
	tools toolsView

	// file-detail overlay (shared by flat & tree)
	fileDetail *fileDetailView

	mode      viewMode
	width     int
	height    int
	gitHeight int
}

func initialModel(dir string) model {
	return model{
		dir:   dir,
		flat:  newFlatView(),
		tree:  newTreeView(),
		tools: newToolsView(),
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(
		doRefresh(m.dir),
		gitTick(),
		doSessionRefresh(m.dir),
		sessionTick(),
	)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.gitHeight = m.height - statsLines
		if m.gitHeight < 4 {
			m.gitHeight = 4
		}
		m = m.relayout()

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "r":
			return m, doRefresh(m.dir)
		case "t", "tab", "g":
			if m.fileDetail == nil && !m.tools.HasOverlay() {
				m.mode = (m.mode + 1) % 3
				m = m.relayout()
			}
			return m, nil
		}
		return m.routeKey(msg)

	case refreshMsg:
		m.repoRoot = msg.repoRoot
		m.branch = msg.branch
		m.err = msg.err
		m.flat = m.flat.SetData(msg.files, msg.repoRoot)
		m.tree = m.tree.SetData(msg.treeRoot, msg.repoRoot)

	case sessionMsg:
		m.sessionStats = msg.current
		m.allTime = msg.allTime
		m.sessionErr = msg.err
		m.tools = m.tools.SetData(msg.tools)

	case fileDiffMsg:
		if m.fileDetail != nil {
			d := m.fileDetail.SetDiff(msg.path, msg.diff, msg.err)
			m.fileDetail = &d
		}

	case fileContentMsg:
		if m.fileDetail != nil {
			d := m.fileDetail.SetContent(msg.path, msg.content, msg.err)
			m.fileDetail = &d
		}

	case openFileDetailMsg:
		bodyH := m.bodyHeight()
		d := newFileDetailView(msg.path, msg.untracked, m.width, bodyH)
		m.fileDetail = &d

	case openFileViewMsg:
		bodyH := m.bodyHeight()
		d := newFileViewDetail(msg.path, m.width, bodyH)
		m.fileDetail = &d

	case closeOverlayMsg:
		m.fileDetail = nil
		m.tools = m.tools.CloseOverlay()

	case gitTickMsg:
		return m, tea.Batch(doRefresh(m.dir), gitTick())

	case sessionTickMsg:
		return m, tea.Batch(doSessionRefresh(m.dir), sessionTick())
	}
	return m, nil
}

func (m model) bodyHeight() int {
	body := m.gitHeight - m.chromeRows()
	if body < 1 {
		body = 1
	}
	return body
}

func (m model) relayout() model {
	bodyH := m.bodyHeight()
	m.flat = m.flat.SetSize(m.width, bodyH)
	m.tree = m.tree.SetSize(m.width, bodyH)
	m.tools = m.tools.SetSize(m.width, bodyH)
	if m.fileDetail != nil {
		d := m.fileDetail.SetSize(m.width, bodyH)
		m.fileDetail = &d
	}
	return m
}

func (m model) routeKey(msg tea.Msg) (model, tea.Cmd) {
	if m.fileDetail != nil {
		d, cmd, _ := m.fileDetail.Update(msg)
		m.fileDetail = &d
		return m, cmd
	}
	var cmd tea.Cmd
	switch m.mode {
	case modeFlat:
		m.flat, cmd = m.flat.Update(msg)
	case modeTree:
		m.tree, cmd = m.tree.Update(msg)
	case modeTools:
		m.tools, cmd = m.tools.Update(msg)
	}
	return m, cmd
}

// ── view ─────────────────────────────────────────────────────────────────────

func (m model) View() string {
	git := lipgloss.NewStyle().Height(m.gitHeight).Render(m.viewGit())
	return git + m.viewSession()
}

func (m model) viewGit() string {
	if m.err != nil {
		return theme.Muted.Render("  not a git repo") + "\n"
	}

	var lines []string
	lines = append(lines, headerLine(m.repoRoot, m.branch))
	lines = append(lines, divider(m.width))

	if m.fileDetail != nil {
		overlayLabel := "diff"
		if m.fileDetail.isView {
			overlayLabel = "view"
		}
		lines = append(lines, m.fileDetail.View())
		lines = append(lines, divider(m.width))
		lines = append(lines, m.overlayFooter(overlayLabel))
		return strings.Join(lines, "\n") + "\n"
	}

	switch m.mode {
	case modeFlat:
		lines = append(lines, m.flat.View())
		lines = append(lines, divider(m.width))
		lines = append(lines, m.flat.Summary())
		lines = append(lines, m.modeFooter())
	case modeTree:
		lines = append(lines, m.tree.View())
		lines = append(lines, divider(m.width))
		lines = append(lines, m.modeFooter())
	case modeTools:
		lines = append(lines, m.tools.View())
		lines = append(lines, divider(m.width))
		if m.tools.HasOverlay() {
			lines = append(lines, m.overlayFooter("detail"))
		} else {
			lines = append(lines, m.modeFooter())
		}
	}
	return strings.Join(lines, "\n") + "\n"
}

func (m model) modeFooter() string {
	switch m.mode {
	case modeFlat:
		return " " + theme.Cyan.Render("flat") + "  " +
			theme.Muted.Render("g/t:tree  ⏎:diff  o:edit  q:quit")
	case modeTree:
		return " " + theme.Cyan.Render("tree") + "  " +
			theme.Muted.Render("g/t:tools  ⏎:view  o:edit  q:quit")
	case modeTools:
		return " " + theme.Cyan.Render("tools") + " " +
			theme.Muted.Render("["+m.tools.FilterLabel()+"]  ") +
			theme.Muted.Render("g/t:flat f:filter ⏎:detail q:quit")
	}
	return ""
}

func (m model) overlayFooter(label string) string {
	return " " + theme.Cyan.Render(label) + "  " +
		theme.Muted.Render("esc/⏎:back  j/k:scroll  q:quit")
}
