package bdtui_test

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestHandleKeyDeleteConfirmTabTogglesMode(t *testing.T) {
	t.Parallel()

	m := model{
		Mode: ModeConfirmDelete,
		ConfirmDelete: &ConfirmDelete{
			IssueID: "bdtui-56i.33",
			Mode:    DeleteModeForce,
			Preview: "preview",
		},
	}

	next, cmd := m.HandleKey(tea.KeyMsg{Type: tea.KeyTab})
	got := next.(model)

	if got.ConfirmDelete == nil {
		t.Fatalf("expected confirm delete to stay open")
	}
	if got.ConfirmDelete.Mode != DeleteModeCascade {
		t.Fatalf("expected tab to toggle mode to cascade, got %q", got.ConfirmDelete.Mode)
	}
	if got.ConfirmDelete.Selected != 1 {
		t.Fatalf("expected selected index 1, got %d", got.ConfirmDelete.Selected)
	}
	if cmd != nil {
		t.Fatalf("expected no command on tab toggle")
	}
}

func TestHandleKeyDeleteConfirmDigitKeysDoNotSwitchMode(t *testing.T) {
	t.Parallel()

	m := model{
		Mode: ModeConfirmDelete,
		ConfirmDelete: &ConfirmDelete{
			IssueID: "bdtui-56i.33",
			Mode:    DeleteModeForce,
			Preview: "preview",
		},
	}

	next, cmd := m.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("2")})
	got := next.(model)

	if got.ConfirmDelete == nil {
		t.Fatalf("expected confirm delete to stay open")
	}
	if got.ConfirmDelete.Mode != DeleteModeForce {
		t.Fatalf("expected digit key to leave mode unchanged, got %q", got.ConfirmDelete.Mode)
	}
	if got.ConfirmDelete.Selected != 0 {
		t.Fatalf("expected selected index to stay 0, got %d", got.ConfirmDelete.Selected)
	}
	if cmd != nil {
		t.Fatalf("expected no command on digit key")
	}
}
