package main

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
		uiFocused: false,
	}

	src := lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Render("alert")
	got := m.applyFocusDimming(src)
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
		uiFocused: true,
	}

	src := lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Render("alert")
	got := m.applyFocusDimming(src)
	if got != src {
		t.Fatalf("expected unchanged output, got %q", got)
	}
}
