package ui

import "github.com/charmbracelet/lipgloss"

var (
	Title = lipgloss.NewStyle().Bold(true)
	Muted = lipgloss.NewStyle().Faint(true)
	Error = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
)
