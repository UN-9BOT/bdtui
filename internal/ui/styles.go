package ui

import "github.com/charmbracelet/lipgloss"

type Styles struct {
	App      lipgloss.Style
	Title    lipgloss.Style
	Footer   lipgloss.Style
	Border   lipgloss.Style
	Active   lipgloss.Style
	Selected lipgloss.Style
	Dim      lipgloss.Style
	Error    lipgloss.Style
	Warning  lipgloss.Style
	Success  lipgloss.Style
	HelpBox  lipgloss.Style
}

func NewStyles() Styles {
	return Styles{
		App: lipgloss.NewStyle().Padding(0, 1),
		Title: lipgloss.NewStyle().
			Foreground(lipgloss.Color("229")).
			Background(lipgloss.Color("24")).
			Bold(true).
			Padding(0, 1),
		Footer: lipgloss.NewStyle().
			Foreground(lipgloss.Color("250")).
			Background(lipgloss.Color("236")).
			Padding(0, 1),
		Border: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("241")),
		Active: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("39")),
		Selected: lipgloss.NewStyle().
			Foreground(lipgloss.Color("230")).
			Background(lipgloss.Color("31")).
			Bold(true),
		Dim:     lipgloss.NewStyle().Foreground(lipgloss.Color("244")),
		Error:   lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Bold(true),
		Warning: lipgloss.NewStyle().Foreground(lipgloss.Color("221")).Bold(true),
		Success: lipgloss.NewStyle().Foreground(lipgloss.Color("84")).Bold(true),
		HelpBox: lipgloss.NewStyle().
			Border(lipgloss.DoubleBorder()).
			BorderForeground(lipgloss.Color("63")).
			Padding(1, 2),
	}
}
