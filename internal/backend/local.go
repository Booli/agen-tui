package backend

import (
	"os"
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
func (LocalBackend) EditTarget(repoRoot, relPath string) string               { return filepath.Join(repoRoot, relPath) }
func (LocalBackend) ActiveSessionFile(cwd string) string                      { return session.ActiveFile(cwd) }
func (LocalBackend) ProjectSessionFiles(cwd string) []string                  { return session.ProjectFiles(cwd) }
func (LocalBackend) ReadSessionBytes(path string) ([]byte, error)             { return os.ReadFile(path) }
