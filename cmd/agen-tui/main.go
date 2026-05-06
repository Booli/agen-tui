package main

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/pimrutgers/agen-tui/internal/backend"
)

func main() {
	var b backend.Backend
	var dir string

	if len(os.Args) > 1 {
		arg := os.Args[1]
		// scp-style remote: user@host:/path or host:/path
		// Require a colon that is not the first character and not part of a
		// Windows drive letter (not relevant here, but good hygiene).
		if i := strings.Index(arg, ":"); i > 0 {
			host := arg[:i]
			dir = arg[i+1:]
			if dir == "" {
				dir = "."
			}
			b = backend.NewSSHBackend(host)
		} else {
			dir = arg
			b = backend.LocalBackend{}
		}
	} else {
		if cwd, err := os.Getwd(); err == nil {
			dir = cwd
		} else {
			dir = "."
		}
		b = backend.LocalBackend{}
	}

	p := tea.NewProgram(initialModel(dir, b), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
