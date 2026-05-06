package backend

import "github.com/pimrutgers/agen-tui/internal/git"

// Backend abstracts all I/O so the TUI can work against a local or remote
// working directory. LocalBackend wraps the real filesystem and git; a future
// SSHBackend will forward the same operations over an SSH connection.
type Backend interface {
	// Git
	Root(dir string) string
	Branch(root string) string
	Status(root string) ([]git.FileStatus, error)
	AllFiles(root string) ([]string, error)
	Diff(root, path string, untracked bool) (string, error)

	// Files
	ReadFile(absPath string) ([]byte, error)
	// EditTarget returns the path to open in $EDITOR for a repo-relative file.
	// LocalBackend returns filepath.Join(repoRoot, relPath);
	// SSHBackend returns scp://host/<absPath>.
	EditTarget(repoRoot, relPath string) string

	// Session JSONL
	ActiveSessionFile(cwd string) string
	ProjectSessionFiles(cwd string) []string
	ReadSessionBytes(path string) ([]byte, error)
}
