package tunnel

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// State is the user-intended state of a registry entry. The actual
// runtime state may diverge (a stopped pid for an Up entry → Dead).
type State string

const (
	StateUp   State = "up"
	StateDown State = "down"
)

// Entry is a single tunnel persisted on disk. ID = host|spec is the
// stable identifier used to address an entry across instances.
type Entry struct {
	ID        string    `json:"id"`
	Host      string    `json:"host"`
	Spec      string    `json:"spec"` // canonical "LOCAL:RHOST:REMOTE"
	Wanted    State     `json:"wanted"`
	PID       int       `json:"pid,omitempty"`
	StartedAt time.Time `json:"started_at,omitempty"`
	LastError string    `json:"last_error,omitempty"`
}

// Registry is the on-disk catalog. Loaded by every agen-tui instance on
// startup and refreshed periodically; writes go through WithLock.
type Registry struct {
	Version int     `json:"version"`
	Entries []Entry `json:"entries"`
}

const registryVersion = 1

// pathOverride is set in tests to redirect Path() to a temp dir.
var pathOverride string

// Path returns the absolute registry file path, creating ~/.config/agen-tui
// (mode 0700) on first call.
func Path() (string, error) {
	if pathOverride != "" {
		return pathOverride, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".config", "agen-tui")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return filepath.Join(dir, "tunnels.json"), nil
}

// LogPath returns where Spawn redirects child stdout/stderr.
func LogPath() (string, error) {
	p, err := Path()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(p), "tunnels.log"), nil
}

// MakeID is the stable identifier used to address an entry. Exported so
// the UI can construct it without first loading the registry.
func MakeID(host, spec string) string {
	return host + "|" + spec
}

// Load reads the registry from disk. Returns an empty registry when the
// file doesn't exist (first run); a corrupt file is an error so we don't
// silently drop persisted state on a typo.
func Load() (*Registry, error) {
	p, err := Path()
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return &Registry{Version: registryVersion}, nil
	}
	if err != nil {
		return nil, err
	}
	if len(b) == 0 {
		return &Registry{Version: registryVersion}, nil
	}
	var r Registry
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, fmt.Errorf("registry corrupt: %w", err)
	}
	return &r, nil
}

// Save writes the registry atomically: serialize → write to .tmp (mode
// 0600) → rename. The rename is the commit point on POSIX, so a crash
// mid-write leaves the previous version intact.
func Save(r *Registry) error {
	p, err := Path()
	if err != nil {
		return err
	}
	if r.Version == 0 {
		r.Version = registryVersion
	}
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

// WithLock takes an exclusive flock on a sidecar lock file, runs fn with
// the freshly-loaded registry, and saves on success. Any read-modify-write
// against the registry must go through this so concurrent agen-tui
// instances serialize properly.
//
// fn returning an error skips the save (so a failed Add doesn't persist
// half-mutated state).
func WithLock(fn func(r *Registry) error) error {
	p, err := Path()
	if err != nil {
		return err
	}
	lockPath := p + ".lock"
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)

	r, err := Load()
	if err != nil {
		return err
	}
	if err := fn(r); err != nil {
		return err
	}
	return Save(r)
}

// Find returns a pointer to the entry with the given ID, or nil.
// Callers may mutate the returned pointer in place.
func (r *Registry) Find(id string) *Entry {
	for i := range r.Entries {
		if r.Entries[i].ID == id {
			return &r.Entries[i]
		}
	}
	return nil
}

// validateHost rejects strings that contain shell-meta or whitespace —
// even though we use exec.Command (not /bin/sh), an attacker-supplied
// host that begins with `-` would be parsed by ssh as a flag, which is
// the classic argument-injection vector.
func validateHost(host string) error {
	host = strings.TrimSpace(host)
	if host == "" {
		return fmt.Errorf("host is empty")
	}
	if strings.HasPrefix(host, "-") {
		return fmt.Errorf("host %q starts with '-' (argument injection)", host)
	}
	for _, r := range host {
		// ssh hosts allow a-zA-Z0-9._-@ — anything outside is suspicious.
		if !(r == '.' || r == '-' || r == '_' || r == '@' ||
			(r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9')) {
			return fmt.Errorf("host %q contains invalid char %q", host, r)
		}
	}
	return nil
}

// Add validates and registers a new tunnel, spawns the ssh child, and
// stores the resulting PID. Idempotent in the sense that adding the same
// host+spec twice returns ErrExists rather than spawning a duplicate.
func (r *Registry) Add(host, spec string) (*Entry, error) {
	if err := validateHost(host); err != nil {
		return nil, err
	}
	parsed, err := ParseSpec(spec)
	if err != nil {
		return nil, err
	}
	canonical := parsed.String()
	id := MakeID(host, canonical)
	if r.Find(id) != nil {
		return nil, fmt.Errorf("tunnel %s already exists", id)
	}
	pid, err := Spawn(host, canonical)
	if err != nil {
		return nil, err
	}
	r.Entries = append(r.Entries, Entry{
		ID:        id,
		Host:      host,
		Spec:      canonical,
		Wanted:    StateUp,
		PID:       pid,
		StartedAt: time.Now(),
	})
	return &r.Entries[len(r.Entries)-1], nil
}

// Remove kills the ssh child (if any) and drops the entry. Killing a
// non-existent or recycled PID is silently tolerated — better to leave
// the registry consistent than fail mid-cleanup.
func (r *Registry) Remove(id string) error {
	for i, e := range r.Entries {
		if e.ID != id {
			continue
		}
		if e.PID > 0 {
			KillTunnel(e.PID)
		}
		r.Entries = append(r.Entries[:i], r.Entries[i+1:]...)
		return nil
	}
	return fmt.Errorf("tunnel %s not found", id)
}

// Toggle flips the desired state: a running entry is killed and marked
// Down; a stopped entry is respawned with a fresh PID.
func (r *Registry) Toggle(id string) error {
	e := r.Find(id)
	if e == nil {
		return fmt.Errorf("tunnel %s not found", id)
	}
	if e.Wanted == StateUp && e.PID > 0 && PIDAlive(e.PID) {
		KillTunnel(e.PID)
		e.PID = 0
		e.Wanted = StateDown
		e.LastError = ""
		return nil
	}
	pid, err := Spawn(e.Host, e.Spec)
	if err != nil {
		e.LastError = err.Error()
		return err
	}
	e.PID = pid
	e.Wanted = StateUp
	e.StartedAt = time.Now()
	e.LastError = ""
	return nil
}

// Reconcile syncs runtime state with kernel state: entries whose Wanted
// is Up but whose PID is dead get their PID cleared (status will then
// resolve to StatusDead). Returns true if anything changed.
func (r *Registry) Reconcile() bool {
	changed := false
	for i := range r.Entries {
		e := &r.Entries[i]
		if e.Wanted == StateUp && e.PID > 0 && !PIDAlive(e.PID) {
			e.PID = 0
			changed = true
		}
	}
	return changed
}

// ProbeStatus computes the current runtime status by combining intent,
// PID liveness, and a quick local-port dial.
func ProbeStatus(e *Entry) Status {
	if e.Wanted == StateDown {
		return StatusStopped
	}
	if e.PID == 0 || !PIDAlive(e.PID) {
		return StatusDead
	}
	if dialOK(e.Spec) {
		return StatusUp
	}
	return StatusStarting
}

func dialOK(spec string) bool {
	s, err := ParseSpec(spec)
	if err != nil {
		return false
	}
	addr := net.JoinHostPort("127.0.0.1", fmt.Sprintf("%d", s.LocalPort))
	conn, err := net.DialTimeout("tcp", addr, 250*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}
