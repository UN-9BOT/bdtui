package bdtui_test

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestHandleBoardKeyShiftNCreatesFormWithSelectedParent(t *testing.T) {
	t.Parallel()

	issue := Issue{
		ID:      "bdtui-56i.1",
		Title:   "child task",
		Status:  StatusOpen,
		Display: StatusOpen,
	}

	m := model{
		Mode:   ModeBoard,
		Issues: []Issue{issue},
		ByID:   map[string]*Issue{issue.ID: &issue},
		Columns: map[Status][]Issue{
			StatusOpen:       {issue},
			StatusInProgress: {},
			StatusBlocked:    {},
			StatusClosed:     {},
		},
		SelectedIdx: map[Status]int{
			StatusOpen:       0,
			StatusInProgress: 0,
			StatusBlocked:    0,
			StatusClosed:     0,
		},
	}

	next, _ := m.HandleBoardKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("N")})
	got := next.(model)

	if got.Mode != ModeCreate {
		t.Fatalf("expected mode %s, got %s", ModeCreate, got.Mode)
	}
	if got.Form == nil {
		t.Fatalf("expected create form")
	}
	if got.Form.Parent != issue.ID {
		t.Fatalf("expected parent %q, got %q", issue.ID, got.Form.Parent)
	}
}

func TestHandleBoardKeyNCreatesFormWithoutParentPrefill(t *testing.T) {
	t.Parallel()

	issue := Issue{
		ID:      "bdtui-56i.1",
		Title:   "child task",
		Status:  StatusOpen,
		Display: StatusOpen,
	}

	m := model{
		Mode:   ModeBoard,
		Issues: []Issue{issue},
		Columns: map[Status][]Issue{
			StatusOpen:       {issue},
			StatusInProgress: {},
			StatusBlocked:    {},
			StatusClosed:     {},
		},
		SelectedIdx: map[Status]int{
			StatusOpen:       0,
			StatusInProgress: 0,
			StatusBlocked:    0,
			StatusClosed:     0,
		},
	}

	next, _ := m.HandleBoardKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	got := next.(model)

	if got.Mode != ModeCreate {
		t.Fatalf("expected mode %s, got %s", ModeCreate, got.Mode)
	}
	if got.Form == nil {
		t.Fatalf("expected create form")
	}
	if got.Form.Parent != "" {
		t.Fatalf("expected empty parent, got %q", got.Form.Parent)
	}
}

func TestHandleBoardKeyShiftNOnClosedOpensConfirmModal(t *testing.T) {
	t.Parallel()

	issue := Issue{
		ID:      "bdtui-56i.21",
		Title:   "closed parent",
		Status:  StatusClosed,
		Display: StatusClosed,
	}

	m := model{
		Mode:   ModeBoard,
		Issues: []Issue{issue},
		Columns: map[Status][]Issue{
			StatusOpen:       {},
			StatusInProgress: {},
			StatusBlocked:    {},
			StatusClosed:     {issue},
		},
		SelectedCol: 3,
		SelectedIdx: map[Status]int{
			StatusOpen:       0,
			StatusInProgress: 0,
			StatusBlocked:    0,
			StatusClosed:     0,
		},
	}

	next, _ := m.HandleBoardKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("N")})
	got := next.(model)

	if got.Mode != ModeConfirmClosedParentCreate {
		t.Fatalf("expected mode %s, got %s", ModeConfirmClosedParentCreate, got.Mode)
	}
	if got.Form != nil {
		t.Fatalf("expected no create form while confirm is open")
	}
	if got.ConfirmClosedParentCreate == nil {
		t.Fatalf("expected confirm state")
	}
	if got.ConfirmClosedParentCreate.ParentID != issue.ID {
		t.Fatalf("expected parent id %q, got %q", issue.ID, got.ConfirmClosedParentCreate.ParentID)
	}
	if got.ConfirmClosedParentCreate.TargetStatus != StatusInProgress {
		t.Fatalf("expected target status %s, got %s", StatusInProgress, got.ConfirmClosedParentCreate.TargetStatus)
	}
}
