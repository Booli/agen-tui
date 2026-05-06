package session

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pimrutgers/agen-tui/internal/ui"
)

type ToolCall struct {
	ID         string
	Name       string
	Summary    string
	Time       time.Time
	Done       bool
	Error      bool
	Added      int             // lines added (Write/Edit/MultiEdit)
	Deleted    int             // lines deleted
	InputRaw   json.RawMessage // original tool_use input, kept for the detail view
	ResultText string          // concatenated text from the paired tool_result, capped
}

// resultCap caps the size of any single captured tool_result so that a
// very large output (e.g. a Read of a big file) can't blow up memory.
const resultCap = 16 * 1024

type contentBlock struct {
	Type      string          `json:"type"`
	Name      string          `json:"name"`
	ID        string          `json:"id"`
	Input     json.RawMessage `json:"input"`
	ToolUseID string          `json:"tool_use_id"`
	IsError   bool            `json:"is_error"`
	// Text appears on text blocks; Content appears on tool_result blocks
	// where the result is itself a list of nested blocks (or a plain string).
	Text    string          `json:"text"`
	Content json.RawMessage `json:"content"`
}

type assistantContent struct {
	Content []contentBlock `json:"content"`
}

type userContent struct {
	Content json.RawMessage `json:"content"`
}

// RecentTools parses a JSONL session file and returns tool calls in chronological
// order. If limit > 0, only the last `limit` tool calls are kept.
func RecentTools(path string, limit int) ([]ToolCall, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return RecentToolsReader(f, limit)
}

// RecentToolsBytes parses tool calls from raw JSONL bytes.
func RecentToolsBytes(data []byte, limit int) ([]ToolCall, error) {
	return RecentToolsReader(bytes.NewReader(data), limit)
}

// RecentToolsReader parses tool calls from a reader.
func RecentToolsReader(r io.Reader, limit int) ([]ToolCall, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 4*1024*1024), 4*1024*1024)

	type pendingResult struct {
		isError bool
		text    string
	}

	var calls []ToolCall
	byID := map[string]int{}
	results := map[string]pendingResult{}

	apply := func(idx int, p pendingResult) {
		calls[idx].Done = true
		calls[idx].Error = p.isError
		calls[idx].ResultText = p.text
	}

	for scanner.Scan() {
		var r record
		if err := json.Unmarshal(scanner.Bytes(), &r); err != nil {
			continue
		}
		ts, _ := time.Parse(time.RFC3339, r.Timestamp)

		switch r.Type {
		case "assistant":
			var ac assistantContent
			if err := json.Unmarshal(r.Message, &ac); err != nil {
				continue
			}
			for _, b := range ac.Content {
				if b.Type != "tool_use" {
					continue
				}
				idx := len(calls)
				byID[b.ID] = idx
				added, deleted := lineDelta(b.Name, b.Input)
				calls = append(calls, ToolCall{
					ID:       b.ID,
					Name:     b.Name,
					Time:     ts,
					Summary:  summarize(b.Name, b.Input),
					Added:    added,
					Deleted:  deleted,
					InputRaw: append(json.RawMessage{}, b.Input...),
				})
				// JSONL ordering can place the tool_result *before* its
				// matching tool_use; if we already buffered the result,
				// pair it now.
				if p, ok := results[b.ID]; ok {
					apply(idx, p)
					delete(results, b.ID)
				}
			}
		case "user":
			var uc userContent
			if err := json.Unmarshal(r.Message, &uc); err != nil {
				continue
			}
			var blocks []contentBlock
			if err := json.Unmarshal(uc.Content, &blocks); err != nil {
				continue // user content was a plain string, not tool results
			}
			for _, b := range blocks {
				if b.Type != "tool_result" || b.ToolUseID == "" {
					continue
				}
				p := pendingResult{isError: b.IsError, text: extractResultText(b.Content)}
				if idx, ok := byID[b.ToolUseID]; ok {
					apply(idx, p)
					continue
				}
				results[b.ToolUseID] = p // tool_use not seen yet — buffer
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return calls, err
	}

	if limit > 0 && len(calls) > limit {
		calls = calls[len(calls)-limit:]
	}
	return calls, nil
}

func summarize(name string, input json.RawMessage) string {
	var m map[string]any
	if err := json.Unmarshal(input, &m); err != nil {
		return ""
	}
	getStr := func(k string) string {
		v, _ := m[k].(string)
		return v
	}

	switch name {
	case "Bash":
		if d := getStr("description"); d != "" {
			return d
		}
		return firstLine(getStr("command"), 120)
	case "Read":
		return filepath.Base(getStr("file_path"))
	case "Edit", "Write", "NotebookEdit":
		return filepath.Base(getStr("file_path"))
	case "Grep":
		p := getStr("pattern")
		if path := getStr("path"); path != "" {
			return fmt.Sprintf("%q in %s", p, path)
		}
		return fmt.Sprintf("%q", p)
	case "Glob":
		return getStr("pattern")
	case "WebFetch":
		return getStr("url")
	case "WebSearch":
		return getStr("query")
	case "Agent":
		if d := getStr("description"); d != "" {
			return d
		}
		return getStr("subagent_type")
	case "TodoWrite", "TaskCreate":
		if todos, ok := m["todos"].([]any); ok {
			return fmt.Sprintf("%d items", len(todos))
		}
	case "Skill":
		return getStr("skill")
	case "ToolSearch":
		return getStr("query")
	}

	for _, k := range []string{"file_path", "path", "url", "query", "command", "pattern", "description"} {
		if v := getStr(k); v != "" {
			return firstLine(v, 120)
		}
	}
	return ""
}

// extractResultText pulls the visible text out of a tool_result `content`
// field, which is either a plain string or an array of content blocks.
func extractResultText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return capString(s, resultCap)
	}
	var blocks []contentBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return ""
	}
	var b strings.Builder
	for _, blk := range blocks {
		if blk.Text != "" {
			b.WriteString(blk.Text)
			b.WriteByte('\n')
		}
		if b.Len() >= resultCap {
			break
		}
	}
	return capString(strings.TrimRight(b.String(), "\n"), resultCap)
}

