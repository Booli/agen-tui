package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/pimrutgers/agen-tui/internal/backend"
	"github.com/pimrutgers/agen-tui/internal/filetree"
	"github.com/pimrutgers/agen-tui/internal/git"
	"github.com/pimrutgers/agen-tui/internal/session"
	"github.com/pimrutgers/agen-tui/internal/ui"
)

// ── messages ──────────────────────────────────────────────────────────────────

type refreshMsg struct {
	repoRoot string
	branch   string
	files    []git.FileStatus
	treeRoot *filetree.Node
	err      error
}

// sessionStreamMsg is emitted once a tail -F has been opened on the active
// JSONL file. The model stores cancel for cleanup and starts pulling chunks.
type sessionStreamMsg struct {
	ch        <-chan []byte
	cancel    func()
	sessionID string
	err       error
}

// sessionChunkMsg carries a chunk of bytes from the active session tail,
// tagged with its source channel so a stale chunk from a just-rotated
// stream can be discarded instead of corrupting the new parser.
type sessionChunkMsg struct {
	ch    <-chan []byte
	chunk []byte
}

// sessionStreamEndMsg signals the tail has closed (process exited or EOF).
// ch identifies which stream ended, so a stale message from a rotated stream
// doesn't clobber the new stream's state.
type sessionStreamEndMsg struct{ ch <-chan []byte }

// sessionReconnectMsg is fired after a backoff delay; the model uses it to
// retry startSessionStream when the previous attempt failed or the stream
// dropped (e.g., ssh connection blip, no JSONL file yet on a fresh repo).
type sessionReconnectMsg struct{}

// allTimeMsg carries the slow-cadence project-wide totals plus the current
// active path (so the model can detect rotation and restart the stream),
// and the refreshed file-size cache so the next scan can skip unchanged files.
type allTimeMsg struct {
	allTime    session.Stats
	activePath string
	cache      map[string]cachedSession
}

type fileDiffMsg struct {
	path      string
	untracked bool
	diff      string
	err       error
}

// openFileDetailMsg asks the parent to switch into the file-detail overlay.
// The model's Update handler dispatches the diff fetch using the backend.
type openFileDetailMsg struct {
	path      string
	untracked bool
}

// closeOverlayMsg asks the parent to dismiss any active overlay.
type closeOverlayMsg struct{}

type allTimeTickMsg time.Time
type gitTickMsg time.Time

// openFileViewMsg asks the parent to switch into a syntax-highlighted
// file-view overlay (not diff). The model's Update handler fetches content.
type openFileViewMsg struct {
	path string
}

type fileContentMsg struct {
	path    string
	content string
	err     error
}

// openInPaneMsg asks the parent to open the file in a tmux editor pane.
// The model's Update handler resolves the editor path via the backend.
type openInPaneMsg struct {
	repoRoot string
	relPath  string
}

// ── command factories ─────────────────────────────────────────────────────────

func doRefresh(dir string, b backend.Backend) tea.Cmd {
	return func() tea.Msg {
		snap, err := b.Snapshot(dir)
		if err != nil || snap.Root == "" {
			if err == nil {
				err = fmt.Errorf("not a git repo")
			}
			return refreshMsg{err: err}
		}
		statusMap := make(map[string]string, len(snap.Status))
		for _, f := range snap.Status {
			statusMap[f.Path] = string([]byte{f.X, f.Y})
		}
		treeRoot := filetree.Build(snap.AllFiles, statusMap)
		return refreshMsg{
			repoRoot: snap.Root,
			branch:   snap.Branch,
			files:    snap.Status,
			treeRoot: treeRoot,
		}
	}
}

// startSessionStream resolves the active JSONL path and opens a tail -F on it.
// The returned msg carries the chunk channel + cancel so the model can drain
// and clean up.
func startSessionStream(dir string, b backend.Backend) tea.Cmd {
	return func() tea.Msg {
		activePath := b.ActiveSessionFile(dir)
		if activePath == "" {
			return sessionStreamMsg{err: fmt.Errorf("no session file")}
		}
		ch, cancel, err := b.StreamSessionBytes(activePath)
		if err != nil {
			return sessionStreamMsg{err: err}
		}
		return sessionStreamMsg{ch: ch, cancel: cancel, sessionID: sessionIDFromPath(activePath)}
	}
}

// waitForSessionChunk blocks on ch and emits a chunk msg, or sessionStreamEndMsg
// when the channel closes. The model re-issues this cmd after each chunk.
func waitForSessionChunk(ch <-chan []byte) tea.Cmd {
	return func() tea.Msg {
		chunk, ok := <-ch
		if !ok {
			return sessionStreamEndMsg{ch: ch}
		}
		return sessionChunkMsg{ch: ch, chunk: chunk}
	}
}

