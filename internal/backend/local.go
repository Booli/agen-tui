package backend

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/pimrutgers/agen-tui/internal/git"
	"github.com/pimrutgers/agen-tui/internal/session"
)

// LocalBackend delegates every operation to the local filesystem and git.
type LocalBackend struct{}

func (LocalBackend) Root(dir string) string                                  { return git.Root(dir) }
func (LocalBackend) Branch(root string) string                               { return git.Branch(root) }
func (LocalBackend) Status(root string) ([]git.FileStatus, error)            { return git.Status(root) }
func (LocalBackend) AllFiles(root string) ([]string, error)                  { return git.AllFiles(root) }
func (LocalBackend) Diff(root, path string, untracked bool) (string, error)  { return git.Diff(root, path, untracked) }
func (LocalBackend) ReadFile(absPath string) ([]byte, error)                  { return os.ReadFile(absPath) }
func (LocalBackend) ActiveSessionFile(cwd string) string                      { return session.ActiveFile(cwd) }
func (LocalBackend) ProjectSessionFiles(cwd string) []string                  { return session.ProjectFiles(cwd) }
func (LocalBackend) ReadSessionBytes(path string) ([]byte, error)             { return os.ReadFile(path) }

func (b LocalBackend) Snapshot(dir string) (Snapshot, error) {
	root := b.Root(dir)
	if root == "" {
		return Snapshot{}, nil
	}
	branch := b.Branch(root)
	status, _ := b.Status(root)
	files, err := b.AllFiles(root)
	return Snapshot{Root: root, Branch: branch, Status: status, AllFiles: files}, err
}

func (LocalBackend) ProjectSessionFilesInfo(cwd string) []SessionFileInfo {
	paths := session.ProjectFiles(cwd)
	out := make([]SessionFileInfo, 0, len(paths))
	for _, p := range paths {
		fi, err := os.Stat(p)
		if err != nil {
			continue
		}
		out = append(out, SessionFileInfo{Path: p, Size: fi.Size()})
	}
	return out
}

func (LocalBackend) StreamSessionBytes(path string) (<-chan []byte, func(), error) {
	return streamTail(exec.Command("tail", "-F", "-c", "+0", path))
}

func (LocalBackend) EditPaneCmd(repoRoot, relPath string) (cwd, cmd string) {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vim"
	}
	abs := filepath.Join(repoRoot, relPath)
	return repoRoot, fmt.Sprintf("%s %s", editor, ShellQuote(abs))
}
