package session

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Stats struct {
	SessionID         string
	Model             string
	Prompts           int // user-typed messages (excludes tool_result roundtrips)
	ToolCalls         int // total tool_use blocks across all assistant messages
	InputTokens       int64
	OutputTokens      int64
	CacheReadTokens   int64
	CacheCreateTokens int64
	StartTime         time.Time
	LastTime          time.Time

	// Sessions counts how many session files contributed to this Stats.
	// 0 for a single Parse(); positive only when ParseProject filled it.
	Sessions int
}

func (s Stats) CacheHitPct() float64 {
	total := s.CacheReadTokens + s.InputTokens
	if total == 0 {
		return 0
	}
	return float64(s.CacheReadTokens) / float64(total) * 100
}

func (s Stats) Add(o Stats) Stats {
	s.InputTokens += o.InputTokens
	s.OutputTokens += o.OutputTokens
	s.CacheReadTokens += o.CacheReadTokens
	s.CacheCreateTokens += o.CacheCreateTokens
	s.Prompts += o.Prompts
	s.ToolCalls += o.ToolCalls
	return s
}

// ActiveFile returns the most recently modified JSONL file for the given cwd.
func ActiveFile(cwd string) string {
	slug := strings.ReplaceAll(cwd, "/", "-")
	dir := filepath.Join(os.Getenv("HOME"), ".claude", "projects", slug)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}

	var latest string
	var latestMod time.Time
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		info, _ := e.Info()
		if info != nil && info.ModTime().After(latestMod) {
			latestMod = info.ModTime()
			latest = filepath.Join(dir, e.Name())
		}
	}
	return latest
}

// ProjectFiles returns all JSONL session files for the given cwd's project.
func ProjectFiles(cwd string) []string {
	slug := strings.ReplaceAll(cwd, "/", "-")
	dir := filepath.Join(os.Getenv("HOME"), ".claude", "projects", slug)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".jsonl") {
			files = append(files, filepath.Join(dir, e.Name()))
		}
	}
	return files
}

type record struct {
	Type      string          `json:"type"`
	Message   json.RawMessage `json:"message"`
	Timestamp string          `json:"timestamp"`
}

type assistantMsg struct {
	Model string `json:"model"`
	Usage struct {
		InputTokens              int64 `json:"input_tokens"`
		OutputTokens             int64 `json:"output_tokens"`
		CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
		CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
	} `json:"usage"`
	Content []struct {
		Type string `json:"type"`
	} `json:"content"`
}

// userContentItem matches one element of a user message's content
// array; we only care whether it's a tool_result block.
type userContentItem struct {
	Type string `json:"type"`
}

// Parse reads a single session JSONL and returns aggregated stats.
func Parse(path string) (Stats, error) {
	f, err := os.Open(path)
	if err != nil {
		return Stats{}, err
	}
	defer f.Close()
	s, err := ParseReader(f)
	s.SessionID = strings.TrimSuffix(filepath.Base(path), ".jsonl")
	return s, err
}

// ParseBytes parses session stats from raw JSONL bytes.
func ParseBytes(data []byte) (Stats, error) {
	return ParseReader(bytes.NewReader(data))
}

// ParseReader parses session stats from a reader.
func ParseReader(r io.Reader) (Stats, error) {
	var s Stats

	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 2*1024*1024), 2*1024*1024)

	for scanner.Scan() {
		var r record
		if err := json.Unmarshal(scanner.Bytes(), &r); err != nil {
			continue
		}

		if r.Timestamp != "" {
			t, err := time.Parse(time.RFC3339, r.Timestamp)
			if err == nil {
				if s.StartTime.IsZero() || t.Before(s.StartTime) {
					s.StartTime = t
				}
				if t.After(s.LastTime) {
					s.LastTime = t
				}
			}
		}

		switch r.Type {
		case "user":
			// Prompts only count when the message is a real user
			// message (string content, or array content without any
			// tool_result blocks). Tool-result roundtrips are also
			// emitted as `user` records but should not inflate the
			// prompt count.
			if !hasToolResult(r.Message) {
				s.Prompts++
			}
		case "assistant":
			var msg assistantMsg
			if err := json.Unmarshal(r.Message, &msg); err != nil {
				continue
			}
			if s.Model == "" && msg.Model != "" {
				s.Model = msg.Model
			}
			s.InputTokens += msg.Usage.InputTokens
			s.OutputTokens += msg.Usage.OutputTokens
			s.CacheReadTokens += msg.Usage.CacheReadInputTokens
			s.CacheCreateTokens += msg.Usage.CacheCreationInputTokens
			for _, c := range msg.Content {
				if c.Type == "tool_use" {
					s.ToolCalls++
				}
			}
		}
	}

	return s, scanner.Err()
}

// hasToolResult returns true if the user message's `content` field is
// an array containing at least one tool_result block. Falls back to
// false when content is a plain string (a real text prompt).
func hasToolResult(rawMsg json.RawMessage) bool {
	var wrapper struct {
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(rawMsg, &wrapper); err != nil {
		return false
	}
	var items []userContentItem
	if err := json.Unmarshal(wrapper.Content, &items); err != nil {
		return false // content was a string
	}
	for _, it := range items {
		if it.Type == "tool_result" {
			return true
		}
	}
	return false
}

// ParseProject aggregates stats across all sessions for the given
// cwd's project, recording how many sessions contributed.
func ParseProject(cwd string) Stats {
	var total Stats
	for _, f := range ProjectFiles(cwd) {
		s, err := Parse(f)
		if err == nil {
			total = total.Add(s)
			total.Sessions++
		}
	}
	return total
}
