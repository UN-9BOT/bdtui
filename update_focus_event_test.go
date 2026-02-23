package main

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestUpdateBlurAndFocusToggleUIFocusState(t *testing.T) {
	t.Parallel()

	m := model{uiFocused: true}

	next, cmd := m.Update(tea.BlurMsg{})
	got := next.(model)
	if got.uiFocused {
		t.Fatalf("expected uiFocused=false after blur")
	}
	if cmd != nil {
		t.Fatalf("expected nil cmd on blur")
	}

	next, cmd = got.Update(tea.FocusMsg{})
	got = next.(model)
	if !got.uiFocused {
		t.Fatalf("expected uiFocused=true after focus")
	}
	if cmd != nil {
		t.Fatalf("expected nil cmd on focus")
	}
}