func capString(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "\n… (truncated)"
}

// lineDelta returns approximate (added, deleted) line counts for tools
// that mutate files. For Write the entire content is "added". For Edit
// and MultiEdit we count the line difference between old_string and
// new_string (and the same per edit for MultiEdit). Returns (0,0) for
// any tool that doesn't fit this shape.
func lineDelta(name string, input json.RawMessage) (int, int) {
	switch name {
	case "Write":
		var m struct {
			Content string `json:"content"`
		}
		if err := json.Unmarshal(input, &m); err == nil && m.Content != "" {
			return countLines(m.Content), 0
		}
	case "Edit":
		var m struct {
			Old string `json:"old_string"`
			New string `json:"new_string"`
		}
		if err := json.Unmarshal(input, &m); err == nil {
			return diffCounts(m.Old, m.New)
		}
	case "MultiEdit":
		var m struct {
			Edits []struct {
				Old string `json:"old_string"`
				New string `json:"new_string"`
			} `json:"edits"`
		}
		if err := json.Unmarshal(input, &m); err == nil {
			var a, d int
			for _, e := range m.Edits {
				ea, ed := diffCounts(e.Old, e.New)
				a += ea
				d += ed
			}
			return a, d
		}
	}
	return 0, 0
}

func countLines(s string) int {
	if s == "" {
		return 0
	}
	n := strings.Count(s, "\n")
	if !strings.HasSuffix(s, "\n") {
		n++
	}
	return n
}

// diffCounts returns precise (added, deleted) line counts between old
// and new using the same LCS used for the Edit detail diff. Falls back
// to (len(b), len(a)) for very large inputs to bound cost.
func diffCounts(old, neu string) (int, int) {
	a := strings.Split(old, "\n")
	b := strings.Split(neu, "\n")
	if len(a) > ui.DiffMaxLines || len(b) > ui.DiffMaxLines {
		return len(b), len(a)
	}
	rows := ui.LCSDiff(a, b)
	var added, deleted int
	for _, r := range rows {
		switch r.Kind {
		case '+':
			added++
		case '-':
			deleted++
		}
	}
	return added, deleted
}

func firstLine(s string, max int) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > max {
		s = s[:max-1] + "…"
	}
	return s
}
