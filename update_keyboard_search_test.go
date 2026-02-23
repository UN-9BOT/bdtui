package main

import (
	"testing"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

func TestHandleSearchKeyUpdatesQueryInteractively(t *testing.T) {
	t.Parallel()

	m := newSearchTestModel()
	m.computeColumns()

	next, _ := m.handleSearchKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("beta")})
	m = next.(model)

	if m.searchQuery != "beta" {
		t.Fatalf("expected searchQuery=beta, got %q", m.searchQuery)
	}
	col := m.columns[StatusOpen]
	if len(col) != 1 || col[0].ID != "bdtui-2" {
		t.Fatalf("expected only bdtui-2 after interactive search, got %+v", col)
	}
}

func TestHandleSearchKeyEscCancelsAndRestoresPreviousQuery(t *testing.T) {
	t.Parallel()

	m := newSearchTestModel()
	m.mode = ModeBoard
	m.searchQuery = "alpha"
	m.computeColumns()

	next, _ := m.handleBoardKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m = next.(model)
	if m.mode != ModeSearch {
		t.Fatalf("expected mode search, got %s", m.mode)
	}

	m.searchInput.SetValue("beta")
	next, _ = m.handleSearchKey(tea.KeyMsg{Type: tea.KeyLeft})
	m = next.(model)
	if m.searchQuery != "beta" {
		t.Fatalf("expected updated interactive query beta, got %q", m.searchQuery)
	}

	next, _ = m.handleSearchKey(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(model)

	if m.mode != ModeBoard {
		t.Fatalf("expected mode board after esc, got %s", m.mode)
	}
	if m.searchQuery != "alpha" {
		t.Fatalf("expected restored query alpha, got %q", m.searchQuery)
	}
	col := m.columns[StatusOpen]
	if len(col) != 1 || col[0].ID != "bdtui-1" {
		t.Fatalf("expected only bdtui-1 after restore, got %+v", col)
	}
}

func TestHandleSearchKeyEnterKeepsInteractiveQuery(t *testing.T) {
	t.Parallel()

	m := newSearchTestModel()

	next, _ := m.handleSearchKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("beta")})
	m = next.(model)
	next, _ = m.handleSearchKey(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)

	if m.mode != ModeBoard {
		t.Fatalf("expected mode board after enter, got %s", m.mode)
	}
	if m.searchQuery != "beta" {
		t.Fatalf("expected query beta after enter, got %q", m.searchQuery)
	}
	if m.toast != "search updated" {
		t.Fatalf("expected toast search updated, got %q", m.toast)
	}
}

func newSearchTestModel() model {
	input := textinput.New()
	input.Prompt = "search> "
	input.Focus()

	return model{
		mode:        ModeSearch,
		sortMode:    SortModeStatusDateOnly,
		searchInput: input,
		filter: Filter{
			Status:   "any",
			Priority: "any",
		},
		issues: []Issue{
			{
				ID:      "bdtui-1",
				Title:   "alpha issue",
				Display: StatusOpen,
				Status:  StatusOpen,
			},
			{
				ID:      "bdtui-2",
				Title:   "beta issue",
				Display: StatusOpen,
				Status:  StatusOpen,
			},
		},
		columns: map[Status][]Issue{
			StatusOpen:       {},
			StatusInProgress: {},
			StatusBlocked:    {},
			StatusClosed:     {},
		},
		columnDepths: map[Status]map[string]int{
			StatusOpen:       {},
			StatusInProgress: {},
			StatusBlocked:    {},
			StatusClosed:     {},
		},
		selectedCol: 0,
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
	}
}
