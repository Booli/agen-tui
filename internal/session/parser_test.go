package session

import (
	"reflect"
	"testing"
)

const sampleJSONL = `{"type":"user","timestamp":"2026-01-01T10:00:00Z","message":{"content":"hi"}}
{"type":"assistant","timestamp":"2026-01-01T10:00:01Z","message":{"model":"claude-opus-4-7","usage":{"input_tokens":100,"output_tokens":50,"cache_read_input_tokens":200},"content":[{"type":"text","text":"ok"},{"type":"tool_use","id":"t1","name":"Read","input":{"file_path":"/foo"}}]}}
{"type":"user","timestamp":"2026-01-01T10:00:02Z","message":{"content":[{"type":"tool_result","tool_use_id":"t1","content":"file contents"}]}}
{"type":"assistant","timestamp":"2026-01-01T10:00:03Z","message":{"model":"claude-opus-4-7","usage":{"input_tokens":120,"output_tokens":80},"content":[{"type":"tool_use","id":"t2","name":"Bash","input":{"command":"ls"}}]}}
`

// Feeding the same bytes byte-by-byte must yield identical Stats and tools
// to a single bulk parse — this is the contract that makes streaming safe.
func TestParserMatchesBulkParse(t *testing.T) {
	wantStats, err := ParseBytes([]byte(sampleJSONL))
	if err != nil {
		t.Fatal(err)
	}
	wantTools, err := RecentToolsBytes([]byte(sampleJSONL), 0)
	if err != nil {
		t.Fatal(err)
	}

	p := NewParser()
	for _, b := range []byte(sampleJSONL) {
		p.Append([]byte{b})
	}

	if !reflect.DeepEqual(p.Stats, wantStats) {
		t.Errorf("Stats mismatch:\n got %+v\nwant %+v", p.Stats, wantStats)
	}
	gotTools := p.RecentTools(0)
	if len(gotTools) != len(wantTools) {
		t.Fatalf("len(tools) = %d, want %d", len(gotTools), len(wantTools))
	}
	for i := range gotTools {
		if gotTools[i].ID != wantTools[i].ID ||
			gotTools[i].Name != wantTools[i].Name ||
			gotTools[i].Done != wantTools[i].Done ||
			gotTools[i].ResultText != wantTools[i].ResultText {
			t.Errorf("tool[%d]:\n got %+v\nwant %+v", i, gotTools[i], wantTools[i])
		}
	}
}

func TestParserHandlesPartialTrailingLine(t *testing.T) {
	p := NewParser()
	p.Append([]byte(`{"type":"user","timestamp":"2026-01-01T10:00:00Z","message":{"con`))
	if p.Stats.Prompts != 0 {
		t.Errorf("expected 0 prompts before line completes, got %d", p.Stats.Prompts)
	}
	p.Append([]byte(`tent":"hi"}}` + "\n"))
	if p.Stats.Prompts != 1 {
		t.Errorf("expected 1 prompt after line completes, got %d", p.Stats.Prompts)
	}
}
