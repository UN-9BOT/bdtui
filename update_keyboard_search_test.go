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
	if m.searchQuery != "beta" {
		t.Fatalf("expected query beta to stay after esc, got %q", m.searchQuery)
	}
	col := m.columns[StatusOpen]
	if len(col) != 1 || col[0].ID != "bdtui-2" {
		t.Fatalf("expected only bdtui-2 after esc keep, got %+v", col)
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
	if m.toast != "" {
		t.Fatalf("expected no toast on search enter, got %q", m.toast)
	}
}

func TestHandleBoardKeyFAlsoFocusesSearch(t *testing.T) {
	t.Parallel()

	m := newSearchTestModel()
	m.mode = ModeBoard

	next, _ := m.handleBoardKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	m = next.(model)

	if m.mode != ModeSearch {
		t.Fatalf("expected mode search after f, got %s", m.mode)
	}
}

func TestHandleSearchKeyCtrlFAndTabCycleFiltersLive(t *testing.T) {
	t.Parallel()

	m := newSearchTestModel()
	m.mode = ModeBoard
	m.computeColumns()

	next, _ := m.handleBoardKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m = next.(model)
	next, _ = m.handleSearchKey(tea.KeyMsg{Type: tea.KeyCtrlF})
	m = next.(model)
	if !m.searchExpanded {
		t.Fatalf("expected expanded filters after ctrl+f")
	}

	next, _ = m.handleSearchKey(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(model)
	if m.filter.Assignee != "alice" {
		t.Fatalf("expected assignee filter alice after first tab, got %q", m.filter.Assignee)
	}
	col := m.columns[StatusOpen]
	if len(col) != 1 || col[0].ID != "bdtui-1" {
		t.Fatalf("expected only alice issue after assignee filter, got %+v", col)
	}

	next, _ = m.handleSearchKey(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(model)
	next, _ = m.handleSearchKey(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(model)
	if m.filter.Label != "alpha" {
		t.Fatalf("expected label filter alpha after tab, got %q", m.filter.Label)
	}
	col = m.columns[StatusOpen]
	if len(col) != 1 || col[0].ID != "bdtui-1" {
		t.Fatalf("expected only bdtui-1 after label filter, got %+v", col)
	}
}

func TestHandleKeyCtrlCClearsSearchAndFiltersGlobally(t *testing.T) {
	t.Parallel()

	m := newSearchTestModel()
	m.mode = ModeBoard
	m.searchQuery = "alpha"
	m.searchInput.SetValue("alpha")
	m.filter = Filter{
		Assignee: "alice",
		Label:    "alpha",
		Status:   "open",
		Priority: "2",
		Type:     "task",
	}
	m.computeColumns()

	next, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlC})
	m = next.(model)

	if m.searchQuery != "" {
		t.Fatalf("expected cleared search query, got %q", m.searchQuery)
	}
	if m.filter.Assignee != "" || m.filter.Label != "" || m.filter.Status != "any" || m.filter.Priority != "any" || m.filter.Type != "any" {
		t.Fatalf("expected cleared filters, got %+v", m.filter)
	}
	if m.mode != ModeBoard {
		t.Fatalf("expected mode to remain board, got %s", m.mode)
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
			Type:     "any",
		},
		issues: []Issue{
			{
				ID:        "bdtui-1",
				Title:     "alpha issue",
				Assignee:  "alice",
				Labels:    []string{"alpha"},
				IssueType: "task",
				Display:   StatusOpen,
				Status:    StatusOpen,
			},
			{
				ID:        "bdtui-2",
				Title:     "beta issue",
				Assignee:  "bob",
				Labels:    []string{"beta"},
				IssueType: "bug",
				Display:   StatusOpen,
				Status:    StatusOpen,
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
