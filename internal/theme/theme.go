package theme

import "github.com/charmbracelet/lipgloss"

var (
	Modified  = lipgloss.NewStyle().Foreground(lipgloss.Color("214")) // orange
	Staged    = lipgloss.NewStyle().Foreground(lipgloss.Color("78"))  // green
	Untracked = lipgloss.NewStyle().Foreground(lipgloss.Color("203")) // red
	Deleted   = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	Conflict  = lipgloss.NewStyle().Foreground(lipgloss.Color("197")).Bold(true)
	Muted     = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	Bold      = lipgloss.NewStyle().Bold(true)
	Cyan      = lipgloss.NewStyle().Foreground(lipgloss.Color("81"))

	Added   = Staged
	Renamed = lipgloss.NewStyle().Foreground(lipgloss.Color("75")) // blue

	DiffAdd = lipgloss.NewStyle().Foreground(lipgloss.Color("78"))
	DiffDel = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
)
