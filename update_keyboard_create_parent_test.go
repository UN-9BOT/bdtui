package main

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
		mode:   ModeBoard,
		issues: []Issue{issue},
		byID:   map[string]*Issue{issue.ID: &issue},
		columns: map[Status][]Issue{
			StatusOpen:       {issue},
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
	}

	next, _ := m.handleBoardKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("N")})
	got := next.(model)

	if got.mode != ModeCreate {
		t.Fatalf("expected mode %s, got %s", ModeCreate, got.mode)
	}
	if got.form == nil {
		t.Fatalf("expected create form")
	}
	if got.form.Parent != issue.ID {
		t.Fatalf("expected parent %q, got %q", issue.ID, got.form.Parent)
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
		mode:   ModeBoard,
		issues: []Issue{issue},
		columns: map[Status][]Issue{
			StatusOpen:       {issue},
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
	}

	next, _ := m.handleBoardKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	got := next.(model)

	if got.mode != ModeCreate {
		t.Fatalf("expected mode %s, got %s", ModeCreate, got.mode)
	}
	if got.form == nil {
		t.Fatalf("expected create form")
	}
	if got.form.Parent != "" {
		t.Fatalf("expected empty parent, got %q", got.form.Parent)
	}
}
