package main

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pimrutgers/agen-tui/internal/session"
	"github.com/pimrutgers/agen-tui/internal/theme"
	"github.com/pimrutgers/agen-tui/internal/ui"
)

// renderDetail returns the body lines for the detail pane of a single
// tool call. Each tool gets a tailored layout — diffs for edits, file
// previews for writes/reads, command + output for bash, etc.
func renderDetail(t session.ToolCall, width int, label func(string) string) []string {
	colW := width - 2
	if colW < 10 {
		colW = 10
	}

	var input map[string]json.RawMessage
	_ = json.Unmarshal(t.InputRaw, &input)
	str := func(k string) string {
		var s string
		_ = json.Unmarshal(input[k], &s)
		return s
	}

	switch t.Name {
	case "Edit":
		return detailEdit(str("file_path"), str("old_string"), str("new_string"), str("replace_all"), t, colW, label)
	case "MultiEdit":
		return detailMultiEdit(str("file_path"), input["edits"], t, colW, label)
	case "Write":
		return detailWrite(str("file_path"), str("content"), t, colW, label)
	case "Read":
		return detailReadLike(str("file_path"), t, colW, label)
	case "Bash":
		return detailBash(str("command"), str("description"), t, colW, label)
	case "Grep":
		return detailGrep(input, t, colW, label)
	case "Glob":
		return detailKeyResult([]kv{{"pattern", str("pattern")}, {"path", str("path")}}, t, colW, label)
	case "Agent":
		return detailAgent(str("description"), str("subagent_type"), str("prompt"), t, colW, label)
	case "WebFetch":
		return detailKeyResult([]kv{{"url", str("url")}, {"prompt", str("prompt")}}, t, colW, label)
	case "WebSearch":
		return detailKeyResult([]kv{{"query", str("query")}}, t, colW, label)
	case "Skill":
		return detailKeyResult([]kv{{"skill", str("skill")}, {"args", str("args")}}, t, colW, label)
	case "TodoWrite", "TaskCreate":
		return detailTodos(input["todos"], t, colW, label)
	}
	return detailGeneric(input, t, colW, label)
}

// ── per-tool renderers ───────────────────────────────────────────────────────

func detailEdit(path, old, neu, replaceAll string, t session.ToolCall, w int, label func(string) string) []string {
	out := []string{" " + theme.Muted.Render("file") + "  " + filepath.Base(path)}
	if replaceAll == "true" {
		out = append(out, " "+theme.Muted.Render("(replace_all)"))
	}
	out = append(out, label("diff"))
	out = append(out, ui.RenderDiff(old, neu, path, w)...)
	return appendResult(out, t, w, label)
}

func detailMultiEdit(path string, editsRaw json.RawMessage, t session.ToolCall, w int, label func(string) string) []string {
	out := []string{" " + theme.Muted.Render("file") + "  " + filepath.Base(path)}
	var edits []struct {
		Old        string `json:"old_string"`
		New        string `json:"new_string"`
		ReplaceAll bool   `json:"replace_all"`
	}
	_ = json.Unmarshal(editsRaw, &edits)
	for i, e := range edits {
		out = append(out, label("edit "+ui.Itoa(i+1)+"/"+ui.Itoa(len(edits))))
		out = append(out, ui.RenderDiff(e.Old, e.New, path, w)...)
	}
	return appendResult(out, t, w, label)
}

func detailWrite(path, content string, t session.ToolCall, w int, label func(string) string) []string {
	lines := strings.Split(content, "\n")
	out := []string{
		" " + theme.Muted.Render("file") + "  " + filepath.Base(path),
		" " + theme.Muted.Render("size") + "  " + ui.Itoa(len(lines)) + " lines",
		label("content"),
	}
	highlighted := ui.Highlight(content, path)
	hlLines := strings.Split(highlighted, "\n")
	if len(hlLines) != len(lines) {
		hlLines = lines // fallback if chroma split changed line count
	}
	out = append(out, ui.NumberedHead(hlLines, 20, w-5)...)
	return appendResult(out, t, w, label)
}

func detailReadLike(path string, t session.ToolCall, w int, label func(string) string) []string {
	out := []string{" " + theme.Muted.Render("file") + "  " + filepath.Base(path)}
	return appendResult(out, t, w, label)
}

func detailBash(cmd, desc string, t session.ToolCall, w int, label func(string) string) []string {
	out := []string{}
	if desc != "" {
		out = append(out, " "+theme.Muted.Render("desc")+"  "+desc)
	}
	out = append(out, label("command"))
	for _, line := range ui.WrapLines(cmd, w-1) {
		out = append(out, " "+line)
	}
	return appendResult(out, t, w, label)
}

