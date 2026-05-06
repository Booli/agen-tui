package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSummarize(t *testing.T) {
	cases := []struct {
		name  string
		tool  string
		input string
		want  string
	}{
		{"bash with description", "Bash",
			`{"command":"ls -la","description":"list files"}`, "list files"},
		{"bash without description", "Bash",
			`{"command":"echo hi"}`, "echo hi"},
		{"bash multiline command first line only", "Bash",
			`{"command":"echo one\necho two"}`, "echo one"},
		{"read", "Read",
			`{"file_path":"/a/b/c/file.go"}`, "file.go"},
		{"edit", "Edit",
			`{"file_path":"/tmp/x.txt","old_string":"a","new_string":"b"}`, "x.txt"},
		{"write", "Write",
			`{"file_path":"/tmp/y.md","content":"hi"}`, "y.md"},
		{"grep with path", "Grep",
			`{"pattern":"foo","path":"src"}`, `"foo" in src`},
		{"grep without path", "Grep",
			`{"pattern":"bar"}`, `"bar"`},
		{"glob", "Glob",
			`{"pattern":"**/*.go"}`, "**/*.go"},
		{"webfetch", "WebFetch",
			`{"url":"https://example.com","prompt":"x"}`, "https://example.com"},
		{"websearch", "WebSearch",
			`{"query":"go testing"}`, "go testing"},
		{"agent description", "Agent",
			`{"description":"audit branch","subagent_type":"general-purpose","prompt":"..."}`,
			"audit branch"},
		{"agent fallback to subagent_type", "Agent",
			`{"subagent_type":"general-purpose","prompt":"..."}`, "general-purpose"},
		{"todowrite", "TodoWrite",
			`{"todos":[{"content":"a"},{"content":"b"},{"content":"c"}]}`, "3 items"},
		{"skill", "Skill",
			`{"skill":"loop"}`, "loop"},
		{"unknown tool falls back to first known key", "MCPThing",
			`{"file_path":"/p/q.go"}`, "/p/q.go"},
		{"unknown tool with no known keys", "Mystery",
			`{"flarble":"glorp"}`, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := summarize(tc.tool, json.RawMessage(tc.input))
			if got != tc.want {
				t.Fatalf("summarize(%s, %s) = %q, want %q",
					tc.tool, tc.input, got, tc.want)
			}
		})
	}
}

func TestFirstLine(t *testing.T) {
	if got := firstLine("abc\ndef", 80); got != "abc" {
		t.Fatalf("got %q", got)
	}
	if got := firstLine("aaaaa", 4); got != "aaa…" {
		t.Fatalf("got %q", got)
	}
	if got := firstLine("abc", 80); got != "abc" {
		t.Fatalf("got %q", got)
	}
}

