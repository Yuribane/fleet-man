package tui

import "github.com/charmbracelet/lipgloss"

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("170")).
			PaddingBottom(1)

	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("39"))

	selectedStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("170"))

	dimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240"))

	statusRunning = lipgloss.NewStyle().
			Foreground(lipgloss.Color("42"))

	statusStopped = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196"))

	statusCreating = lipgloss.NewStyle().
			Foreground(lipgloss.Color("214"))

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			PaddingTop(1)

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Bold(true)
)
