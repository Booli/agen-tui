package backend

import (
	"fmt"
	"os/exec"
	"path"
	"strings"

	"github.com/pimrutgers/agen-tui/internal/git"
)

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
	out, err := b.run(fmt.Sprintf("git -C %q rev-parse --show-toplevel 2>/dev/null", dir))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func (b SSHBackend) Branch(root string) string {
	out, err := b.run(fmt.Sprintf("git -C %q symbolic-ref --short HEAD 2>/dev/null || git -C %q rev-parse --short HEAD 2>/dev/null", root, root))
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

func (b SSHBackend) Status(root string) ([]git.FileStatus, error) {
	out, err := b.run(fmt.Sprintf("git -C %q status --porcelain 2>/dev/null", root))
	if err != nil {
		return nil, err
	}
	// numstat is skipped over SSH to avoid per-file round trips;
	// Added/Deleted stay 0 — acceptable for remote sessions.
	return git.ParseStatusOutput(string(out)), nil
}

func (b SSHBackend) AllFiles(root string) ([]string, error) {
	tracked, err := b.run(fmt.Sprintf("git -C %q ls-files 2>/dev/null", root))
	if err != nil {
		return nil, err
	}
	untracked, _ := b.run(fmt.Sprintf("git -C %q ls-files --others --exclude-standard 2>/dev/null", root))
	return git.ParseFilesOutput(string(tracked), string(untracked)), nil
}

func (b SSHBackend) Diff(root, filePath string, untracked bool) (string, error) {
	var cmd string
	if untracked {
		cmd = fmt.Sprintf("cat %q", path.Join(root, filePath))
	} else {
		cmd = fmt.Sprintf("git -C %q diff HEAD --no-color -- %q 2>/dev/null || git -C %q diff --cached --no-color -- %q 2>/dev/null", root, filePath, root, filePath)
	}
	out, err := b.run(cmd)
	return string(out), err
}

func (b SSHBackend) ReadFile(absPath string) ([]byte, error) {
	return b.run(fmt.Sprintf("cat %q", absPath))
}

func (b SSHBackend) EditTarget(repoRoot, relPath string) string {
	// scp:// URL for vim — double slash needed before absolute path.
	abs := path.Join(repoRoot, relPath)
	return fmt.Sprintf("scp://%s/%s", b.host, abs)
}

// ActiveSessionFile returns the most recently modified JSONL path for cwd on
// the remote. The returned string is a remote absolute path; ReadSessionBytes
// will fetch it with ssh cat.
func (b SSHBackend) ActiveSessionFile(cwd string) string {
	slug := strings.ReplaceAll(cwd, "/", "-")
	dir := fmt.Sprintf("~/.claude/projects/%s", slug)
	out, err := b.run(fmt.Sprintf("ls -t %s/*.jsonl 2>/dev/null | head -1", dir))
	if err != nil {
		return ""
	}
	p := strings.TrimSpace(string(out))
	if p == "" {
		return ""
	}
	// Expand the leading ~ to an absolute path so ReadSessionBytes can use it.
	out2, err := b.run(fmt.Sprintf("echo %s", p))
	if err != nil {
		return p
	}
	return strings.TrimSpace(string(out2))
}

// ProjectSessionFiles returns all JSONL paths for cwd's project on the remote.
func (b SSHBackend) ProjectSessionFiles(cwd string) []string {
	slug := strings.ReplaceAll(cwd, "/", "-")
	dir := fmt.Sprintf("~/.claude/projects/%s", slug)
	out, err := b.run(fmt.Sprintf("ls %s/*.jsonl 2>/dev/null", dir))
	if err != nil {
		return nil
	}
	// Expand tildes via realpath or echo
	var paths []string
	for _, line := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "~") {
			exp, err := b.run(fmt.Sprintf("echo %s", line))
			if err == nil {
				line = strings.TrimSpace(string(exp))
			}
		}
		paths = append(paths, line)
	}
	return paths
}

func (b SSHBackend) ReadSessionBytes(remotePath string) ([]byte, error) {
	return b.run(fmt.Sprintf("cat %q", remotePath))
}
