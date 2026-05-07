// Package tunnel manages headless SSH local-forward processes
// (`ssh -N -L LOCAL:HOST:REMOTE host`). Each Tunnel owns one ssh
// child process; the TUI starts them at boot, polls Probe on a
// ticker, and calls Stop on shutdown.
package tunnel

import (
	"errors"
	"fmt"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Status int

const (
	StatusStopped Status = iota
	StatusStarting
	StatusUp
	StatusPortBusy
	StatusError
)

func (s Status) String() string {
	switch s {
	case StatusStopped:
		return "stopped"
	case StatusStarting:
		return "starting"
	case StatusUp:
		return "up"
	case StatusPortBusy:
		return "port-busy"
	case StatusError:
		return "error"
	}
	return "unknown"
}

// Spec describes a single local forward. Mirrors ssh's -L syntax:
// LOCAL:REMOTEHOST:REMOTE. RemoteHost defaults to "localhost".
type Spec struct {
	LocalPort  int
	RemoteHost string
	RemotePort int
}

func (s Spec) String() string {
	return fmt.Sprintf("%d:%s:%d", s.LocalPort, s.RemoteHost, s.RemotePort)
}

// ParseSpec accepts either "PORT" (shorthand for PORT:localhost:PORT)
// or the full "LOCAL:HOST:REMOTE" form.
func ParseSpec(s string) (Spec, error) {
	parts := strings.Split(s, ":")
	switch len(parts) {
	case 1:
		p, err := strconv.Atoi(parts[0])
		if err != nil || p <= 0 || p > 65535 {
			return Spec{}, fmt.Errorf("invalid port %q", parts[0])
		}
		return Spec{LocalPort: p, RemoteHost: "localhost", RemotePort: p}, nil
	case 3:
		lp, err1 := strconv.Atoi(parts[0])
		rp, err2 := strconv.Atoi(parts[2])
		if err1 != nil || err2 != nil || parts[1] == "" {
			return Spec{}, fmt.Errorf("invalid forward %q (want LOCAL:HOST:REMOTE)", s)
		}
		return Spec{LocalPort: lp, RemoteHost: parts[1], RemotePort: rp}, nil
	}
	return Spec{}, fmt.Errorf("invalid forward %q (use PORT or LOCAL:HOST:REMOTE)", s)
}

// Tunnel is a single managed `ssh -N -L` process.
type Tunnel struct {
	Spec Spec
	Host string

	mu     sync.Mutex
	status Status
	err    error
	cmd    *exec.Cmd
}

func New(host string, spec Spec) *Tunnel {
	return &Tunnel{Host: host, Spec: spec}
}

// Start spawns ssh -N -L in the background. It first probes the local
// port: if something else is listening, status becomes StatusPortBusy
// and no ssh process is started.
func (t *Tunnel) Start() {
	t.mu.Lock()
	if t.status == StatusUp || t.status == StatusStarting {
		t.mu.Unlock()
		return
	}

	addr := fmt.Sprintf("127.0.0.1:%d", t.Spec.LocalPort)
	if ln, err := net.Listen("tcp", addr); err != nil {
		t.status = StatusPortBusy
		t.err = err
		t.mu.Unlock()
		return
	} else {
		_ = ln.Close()
	}

	cmd := exec.Command("ssh", "-N",
		"-o", "ExitOnForwardFailure=yes",
		"-o", "ServerAliveInterval=30",
		"-L", t.Spec.String(),
		t.Host)
	if err := cmd.Start(); err != nil {
		t.status = StatusError
		t.err = err
		t.mu.Unlock()
		return
	}
	t.cmd = cmd
	t.status = StatusStarting
	t.err = nil
	t.mu.Unlock()

	go t.watch(cmd)
}

func (t *Tunnel) watch(cmd *exec.Cmd) {
	werr := cmd.Wait()
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.cmd != cmd {
		return
	}
	t.cmd = nil
	if werr != nil && !errors.Is(werr, exec.ErrNotFound) {
		t.status = StatusError
		t.err = werr
		return
	}
	t.status = StatusStopped
}

// Probe verifies the forward by dialing the local port. Promotes
// Starting → Up on the first successful dial; demotes Up → Starting
// if a dial fails (the watcher will set Error/Stopped if the process
// dies).
func (t *Tunnel) Probe() {
	t.mu.Lock()
	cmd := t.cmd
	status := t.status
	port := t.Spec.LocalPort
	t.mu.Unlock()

	if cmd == nil {
		return
	}
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 250*time.Millisecond)

	t.mu.Lock()
	defer t.mu.Unlock()
	if t.cmd != cmd {
		return
	}
	if err == nil {
		_ = conn.Close()
		t.status = StatusUp
		t.err = nil
		return
	}
	if status == StatusUp {
		t.status = StatusStarting
	}
}

// Stop kills the ssh child if running. Safe to call multiple times.
func (t *Tunnel) Stop() {
	t.mu.Lock()
	cmd := t.cmd
	t.cmd = nil
	if t.status != StatusError && t.status != StatusPortBusy {
		t.status = StatusStopped
	}
	t.mu.Unlock()
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}

// Snapshot returns the current status and last error.
func (t *Tunnel) Snapshot() (Status, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.status, t.err
}
