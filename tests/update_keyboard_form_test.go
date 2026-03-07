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

func TestHandleFormKey_TabOnTitleIsNoOp(t *testing.T) {
	m := model{
		Mode: ModeCreate,
		Form: newIssueFormCreate(nil),
	}
	m.Form.Title = "demo"
	m.Form.Input.SetValue("demo")

	next, cmd := m.HandleFormKey(tea.KeyMsg{Type: tea.KeyTab})
	got := next.(model)

	if got.Form.Cursor != 0 {
		t.Fatalf("expected cursor to stay on title, got %d", got.Form.Cursor)
	}
	if got.Form.Status != StatusOpen {
		t.Fatalf("expected status to stay open, got %q", got.Form.Status)
	}
	if got.Form.Title != "demo" {
		t.Fatalf("expected title to stay unchanged, got %q", got.Form.Title)
	}
	if cmd != nil {
		t.Fatalf("expected no command on title tab")
	}
}

func TestHandleFormKey_ShiftTabOnTitleIsNoOp(t *testing.T) {
	m := model{
		Mode: ModeCreate,
		Form: newIssueFormCreate(nil),
	}
	m.Form.Title = "demo"
	m.Form.Input.SetValue("demo")

	next, cmd := m.HandleFormKey(tea.KeyMsg{Type: tea.KeyShiftTab})
	got := next.(model)

	if got.Form.Cursor != 0 {
		t.Fatalf("expected cursor to stay on title, got %d", got.Form.Cursor)
	}
	if got.Form.Priority != 2 {
		t.Fatalf("expected priority to stay 2, got %d", got.Form.Priority)
	}
	if got.Form.Title != "demo" {
		t.Fatalf("expected title to stay unchanged, got %q", got.Form.Title)
	}
	if cmd != nil {
		t.Fatalf("expected no command on title shift+tab")
	}
}

func TestHandleFormKey_TabCyclesEnumField(t *testing.T) {
	m := model{
		Mode: ModeCreate,
		Form: newIssueFormCreate(nil),
	}
	m.Form.Cursor = 1

	next, cmd := m.HandleFormKey(tea.KeyMsg{Type: tea.KeyTab})
	got := next.(model)

	if got.Form.Cursor != 1 {
		t.Fatalf("expected cursor to stay on status field, got %d", got.Form.Cursor)
	}
	if got.Form.Status != StatusInProgress {
		t.Fatalf("expected status to cycle to in_progress, got %q", got.Form.Status)
	}
	if cmd != nil {
		t.Fatalf("expected no command on enum tab")
	}
}
