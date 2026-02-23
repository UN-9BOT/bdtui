package bdtui_test

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestHelpMaxScrollIsPositiveOnSmallHeight(t *testing.T) {
	t.Parallel()

	m := model{
		Height: 12,
		Keymap: defaultKeymap(),
	}

	if got := m.HelpMaxScroll(); got <= 0 {
		t.Fatalf("expected helpMaxScroll > 0, got %d", got)
	}
}

func TestHandleHelpKeyScrollIsClamped(t *testing.T) {
	t.Parallel()

	m := model{
		Height: 12,
		Mode:   ModeHelp,
		Keymap: defaultKeymap(),
	}

	for i := 0; i < 100; i++ {
		next, _ := m.HandleHelpKey(tea.KeyMsg{Type: tea.KeyDown})
		m = next.(model)
	}
	maxOffset := m.HelpMaxScroll()
	if m.HelpScroll != maxOffset {
		t.Fatalf("expected helpScroll to clamp at %d, got %d", maxOffset, m.HelpScroll)
	}

	next, _ := m.HandleHelpKey(tea.KeyMsg{Type: tea.KeyUp})
	m = next.(model)
	if m.HelpScroll != maxOffset-1 {
		t.Fatalf("expected helpScroll to decrease to %d, got %d", maxOffset-1, m.HelpScroll)
	}
}

func TestHandleHelpKeyRunesUpdateQuery(t *testing.T) {
	t.Parallel()

	m := model{
		Height:     12,
		Mode:       ModeHelp,
		Keymap:     defaultKeymap(),
		HelpScroll: 3,
	}

	next, _ := m.HandleHelpKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = next.(model)

	if m.HelpQuery != "j" {
		t.Fatalf("expected helpQuery to be 'j', got %q", m.HelpQuery)
	}
	if m.HelpScroll != 0 {
		t.Fatalf("expected helpScroll to reset to 0, got %d", m.HelpScroll)
	}
}

func TestHelpContentLinesFiltersByActionValue(t *testing.T) {
	t.Parallel()

	m := model{
		Keymap:    defaultKeymap(),
		HelpQuery: "help",
	}

	lines := m.HelpContentLines()
	all := strings.Join(lines, "\n")
	if !strings.Contains(all, "?: help") {
		t.Fatalf("expected help line to stay when filtering by action value")
	}

	m.HelpQuery = "?"
	lines = m.HelpContentLines()
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
		Mode:      ModeBoard,
		HelpQuery: "old",
	}

	next, _ := m.HandleBoardKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	m = next.(model)
	if m.Mode != ModeHelp {
		t.Fatalf("expected mode help, got %s", m.Mode)
	}
	if m.HelpQuery != "" {
		t.Fatalf("expected helpQuery to reset, got %q", m.HelpQuery)
	}
}

func TestRenderHelpModalFitsViewport(t *testing.T) {
	t.Parallel()

	m := model{
		Height: 12,
		Mode:   ModeHelp,
		Keymap: defaultKeymap(),
	}

	out := m.RenderHelpModal()
	lines := strings.Split(out, "\n")
	expectedMax := m.HelpViewportContentLines() + 2 // spacer + footer
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
		Height: 12,
		Mode:   ModeHelp,
		Keymap: defaultKeymap(),
	}
	maxOffset := m.HelpMaxScroll()
	if maxOffset <= 1 {
		t.Fatalf("expected scrollable help, got maxOffset=%d", maxOffset)
	}

	m.HelpScroll = 0
	w0 := maxLineWidth(m.RenderHelpModal())

	m.HelpScroll = maxOffset / 2
	w1 := maxLineWidth(m.RenderHelpModal())

	m.HelpScroll = maxOffset
	w2 := maxLineWidth(m.RenderHelpModal())

	if w0 != w1 || w1 != w2 {
		t.Fatalf("expected stable widths, got w0=%d w1=%d w2=%d", w0, w1, w2)
	}
}

func TestRenderHelpModalShowsFilterBoxBeforeHotkeys(t *testing.T) {
	t.Parallel()

	m := model{
		Height: 18,
		Mode:   ModeHelp,
		Keymap: defaultKeymap(),
	}

	out := m.RenderHelpModal()
	filterPos := strings.Index(out, "Filter")
	hotkeysPos := strings.Index(out, "Hotkeys")
	if filterPos == -1 {
		t.Fatalf("expected filter box header in help modal: %q", out)
	}
	if hotkeysPos == -1 {
		t.Fatalf("expected hotkeys header in help modal: %q", out)
	}
	if filterPos >= hotkeysPos {
		t.Fatalf("expected filter box before Hotkeys, got filterPos=%d hotkeysPos=%d", filterPos, hotkeysPos)
	}
}

func TestRenderHelpModalShowsFilterCursor(t *testing.T) {
	t.Parallel()

	m := model{
		Height:    18,
		Mode:      ModeHelp,
		Keymap:    defaultKeymap(),
		HelpQuery: "he",
	}

	out := m.RenderHelpModal()
	if !strings.Contains(out, "he▏") {
		t.Fatalf("expected help filter cursor in modal, got %q", out)
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
