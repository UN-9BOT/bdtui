package bdtui_test

import (
	"testing"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

func TestHandleSearchKeyUpdatesQueryInteractively(t *testing.T) {
	t.Parallel()

	m := newSearchTestModel()
	m.ComputeColumns()

	next, _ := m.HandleSearchKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("beta")})
	m = next.(model)

	if m.SearchQuery != "beta" {
		t.Fatalf("expected searchQuery=beta, got %q", m.SearchQuery)
	}
	col := m.Columns[StatusOpen]
	if len(col) != 1 || col[0].ID != "bdtui-2" {
		t.Fatalf("expected only bdtui-2 after interactive search, got %+v", col)
	}
}

func TestHandleSearchKeyEscCancelsAndRestoresPreviousQuery(t *testing.T) {
	t.Parallel()

	m := newSearchTestModel()
	m.Mode = ModeBoard
	m.SearchQuery = "alpha"
	m.ComputeColumns()

	next, _ := m.HandleBoardKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m = next.(model)
	if m.Mode != ModeSearch {
		t.Fatalf("expected mode search, got %s", m.Mode)
	}

	m.SearchInput.SetValue("beta")
	next, _ = m.HandleSearchKey(tea.KeyMsg{Type: tea.KeyLeft})
	m = next.(model)
	if m.SearchQuery != "beta" {
		t.Fatalf("expected updated interactive query beta, got %q", m.SearchQuery)
	}

	next, _ = m.HandleSearchKey(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(model)

	if m.Mode != ModeBoard {
		t.Fatalf("expected mode board after esc, got %s", m.Mode)
	}
	if m.SearchQuery != "beta" {
		t.Fatalf("expected query beta to stay after esc, got %q", m.SearchQuery)
	}
	col := m.Columns[StatusOpen]
	if len(col) != 1 || col[0].ID != "bdtui-2" {
		t.Fatalf("expected only bdtui-2 after esc keep, got %+v", col)
	}
}

func TestHandleSearchKeyEnterKeepsInteractiveQuery(t *testing.T) {
	t.Parallel()

	m := newSearchTestModel()

	next, _ := m.HandleSearchKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("beta")})
	m = next.(model)
	next, _ = m.HandleSearchKey(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)

	if m.Mode != ModeBoard {
		t.Fatalf("expected mode board after enter, got %s", m.Mode)
	}
	if m.SearchQuery != "beta" {
		t.Fatalf("expected query beta after enter, got %q", m.SearchQuery)
	}
	if m.Toast != "" {
		t.Fatalf("expected no toast on search enter, got %q", m.Toast)
	}
}

func TestHandleBoardKeyFAlsoFocusesSearch(t *testing.T) {
	t.Parallel()

	m := newSearchTestModel()
	m.Mode = ModeBoard

	next, _ := m.HandleBoardKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	m = next.(model)

	if m.Mode != ModeSearch {
		t.Fatalf("expected mode search after f, got %s", m.Mode)
	}
}

func TestHandleSearchKeyCtrlFAndTabCycleFiltersLive(t *testing.T) {
	t.Parallel()

	m := newSearchTestModel()
	m.Mode = ModeBoard
	m.ComputeColumns()

	next, _ := m.HandleBoardKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m = next.(model)
	next, _ = m.HandleSearchKey(tea.KeyMsg{Type: tea.KeyCtrlF})
	m = next.(model)
	if !m.SearchExpanded {
		t.Fatalf("expected expanded filters after ctrl+f")
	}

	next, _ = m.HandleSearchKey(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(model)
	if m.Filter.Assignee != "alice" {
		t.Fatalf("expected assignee filter alice after first tab, got %q", m.Filter.Assignee)
	}
	col := m.Columns[StatusOpen]
	if len(col) != 1 || col[0].ID != "bdtui-1" {
		t.Fatalf("expected only alice issue after assignee filter, got %+v", col)
	}

	next, _ = m.HandleSearchKey(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(model)
	next, _ = m.HandleSearchKey(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(model)
	if m.Filter.Label != "alpha" {
		t.Fatalf("expected label filter alpha after tab, got %q", m.Filter.Label)
	}
	col = m.Columns[StatusOpen]
	if len(col) != 1 || col[0].ID != "bdtui-1" {
		t.Fatalf("expected only bdtui-1 after label filter, got %+v", col)
	}
}

func TestHandleKeyCtrlCClearsSearchAndFiltersGlobally(t *testing.T) {
	t.Parallel()

	m := newSearchTestModel()
	m.Mode = ModeBoard
	m.SearchQuery = "alpha"
	m.SearchInput.SetValue("alpha")
	m.Filter = Filter{
		Assignee: "alice",
		Label:    "alpha",
		Status:   "open",
		Priority: "2",
		Type:     "task",
	}
	m.ComputeColumns()

	next, _ := m.HandleKey(tea.KeyMsg{Type: tea.KeyCtrlC})
	m = next.(model)

	if m.SearchQuery != "" {
		t.Fatalf("expected cleared search query, got %q", m.SearchQuery)
	}
	if m.Filter.Assignee != "" || m.Filter.Label != "" || m.Filter.Status != "any" || m.Filter.Priority != "any" || m.Filter.Type != "any" {
		t.Fatalf("expected cleared filters, got %+v", m.Filter)
	}
	if m.Mode != ModeBoard {
		t.Fatalf("expected mode to remain board, got %s", m.Mode)
	}
}

func newSearchTestModel() model {
	input := textinput.New()
	input.Prompt = "search> "
	input.Focus()

	return model{
		Mode:        ModeSearch,
		SortMode:    SortModeStatusDateOnly,
		SearchInput: input,
		Filter: Filter{
			Status:   "any",
			Priority: "any",
			Type:     "any",
		},
		Issues: []Issue{
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
		Columns: map[Status][]Issue{
			StatusOpen:       {},
			StatusInProgress: {},
			StatusBlocked:    {},
			StatusClosed:     {},
		},
		ColumnDepths: map[Status]map[string]int{
			StatusOpen:       {},
			StatusInProgress: {},
			StatusBlocked:    {},
			StatusClosed:     {},
		},
		SelectedCol: 0,
		SelectedIdx: map[Status]int{
			StatusOpen:       0,
			StatusInProgress: 0,
			StatusBlocked:    0,
			StatusClosed:     0,
		},
		ScrollOffset: map[Status]int{
			StatusOpen:       0,
			StatusInProgress: 0,
			StatusBlocked:    0,
			StatusClosed:     0,
		},
	}
}
