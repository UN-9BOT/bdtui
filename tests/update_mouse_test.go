package bdtui_test

import (
	"testing"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

func TestHandleMouseSelectsIssueOnLeftClickPressInBoardMode(t *testing.T) {
	t.Parallel()

	m, parentID, childID := newMouseTestModel()
	m.SelectIssueByID(parentID)

	rows, issueRowIndex := m.BuildColumnRows(StatusBlocked)
	childRow := issueRowIndex[childID]
	if childRow <= 0 {
		t.Fatalf("expected child row with ghost parent above, got row=%d rows=%d", childRow, len(rows))
	}

	x, y := mouseClickCoordsForRow(m, StatusBlocked, childRow)
	next, cmd := m.HandleMouse(tea.MouseMsg{
		X:      x,
		Y:      y,
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
	})
	got := next.(model)

	if cmd != nil {
		t.Fatalf("expected nil cmd on mouse selection, got %v", cmd)
	}
	if got.CurrentIssue() == nil || got.CurrentIssue().ID != childID {
		t.Fatalf("expected selected issue %q, got %+v", childID, got.CurrentIssue())
	}
	if got.SelectedCol != statusIndex(StatusBlocked) {
		t.Fatalf("expected selectedCol=%d, got %d", statusIndex(StatusBlocked), got.SelectedCol)
	}
	if got.SelectedIdx[StatusBlocked] != 0 {
		t.Fatalf("expected blocked selectedIdx=0, got %d", got.SelectedIdx[StatusBlocked])
	}
}

func TestHandleMouseClickOnHeaderDoesNotMoveSelection(t *testing.T) {
	t.Parallel()

	m, parentID, _ := newMouseTestModel()
	m.SelectIssueByID(parentID)

	x, y := mouseClickCoordsForHeader(m, StatusOpen)
	next, _ := m.HandleMouse(tea.MouseMsg{
		X:      x,
		Y:      y,
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
	})
	got := next.(model)

	if got.CurrentIssue() == nil || got.CurrentIssue().ID != parentID {
		t.Fatalf("expected selection unchanged on header click, got %+v", got.CurrentIssue())
	}
}

func TestHandleMouseGhostClickFocusesParent(t *testing.T) {
	t.Parallel()

	m, parentID, childID := newMouseTestModel()
	m.SelectIssueByID(childID)

	rows, _ := m.BuildColumnRows(StatusBlocked)
	if len(rows) < 2 || !rows[0].Ghost {
		t.Fatalf("expected first row ghost parent, rows=%+v", rows)
	}

	x, y := mouseClickCoordsForRow(m, StatusBlocked, 0)
	next, _ := m.HandleMouse(tea.MouseMsg{
		X:      x,
		Y:      y,
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
	})
	got := next.(model)

	if got.CurrentIssue() == nil || got.CurrentIssue().ID != parentID {
		t.Fatalf("expected parent %q selected, got %+v", parentID, got.CurrentIssue())
	}
	if got.SelectedCol != statusIndex(StatusOpen) {
		t.Fatalf("expected selectedCol=%d, got %d", statusIndex(StatusOpen), got.SelectedCol)
	}
}

func TestHandleMouseGhostClickClearsFiltersWhenParentHidden(t *testing.T) {
	t.Parallel()

	m, parentID, childID := newMouseTestModel()
	m.SearchQuery = "child"
	m.SearchInput.SetValue("child")
	m.Filter.Status = "blocked"
	m.ComputeColumns()
	m.NormalizeSelectionBounds()
	m.SelectIssueByID(childID)

	rows, _ := m.BuildColumnRows(StatusBlocked)
	if len(rows) < 2 || !rows[0].Ghost {
		t.Fatalf("expected ghost row for hidden parent, rows=%+v", rows)
	}
	if len(m.Columns[StatusOpen]) != 0 {
		t.Fatalf("expected open column hidden by filter before click, got %d issues", len(m.Columns[StatusOpen]))
	}

	x, y := mouseClickCoordsForRow(m, StatusBlocked, 0)
	next, _ := m.HandleMouse(tea.MouseMsg{
		X:      x,
		Y:      y,
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
	})
	got := next.(model)

	if got.SearchQuery != "" {
		t.Fatalf("expected searchQuery cleared, got %q", got.SearchQuery)
	}
	if got.Filter.Assignee != "" || got.Filter.Label != "" || got.Filter.Status != "any" || got.Filter.Priority != "any" || got.Filter.Type != "any" {
		t.Fatalf("expected filters cleared, got %+v", got.Filter)
	}
	if got.CurrentIssue() == nil || got.CurrentIssue().ID != parentID {
		t.Fatalf("expected parent %q selected after clearing filters, got %+v", parentID, got.CurrentIssue())
	}
}

func TestHandleMouseIgnoresNonLeftPress(t *testing.T) {
	t.Parallel()

	m, parentID, _ := newMouseTestModel()
	m.SelectIssueByID(parentID)
	before := m.SelectedCol

	next, _ := m.HandleMouse(tea.MouseMsg{
		X:      5,
		Y:      5,
		Action: tea.MouseActionRelease,
		Button: tea.MouseButtonLeft,
	})
	got := next.(model)

	if got.SelectedCol != before {
		t.Fatalf("expected selectedCol unchanged for release event, got %d", got.SelectedCol)
	}
	if got.CurrentIssue() == nil || got.CurrentIssue().ID != parentID {
		t.Fatalf("expected same selected issue %q, got %+v", parentID, got.CurrentIssue())
	}
}

func TestHandleMouseIgnoresClicksOutsideBoard(t *testing.T) {
	t.Parallel()

	m, parentID, _ := newMouseTestModel()
	m.SelectIssueByID(parentID)

	next, _ := m.HandleMouse(tea.MouseMsg{
		X:      0,
		Y:      0,
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
	})
	got := next.(model)
	if got.CurrentIssue() == nil || got.CurrentIssue().ID != parentID {
		t.Fatalf("expected selection unchanged when clicking outside board, got %+v", got.CurrentIssue())
	}
}

func TestHandleMouseIgnoredOutsideBoardMode(t *testing.T) {
	t.Parallel()

	m, parentID, _ := newMouseTestModel()
	m.Mode = ModeDetails
	m.SelectIssueByID(parentID)

	next, _ := m.HandleMouse(tea.MouseMsg{
		X:      10,
		Y:      5,
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
	})
	got := next.(model)
	if got.CurrentIssue() == nil || got.CurrentIssue().ID != parentID {
		t.Fatalf("expected selection unchanged outside board mode, got %+v", got.CurrentIssue())
	}
}

func newMouseTestModel() (model, string, string) {
	search := textinput.New()
	search.Prompt = "search> "

	parent := Issue{
		ID:        "bdtui-parent",
		Title:     "Parent",
		Status:    StatusOpen,
		Display:   StatusOpen,
		Priority:  1,
		IssueType: "epic",
		UpdatedAt: "2026-02-23T10:00:00Z",
	}
	child := Issue{
		ID:        "bdtui-child",
		Title:     "Child",
		Parent:    parent.ID,
		Status:    StatusBlocked,
		Display:   StatusBlocked,
		Priority:  2,
		IssueType: "task",
		UpdatedAt: "2026-02-23T09:00:00Z",
	}
	another := Issue{
		ID:        "bdtui-open-2",
		Title:     "Another Open",
		Status:    StatusOpen,
		Display:   StatusOpen,
		Priority:  3,
		IssueType: "task",
		UpdatedAt: "2026-02-23T08:00:00Z",
	}

	m := model{
		Width:       120,
		Height:      30,
		Mode:        ModeBoard,
		Styles:      newStyles(),
		SortMode:    SortModeStatusDateOnly,
		SearchInput: search,
		Filter: Filter{
			Status:   "any",
			Priority: "any",
			Type:     "any",
		},
		Issues: []Issue{parent, child, another},
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
		SelectedCol: statusIndex(StatusOpen),
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
		ByID: map[string]*Issue{
			parent.ID:  &parent,
			child.ID:   &child,
			another.ID: &another,
		},
	}

	m.ComputeColumns()
	m.NormalizeSelectionBounds()

	return m, parent.ID, child.ID
}

func mouseClickCoordsForRow(m model, status Status, rowIdx int) (int, int) {
	const (
		boardLeft = 1
		boardTop  = 1
	)
	availableWidth := max(20, m.Width-4)
	panelWidth := (availableWidth - (len(statusOrder) - 1)) / len(statusOrder)
	if panelWidth < 20 {
		panelWidth = 20
	}
	outerWidth := panelWidth + 2
	colIdx := statusIndex(status)
	x := boardLeft + (colIdx * outerWidth) + 2
	y := boardTop + 3 + rowIdx
	return x, y
}

func mouseClickCoordsForHeader(m model, status Status) (int, int) {
	const (
		boardLeft = 1
		boardTop  = 1
	)
	availableWidth := max(20, m.Width-4)
	panelWidth := (availableWidth - (len(statusOrder) - 1)) / len(statusOrder)
	if panelWidth < 20 {
		panelWidth = 20
	}
	outerWidth := panelWidth + 2
	colIdx := statusIndex(status)
	x := boardLeft + (colIdx * outerWidth) + 2
	y := boardTop + 1
	return x, y
}

func statusIndex(status Status) int {
	for i, st := range statusOrder {
		if st == status {
			return i
		}
	}
	return 0
}
