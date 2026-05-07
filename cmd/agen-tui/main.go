package main

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/pimrutgers/agen-tui/internal/backend"
	"github.com/pimrutgers/agen-tui/internal/tunnel"
)

func main() {
	forwards, rest, err := parseForwards(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	var b backend.Backend
	var dir string
	var host string
	remote := false

	if len(rest) > 0 {
		arg := rest[0]
		if i := strings.Index(arg, ":"); i > 0 {
			host = arg[:i]
			dir = arg[i+1:]
			if dir == "" {
				dir = "."
			}
			b = backend.NewSSHBackend(host)
			remote = true
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

	if len(forwards) > 0 && !remote {
		fmt.Fprintln(os.Stderr, "-L forwards require an SSH target (user@host:/path)")
		os.Exit(2)
	}

	var tunnels []*tunnel.Tunnel
	for _, spec := range forwards {
		tunnels = append(tunnels, tunnel.New(host, spec))
	}
	defer func() {
		for _, t := range tunnels {
			t.Stop()
		}
	}()

	p := tea.NewProgram(initialModel(dir, b, tunnels), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// parseForwards extracts repeated -L flags ("-L 5137" or "-L=5137:host:80")
// from args and returns the parsed specs alongside the remaining
// positional arguments.
func parseForwards(args []string) ([]tunnel.Spec, []string, error) {
	var specs []tunnel.Spec
	var rest []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-L":
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("-L requires an argument")
			}
			s, err := tunnel.ParseSpec(args[i+1])
			if err != nil {
				return nil, nil, err
			}
			specs = append(specs, s)
			i++
		case strings.HasPrefix(a, "-L="):
			s, err := tunnel.ParseSpec(strings.TrimPrefix(a, "-L="))
			if err != nil {
				return nil, nil, err
			}
			specs = append(specs, s)
		default:
			rest = append(rest, a)
		}
	}
	return specs, rest, nil
}
