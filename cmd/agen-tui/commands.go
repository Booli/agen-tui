package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/pimrutgers/agen-tui/internal/filetree"
	"github.com/pimrutgers/agen-tui/internal/git"
	"github.com/pimrutgers/agen-tui/internal/session"
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

// openFileDetailMsg asks the parent to switch into the file-detail
// overlay. The accompanying doFileDiff command will deliver a
// fileDiffMsg shortly after.
type openFileDetailMsg struct {
	path      string
	untracked bool
}

// closeOverlayMsg asks the parent to dismiss any active overlay.
type closeOverlayMsg struct{}

type sessionTickMsg time.Time
type gitTickMsg time.Time

// ── command factories ─────────────────────────────────────────────────────────

func doRefresh(dir string) tea.Cmd {
	return func() tea.Msg {
		root := git.Root(dir)
		if root == "" {
			return refreshMsg{err: fmt.Errorf("not a git repo")}
		}
		branch := git.Branch(root)
		files, _ := git.Status(root)

		statusMap := make(map[string]string)
		for _, f := range files {
			statusMap[f.Path] = string([]byte{f.X, f.Y})
		}

		allFiles, err := git.AllFiles(root)
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

func doSessionRefresh(dir string) tea.Cmd {
	return func() tea.Msg {
		file := session.ActiveFile(dir)
		if file == "" {
			return sessionMsg{err: fmt.Errorf("no session file")}
		}
		current, err := session.Parse(file)
		if err != nil {
			return sessionMsg{err: err}
		}
		allTime := session.ParseProject(dir)
		tools, _ := session.RecentTools(file, 500)
		return sessionMsg{current: current, allTime: allTime, tools: tools}
	}
}

func doFileDiff(root, path string, untracked bool) tea.Cmd {
	return func() tea.Msg {
		out, err := git.Diff(root, path, untracked)
		return fileDiffMsg{path: path, untracked: untracked, diff: out, err: err}
	}
}

// openFileDetail returns a batch that both signals the parent to open
// an overlay and kicks off the diff fetch.
func openFileDetail(path string, untracked bool, root string) tea.Cmd {
	return tea.Batch(
		func() tea.Msg { return openFileDetailMsg{path: path, untracked: untracked} },
		doFileDiff(root, path, untracked),
	)
}

// closeOverlay is a tiny cmd that dismisses any active detail overlay.
func closeOverlay() tea.Cmd {
	return func() tea.Msg { return closeOverlayMsg{} }
}

// openInPane splits the *main* tmux pane (the one to our left) below
// itself and runs $EDITOR on the file. Falls back to vim when $EDITOR
// is unset. No-op when not running inside tmux.
func openInPane(repoRoot, relPath string) tea.Cmd {
	return func() tea.Msg {
		if os.Getenv("TMUX") == "" {
			return nil
		}
		editor := os.Getenv("EDITOR")
		if editor == "" {
			editor = "vim"
		}
		abs := filepath.Join(repoRoot, relPath)
		// -v splits top/bottom; targeting {left-of} means the new pane
		// is created inside the main work pane, leaving the agen-tui
		// sidebar untouched.
		shell := fmt.Sprintf("%s %q", editor, abs)
		_ = exec.Command("tmux", "split-window", "-v",
			"-t", "{left-of}",
			"-c", repoRoot,
			shell).Run()
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
