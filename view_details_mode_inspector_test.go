package main

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func TestRenderInspectorDetailsModeHighlightsBorder(t *testing.T) {
	prevProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(prevProfile)

	m := model{
		width:  100,
		height: 30,
		mode:   ModeBoard,
		styles: newStyles(),
	}

	boardInspector := m.renderInspector()
	if !strings.Contains(boardInspector, "38;5;241m") {
		t.Fatalf("expected board mode inspector to use neutral border color, got %q", boardInspector)
	}
	if strings.Contains(boardInspector, "38;5;39m") {
		t.Fatalf("did not expect active border color in board mode, got %q", boardInspector)
	}

	m.mode = ModeDetails
	detailsInspector := m.renderInspector()
	if !strings.Contains(detailsInspector, "38;5;39m") {
		t.Fatalf("expected details mode inspector to use active border color, got %q", detailsInspector)
	}
}