// writeJSONL writes one JSON record per line to a temp file and returns the path.
func writeJSONL(t *testing.T, records []string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	var buf []byte
	for _, r := range records {
		buf = append(buf, []byte(r)...)
		buf = append(buf, '\n')
	}
	if err := os.WriteFile(path, buf, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRecentTools(t *testing.T) {
	records := []string{
		// assistant turn with two tool_use blocks
		`{"type":"assistant","timestamp":"2026-05-05T10:00:00Z","message":{"content":[{"type":"tool_use","id":"t1","name":"Read","input":{"file_path":"/a.go"}},{"type":"tool_use","id":"t2","name":"Bash","input":{"command":"go test","description":"run tests"}}]}}`,
		// user turn pairing both results — t1 ok, t2 error
		`{"type":"user","timestamp":"2026-05-05T10:00:01Z","message":{"content":[{"type":"tool_result","tool_use_id":"t1","is_error":false},{"type":"tool_result","tool_use_id":"t2","is_error":true}]}}`,
		// plain user message (string content) — must not crash the parser
		`{"type":"user","timestamp":"2026-05-05T10:00:02Z","message":{"content":"hi"}}`,
		// another tool_use, no result yet (still in flight)
		`{"type":"assistant","timestamp":"2026-05-05T10:00:03Z","message":{"content":[{"type":"tool_use","id":"t3","name":"Edit","input":{"file_path":"/x.txt"}}]}}`,
	}
	path := writeJSONL(t, records)

	tools, err := RecentTools(path, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 3 {
		t.Fatalf("len(tools) = %d, want 3", len(tools))
	}

	// chronological order: t1, t2, t3
	if tools[0].ID != "t1" || tools[0].Name != "Read" {
		t.Errorf("tools[0] = %+v", tools[0])
	}
	if !tools[0].Done || tools[0].Error {
		t.Errorf("t1 should be done & ok, got %+v", tools[0])
	}

	if tools[1].ID != "t2" || tools[1].Summary != "run tests" {
		t.Errorf("tools[1] = %+v", tools[1])
	}
	if !tools[1].Done || !tools[1].Error {
		t.Errorf("t2 should be done & errored, got %+v", tools[1])
	}

	if tools[2].ID != "t3" || tools[2].Done {
		t.Errorf("t3 should be pending, got %+v", tools[2])
	}

	// InputRaw should be captured for the detail view
	if len(tools[1].InputRaw) == 0 {
		t.Error("t2 should have InputRaw populated")
	}
	if !strings.Contains(string(tools[1].InputRaw), `"command":"go test"`) {
		t.Errorf("t2 InputRaw missing command: %s", string(tools[1].InputRaw))
	}
}

func TestRecentToolsResultText(t *testing.T) {
	records := []string{
		`{"type":"assistant","timestamp":"2026-05-05T10:00:00Z","message":{"content":[{"type":"tool_use","id":"a","name":"Bash","input":{"command":"echo hi"}},{"type":"tool_use","id":"b","name":"Read","input":{"file_path":"/x"}}]}}`,
		// a: result content is a plain string
		`{"type":"user","timestamp":"2026-05-05T10:00:01Z","message":{"content":[{"type":"tool_result","tool_use_id":"a","is_error":false,"content":"hi"}]}}`,
		// b: result content is an array of text blocks
		`{"type":"user","timestamp":"2026-05-05T10:00:02Z","message":{"content":[{"type":"tool_result","tool_use_id":"b","is_error":false,"content":[{"type":"text","text":"line one"},{"type":"text","text":"line two"}]}]}}`,
	}
	path := writeJSONL(t, records)

	tools, err := RecentTools(path, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 2 {
		t.Fatalf("got %d tools, want 2", len(tools))
	}
	if tools[0].ResultText != "hi" {
		t.Errorf("plain-string result: got %q", tools[0].ResultText)
	}
	if tools[1].ResultText != "line one\nline two" {
		t.Errorf("array-of-blocks result: got %q", tools[1].ResultText)
	}
}

func TestRecentToolsLineDelta(t *testing.T) {
	records := []string{
		// Write: 3 lines of content → +3
		`{"type":"assistant","timestamp":"2026-05-05T10:00:00Z","message":{"content":[{"type":"tool_use","id":"w","name":"Write","input":{"file_path":"/x","content":"a\nb\nc\n"}}]}}`,
		// Edit: replace 1 of 3 lines → +1, -1
		`{"type":"assistant","timestamp":"2026-05-05T10:00:01Z","message":{"content":[{"type":"tool_use","id":"e","name":"Edit","input":{"file_path":"/x","old_string":"a\nb\nc","new_string":"a\nB\nc"}}]}}`,
		// MultiEdit: two edits, each adds 1
		`{"type":"assistant","timestamp":"2026-05-05T10:00:02Z","message":{"content":[{"type":"tool_use","id":"me","name":"MultiEdit","input":{"file_path":"/x","edits":[{"old_string":"a","new_string":"a\nb"},{"old_string":"c","new_string":"c\nd"}]}}]}}`,
		// Bash: no delta
		`{"type":"assistant","timestamp":"2026-05-05T10:00:03Z","message":{"content":[{"type":"tool_use","id":"b","name":"Bash","input":{"command":"ls"}}]}}`,
	}
	path := writeJSONL(t, records)
	tools, err := RecentTools(path, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 4 {
		t.Fatalf("want 4 tools, got %d", len(tools))
	}
	if tools[0].Added != 3 || tools[0].Deleted != 0 {
		t.Errorf("Write: got +%d/-%d, want +3/-0", tools[0].Added, tools[0].Deleted)
	}
	if tools[1].Added != 1 || tools[1].Deleted != 1 {
		t.Errorf("Edit: got +%d/-%d, want +1/-1", tools[1].Added, tools[1].Deleted)
	}
	if tools[2].Added != 2 || tools[2].Deleted != 0 {
		t.Errorf("MultiEdit: got +%d/-%d, want +2/-0", tools[2].Added, tools[2].Deleted)
	}
	if tools[3].Added != 0 || tools[3].Deleted != 0 {
		t.Errorf("Bash: got +%d/-%d, want 0/0", tools[3].Added, tools[3].Deleted)
	}
}

// Real session JSONLs sometimes emit a tool_result line *before* its
// matching tool_use. The parser must still pair them.
func TestRecentToolsOutOfOrder(t *testing.T) {
	records := []string{
		// result first
		`{"type":"user","timestamp":"2026-05-05T10:00:00Z","message":{"content":[{"type":"tool_result","tool_use_id":"early","content":"hello"}]}}`,
		// then the matching tool_use
		`{"type":"assistant","timestamp":"2026-05-05T10:00:01Z","message":{"content":[{"type":"tool_use","id":"early","name":"Edit","input":{"file_path":"/x"}}]}}`,
	}
	path := writeJSONL(t, records)

	tools, err := RecentTools(path, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 1 {
		t.Fatalf("want 1 tool, got %d", len(tools))
	}
	if !tools[0].Done {
		t.Fatal("expected tool to be paired (Done=true), but it's still in flight")
	}
	if tools[0].ResultText != "hello" {
		t.Errorf("ResultText = %q, want %q", tools[0].ResultText, "hello")
	}
}

func TestExtractResultTextCap(t *testing.T) {
	big := make([]byte, resultCap+1000)
	for i := range big {
		big[i] = 'x'
	}
	raw, _ := json.Marshal(string(big))
	got := extractResultText(raw)
	if len(got) <= resultCap {
		t.Fatalf("expected truncation marker, got len=%d", len(got))
	}
	if !strings.Contains(got, "(truncated)") {
		t.Fatal("expected '(truncated)' marker")
	}
}

func TestRecentToolsLimit(t *testing.T) {
	var records []string
	for i := 0; i < 10; i++ {
		records = append(records, `{"type":"assistant","timestamp":"2026-05-05T10:00:00Z","message":{"content":[`+
			`{"type":"tool_use","id":"id`+itoa(i)+`","name":"Read","input":{"file_path":"/a.go"}}`+
			`]}}`)
	}
	path := writeJSONL(t, records)

	tools, err := RecentTools(path, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 3 {
		t.Fatalf("got %d, want 3", len(tools))
	}
	// limit keeps the *last* N — so id7, id8, id9
	if tools[0].ID != "id7" || tools[2].ID != "id9" {
		t.Fatalf("limit kept wrong slice: %v / %v", tools[0].ID, tools[2].ID)
	}
}

func TestRecentToolsBadFile(t *testing.T) {
	if _, err := RecentTools("/no/such/path", 0); err == nil {
		t.Fatal("expected error for missing file")
	}
}

// TestRecentToolsBytes verifies that RecentToolsBytes produces the same
// results as RecentTools on the same content.
func TestRecentToolsBytes(t *testing.T) {
	records := []string{
		`{"type":"assistant","timestamp":"2026-05-05T10:00:00Z","message":{"content":[{"type":"tool_use","id":"t1","name":"Read","input":{"file_path":"/a.go"}}]}}`,
		`{"type":"user","timestamp":"2026-05-05T10:00:01Z","message":{"content":[{"type":"tool_result","tool_use_id":"t1","is_error":false}]}}`,
	}
	path := writeJSONL(t, records)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	byPath, err := RecentTools(path, 0)
	if err != nil {
		t.Fatal(err)
	}
	byBytes, err := RecentToolsBytes(data, 0)
	if err != nil {
		t.Fatal(err)
	}

	if len(byPath) != len(byBytes) {
		t.Fatalf("len mismatch: path=%d bytes=%d", len(byPath), len(byBytes))
	}
	for i := range byPath {
		if byPath[i].ID != byBytes[i].ID || byPath[i].Name != byBytes[i].Name || byPath[i].Done != byBytes[i].Done {
			t.Errorf("call[%d] mismatch: %+v vs %+v", i, byPath[i], byBytes[i])
		}
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var buf [20]byte
	n := len(buf)
	for i > 0 {
		n--
		buf[n] = byte('0' + i%10)
		i /= 10
	}
	return string(buf[n:])
}
