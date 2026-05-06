package theme

import "github.com/charmbracelet/lipgloss"

var (
	Modified  = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))  // ANSI yellow (orange in most themes)
	Staged    = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))  // ANSI green
	Untracked = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))  // ANSI red
	Deleted   = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))  // ANSI red
	Conflict  = lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Bold(true)
	Muted     = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))  // ANSI bright-black (dark gray)
	Bold      = lipgloss.NewStyle().Bold(true)
	Cyan      = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))  // ANSI cyan

	Added   = Staged
	Renamed = lipgloss.NewStyle().Foreground(lipgloss.Color("4"))  // ANSI blue

	DiffAdd = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))  // ANSI green
	DiffDel = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))  // ANSI red
)
