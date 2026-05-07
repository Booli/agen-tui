package session

import (
	"bytes"
	"encoding/json"
	"time"
)

// maxLineBytes caps the in-progress (un-newlined) leftover buffer. A pathological
// JSONL stream with no newlines must not grow memory unboundedly. 4 MiB matches
// 2× the bulk parser's per-line limit, which is comfortably above any real record.
const maxLineBytes = 4 * 1024 * 1024

// Parser is a stateful incremental JSONL session parser. Feed it chunks of
// bytes via Append; it processes only complete new lines and accumulates
// Stats and tool calls without re-scanning earlier content. Designed for
// tail -F streams: O(new bytes) per Append, not O(total).
type Parser struct {
	Stats Stats

	calls          []ToolCall
	byID           map[string]int
	pendingResults map[string]pendingResult
	leftover       []byte
}

type pendingResult struct {
	isError bool
	text    string
}

func NewParser() *Parser {
	return &Parser{
		byID:           map[string]int{},
		pendingResults: map[string]pendingResult{},
	}
}

// Append feeds raw JSONL bytes to the parser. Any partial trailing line is
// retained until the next Append completes it. If a single line exceeds
// maxLineBytes the buffer is dropped — a real Claude record never approaches
// that, so this only fires on corrupt input.
func (p *Parser) Append(chunk []byte) {
	data := chunk
	if len(p.leftover) > 0 {
		data = append(p.leftover, chunk...)
		p.leftover = p.leftover[:0]
	}
	for {
		i := bytes.IndexByte(data, '\n')
		if i < 0 {
			if len(data) > maxLineBytes {
				return // drop oversize partial line
			}
			p.leftover = append(p.leftover, data...)
			return
		}
		p.processLine(data[:i])
		data = data[i+1:]
	}
}

// RecentTools returns the most recent tool calls. limit <= 0 returns all.
func (p *Parser) RecentTools(limit int) []ToolCall {
	if limit > 0 && len(p.calls) > limit {
		return p.calls[len(p.calls)-limit:]
	}
	return p.calls
}

func (p *Parser) processLine(line []byte) {
	if len(line) == 0 {
		return
	}
	var r record
	if err := json.Unmarshal(line, &r); err != nil {
		return
	}

	if r.Timestamp != "" {
		t, err := time.Parse(time.RFC3339, r.Timestamp)
		if err == nil {
			if p.Stats.StartTime.IsZero() || t.Before(p.Stats.StartTime) {
				p.Stats.StartTime = t
			}
			if t.After(p.Stats.LastTime) {
				p.Stats.LastTime = t
			}
		}
	}

	switch r.Type {
	case "user":
		if !hasToolResult(r.Message) {
			p.Stats.Prompts++
		}
		p.processUserToolResults(r)
	case "assistant":
		p.processAssistant(r)
	}
}

func (p *Parser) processAssistant(r record) {
	var msg assistantMsg
	if err := json.Unmarshal(r.Message, &msg); err != nil {
		return
	}
	if p.Stats.Model == "" && msg.Model != "" {
		p.Stats.Model = msg.Model
	}
	p.Stats.InputTokens += msg.Usage.InputTokens
	p.Stats.OutputTokens += msg.Usage.OutputTokens
	p.Stats.CacheReadTokens += msg.Usage.CacheReadInputTokens
	p.Stats.CacheCreateTokens += msg.Usage.CacheCreationInputTokens
	for _, c := range msg.Content {
		if c.Type == "tool_use" {
			p.Stats.ToolCalls++
		}
	}

	var ac assistantContent
	if err := json.Unmarshal(r.Message, &ac); err != nil {
		return
	}
	ts, _ := time.Parse(time.RFC3339, r.Timestamp)
	for _, b := range ac.Content {
		if b.Type != "tool_use" {
			continue
		}
		idx := len(p.calls)
		p.byID[b.ID] = idx
		added, deleted := lineDelta(b.Name, b.Input)
		p.calls = append(p.calls, ToolCall{
			ID:       b.ID,
			Name:     b.Name,
			Time:     ts,
			Summary:  summarize(b.Name, b.Input),
			Added:    added,
			Deleted:  deleted,
			InputRaw: append(json.RawMessage{}, b.Input...),
		})
		if pr, ok := p.pendingResults[b.ID]; ok {
			p.applyResult(idx, pr)
			delete(p.pendingResults, b.ID)
		}
	}
}

func (p *Parser) processUserToolResults(r record) {
	var uc userContent
	if err := json.Unmarshal(r.Message, &uc); err != nil {
		return
	}
	var blocks []contentBlock
	if err := json.Unmarshal(uc.Content, &blocks); err != nil {
		return
	}
	for _, b := range blocks {
		if b.Type != "tool_result" || b.ToolUseID == "" {
			continue
		}
		pr := pendingResult{isError: b.IsError, text: extractResultText(b.Content)}
		if idx, ok := p.byID[b.ToolUseID]; ok {
			p.applyResult(idx, pr)
			continue
		}
		p.pendingResults[b.ToolUseID] = pr
	}
}

func (p *Parser) applyResult(idx int, pr pendingResult) {
	p.calls[idx].Done = true
	p.calls[idx].Error = pr.isError
	p.calls[idx].ResultText = pr.text
}
