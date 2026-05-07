// Package tunnel manages SSH local-forward processes via a per-user
// on-disk registry (~/.config/agen-tui/tunnels.json). Tunnels are spawned
// detached so they outlive the agen-tui instance that created them, and
// every other agen-tui process on the machine reads the same registry —
// so a tunnel added in one terminal is visible from any other.
package tunnel

import (
	"fmt"
	"strconv"
	"strings"
)

// Status reports the runtime state of a registry Entry. It's the
// combination of user intent (Entry.Wanted) and observed state (PID
// liveness + local-port reachability).
type Status int

const (
	StatusStopped  Status = iota // user-stopped
	StatusStarting               // pid alive, local port not yet listening
	StatusUp                     // pid alive, local port listening
	StatusDead                   // wanted=up but pid missing
	StatusPortBusy               // could not start: another process held the port
	StatusError                  // misc failure (see Entry.LastError)
)

func (s Status) String() string {
	switch s {
	case StatusStopped:
		return "stopped"
	case StatusStarting:
		return "starting"
	case StatusUp:
		return "up"
	case StatusDead:
		return "dead"
	case StatusPortBusy:
		return "port-busy"
	case StatusError:
		return "error"
	}
	return "unknown"
}

// Spec describes a single local forward, mirroring ssh's -L syntax
// (LOCAL:REMOTEHOST:REMOTE).
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
//
// Validation is strict: ports must be 1–65535, the remote host must be
// non-empty and only contain hostname characters (a–z, 0–9, '.', '-',
// '_'). The remote host is *embedded* in the -L argument (not a
// separate argv element), so loose chars there could be parsed by ssh
// in surprising ways.
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
		if err1 != nil || lp <= 0 || lp > 65535 {
			return Spec{}, fmt.Errorf("invalid local port %q", parts[0])
		}
		if err2 != nil || rp <= 0 || rp > 65535 {
			return Spec{}, fmt.Errorf("invalid remote port %q", parts[2])
		}
		rhost := strings.TrimSpace(parts[1])
		if err := validateRemoteHost(rhost); err != nil {
			return Spec{}, err
		}
		return Spec{LocalPort: lp, RemoteHost: rhost, RemotePort: rp}, nil
	}
	return Spec{}, fmt.Errorf("invalid forward %q (use PORT or LOCAL:HOST:REMOTE)", s)
}

// validateRemoteHost permits hostname characters only. The remote host
// is embedded in the -L argument string, not a separate argv element,
// so we keep it conservative (no whitespace, no shell-meta).
func validateRemoteHost(h string) error {
	if h == "" {
		return fmt.Errorf("remote host is empty")
	}
	for _, r := range h {
		if !(r == '.' || r == '-' || r == '_' ||
			(r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9')) {
			return fmt.Errorf("remote host %q contains invalid char %q", h, r)
		}
	}
	return nil
}
