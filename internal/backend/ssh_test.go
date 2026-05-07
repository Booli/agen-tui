package backend

import (
	"fmt"
	"testing"
)

// mockSSH returns a runner that serves canned responses keyed by a substring
// of the command. If no key matches, it returns an empty result.
func mockSSH(responses map[string]string) func(string) ([]byte, error) {
	return func(cmd string) ([]byte, error) {
		for k, v := range responses {
			if containsStr(cmd, k) {
				return []byte(v), nil
			}
		}
		return nil, fmt.Errorf("mock: no response for: %s", cmd)
	}
}

func containsStr(s, sub string) bool {
	return len(sub) > 0 && len(s) >= len(sub) && (s == sub || len(s) > 0 && findStr(s, sub))
}

func findStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// Compile-time assertion.
var _ Backend = SSHBackend{}

func TestSSHBackendRoot(t *testing.T) {
	b := SSHBackend{host: "testhost", run: mockSSH(map[string]string{
		"rev-parse --show-toplevel": "/home/user/myproject\n",
	})}
	if got := b.Root("/home/user/myproject"); got != "/home/user/myproject" {
		t.Errorf("Root = %q, want /home/user/myproject", got)
	}
}

func TestSSHBackendRootNotGitRepo(t *testing.T) {
	b := SSHBackend{host: "testhost", run: func(cmd string) ([]byte, error) {
		return nil, fmt.Errorf("exit status 128")
	}}
	if got := b.Root("/tmp"); got != "" {
		t.Errorf("Root = %q, want empty for non-repo", got)
	}
}

func TestSSHBackendBranch(t *testing.T) {
	b := SSHBackend{host: "testhost", run: mockSSH(map[string]string{
		"symbolic-ref": "main\n",
	})}
	if got := b.Branch("/home/user/myproject"); got != "main" {
		t.Errorf("Branch = %q, want main", got)
	}
}

func TestSSHBackendStatus(t *testing.T) {
	porcelain := " M internal/foo.go\n?? untracked.txt\nA  staged.go\n"
	b := SSHBackend{host: "testhost", run: mockSSH(map[string]string{
		"status --porcelain": porcelain,
	})}
	files, err := b.Status("/repo")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 3 {
		t.Fatalf("len(files) = %d, want 3", len(files))
	}
	if files[0].Path != "internal/foo.go" {
		t.Errorf("files[0].Path = %q", files[0].Path)
	}
	if !files[1].IsUntracked() {
		t.Errorf("files[1] should be untracked")
	}
	if !files[2].IsStaged() {
		t.Errorf("files[2] should be staged")
	}
}

func TestSSHBackendAllFiles(t *testing.T) {
	b := SSHBackend{host: "testhost", run: func(cmd string) ([]byte, error) {
		if findStr(cmd, "--others") {
			return []byte("new.go\n"), nil
		}
		return []byte("main.go\nlib.go\n"), nil
	}}
	files, err := b.AllFiles("/repo")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 3 {
		t.Fatalf("len(files) = %d, want 3", len(files))
	}
}

func TestSSHBackendEditPaneCmd(t *testing.T) {
	b := SSHBackend{host: "user@10.0.0.1"}
	cwd, cmd := b.EditPaneCmd("/home/user/project", "src/main.go")
	if cwd != "" {
		t.Errorf("cwd = %q, want empty for SSH", cwd)
	}
	want := `ssh -t user@10.0.0.1 'vim '\''/home/user/project/src/main.go'\'''`
	if cmd != want {
		t.Errorf("cmd = %q, want %q", cmd, want)
	}
}

func TestSSHBackendSnapshot(t *testing.T) {
	// Build a remote response that splits cleanly on the sentinel.
	sep := snapshotSentinel
	out := "/repo" + sep +
		"main" + sep +
		" M foo.go\n?? bar.go\n" + sep +
		"foo.go\nbaz.go\n" + sep +
		"bar.go\n"
	b := SSHBackend{host: "h", run: func(cmd string) ([]byte, error) {
		return []byte(out), nil
	}}
	snap, err := b.Snapshot("/repo")
	if err != nil {
		t.Fatal(err)
	}
	if snap.Root != "/repo" || snap.Branch != "main" {
		t.Errorf("Snapshot Root/Branch = %q/%q", snap.Root, snap.Branch)
	}
	if len(snap.Status) != 2 {
		t.Errorf("len(Status) = %d, want 2", len(snap.Status))
	}
	if len(snap.AllFiles) != 3 {
		t.Errorf("len(AllFiles) = %d, want 3 (tracked+untracked combined)", len(snap.AllFiles))
	}
}

func TestSSHBackendSnapshotEmptyOnNonRepo(t *testing.T) {
	// Remote script exits early when not a repo — empty stdout.
	b := SSHBackend{host: "h", run: func(cmd string) ([]byte, error) {
		return []byte(""), nil
	}}
	snap, err := b.Snapshot("/tmp")
	if err != nil {
		t.Fatal(err)
	}
	if snap.Root != "" {
		t.Errorf("expected empty Snapshot, got %+v", snap)
	}
}

func TestSSHBackendProjectSessionFilesInfo(t *testing.T) {
	out := "1024 /home/u/.claude/projects/x/aaa.jsonl\n2048 /home/u/.claude/projects/x/bbb.jsonl\n"
	b := SSHBackend{host: "h", run: func(cmd string) ([]byte, error) {
		return []byte(out), nil
	}}
	infos := b.ProjectSessionFilesInfo("/some/cwd")
	if len(infos) != 2 {
		t.Fatalf("len(infos) = %d, want 2", len(infos))
	}
	if infos[0].Size != 1024 || infos[1].Size != 2048 {
		t.Errorf("sizes = %d/%d, want 1024/2048", infos[0].Size, infos[1].Size)
	}
	if infos[0].Path != "/home/u/.claude/projects/x/aaa.jsonl" {
		t.Errorf("path[0] = %q", infos[0].Path)
	}
}

func TestSSHBackendReadSessionBytes(t *testing.T) {
	content := `{"type":"user","message":{"content":"hi"}}` + "\n"
	b := SSHBackend{host: "testhost", run: mockSSH(map[string]string{
		"cat": content,
	})}
	data, err := b.ReadSessionBytes("/home/user/.claude/projects/slug/session.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != content {
		t.Errorf("ReadSessionBytes content mismatch")
	}
}
