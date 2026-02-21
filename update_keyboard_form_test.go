package main

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestHandleFormKey_CreateEscClosesWhenTitleEmpty(t *testing.T) {
	m := model{
		mode: ModeCreate,
		form: newIssueFormCreate(nil),
	}

	next, cmd := m.handleFormKey(tea.KeyMsg{Type: tea.KeyEsc})
	got := next.(model)

	if got.mode != ModeBoard {
		t.Fatalf("expected mode board, got %s", got.mode)
	}
	if got.form != nil {
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
		mode: ModeCreate,
		form: form,
	}

	next, cmd := m.handleFormKey(tea.KeyMsg{Type: tea.KeyEsc})
	got := next.(model)

	if got.mode != ModeBoard {
		t.Fatalf("expected mode board, got %s", got.mode)
	}
	if got.form != nil {
		t.Fatalf("expected form to be cleared")
	}
	if cmd == nil {
		t.Fatalf("expected save command when title is present")
	}
}
