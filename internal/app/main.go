package app

import (
	"fmt"

	"bdtui/internal/logger"

	tea "github.com/charmbracelet/bubbletea"
)

func Run(args []string) error {
	if err := logger.Init(); err != nil {
		return fmt.Errorf("logger init: %w", err)
	}
	defer logger.Close()

	cfg, err := parseConfig(args)
	if err != nil {
		logger.Error("config error: %v", err)
		return fmt.Errorf("config error: %w", err)
	}

	m, err := newModel(cfg)
	if err != nil {
		logger.Error("init error: %v", err)
		return fmt.Errorf("init error: %w", err)
	}

	p := tea.NewProgram(
		m,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
		tea.WithReportFocus(),
	)

	if _, err := p.Run(); err != nil {
		logger.Error("runtime error: %v", err)
		return fmt.Errorf("runtime error: %w", err)
	}
	return nil
}
