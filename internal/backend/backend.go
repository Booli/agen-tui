package backend

import "github.com/pimrutgers/agen-tui/internal/git"

// Snapshot bundles every piece of repo state the refresh tick needs, so SSH
// can fetch them in a single round-trip instead of 4–5 sequential calls.
// Local can fan out cheaply or batch — either is fine.
type Snapshot struct {
	Root     string
	Branch   string
	Status   []git.FileStatus
	AllFiles []string
}

// SessionFileInfo names a project JSONL with its current size. Size acts as
// a content fingerprint for append-only JSONL — if it hasn't grown, neither
// have its stats.
type SessionFileInfo struct {
	Path string
	Size int64
}

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
	// Snapshot returns Root+Branch+Status+AllFiles in one shot.
	// SSH implementations should batch into a single ssh invocation.
	Snapshot(dir string) (Snapshot, error)

	// Files
	ReadFile(absPath string) ([]byte, error)
	// EditPaneCmd returns the shell command and tmux start-directory for
	// opening repoRoot/relPath in an editor pane.
	// LocalBackend: cwd = repoRoot, cmd = "<editor> '<abs>'".
	// SSHBackend:   cwd = ""        (don't pass -c — repoRoot is remote),
	//               cmd = "ssh -t <host> 'vim <quoted abs>'".
	EditPaneCmd(repoRoot, relPath string) (cwd, cmd string)

	// Session JSONL
	ActiveSessionFile(cwd string) string
	ProjectSessionFiles(cwd string) []string
	// ProjectSessionFilesInfo augments ProjectSessionFiles with per-file
	// sizes so callers can skip re-reading unchanged files.
	ProjectSessionFilesInfo(cwd string) []SessionFileInfo
	ReadSessionBytes(path string) ([]byte, error)
	// StreamSessionBytes opens a long-lived `tail -F` on the JSONL file and
	// emits chunks (from byte 0, then any appends) on the returned channel.
	// The cancel func kills the underlying process and closes the channel.
	StreamSessionBytes(path string) (<-chan []byte, func(), error)
}
