package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	dir := "."
	if len(os.Args) > 1 {
		dir = os.Args[1]
	} else if cwd, err := os.Getwd(); err == nil {
		dir = cwd
	}

	p := tea.NewProgram(initialModel(dir), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
