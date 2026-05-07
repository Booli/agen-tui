package tunnel

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

// withTempRegistry redirects Path() to a fresh temp dir for the duration
// of t. Restores the previous override on cleanup.
func withTempRegistry(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	prev := pathOverride
	pathOverride = filepath.Join(dir, "tunnels.json")
	t.Cleanup(func() { pathOverride = prev })
	return dir
}

// stubSpawnListening replaces the real spawn with one that starts a
// `nc` process bound to the loopback port (so probes succeed) and
// also relaxes the IsTunnelProcess guard so tests can kill it.
// Returns a cleanup that restores both and reaps any leftover nc.
func stubSpawnListening(t *testing.T) (cleanup func()) {
	t.Helper()
	var spawned []*exec.Cmd
	var mu sync.Mutex

	restoreSpawn := SetSpawnFunc(func(host, spec string) (int, error) {
		s, err := ParseSpec(spec)
		if err != nil {
			return 0, err
		}
		cmd := exec.Command("nc", "-l", strconv.Itoa(s.LocalPort))
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		if err := cmd.Start(); err != nil {
			return 0, err
		}
		mu.Lock()
		spawned = append(spawned, cmd)
		mu.Unlock()
		// Reap on exit so the test runner's PIDAlive doesn't see a
		// zombie as alive (kill(zombie, 0) succeeds — but in production
		// init reaps because the parent agen-tui has gone away).
		go func() { _ = cmd.Wait() }()
		time.Sleep(50 * time.Millisecond)
		return cmd.Process.Pid, nil
	})

	prevPredicate := IsTunnelProcessFunc
	IsTunnelProcessFunc = func(pid int) bool { return PIDAlive(pid) }

	return func() {
		restoreSpawn()
		IsTunnelProcessFunc = prevPredicate
		mu.Lock()
		defer mu.Unlock()
		for _, c := range spawned {
			if c.Process != nil {
				_ = syscall.Kill(-c.Process.Pid, syscall.SIGKILL)
				_, _ = c.Process.Wait()
			}
		}
	}
}

func freePort(t *testing.T) int {
	t.Helper()
	for i := 0; i < 5; i++ {
		// Open + close to find an ephemeral port. Mild race but fine for tests.
		ln, err := exec.Command("sh", "-c", "echo $$").Output()
		_ = ln
		_ = err
		// Simpler: ask the kernel directly.
		break
	}
	// Use net package idiom from the tunnel pkg itself.
	cmd := exec.Command("sh", "-c", "python3 -c 'import socket; s=socket.socket(); s.bind((\"\",0)); print(s.getsockname()[1])'")
	out, err := cmd.Output()
	if err == nil {
		p, _ := strconv.Atoi(strings.TrimSpace(string(out)))
		if p > 0 {
			return p
		}
	}
	t.Fatalf("could not find a free port")
	return 0
}

func TestLoadEmptyRegistry(t *testing.T) {
	withTempRegistry(t)
	r, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Entries) != 0 {
		t.Errorf("empty registry should have 0 entries, got %d", len(r.Entries))
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	withTempRegistry(t)
	r := &Registry{
		Version: 1,
		Entries: []Entry{
			{ID: "a|5137:localhost:5137", Host: "a", Spec: "5137:localhost:5137", Wanted: StateUp, PID: 1234},
			{ID: "b|8080:db:5432", Host: "b", Spec: "8080:db:5432", Wanted: StateDown},
		},
	}
	if err := Save(r); err != nil {
		t.Fatal(err)
	}
	r2, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(r2.Entries) != 2 {
		t.Fatalf("len = %d, want 2", len(r2.Entries))
	}
	if r2.Entries[0].Host != "a" || r2.Entries[1].Host != "b" {
		t.Errorf("entries: %+v", r2.Entries)
	}
}

func TestSaveAtomicViaRename(t *testing.T) {
	dir := withTempRegistry(t)
	if err := Save(&Registry{Version: 1, Entries: []Entry{{ID: "x", Host: "x", Spec: "1:y:1"}}}); err != nil {
		t.Fatal(err)
	}
	// .tmp must not linger after a successful Save.
	if _, err := os.Stat(filepath.Join(dir, "tunnels.json.tmp")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf(".tmp file should be renamed away, got err=%v", err)
	}
}

