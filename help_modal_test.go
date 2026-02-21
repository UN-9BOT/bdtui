package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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

func TestRenderHelpModalWidthStableAcrossScroll(t *testing.T) {
	t.Parallel()

	m := model{
		height: 12,
		mode:   ModeHelp,
		keymap: defaultKeymap(),
	}
	maxOffset := m.helpMaxScroll()
	if maxOffset <= 1 {
		t.Fatalf("expected scrollable help, got maxOffset=%d", maxOffset)
	}

	m.helpScroll = 0
	w0 := maxLineWidth(m.renderHelpModal())

	m.helpScroll = maxOffset / 2
	w1 := maxLineWidth(m.renderHelpModal())

	m.helpScroll = maxOffset
	w2 := maxLineWidth(m.renderHelpModal())

	if w0 != w1 || w1 != w2 {
		t.Fatalf("expected stable widths, got w0=%d w1=%d w2=%d", w0, w1, w2)
	}
}

func maxLineWidth(s string) int {
	maxW := 0
	for _, line := range strings.Split(s, "\n") {
		w := lipgloss.Width(line)
		if w > maxW {
			maxW = w
		}
	}
	return maxW
}
