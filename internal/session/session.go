package session

import (
	"bufio"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Stats struct {
	SessionID         string
	Model             string
	Turns             int
	InputTokens       int64
	OutputTokens      int64
	CacheReadTokens   int64
	CacheCreateTokens int64
	StartTime         time.Time
	LastTime          time.Time
}

func (s Stats) CacheHitPct() float64 {
	total := s.CacheReadTokens + s.InputTokens
	if total == 0 {
		return 0
	}
	return float64(s.CacheReadTokens) / float64(total) * 100
}

func (s Stats) CostUSD() float64 {
	var inR, outR, readR, createR float64
	switch {
	case strings.Contains(s.Model, "opus"):
		inR, outR, readR, createR = 15, 75, 1.5, 18.75
	case strings.Contains(s.Model, "haiku"):
		inR, outR, readR, createR = 0.8, 4, 0.08, 1.0
	default: // sonnet
		inR, outR, readR, createR = 3, 15, 0.3, 3.75
	}
	cost := float64(s.InputTokens)/1e6*inR +
		float64(s.OutputTokens)/1e6*outR +
		float64(s.CacheReadTokens)/1e6*readR +
		float64(s.CacheCreateTokens)/1e6*createR
	return math.Round(cost*100) / 100
}

func (s Stats) Add(o Stats) Stats {
	s.InputTokens += o.InputTokens
	s.OutputTokens += o.OutputTokens
	s.CacheReadTokens += o.CacheReadTokens
	s.CacheCreateTokens += o.CacheCreateTokens
	s.Turns += o.Turns
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
}

// Parse reads a single session JSONL and returns aggregated stats.
func Parse(path string) (Stats, error) {
	f, err := os.Open(path)
	if err != nil {
		return Stats{}, err
	}
	defer f.Close()

	var s Stats
	s.SessionID = strings.TrimSuffix(filepath.Base(path), ".jsonl")

	scanner := bufio.NewScanner(f)
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
			s.Turns++
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
		}
	}

	return s, scanner.Err()
}

// ParseProject aggregates stats across all sessions for the given cwd's project.
func ParseProject(cwd string) Stats {
	var total Stats
	for _, f := range ProjectFiles(cwd) {
		s, err := Parse(f)
		if err == nil {
			total = total.Add(s)
		}
	}
	return total
}