func TestLoadCorruptIsError(t *testing.T) {
	dir := withTempRegistry(t)
	if err := os.WriteFile(filepath.Join(dir, "tunnels.json"), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(); err == nil {
		t.Errorf("Load on corrupt file should error")
	}
}

func TestPathCreatesDirWith0700(t *testing.T) {
	// Test against real Path() resolution by overriding HOME.
	tmp := t.TempDir()
	prev := os.Getenv("HOME")
	prevOverride := pathOverride
	os.Setenv("HOME", tmp)
	pathOverride = "" // force the home-dir branch
	t.Cleanup(func() {
		os.Setenv("HOME", prev)
		pathOverride = prevOverride
	})
	p, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(p, tmp) {
		t.Errorf("Path %q is not under HOME %q", p, tmp)
	}
	st, err := os.Stat(filepath.Dir(p))
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o700 {
		t.Errorf("dir mode = %o, want 0700", st.Mode().Perm())
	}
}

func TestAddRejectsInvalidHost(t *testing.T) {
	withTempRegistry(t)
	r := &Registry{}
	if _, err := r.Add("-oProxyCommand=foo", "5137"); err == nil {
		t.Error("Add with -arg host should fail")
	}
	if _, err := r.Add("good-host", "abc"); err == nil {
		t.Error("Add with bad spec should fail")
	}
}

func TestAddDuplicateRejected(t *testing.T) {
	withTempRegistry(t)
	cleanup := stubSpawnListening(t)
	defer cleanup()
	r := &Registry{}
	port := freePort(t)
	spec := strconv.Itoa(port)
	if _, err := r.Add("host", spec); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Add("host", spec); err == nil {
		t.Error("duplicate Add should fail")
	}
	if len(r.Entries) != 1 {
		t.Errorf("len = %d, want 1", len(r.Entries))
	}
}

func TestAddSpawnsProbeCanReach(t *testing.T) {
	withTempRegistry(t)
	cleanup := stubSpawnListening(t)
	defer cleanup()
	r := &Registry{}
	port := freePort(t)
	e, err := r.Add("host", strconv.Itoa(port))
	if err != nil {
		t.Fatal(err)
	}
	// Probe should report Up (PID alive + port listening).
	got := ProbeStatus(e)
	if got != StatusUp {
		t.Errorf("ProbeStatus = %v, want StatusUp", got)
	}
}

func TestRemoveKillsProcess(t *testing.T) {
	withTempRegistry(t)
	cleanup := stubSpawnListening(t)
	defer cleanup()
	r := &Registry{}
	port := freePort(t)
	e, err := r.Add("host", strconv.Itoa(port))
	if err != nil {
		t.Fatal(err)
	}
	pid := e.PID
	if err := r.Remove(e.ID); err != nil {
		t.Fatal(err)
	}
	if len(r.Entries) != 0 {
		t.Errorf("entries after remove = %d, want 0", len(r.Entries))
	}
	// KillTunnel waits up to 500 ms before SIGKILL, then the kernel reaps.
	// Allow generous slack for slow CI.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if !PIDAlive(pid) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if PIDAlive(pid) {
		t.Errorf("PID %d still alive after Remove + 2s", pid)
	}
}

func TestToggleFlipsState(t *testing.T) {
	withTempRegistry(t)
	cleanup := stubSpawnListening(t)
	defer cleanup()
	r := &Registry{}
	port := freePort(t)
	e, err := r.Add("host", strconv.Itoa(port))
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Toggle(e.ID); err != nil {
		t.Fatal(err)
	}
	if e2 := r.Find(e.ID); e2 == nil || e2.Wanted != StateDown || e2.PID != 0 {
		t.Errorf("after toggle off: %+v", e2)
	}
	// Toggle back on — needs a fresh port since first stub may still hold a TIME_WAIT slot.
	port2 := freePort(t)
	r.Entries[0].Spec = strconv.Itoa(port2) + ":localhost:" + strconv.Itoa(port2)
	r.Entries[0].ID = MakeID("host", r.Entries[0].Spec)
	if err := r.Toggle(r.Entries[0].ID); err != nil {
		t.Fatal(err)
	}
	if r.Entries[0].Wanted != StateUp || r.Entries[0].PID == 0 {
		t.Errorf("after toggle on: %+v", r.Entries[0])
	}
}