func detailGrep(input map[string]json.RawMessage, t session.ToolCall, w int, label func(string) string) []string {
	get := func(k string) string {
		var s string
		_ = json.Unmarshal(input[k], &s)
		return s
	}
	pairs := []kv{
		{"pattern", get("pattern")},
		{"path", get("path")},
		{"glob", get("glob")},
		{"type", get("type")},
		{"output_mode", get("output_mode")},
	}
	return detailKeyResult(pairs, t, w, label)
}

func detailAgent(desc, subagent, prompt string, t session.ToolCall, w int, label func(string) string) []string {
	out := []string{
		" " + theme.Muted.Render("agent") + "  " + subagent,
		" " + theme.Muted.Render("desc")  + "  " + desc,
		label("prompt"),
	}
	for _, line := range ui.NumberedHead(strings.Split(prompt, "\n"), 8, w-5) {
		out = append(out, line)
	}
	return appendResult(out, t, w, label)
}

func detailTodos(todosRaw json.RawMessage, t session.ToolCall, w int, label func(string) string) []string {
	var todos []struct {
		Content    string `json:"content"`
		Status     string `json:"status"`
		ActiveForm string `json:"activeForm"`
	}
	_ = json.Unmarshal(todosRaw, &todos)
	out := []string{label("todos")}
	for _, td := range todos {
		var marker string
		switch td.Status {
		case "completed":
			marker = theme.Staged.Render("✓")
		case "in_progress":
			marker = theme.Modified.Render("●")
		default:
			marker = theme.Muted.Render("○")
		}
		out = append(out, " "+marker+" "+ui.TruncRunes(td.Content, w-3))
	}
	return appendResult(out, t, w, label)
}

func detailGeneric(input map[string]json.RawMessage, t session.ToolCall, w int, label func(string) string) []string {
	out := []string{label("input")}
	out = append(out, renderInputKV(t.InputRaw, w)...)
	return appendResult(out, t, w, label)
}

type kv struct{ k, v string }

func detailKeyResult(pairs []kv, t session.ToolCall, w int, label func(string) string) []string {
	var out []string
	for _, p := range pairs {
		if p.v == "" {
			continue
		}
		out = append(out, " "+theme.Muted.Render(p.k)+"  "+ui.TruncRunes(p.v, w-len(p.k)-3))
	}
	return appendResult(out, t, w, label)
}

// ── shared helpers ───────────────────────────────────────────────────────────

// renderInputKV pretty-prints the tool input as `key: value` lines.
// Short scalar values stay on the same line as the key; longer or
// multi-line values move to indented lines underneath.
func renderInputKV(raw json.RawMessage, width int) []string {
	if len(raw) == 0 {
		return []string{" " + theme.Muted.Render("(no input)")}
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		var out []string
		for _, line := range ui.WrapLines(prettyJSON(raw), width-1) {
			out = append(out, " "+line)
		}
		return out
	}

	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var out []string
	for _, k := range keys {
		out = append(out, renderField(k, fields[k], width)...)
	}
	if len(out) == 0 {
		return []string{" " + theme.Muted.Render("(no input)")}
	}
	return out
}

func renderField(key string, raw json.RawMessage, width int) []string {
	keyStyled := theme.Muted.Render(key)
	indent := "   "

	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		if !strings.Contains(s, "\n") && len(key)+2+len(s) <= width-1 {
			return []string{" " + keyStyled + "  " + s}
		}
		out := []string{" " + keyStyled}
		for _, line := range ui.WrapLines(s, width-len(indent)-1) {
			out = append(out, indent+line)
		}
		return out
	}

	tok := strings.TrimSpace(string(raw))
	if !strings.HasPrefix(tok, "{") && !strings.HasPrefix(tok, "[") {
		if len(key)+2+len(tok) <= width-1 {
			return []string{" " + keyStyled + "  " + tok}
		}
	}

	out := []string{" " + keyStyled}
	for _, line := range ui.WrapLines(prettyJSON(raw), width-len(indent)-1) {
		out = append(out, indent+line)
	}
	return out
}

func prettyJSON(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var buf bytes.Buffer
	if err := json.Indent(&buf, raw, "", "  "); err != nil {
		return string(raw)
	}
	return buf.String()
}

func appendResult(body []string, t session.ToolCall, w int, label func(string) string) []string {
	switch {
	case t.ResultText != "":
		body = append(body, label("result"))
		body = append(body, ui.HeadTailLines(t.ResultText, 30, 5, w-1)...)
	case t.Done:
		body = append(body, label("result"), " "+theme.Muted.Render("(empty)"))
	default:
		body = append(body, label("result"), " "+theme.Muted.Render("(in flight)"))
	}
	return body
}

