package bdtui_test

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func TestApplyFocusDimmingDimsWhenPaneNotFocused(t *testing.T) {
	prevProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(prevProfile)

	m := model{
		UIFocused: false,
	}

	src := lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Render("alert")
	got := m.ApplyFocusDimming(src)
	if strings.Contains(got, "38;5;203m") {
		t.Fatalf("expected source color to be removed, got %q", got)
	}
	if !strings.Contains(got, "38;5;241m") {
		t.Fatalf("expected dim color, got %q", got)
	}
}

func TestApplyFocusDimmingSkipsWhenFocused(t *testing.T) {
	prevProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(prevProfile)

	m := model{
		UIFocused: true,
	}

	src := lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Render("alert")
	got := m.ApplyFocusDimming(src)
	if got != src {
		t.Fatalf("expected unchanged output, got %q", got)
	}
}
