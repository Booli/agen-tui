package tunnel

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// SpawnFunc is the function used to start a tunnel. Production uses
// realSpawn; tests inject a stub via SetSpawnFunc.
type SpawnFunc func(host, spec string) (int, error)

var spawnFn SpawnFunc = realSpawn

// SetSpawnFunc swaps the spawn implementation. Returns a restore func
// the caller should defer to put the previous implementation back.
func SetSpawnFunc(f SpawnFunc) func() {
	prev := spawnFn
	spawnFn = f
	return func() { spawnFn = prev }
}

// Spawn starts a detached `ssh -N -L spec host` and returns its PID.
//
// Detachment uses Setsid so the child becomes its own session/group
// leader, fully decoupled from the agen-tui controlling terminal. We
// don't Wait() on it and call Process.Release() so the Go runtime
// stops tracking it; when agen-tui exits the kernel reparents to init
// (PPID=1) and the tunnel keeps running.
func Spawn(host, spec string) (int, error) { return spawnFn(host, spec) }

func realSpawn(host, spec string) (int, error) {
	if err := validateHost(host); err != nil {
		return 0, err
	}
	if _, err := ParseSpec(spec); err != nil {
		return 0, err
	}

	logF, err := openLog()
	if err != nil {
		return 0, err
	}

	cmd := exec.Command("ssh", "-N",
		"-o", "ExitOnForwardFailure=yes",
		"-o", "ServerAliveInterval=30",
		// Bypass user ControlMaster so the slave doesn't hand off to a
		// mux and exit immediately — see registry.Add for context.
		"-o", "ControlMaster=no",
		"-o", "ControlPath=none",
		"-L", spec,
		host)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Stdin = nil
	cmd.Stdout = logF
	cmd.Stderr = logF

	if err := cmd.Start(); err != nil {
		_ = logF.Close()
		return 0, err
	}
	pid := cmd.Process.Pid
	// Release tells the runtime to stop reaping so the child survives
	// our exit. After Release, cmd.Process is unusable.
	if err := cmd.Process.Release(); err != nil {
		_ = logF.Close()
		return pid, fmt.Errorf("release child: %w", err)
	}
	_ = logF.Close()
	return pid, nil
}

func openLog() (*os.File, error) {
	logPath, err := LogPath()
	if err != nil {
		return nil, err
	}
	return os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
}

// PIDAlive reports whether pid is a running process the caller could
// signal. Uses signal 0 (the no-op signal) — kernel returns ESRCH when
// the PID doesn't exist.
func PIDAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	return syscall.Kill(pid, 0) == nil
}

// IsTunnelProcessFunc is the predicate used by KillTunnel to refuse
// signaling a PID that no longer matches our `ssh` child (e.g. PID was
// recycled). Tests swap this for a permissive variant.
var IsTunnelProcessFunc = isTunnelProcessReal

// IsTunnelProcess returns true if pid still resolves to an `ssh` binary.
func IsTunnelProcess(pid int) bool { return IsTunnelProcessFunc(pid) }

func isTunnelProcessReal(pid int) bool {
	if pid <= 0 {
		return false
	}
	out, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "comm=").Output()
	if err != nil {
		return false
	}
	name := strings.TrimSpace(string(out))
	// ps -o comm= can return either "ssh" or a full path on some systems.
	return name == "ssh" || strings.HasSuffix(name, "/ssh")
}

// KillTunnel terminates a tunnel by PID. SIGTERM first to let ssh close
// the forward gracefully (writes "Killed by signal 15." to the log),
// then SIGKILL after a short grace period as a fallback. We skip the
// signal entirely if the PID no longer points at our `ssh` process.
func KillTunnel(pid int) {
	if pid <= 0 {
		return
	}
	if !PIDAlive(pid) || !IsTunnelProcess(pid) {
		return
	}
	// Negative PID = signal entire process group (we Setsid the child,
	// so its session/group is rooted at this PID).
	_ = syscall.Kill(-pid, syscall.SIGTERM)

	// Give it up to 500 ms to exit cleanly.
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if !PIDAlive(pid) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	_ = syscall.Kill(-pid, syscall.SIGKILL)
}