// doAllTimeRefresh sums every project JSONL for the lifetime totals. The
// cache is keyed by path; entries are reused when the file's size is
// unchanged (JSONL is append-only, so size == fingerprint). Updated cache
// rides back on the message so the model can swap it in.
func doAllTimeRefresh(dir string, b backend.Backend, cache map[string]cachedSession) tea.Cmd {
	return func() tea.Msg {
		infos := b.ProjectSessionFilesInfo(dir)
		next := make(map[string]cachedSession, len(infos))
		var allTime session.Stats
		for _, fi := range infos {
			if c, ok := cache[fi.Path]; ok && c.size == fi.Size {
				next[fi.Path] = c
				allTime = allTime.Add(c.stats)
				allTime.Sessions++
				continue
			}
			d, err := b.ReadSessionBytes(fi.Path)
			if err != nil {
				continue
			}
			s, err := session.ParseBytes(d)
			if err != nil {
				continue
			}
			next[fi.Path] = cachedSession{size: fi.Size, stats: s}
			allTime = allTime.Add(s)
			allTime.Sessions++
		}
		return allTimeMsg{
			allTime:    allTime,
			activePath: b.ActiveSessionFile(dir),
			cache:      next,
		}
	}
}

func doFileDiff(root, path string, untracked bool, b backend.Backend) tea.Cmd {
	return func() tea.Msg {
		out, err := b.Diff(root, path, untracked)
		return fileDiffMsg{path: path, untracked: untracked, diff: out, err: err}
	}
}

func doFileRead(root, path string, b backend.Backend) tea.Cmd {
	return func() tea.Msg {
		abs := filepath.Join(root, path)
		data, err := b.ReadFile(abs)
		if err != nil {
			return fileContentMsg{path: path, err: err}
		}
		return fileContentMsg{path: path, content: ui.Highlight(string(data), path)}
	}
}

// openFileDetail signals the parent to open a diff overlay. The model's
// Update dispatches doFileDiff using the backend.
func openFileDetail(path string, untracked bool, root string) tea.Cmd {
	return func() tea.Msg { return openFileDetailMsg{path: path, untracked: untracked} }
}

// openFileView signals the parent to open a file-view overlay. The model's
// Update dispatches doFileRead using the backend.
func openFileView(path, root string) tea.Cmd {
	return func() tea.Msg { return openFileViewMsg{path: path} }
}

// closeOverlay dismisses any active detail overlay.
func closeOverlay() tea.Cmd {
	return func() tea.Msg { return closeOverlayMsg{} }
}

// openInPane signals the parent to open the file in an editor tmux pane.
func openInPane(repoRoot, relPath string) tea.Cmd {
	return func() tea.Msg { return openInPaneMsg{repoRoot: repoRoot, relPath: relPath} }
}

// runInPane splits the tmux pane and runs the backend's editor command.
// Local: $EDITOR on the absolute path. SSH: ssh -t host vim '<remote path>'.
func runInPane(repoRoot, relPath string, b backend.Backend) tea.Cmd {
	return func() tea.Msg {
		if os.Getenv("TMUX") == "" {
			return nil
		}
		cwd, shellCmd := b.EditPaneCmd(repoRoot, relPath)
		args := []string{"split-window", "-v", "-t", "{left-of}"}
		if cwd != "" {
			args = append(args, "-c", cwd)
		}
		args = append(args, shellCmd)
		_ = exec.Command("tmux", args...).Run()
		return nil
	}
}

func gitTick() tea.Cmd {
	return tea.Tick(5*time.Second, func(t time.Time) tea.Msg {
		return gitTickMsg(t)
	})
}

func allTimeTick() tea.Cmd {
	return tea.Tick(30*time.Second, func(t time.Time) tea.Msg {
		return allTimeTickMsg(t)
	})
}

// sessionReconnect schedules a reconnect attempt after delay.
func sessionReconnect(delay time.Duration) tea.Cmd {
	return tea.Tick(delay, func(time.Time) tea.Msg {
		return sessionReconnectMsg{}
	})
}

// sessionIDFromPath extracts the session ID (UUID) from a JSONL file path.
func sessionIDFromPath(path string) string {
	base := filepath.Base(path)
	if len(base) > 5 && base[len(base)-5:] == ".jsonl" {
		return base[:len(base)-5]
	}
	return base
}
