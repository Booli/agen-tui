package session

import (
	"os"
	"testing"
)

func TestParseBytesMatchesParse(t *testing.T) {
	records := []string{
		`{"type":"user","timestamp":"2026-05-05T10:00:00Z","message":{"content":"hello"}}`,
		`{"type":"assistant","timestamp":"2026-05-05T10:00:01Z","message":{"model":"claude-sonnet-4-6","usage":{"input_tokens":10,"output_tokens":20},"content":[{"type":"tool_use"}]}}`,
	}
	path := writeJSONL(t, records)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	byPath, err := Parse(path)
	if err != nil {
		t.Fatal(err)
	}
	byBytes, err := ParseBytes(data)
	if err != nil {
		t.Fatal(err)
	}

	if byPath.Prompts != byBytes.Prompts {
		t.Errorf("Prompts: path=%d bytes=%d", byPath.Prompts, byBytes.Prompts)
	}
	if byPath.ToolCalls != byBytes.ToolCalls {
		t.Errorf("ToolCalls: path=%d bytes=%d", byPath.ToolCalls, byBytes.ToolCalls)
	}
	if byPath.InputTokens != byBytes.InputTokens {
		t.Errorf("InputTokens: path=%d bytes=%d", byPath.InputTokens, byBytes.InputTokens)
	}
	if byPath.Model != byBytes.Model {
		t.Errorf("Model: path=%q bytes=%q", byPath.Model, byBytes.Model)
	}
	// Parse() sets SessionID from the filename; ParseBytes() doesn't — that's expected
}
