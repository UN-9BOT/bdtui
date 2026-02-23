package main

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestHandleConfirmClosedParentCreateKeyCancel(t *testing.T) {
	t.Parallel()

	m := model{
		mode: ModeConfirmClosedParentCreate,
		confirmClosedParentCreate: &ConfirmClosedParentCreate{
			ParentID:     "bdtui-56i.21",
			ParentTitle:  "closed parent",
			TargetStatus: StatusInProgress,
		},
	}

	next, cmd := m.handleConfirmClosedParentCreateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	got := next.(model)

	if got.mode != ModeBoard {
		t.Fatalf("expected mode %s, got %s", ModeBoard, got.mode)
	}
	if got.confirmClosedParentCreate != nil {
		t.Fatalf("expected confirm state to be cleared")
	}
	if got.toast != "task creation canceled" {
		t.Fatalf("expected cancel toast, got %q", got.toast)
	}
	if cmd != nil {
		t.Fatalf("expected no cmd on cancel")
	}
}

func TestHandleConfirmClosedParentCreateKeyConfirmReturnsCmd(t *testing.T) {
	t.Parallel()

	m := model{
		mode:   ModeConfirmClosedParentCreate,
		client: NewBdClient("."),
		confirmClosedParentCreate: &ConfirmClosedParentCreate{
			ParentID:     "bdtui-56i.21",
			ParentTitle:  "closed parent",
			TargetStatus: StatusInProgress,
		},
	}

	next, cmd := m.handleConfirmClosedParentCreateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	got := next.(model)

	if got.mode != ModeBoard {
		t.Fatalf("expected mode %s, got %s", ModeBoard, got.mode)
	}
	if got.confirmClosedParentCreate != nil {
		t.Fatalf("expected confirm state to be cleared")
	}
	if cmd == nil {
		t.Fatalf("expected cmd on confirm")
	}
}

func TestUpdateReopenParentForCreateMsgSuccessOpensCreate(t *testing.T) {
	t.Parallel()

	issue := Issue{
		ID:      "bdtui-56i.21",
		Title:   "closed parent",
		Status:  StatusClosed,
		Display: StatusClosed,
	}
	m := model{
		mode:   ModeBoard,
		issues: []Issue{issue},
		byID:   map[string]*Issue{issue.ID: &issue},
		columns: map[Status][]Issue{
			StatusOpen:       {},
			StatusInProgress: {},
			StatusBlocked:    {},
			StatusClosed:     {issue},
		},
		columnDepths: map[Status]map[string]int{
			StatusOpen:       {},
			StatusInProgress: {},
			StatusBlocked:    {},
			StatusClosed:     {},
		},
		selectedIdx: map[Status]int{
			StatusOpen:       0,
			StatusInProgress: 0,
			StatusBlocked:    0,
			StatusClosed:     0,
		},
		scrollOffset: map[Status]int{
			StatusOpen:       0,
			StatusInProgress: 0,
			StatusBlocked:    0,
			StatusClosed:     0,
		},
		filter: Filter{Status: "any", Priority: "any"},
		client: NewBdClient("."),
	}

	nextModel, cmd := m.Update(reopenParentForCreateMsg{parentID: issue.ID})
	got := nextModel.(model)

	if got.mode != ModeCreate {
		t.Fatalf("expected mode %s, got %s", ModeCreate, got.mode)
	}
	if got.form == nil {
		t.Fatalf("expected create form")
	}
	if got.form.Parent != issue.ID {
		t.Fatalf("expected parent %q, got %q", issue.ID, got.form.Parent)
	}
	if got.toast != "parent moved to in_progress" {
		t.Fatalf("unexpected toast: %q", got.toast)
	}
	if cmd == nil {
		t.Fatalf("expected mutation reload cmd")
	}
}

func TestUpdateReopenParentForCreateMsgError(t *testing.T) {
	t.Parallel()

	m := model{mode: ModeConfirmClosedParentCreate}
	nextModel, cmd := m.Update(reopenParentForCreateMsg{
		parentID: "bdtui-56i.21",
		err:      errors.New("update failed"),
	})
	got := nextModel.(model)

	if got.mode != ModeBoard {
		t.Fatalf("expected mode %s, got %s", ModeBoard, got.mode)
	}
	if got.form != nil {
		t.Fatalf("did not expect create form on error")
	}
	if got.toast == "" || got.toastKind != "error" {
		t.Fatalf("expected error toast, got kind=%q msg=%q", got.toastKind, got.toast)
	}
	if cmd != nil {
		t.Fatalf("expected no cmd on error")
	}
}

func TestRenderConfirmClosedParentCreateModal(t *testing.T) {
	t.Parallel()

	m := model{
		confirmClosedParentCreate: &ConfirmClosedParentCreate{
			ParentID:     "bdtui-56i.21",
			ParentTitle:  "closed parent",
			TargetStatus: StatusInProgress,
		},
	}

	out := m.renderConfirmClosedParentCreateModal()
	if !strings.Contains(out, "Cannot create issue with closed parent") {
		t.Fatalf("expected warning text, got %q", out)
	}
	if !strings.Contains(out, "y confirm | n/Esc cancel") {
		t.Fatalf("expected controls text, got %q", out)
	}
}
