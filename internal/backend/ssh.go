package backend

import (
	"fmt"
	"os/exec"
	"path"
	"strings"

	"github.com/pimrutgers/agen-tui/internal/git"
)

// shellQuote wraps s in single quotes safe for POSIX sh interpolation.
// Embedded single quotes are escaped as '\''.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// SSHBackend runs every operation on a remote host via SSH.
// With SSH ControlMaster configured, each call reuses the existing socket
// from the user's active SSH session — no passwords or new handshakes.
type SSHBackend struct {
	host string            // user@host or plain host
	run  func(string) ([]byte, error) // injectable for tests
}

// NewSSHBackend creates a backend targeting host (user@host or plain host).
func NewSSHBackend(host string) SSHBackend {
	return SSHBackend{
		host: host,
		run: func(cmd string) ([]byte, error) {
			return exec.Command("ssh", host, cmd).Output()
		},
	}
}

func (b SSHBackend) Root(dir string) string {
	out, err := b.run(fmt.Sprintf("git -C %s rev-parse --show-toplevel 2>/dev/null", shellQuote(dir)))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func (b SSHBackend) Branch(root string) string {
	q := shellQuote(root)
	out, err := b.run(fmt.Sprintf("git -C %s symbolic-ref --short HEAD 2>/dev/null || git -C %s rev-parse --short HEAD 2>/dev/null", q, q))
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

func (b SSHBackend) Status(root string) ([]git.FileStatus, error) {
	out, err := b.run(fmt.Sprintf("git -C %s status --porcelain 2>/dev/null", shellQuote(root)))
	if err != nil {
		return nil, err
	}
	// numstat is skipped over SSH to avoid per-file round trips;
	// Added/Deleted stay 0 — acceptable for remote sessions.
	return git.ParseStatusOutput(string(out)), nil
}

func (b SSHBackend) AllFiles(root string) ([]string, error) {
	q := shellQuote(root)
	tracked, err := b.run(fmt.Sprintf("git -C %s ls-files 2>/dev/null", q))
	if err != nil {
		return nil, err
	}
	untracked, _ := b.run(fmt.Sprintf("git -C %s ls-files --others --exclude-standard 2>/dev/null", q))
	return git.ParseFilesOutput(string(tracked), string(untracked)), nil
}

func (b SSHBackend) Diff(root, filePath string, untracked bool) (string, error) {
	var cmd string
	if untracked {
		cmd = fmt.Sprintf("cat %s", shellQuote(path.Join(root, filePath)))
	} else {
		qr := shellQuote(root)
		qf := shellQuote(filePath)
		cmd = fmt.Sprintf("git -C %s diff HEAD --no-color -- %s 2>/dev/null || git -C %s diff --cached --no-color -- %s 2>/dev/null", qr, qf, qr, qf)
	}
	out, err := b.run(cmd)
	return string(out), err
}

func (b SSHBackend) ReadFile(absPath string) ([]byte, error) {
	return b.run(fmt.Sprintf("cat %s", shellQuote(absPath)))
}

func (b SSHBackend) EditTarget(repoRoot, relPath string) string {
	// scp:// URL for vim — double slash needed before absolute path.
	abs := path.Join(repoRoot, relPath)
	return fmt.Sprintf("scp://%s/%s", b.host, abs)
}

// ActiveSessionFile returns the most recently modified JSONL path for cwd on
// the remote. The shell expands ~ before ls runs, so the returned path is
// already absolute; ReadSessionBytes fetches it with ssh cat.
func (b SSHBackend) ActiveSessionFile(cwd string) string {
	slug := strings.ReplaceAll(cwd, "/", "-")
	out, err := b.run(fmt.Sprintf("ls -t ~/.claude/projects/%s/*.jsonl 2>/dev/null | head -1", shellQuote(slug)))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// ProjectSessionFiles returns all JSONL paths for cwd's project on the remote.
func (b SSHBackend) ProjectSessionFiles(cwd string) []string {
	slug := strings.ReplaceAll(cwd, "/", "-")
	out, err := b.run(fmt.Sprintf("ls ~/.claude/projects/%s/*.jsonl 2>/dev/null", shellQuote(slug)))
	if err != nil {
		return nil
	}
	var paths []string
	for _, line := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			paths = append(paths, line)
		}
	}
	return paths
}

func (b SSHBackend) ReadSessionBytes(remotePath string) ([]byte, error) {
	return b.run(fmt.Sprintf("cat %s", shellQuote(remotePath)))
}
