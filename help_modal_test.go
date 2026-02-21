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
		next, _ := m.handleHelpKey(tea.KeyMsg{Type: tea.KeyDown})
		m = next.(model)
	}
	maxOffset := m.helpMaxScroll()
	if m.helpScroll != maxOffset {
		t.Fatalf("expected helpScroll to clamp at %d, got %d", maxOffset, m.helpScroll)
	}

	next, _ := m.handleHelpKey(tea.KeyMsg{Type: tea.KeyUp})
	m = next.(model)
	if m.helpScroll != maxOffset-1 {
		t.Fatalf("expected helpScroll to decrease to %d, got %d", maxOffset-1, m.helpScroll)
	}
}

func TestHandleHelpKeyRunesUpdateQuery(t *testing.T) {
	t.Parallel()

	m := model{
		height:     12,
		mode:       ModeHelp,
		keymap:     defaultKeymap(),
		helpScroll: 3,
	}

	next, _ := m.handleHelpKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = next.(model)

	if m.helpQuery != "j" {
		t.Fatalf("expected helpQuery to be 'j', got %q", m.helpQuery)
	}
	if m.helpScroll != 0 {
		t.Fatalf("expected helpScroll to reset to 0, got %d", m.helpScroll)
	}
}

func TestHelpContentLinesFiltersByActionValue(t *testing.T) {
	t.Parallel()

	m := model{
		keymap:    defaultKeymap(),
		helpQuery: "help",
	}

	lines := m.helpContentLines()
	all := strings.Join(lines, "\n")
	if !strings.Contains(all, "?: help") {
		t.Fatalf("expected help line to stay when filtering by action value")
	}

	m.helpQuery = "?"
	lines = m.helpContentLines()
	all = strings.Join(lines, "\n")
	if strings.Contains(all, "?: help") {
		t.Fatalf("expected key glyph query not to match by key side")
	}
	if !strings.Contains(all, "No matches") {
		t.Fatalf("expected 'No matches' when nothing is found")
	}
}

func TestHandleBoardKeyOpenHelpResetsQuery(t *testing.T) {
	t.Parallel()

	m := model{
		mode:      ModeBoard,
		helpQuery: "old",
	}

	next, _ := m.handleBoardKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	m = next.(model)
	if m.mode != ModeHelp {
		t.Fatalf("expected mode help, got %s", m.mode)
	}
	if m.helpQuery != "" {
		t.Fatalf("expected helpQuery to reset, got %q", m.helpQuery)
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
