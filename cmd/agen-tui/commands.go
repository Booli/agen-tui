package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

type sessionMsg struct {
	current session.Stats
	allTime session.Stats
	tools   []session.ToolCall
	err     error
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

type sessionTickMsg time.Time
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
		root := b.Root(dir)
		if root == "" {
			return refreshMsg{err: fmt.Errorf("not a git repo")}
		}
		branch := b.Branch(root)
		files, _ := b.Status(root)

		statusMap := make(map[string]string)
		for _, f := range files {
			statusMap[f.Path] = string([]byte{f.X, f.Y})
		}

		allFiles, err := b.AllFiles(root)
		var treeRoot *filetree.Node
		if err == nil {
			treeRoot = filetree.Build(allFiles, statusMap)
		}

		return refreshMsg{
			repoRoot: root,
			branch:   branch,
			files:    files,
			treeRoot: treeRoot,
		}
	}
}

func doSessionRefresh(dir string, b backend.Backend) tea.Cmd {
	return func() tea.Msg {
		activePath := b.ActiveSessionFile(dir)
		if activePath == "" {
			return sessionMsg{err: fmt.Errorf("no session file")}
		}
		data, err := b.ReadSessionBytes(activePath)
		if err != nil {
			return sessionMsg{err: err}
		}
		current, err := session.ParseBytes(data)
		current.SessionID = sessionIDFromPath(activePath)
		if err != nil {
			return sessionMsg{err: err}
		}
		tools, _ := session.RecentToolsBytes(data, 500)

		var allTime session.Stats
		for _, f := range b.ProjectSessionFiles(dir) {
			d, err := b.ReadSessionBytes(f)
			if err != nil {
				continue
			}
			s, err := session.ParseBytes(d)
			if err == nil {
				allTime = allTime.Add(s)
				allTime.Sessions++
			}
		}

		return sessionMsg{current: current, allTime: allTime, tools: tools}
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

// runInPane splits the tmux pane to the left and opens $EDITOR on the
// backend-resolved path (local abs path or scp://host/... for SSH).
func runInPane(repoRoot, relPath string, b backend.Backend) tea.Cmd {
	return func() tea.Msg {
		if os.Getenv("TMUX") == "" {
			return nil
		}
		target := b.EditTarget(repoRoot, relPath)
		// scp:// URLs are vim-specific; fall back to vim regardless of $EDITOR.
		editor := os.Getenv("EDITOR")
		if editor == "" || strings.HasPrefix(target, "scp://") {
			editor = "vim"
		}
		_ = exec.Command("tmux", "split-window", "-v",
			"-t", "{left-of}",
			"-c", repoRoot,
			editor, target).Run()
		return nil
	}
}

func gitTick() tea.Cmd {
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
		return gitTickMsg(t)
	})
}

func sessionTick() tea.Cmd {
	return tea.Tick(3*time.Second, func(t time.Time) tea.Msg {
		return sessionTickMsg(t)
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