func TestReconcileMarksDeadEntries(t *testing.T) {
	withTempRegistry(t)
	r := &Registry{
		Entries: []Entry{
			{ID: "a", Host: "a", Spec: "1:l:1", Wanted: StateUp, PID: 1}, // pid 1 = init, alive but bogus
			{ID: "b", Host: "b", Spec: "2:l:2", Wanted: StateUp, PID: 999999}, // very unlikely to exist
			{ID: "c", Host: "c", Spec: "3:l:3", Wanted: StateDown, PID: 0},
		},
	}
	r.Reconcile()
	// Entry b's bogus PID should be cleared.
	if r.Find("b").PID != 0 {
		t.Errorf("dead PID not cleared: %+v", r.Find("b"))
	}
	// Entry c stays Down with PID 0 (no change).
	if r.Find("c").Wanted != StateDown || r.Find("c").PID != 0 {
		t.Errorf("Down entry mutated: %+v", r.Find("c"))
	}
}

func TestWithLockSerializesConcurrentWrites(t *testing.T) {
	withTempRegistry(t)
	var counter atomic.Int32
	const n = 20
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := WithLock(func(r *Registry) error {
				counter.Add(1)
				r.Entries = append(r.Entries, Entry{ID: strconv.Itoa(int(counter.Load()))})
				return nil
			})
			if err != nil {
				t.Errorf("WithLock: %v", err)
			}
		}()
	}
	wg.Wait()
	r, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Entries) != n {
		t.Errorf("len = %d, want %d (lock did not serialize)", len(r.Entries), n)
	}
}

func TestWithLockSkipsSaveOnError(t *testing.T) {
	withTempRegistry(t)
	if err := Save(&Registry{Version: 1, Entries: []Entry{{ID: "original"}}}); err != nil {
		t.Fatal(err)
	}
	myErr := errors.New("nope")
	err := WithLock(func(r *Registry) error {
		r.Entries = append(r.Entries, Entry{ID: "ghost"})
		return myErr
	})
	if !errors.Is(err, myErr) {
		t.Errorf("err = %v, want myErr", err)
	}
	r, _ := Load()
	if len(r.Entries) != 1 || r.Entries[0].ID != "original" {
		t.Errorf("entries after failed lock: %+v", r.Entries)
	}
}

func TestProbeStatusIntents(t *testing.T) {
	if got := ProbeStatus(&Entry{Wanted: StateDown, PID: 0}); got != StatusStopped {
		t.Errorf("Down entry: %v, want Stopped", got)
	}
	if got := ProbeStatus(&Entry{Wanted: StateUp, PID: 0}); got != StatusDead {
		t.Errorf("Up entry with PID=0: %v, want Dead", got)
	}
	if got := ProbeStatus(&Entry{Wanted: StateUp, PID: 999999}); got != StatusDead {
		t.Errorf("Up entry with bogus PID: %v, want Dead", got)
	}
}

func TestPIDAlive(t *testing.T) {
	if PIDAlive(0) || PIDAlive(-1) {
		t.Error("invalid pids should report not-alive")
	}
	if !PIDAlive(os.Getpid()) {
		t.Error("self pid should be alive")
	}
}

func TestKillTunnelGuardsAgainstNonSSH(t *testing.T) {
	// Spawn a long-running sleep we own, then ask KillTunnel to signal it.
	// Because the process's comm is "sleep" (not ssh), the IsTunnelProcess
	// guard must refuse — the sleep should still be alive after.
	cmd := exec.Command("sleep", "30")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	pid := cmd.Process.Pid
	t.Cleanup(func() {
		_ = syscall.Kill(-pid, syscall.SIGKILL)
		_, _ = cmd.Process.Wait()
	})

	if IsTunnelProcess(pid) {
		t.Fatalf("sleep PID %d unexpectedly classified as ssh", pid)
	}
	KillTunnel(pid)
	if !PIDAlive(pid) {
		t.Fatalf("KillTunnel terminated non-ssh PID %d — guard failed", pid)
	}
}
