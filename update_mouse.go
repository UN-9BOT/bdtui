package main

import tea "github.com/charmbracelet/bubbletea"

func (m model) handleMouse(_ tea.MouseMsg) (tea.Model, tea.Cmd) {
	return m, nil
}
