package git

import (
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

type FileStatus struct {
	X, Y     byte   // raw XY codes from git status --porcelain
	Path     string // repo-relative path
	OldPath  string // set on renames
	Added    int    // lines added (from diff --numstat)
	Deleted  int    // lines deleted
}

func (f FileStatus) Symbol() string {
	xy := string([]byte{f.X, f.Y})
	switch {
	case xy == "??":
		return "?"
	case f.X == 'D' || f.Y == 'D':
		return "D"
	case f.X == 'R' || f.Y == 'R':
		return "R"
	case f.X == 'A':
		return "A"
	case f.X == 'U' || f.Y == 'U':
		return "!"
	default:
		return "M"
	}
}

func (f FileStatus) IsUntracked() bool { return f.X == '?' && f.Y == '?' }
func (f FileStatus) IsStaged() bool    { return f.X != ' ' && f.X != '?' }

// Root returns the git root for the given directory, or empty string.
func Root(dir string) string {
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// Branch returns the current branch name.
func Branch(root string) string {
	out, err := exec.Command("git", "-C", root, "symbolic-ref", "--short", "HEAD").Output()
	if err != nil {
		out, err = exec.Command("git", "-C", root, "rev-parse", "--short", "HEAD").Output()
		if err != nil {
			return "unknown"
		}
	}
	return strings.TrimSpace(string(out))
}

// Status returns changed files with diff line counts.
func Status(root string) ([]FileStatus, error) {
	out, err := exec.Command("git", "-C", root, "status", "--porcelain").Output()
	if err != nil {
		return nil, err
	}
	files := ParseStatusOutput(string(out))
	for i, f := range files {
		if !f.IsUntracked() {
			files[i].Added, files[i].Deleted = numstat(root, f.Path)
		}
	}
	return files, nil
}

// ParseStatusOutput parses `git status --porcelain` output into FileStatus
// entries without performing any additional git commands. Added/Deleted are
// always 0 — callers that want line counts must fill them in separately.
func ParseStatusOutput(out string) []FileStatus {
	var files []FileStatus
	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if len(line) < 4 {
			continue
		}
		x, y := line[0], line[1]
		path := line[3:]

		if x == '!' && y == '!' {
			continue
		}

		f := FileStatus{X: x, Y: y}
		if strings.Contains(path, " -> ") {
			parts := strings.SplitN(path, " -> ", 2)
			f.OldPath = parts[0]
			f.Path = parts[1]
		} else {
			f.Path = path
		}
		files = append(files, f)
	}
	return files
}

func numstat(root, path string) (int, int) {
	// Try diff against HEAD first, then --cached for new staged files
	for _, args := range [][]string{
		{"diff", "--numstat", "HEAD", "--", path},
		{"diff", "--numstat", "--cached", "--", path},
	} {
		cmd := append([]string{"-C", root, "git"}, args...)
		// build properly
		out, err := exec.Command("git", append([]string{"-C", root}, args...)...).Output()
		_ = cmd
		if err != nil || len(out) == 0 {
			continue
		}
		parts := strings.Fields(string(out))
		if len(parts) < 2 {
			continue
		}
		a, _ := strconv.Atoi(parts[0])
		d, _ := strconv.Atoi(parts[1])
		if a > 0 || d > 0 {
			return a, d
		}
	}
	return 0, 0
}

// AllFiles returns all tracked and untracked non-ignored files as repo-relative paths.
func AllFiles(root string) ([]string, error) {
	out1, err := exec.Command("git", "-C", root, "ls-files").Output()
	if err != nil {
		return nil, err
	}
	out2, _ := exec.Command("git", "-C", root, "ls-files", "--others", "--exclude-standard").Output()
	return ParseFilesOutput(string(out1), string(out2)), nil
}

// ParseFilesOutput merges the outputs of `git ls-files` and
// `git ls-files --others --exclude-standard`, deduplicating entries.
func ParseFilesOutput(tracked, untracked string) []string {
	seen := make(map[string]bool)
	var files []string
	combined := strings.TrimRight(tracked+untracked, "\n")
	for _, line := range strings.Split(combined, "\n") {
		if line != "" && !seen[line] {
			seen[line] = true
			files = append(files, line)
		}
	}
	return files
}

// Diff returns the unified diff for a single repo-relative path,
// covering both staged and unstaged changes against HEAD. For untracked
// files it returns the file's contents instead — there is no meaningful
// diff against history.
func Diff(root, path string, untracked bool) (string, error) {
	if untracked {
		out, err := exec.Command("cat", filepath.Join(root, path)).Output()
		return string(out), err
	}
	out, err := exec.Command("git", "-C", root,
		"diff", "HEAD", "--no-color", "--", path).Output()
	if err != nil {
		// HEAD may not exist yet (initial commit). Fall back to --cached
		// which still shows the staged contents of newly-added files.
		out, err = exec.Command("git", "-C", root,
			"diff", "--cached", "--no-color", "--", path).Output()
		if err != nil {
			return "", err
		}
	}
	return string(out), nil
}

// ShortPath shortens a repo-relative path for display in a narrow pane.
func ShortPath(path string, maxWidth int) string {
	if len(path) <= maxWidth {
		return path
	}
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	parent := filepath.Base(dir)
	short := "…/" + parent + "/" + base
	if len(short) <= maxWidth {
		return short
	}
	return "…/" + base
}
