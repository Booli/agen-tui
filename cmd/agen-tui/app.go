package main

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/pimrutgers/agen-tui/internal/backend"
	"github.com/pimrutgers/agen-tui/internal/session"
	"github.com/pimrutgers/agen-tui/internal/theme"
	"github.com/pimrutgers/agen-tui/internal/tunnel"
)

// cachedSession lets the all-time refresh skip files whose size hasn't
// changed since the last scan (JSONL is append-only, so size == content key).
type cachedSession struct {
	size  int64
	stats session.Stats
}

type viewMode int

const (
	modeFlat viewMode = iota
	modeTree
	modeTools
	modeTunnels
)

// statsLines is the fixed height of the bottom Claude session strip.
const statsLines = 4

// chromeRows: title(1) + tabsDivider(1) + bottomDivider(1) + footer(1) = 4.
// flat mode adds a summary row.
func (m model) chromeRows() int {
	if m.mode == modeFlat && m.fileDetail == nil {
		return 5
	}
	return 4
}

type model struct {
	dir     string
	host    string // empty == local; non-empty enables the tunnels view
	backend backend.Backend
	// tunnels is a snapshot of the on-disk registry, refreshed on every
	// tunnelTick. The slice is *not* owned by the model — sources of truth
	// are the JSON file and the running ssh PIDs.
	tunnels []tunnel.Entry

	// snapshot data
	repoRoot string
	branch   string
	err      error

	// session strip
	sessionStats session.Stats
	allTime      session.Stats
	sessionErr   error

	// active-session tail streaming
	sessionParser    *session.Parser
	sessionStreamCh  <-chan []byte
	sessionStreamEnd func()
	sessionPath      string
	sessionRetry     time.Duration

	// all-time totals cache: path → {size, stats}. Avoids re-reading
	// project JSONLs whose size hasn't changed since the last scan.
	allTimeCache map[string]cachedSession

	// sub-views
	flat        flatView
	tree        treeView
	tools       toolsView
	tunnelsView tunnelsView

	// file-detail overlay (shared by flat & tree)
	fileDetail *fileDetailView

	mode      viewMode
	width     int
	height    int
	gitHeight int
}

func initialModel(dir, host string, b backend.Backend) model {
	return model{
		dir:         dir,
		host:        host,
		backend:     b,
		flat:        newFlatView(),
		tree:        newTreeView(),
		tools:       newToolsView(),
		tunnelsView: newTunnelsView().SetDefaultHost(host),
	}
}

// modeCount is 4: flat, tree, tools, tunnels. The tunnels view is
// always reachable — in local mode it shows what's currently forwarded
// and lets you add new ones with explicit host.
func (m model) modeCount() viewMode { return 4 }

func (m model) Init() tea.Cmd {
	// Tunnels are loaded from the on-disk registry on every tick — even in
	// local mode, since other agen-tui instances may have added some.
	return tea.Batch(
		doRefresh(m.dir, m.backend),
		gitTick(),
		startSessionStream(m.dir, m.backend),
		doAllTimeRefresh(m.dir, m.backend, m.allTimeCache),
		allTimeTick(),
		loadTunnels(),
		tunnelTick(),
	)
}

