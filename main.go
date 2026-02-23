package bdtui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

func Run(args []string) error {
	cfg, err := parseConfig(args)
	if err != nil {
		return fmt.Errorf("config error: %w", err)
	}

	m, err := newModel(cfg)
	if err != nil {
		return fmt.Errorf("init error: %w", err)
	}

	p := tea.NewProgram(
		m,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
		tea.WithReportFocus(),
	)

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("runtime error: %w", err)
	}
	return nil
}
