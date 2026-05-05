package session

// FilterMode selects which tool calls are visible in the tools view.
type FilterMode int

const (
	// FilterInteresting hides Bash and shows file ops, sub-agents,
	// and web tools. Default.
	FilterInteresting FilterMode = iota
	// FilterEdits shows only file-mutating tools.
	FilterEdits
	// FilterAll shows every tool call.
	FilterAll
)

// Name returns a short label for the filter, used in footer hints.
func (f FilterMode) Name() string {
	switch f {
	case FilterInteresting:
		return "files"
	case FilterEdits:
		return "edits"
	case FilterAll:
		return "all"
	}
	return "?"
}

// Keep decides whether a tool call passes the filter. Errored calls
// always pass through — failures are interesting regardless of tool
// name.
func (f FilterMode) Keep(t ToolCall) bool {
	if t.Error {
		return true
	}
	switch f {
	case FilterAll:
		return true
	case FilterEdits:
		switch t.Name {
		case "Write", "Edit", "MultiEdit", "NotebookEdit":
			return true
		}
		return false
	case FilterInteresting:
		switch t.Name {
		case "Write", "Edit", "MultiEdit", "NotebookEdit",
			"Read", "Grep", "Glob",
			"Agent", "Skill",
			"WebFetch", "WebSearch":
			return true
		}
		return false
	}
	return true
}

// FilterCount returns how many filter modes there are. Useful for
// `(f + 1) % FilterCount` cycling.
const FilterCount = 3