// shutdown runs the cleanup path: cancel the active tail-F stream so
// the child ssh tail isn't orphaned. Tunnels are deliberately NOT
// touched — they live in the on-disk registry and persist across
// agen-tui exits and other instances.
func (m *model) shutdown() {
	if m.sessionStreamEnd != nil {
		m.sessionStreamEnd()
		m.sessionStreamEnd = nil
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case shutdownMsg:
		m.shutdown()
		return m, tea.Quit

	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m = m.relayout()

	case tea.KeyMsg:
		// While any sub-view input is active (tunnels add, tools /search),
		// swallow all keys into the view so digits/letters don't trigger
		// global hotkeys like 1-4 or q.
		if m.mode == modeTunnels && m.tunnelsView.Adding() {
			return m.routeKey(msg)
		}
		if m.mode == modeTools && m.tools.Searching() {
			return m.routeKey(msg)
		}
		if m.mode == modeFlat && m.flat.Searching() {
			return m.routeKey(msg)
		}
		if m.mode == modeTree && m.tree.Searching() {
			return m.routeKey(msg)
		}
		switch msg.String() {
		case "q":
			if m.fileDetail != nil || m.tools.HasOverlay() {
				return m, closeOverlay()
			}
			m.shutdown()
			return m, tea.Quit
		case "ctrl+c":
			m.shutdown()
			return m, tea.Quit
		case "r":
			return m, doRefresh(m.dir, m.backend)
		case "T":
			return m, toggleAllTunnelsCmd(m.tunnels)
		case "1":
			m.mode = modeFlat
			return m.relayout(), nil
		case "2":
			m.mode = modeTree
			return m.relayout(), nil
		case "3":
			m.mode = modeTools
			m.tools = m.tools.Reset()
			return m.relayout(), nil
		case "4":
			m.mode = modeTunnels
			return m.relayout(), nil
		case "t", "tab":
			if m.fileDetail == nil && !m.tools.HasOverlay() {
				m.mode = (m.mode + 1) % m.modeCount()
				if m.mode == modeTools {
					m.tools = m.tools.Reset()
				}
				m = m.relayout()
			}
			return m, nil
		case "shift+tab":
			if m.fileDetail == nil && !m.tools.HasOverlay() {
				m.mode = (m.mode + m.modeCount() - 1) % m.modeCount()
				if m.mode == modeTools {
					m.tools = m.tools.Reset()
				}
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

	case sessionStreamMsg:
		if msg.err != nil {
			m.sessionErr = msg.err
			// Schedule a reconnect attempt — the file may not exist yet.
			return m, sessionReconnect(m.bumpRetry())
		}
		m.sessionStreamCh = msg.ch
		m.sessionStreamEnd = msg.cancel
		m.sessionPath = m.backend.ActiveSessionFile(m.dir)
		m.sessionParser = session.NewParser()
		m.sessionParser.Stats.SessionID = msg.sessionID
		m.sessionStats = m.sessionParser.Stats
		m.sessionErr = nil
		m.sessionRetry = 0 // healthy stream — reset backoff
		return m, waitForSessionChunk(msg.ch)

	case sessionChunkMsg:
		// Discard chunks from a rotated/cancelled stream — they'd corrupt
		// the freshly-started parser otherwise.
		if msg.ch != m.sessionStreamCh || m.sessionParser == nil {
			return m, nil
		}
		m.sessionParser.Append(msg.chunk)
		m.sessionStats = m.sessionParser.Stats
		m.tools = m.tools.SetData(m.sessionParser.RecentTools(500))
		return m, waitForSessionChunk(m.sessionStreamCh)

	case sessionStreamEndMsg:
		// Only clear if the message corresponds to the *current* stream;
		// a stale end-msg from a just-cancelled rotated stream must not
		// clobber the freshly-started one.
		if msg.ch != m.sessionStreamCh {
			return m, nil
		}
		m.sessionStreamCh = nil
		m.sessionStreamEnd = nil
		// Auto-reconnect with exponential backoff (capped at 30s).
		return m, sessionReconnect(m.bumpRetry())

	case sessionReconnectMsg:
		if m.sessionStreamCh != nil {
			return m, nil // already reconnected via rotation path
		}
		return m, startSessionStream(m.dir, m.backend)

	case allTimeMsg:
		m.allTime = msg.allTime
		m.allTimeCache = msg.cache
		// If the active session rotated, kill old stream and start fresh.
		if msg.activePath != "" && msg.activePath != m.sessionPath {
			if m.sessionStreamEnd != nil {
				m.sessionStreamEnd()
				m.sessionStreamEnd = nil
				m.sessionStreamCh = nil
			}
			return m, startSessionStream(m.dir, m.backend)
		}
		// If we never had a stream (initial failure), try again.
		if m.sessionStreamCh == nil && msg.activePath != "" {
			return m, startSessionStream(m.dir, m.backend)
		}

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
		return m, doFileDiff(m.repoRoot, msg.path, msg.untracked, m.backend)

	case openFileViewMsg:
		bodyH := m.bodyHeight()
		d := newFileViewDetail(msg.path, m.width, bodyH)
		m.fileDetail = &d
		return m, doFileRead(m.repoRoot, msg.path, m.backend)

	case openInPaneMsg:
		return m, runInPane(msg.repoRoot, msg.relPath, m.backend)

	case closeOverlayMsg:
		m.fileDetail = nil
		m.tools = m.tools.CloseOverlay()

	case gitTickMsg:
		return m, tea.Batch(doRefresh(m.dir, m.backend), gitTick())

	case allTimeTickMsg:
		return m, tea.Batch(doAllTimeRefresh(m.dir, m.backend, m.allTimeCache), allTimeTick())

	case tunnelTickMsg:
		return m, tea.Batch(loadTunnels(), tunnelTick())

	case tunnelsRefreshedMsg:
		prevHasAny := len(m.tunnels) > 0
		m.tunnels = msg.entries
		m.tunnelsView = m.tunnelsView.SetData(m.tunnels)
		// gitHeight depends on whether the strip is shown — recompute when
		// the count crosses 0 ↔ 1+.
		if prevHasAny != (len(m.tunnels) > 0) {
			m = m.relayout()
		}
		return m, nil

	case addTunnelMsg:
		return m, addTunnel(msg.host, msg.spec)

	case removeTunnelMsg:
		return m, removeTunnel(msg.id)

	case toggleTunnelMsg:
		return m, toggleTunnel(msg.id)
	}
	return m, nil
}

// bumpRetry returns the next reconnect delay using exponential backoff
// (1s → 2s → 4s … capped at 30s) and stores it on the model.
func (m *model) bumpRetry() time.Duration {
	switch {
	case m.sessionRetry == 0:
		m.sessionRetry = time.Second
	case m.sessionRetry < 30*time.Second:
		m.sessionRetry *= 2
		if m.sessionRetry > 30*time.Second {
			m.sessionRetry = 30 * time.Second
		}
	}
	return m.sessionRetry
}

func (m model) bodyHeight() int {
	body := m.gitHeight - m.chromeRows()
	if body < 1 {
		body = 1
	}
	return body
}

// tunnelStripRows is 1 when at least one tunnel exists (always-visible
// status row above the claude strip), else 0.
func (m model) tunnelStripRows() int {
	if len(m.tunnels) > 0 {
		return 1
	}
	return 0
}

func (m model) relayout() model {
	if m.height > 0 {
		m.gitHeight = m.height - statsLines - m.tunnelStripRows()
		if m.gitHeight < 4 {
			m.gitHeight = 4
		}
	}
	bodyH := m.bodyHeight()
	m.flat = m.flat.SetSize(m.width, bodyH)
	m.tree = m.tree.SetSize(m.width, bodyH)
	m.tools = m.tools.SetSize(m.width, bodyH)
	m.tunnelsView = m.tunnelsView.SetSize(m.width, bodyH).SetData(m.tunnels)
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
	case modeTunnels:
		m.tunnelsView, cmd = m.tunnelsView.Update(msg)
	}
	return m, cmd
}

// ── view ─────────────────────────────────────────────────────────────────────

func (m model) View() string {
	// Pad-or-crop to exactly gitHeight rows so a long view body (e.g. a
	// full tools list with multi-line rows) can't push the chrome above
	// it off-screen.
	content := strings.TrimRight(m.viewGit(), "\n")
	lines := strings.Split(content, "\n")
	if len(lines) > m.gitHeight {
		lines = lines[:m.gitHeight]
	}
	for len(lines) < m.gitHeight {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n") + "\n" + m.viewTunnelStrip() + m.viewSession()
}

func (m model) viewGit() string {
	if m.err != nil {
		return theme.Muted.Render("  not a git repo") + "\n"
	}

	var lines []string
	lines = append(lines, titleRow(m.repoRoot, m.branch))
	lines = append(lines, tabsDivider(m.mode, m.width))

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
	case modeTunnels:
		lines = append(lines, m.tunnelsView.View())
		lines = append(lines, divider(m.width))
		lines = append(lines, m.modeFooter())
	}
	return strings.Join(lines, "\n") + "\n"
}

func (m model) modeFooter() string {
	switch m.mode {
	case modeFlat:
		if m.flat.Searching() {
			return theme.Muted.Render(" enter:keep  esc:clear")
		}
		hint := " ⏎:diff  o:edit  /:search  1-4:view  q:quit"
		if m.flat.SearchQuery() != "" {
			hint = " [/" + m.flat.SearchQuery() + "] " + hint
		}
		return theme.Muted.Render(hint)
	case modeTree:
		if m.tree.Searching() {
			return theme.Muted.Render(" enter:keep  esc:clear")
		}
		hint := " ⏎:view  o:edit  /:search  1-4:view  q:quit"
		if m.tree.SearchQuery() != "" {
			hint = " [/" + m.tree.SearchQuery() + "] " + hint
		}
		return theme.Muted.Render(hint)
	case modeTools:
		if m.tools.Searching() {
			return theme.Muted.Render(" enter:keep  esc:clear")
		}
		hint := "["+m.tools.FilterLabel()+"]  f:filter  /:search  ⏎:detail  1-4:view  q:quit"
		if m.tools.SearchQuery() != "" {
			hint = "[/" + m.tools.SearchQuery() + "]  " + hint
		}
		return " " + theme.Muted.Render(hint)
	case modeTunnels:
		if m.tunnelsView.Adding() {
			return theme.Muted.Render(" enter:confirm  esc:cancel")
		}
		return theme.Muted.Render(" a:add  d:del  space:toggle  1-4:view  q:quit")
	}
	return ""
}

func (m model) overlayFooter(label string) string {
	return " " + theme.Cyan.Render(label) + "  " +
		theme.Muted.Render("esc/⏎/q:back  j/k:scroll")
}
