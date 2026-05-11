package backend

import (
	"fmt"
	"os/exec"
	"path"
	"strings"
	"syscall"

	"github.com/pimrutgers/agen-tui/internal/git"
)

// ShellQuote wraps s in single quotes safe for POSIX sh interpolation.
// Embedded single quotes are escaped as '\''.
func ShellQuote(s string) string {
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
	out, err := b.run(fmt.Sprintf("git -C %s rev-parse --show-toplevel 2>/dev/null", ShellQuote(dir)))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func (b SSHBackend) Branch(root string) string {
	q := ShellQuote(root)
	out, err := b.run(fmt.Sprintf("git -C %s symbolic-ref --short HEAD 2>/dev/null || git -C %s rev-parse --short HEAD 2>/dev/null", q, q))
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

func (b SSHBackend) Status(root string) ([]git.FileStatus, error) {
	out, err := b.run(fmt.Sprintf("git -C %s status --porcelain 2>/dev/null", ShellQuote(root)))
	if err != nil {
		return nil, err
	}
	// numstat is skipped over SSH to avoid per-file round trips;
	// Added/Deleted stay 0 — acceptable for remote sessions.
	return git.ParseStatusOutput(string(out)), nil
}

func (b SSHBackend) AllFiles(root string) ([]string, error) {
	q := ShellQuote(root)
	tracked, err := b.run(fmt.Sprintf("git -C %s ls-files 2>/dev/null", q))
	if err != nil {
		return nil, err
	}
	untracked, _ := b.run(fmt.Sprintf("git -C %s ls-files --others --exclude-standard 2>/dev/null", q))
	ignored, _ := b.run(fmt.Sprintf("git -C %s ls-files --others --ignored --exclude-standard 2>/dev/null", q))
	return git.ParseFilesOutput(string(tracked), string(untracked), string(ignored)), nil
}

// snapshotSentinel separates the five outputs of a batched Snapshot.
const snapshotSentinel = "\n---AGEN-SNAPSHOT---\n"

// Snapshot fetches Root+Branch+Status+AllFiles in a single ssh round-trip.
// The remote shell prints each section followed by a sentinel; we split on
// the sentinel to recover the original outputs.
func (b SSHBackend) Snapshot(dir string) (Snapshot, error) {
	sep := ShellQuote(snapshotSentinel)
	script := fmt.Sprintf(`R=$(git -C %s rev-parse --show-toplevel 2>/dev/null) || exit 0
printf '%%s' "$R"
printf '%%s' %s
git -C "$R" symbolic-ref --short HEAD 2>/dev/null || git -C "$R" rev-parse --short HEAD 2>/dev/null
printf '%%s' %s
git -C "$R" status --porcelain 2>/dev/null
printf '%%s' %s
git -C "$R" ls-files 2>/dev/null
printf '%%s' %s
git -C "$R" ls-files --others --exclude-standard 2>/dev/null
printf '%%s' %s
git -C "$R" ls-files --others --ignored --exclude-standard 2>/dev/null`,
		ShellQuote(dir), sep, sep, sep, sep, sep)

	out, err := b.run(script)
	if err != nil {
		return Snapshot{}, err
	}
	parts := strings.Split(string(out), snapshotSentinel)
	if len(parts) < 5 || strings.TrimSpace(parts[0]) == "" {
		return Snapshot{}, nil
	}
	ignored := ""
	if len(parts) >= 6 {
		ignored = parts[5]
	}
	return Snapshot{
		Root:     strings.TrimSpace(parts[0]),
		Branch:   strings.TrimSpace(parts[1]),
		Status:   git.ParseStatusOutput(parts[2]),
		AllFiles: git.ParseFilesOutput(parts[3], parts[4], ignored),
	}, nil
}

// ProjectSessionFilesInfo lists JSONL files with their sizes via one
// `stat`-style call; works on both BSD (macOS) and GNU `stat`.
func (b SSHBackend) ProjectSessionFilesInfo(cwd string) []SessionFileInfo {
	slug := strings.ReplaceAll(cwd, "/", "-")
	// `wc -c < <file>` is portable across macOS/Linux and prints just bytes.
	// We loop over each .jsonl and emit "<size> <path>" per line.
	script := fmt.Sprintf(
		`for f in ~/.claude/projects/%s/*.jsonl; do [ -e "$f" ] || continue; printf '%%s %%s\n' "$(wc -c < "$f")" "$f"; done`,
		ShellQuote(slug),
	)
	out, err := b.run(script)
	if err != nil {
		return nil
	}
	var infos []SessionFileInfo
	for _, line := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// "<size> <path>" — size first, then space, then path (which itself has no spaces in our slug case).
		idx := strings.IndexByte(line, ' ')
		if idx < 1 {
			continue
		}
		var size int64
		fmt.Sscanf(line[:idx], "%d", &size)
		infos = append(infos, SessionFileInfo{Path: line[idx+1:], Size: size})
	}
	return infos
}

func (b SSHBackend) Diff(root, filePath string, untracked bool) (string, error) {
	var cmd string
	if untracked {
		cmd = fmt.Sprintf("cat %s", ShellQuote(path.Join(root, filePath)))
	} else {
		qr := ShellQuote(root)
		qf := ShellQuote(filePath)
		cmd = fmt.Sprintf("git -C %s diff HEAD --no-color -- %s 2>/dev/null || git -C %s diff --cached --no-color -- %s 2>/dev/null", qr, qf, qr, qf)
	}
	out, err := b.run(cmd)
	return string(out), err
}

func (b SSHBackend) ReadFile(absPath string) ([]byte, error) {
	return b.run(fmt.Sprintf("cat %s", ShellQuote(absPath)))
}

// EditPaneCmd returns a tmux shell-command that runs vim on the remote via
// ssh -t. The TUI pane provides the TTY; ControlMaster keeps the spawn fast.
// We don't pass tmux's -c (cwd would be the remote path, which doesn't exist
// locally), so cwd is returned empty.
func (b SSHBackend) EditPaneCmd(repoRoot, relPath string) (cwd, cmd string) {
	abs := path.Join(repoRoot, relPath)
	remoteCmd := fmt.Sprintf("vim %s", ShellQuote(abs))
	return "", fmt.Sprintf("ssh -t %s %s", b.host, ShellQuote(remoteCmd))
}

// ActiveSessionFile returns the most recently modified JSONL path for cwd on
// the remote. The shell expands ~ before ls runs, so the returned path is
// already absolute; ReadSessionBytes fetches it with ssh cat.
func (b SSHBackend) ActiveSessionFile(cwd string) string {
	slug := strings.ReplaceAll(cwd, "/", "-")
	out, err := b.run(fmt.Sprintf("ls -t ~/.claude/projects/%s/*.jsonl 2>/dev/null | head -1", ShellQuote(slug)))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// ProjectSessionFiles returns all JSONL paths for cwd's project on the remote.
func (b SSHBackend) ProjectSessionFiles(cwd string) []string {
	slug := strings.ReplaceAll(cwd, "/", "-")
	out, err := b.run(fmt.Sprintf("ls ~/.claude/projects/%s/*.jsonl 2>/dev/null", ShellQuote(slug)))
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
	return b.run(fmt.Sprintf("cat %s", ShellQuote(remotePath)))
}

func (b SSHBackend) StreamSessionBytes(remotePath string) (<-chan []byte, func(), error) {
	cmd := exec.Command("ssh", b.host, fmt.Sprintf("tail -F -c +0 %s", ShellQuote(remotePath)))
	return streamTail(cmd)
}

// streamTail starts cmd, pipes stdout into a buffered channel, and returns a
// cancel func that kills the process group. Chunks are 4 KiB each.
//
// We put the child in its own process group (Setpgid) so cancel can kill
// the entire group with one syscall. ssh-over-tail spawns no further
// children today, but if it ever did, this prevents orphaned descendants.
func streamTail(cmd *exec.Cmd) (<-chan []byte, func(), error) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, nil, err
	}
	ch := make(chan []byte, 8)
	go func() {
		defer close(ch)
		defer cmd.Wait() // reap the child once stdout closes
		buf := make([]byte, 4096)
		for {
			n, err := out.Read(buf)
			if n > 0 {
				chunk := make([]byte, n)
				copy(chunk, buf[:n])
				ch <- chunk
			}
			if err != nil {
				return
			}
		}
	}()
	cancel := func() {
		if cmd.Process == nil {
			return
		}
		// Negative PID = signal entire process group.
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	return ch, cancel, nil
}
