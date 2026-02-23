package bdtui_test

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestHandleFormKey_CreateEscClosesWhenTitleEmpty(t *testing.T) {
	m := model{
		Mode: ModeCreate,
		Form: newIssueFormCreate(nil),
	}

	next, cmd := m.HandleFormKey(tea.KeyMsg{Type: tea.KeyEsc})
	got := next.(model)

	if got.Mode != ModeBoard {
		t.Fatalf("expected mode board, got %s", got.Mode)
	}
	if got.Form != nil {
		t.Fatalf("expected form to be cleared")
	}
	if cmd != nil {
		t.Fatalf("expected no command on empty-create cancel")
	}
}

func TestHandleFormKey_CreateEscSavesWhenTitlePresent(t *testing.T) {
	form := newIssueFormCreate(nil)
	form.Input.SetValue("new task")

	m := model{
		Mode: ModeCreate,
		Form: form,
	}

	next, cmd := m.HandleFormKey(tea.KeyMsg{Type: tea.KeyEsc})
	got := next.(model)

	if got.Mode != ModeBoard {
		t.Fatalf("expected mode board, got %s", got.Mode)
	}
	if got.Form != nil {
		t.Fatalf("expected form to be cleared")
	}
	if cmd == nil {
		t.Fatalf("expected save command when title is present")
	}
}
