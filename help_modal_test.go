package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestHelpMaxScrollIsPositiveOnSmallHeight(t *testing.T) {
	t.Parallel()

	m := model{
		height: 12,
		keymap: defaultKeymap(),
	}

	if got := m.helpMaxScroll(); got <= 0 {
		t.Fatalf("expected helpMaxScroll > 0, got %d", got)
	}
}

func TestHandleHelpKeyScrollIsClamped(t *testing.T) {
	t.Parallel()

	m := model{
		height: 12,
		mode:   ModeHelp,
		keymap: defaultKeymap(),
	}

	for i := 0; i < 100; i++ {
		next, _ := m.handleHelpKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
		m = next.(model)
	}
	maxOffset := m.helpMaxScroll()
	if m.helpScroll != maxOffset {
		t.Fatalf("expected helpScroll to clamp at %d, got %d", maxOffset, m.helpScroll)
	}

	next, _ := m.handleHelpKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	m = next.(model)
	if m.helpScroll != maxOffset-1 {
		t.Fatalf("expected helpScroll to decrease to %d, got %d", maxOffset-1, m.helpScroll)
	}
}

func TestRenderHelpModalFitsViewport(t *testing.T) {
	t.Parallel()

	m := model{
		height: 12,
		mode:   ModeHelp,
		keymap: defaultKeymap(),
	}

	out := m.renderHelpModal()
	lines := strings.Split(out, "\n")
	expectedMax := m.helpViewportContentLines() + 2 // spacer + footer
	if len(lines) > expectedMax {
		t.Fatalf("expected at most %d lines, got %d", expectedMax, len(lines))
	}
	if !strings.Contains(out, "scroll") {
		t.Fatalf("expected help modal to show scroll hint")
	}
}
