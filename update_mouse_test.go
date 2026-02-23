package main

import (
	"testing"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

func TestHandleMouseSelectsIssueOnLeftClickPressInBoardMode(t *testing.T) {
	t.Parallel()

	m, parentID, childID := newMouseTestModel()
	m.selectIssueByID(parentID)

	rows, issueRowIndex := m.buildColumnRows(StatusBlocked)
	childRow := issueRowIndex[childID]
	if childRow <= 0 {
		t.Fatalf("expected child row with ghost parent above, got row=%d rows=%d", childRow, len(rows))
	}

	x, y := mouseClickCoordsForRow(m, StatusBlocked, childRow)
	next, cmd := m.handleMouse(tea.MouseMsg{
		X:      x,
		Y:      y,
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
	})
	got := next.(model)

	if cmd != nil {
		t.Fatalf("expected nil cmd on mouse selection, got %v", cmd)
	}
	if got.currentIssue() == nil || got.currentIssue().ID != childID {
		t.Fatalf("expected selected issue %q, got %+v", childID, got.currentIssue())
	}
	if got.selectedCol != statusIndex(StatusBlocked) {
		t.Fatalf("expected selectedCol=%d, got %d", statusIndex(StatusBlocked), got.selectedCol)
	}
	if got.selectedIdx[StatusBlocked] != 0 {
		t.Fatalf("expected blocked selectedIdx=0, got %d", got.selectedIdx[StatusBlocked])
	}
}

func TestHandleMouseClickOnHeaderDoesNotMoveSelection(t *testing.T) {
	t.Parallel()

	m, parentID, _ := newMouseTestModel()
	m.selectIssueByID(parentID)

	x, y := mouseClickCoordsForHeader(m, StatusOpen)
	next, _ := m.handleMouse(tea.MouseMsg{
		X:      x,
		Y:      y,
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
	})
	got := next.(model)

	if got.currentIssue() == nil || got.currentIssue().ID != parentID {
		t.Fatalf("expected selection unchanged on header click, got %+v", got.currentIssue())
	}
}

func TestHandleMouseGhostClickFocusesParent(t *testing.T) {
	t.Parallel()

	m, parentID, childID := newMouseTestModel()
	m.selectIssueByID(childID)

	rows, _ := m.buildColumnRows(StatusBlocked)
	if len(rows) < 2 || !rows[0].ghost {
		t.Fatalf("expected first row ghost parent, rows=%+v", rows)
	}

	x, y := mouseClickCoordsForRow(m, StatusBlocked, 0)
	next, _ := m.handleMouse(tea.MouseMsg{
		X:      x,
		Y:      y,
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
	})
	got := next.(model)

	if got.currentIssue() == nil || got.currentIssue().ID != parentID {
		t.Fatalf("expected parent %q selected, got %+v", parentID, got.currentIssue())
	}
	if got.selectedCol != statusIndex(StatusOpen) {
		t.Fatalf("expected selectedCol=%d, got %d", statusIndex(StatusOpen), got.selectedCol)
	}
}

func TestHandleMouseGhostClickClearsFiltersWhenParentHidden(t *testing.T) {
	t.Parallel()

	m, parentID, childID := newMouseTestModel()
	m.searchQuery = "child"
	m.searchInput.SetValue("child")
	m.filter.Status = "blocked"
	m.computeColumns()
	m.normalizeSelectionBounds()
	m.selectIssueByID(childID)

	rows, _ := m.buildColumnRows(StatusBlocked)
	if len(rows) < 2 || !rows[0].ghost {
		t.Fatalf("expected ghost row for hidden parent, rows=%+v", rows)
	}
	if len(m.columns[StatusOpen]) != 0 {
		t.Fatalf("expected open column hidden by filter before click, got %d issues", len(m.columns[StatusOpen]))
	}

	x, y := mouseClickCoordsForRow(m, StatusBlocked, 0)
	next, _ := m.handleMouse(tea.MouseMsg{
		X:      x,
		Y:      y,
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
	})
	got := next.(model)

	if got.searchQuery != "" {
		t.Fatalf("expected searchQuery cleared, got %q", got.searchQuery)
	}
	if got.filter.Assignee != "" || got.filter.Label != "" || got.filter.Status != "any" || got.filter.Priority != "any" || got.filter.Type != "any" {
		t.Fatalf("expected filters cleared, got %+v", got.filter)
	}
	if got.currentIssue() == nil || got.currentIssue().ID != parentID {
		t.Fatalf("expected parent %q selected after clearing filters, got %+v", parentID, got.currentIssue())
	}
}

func TestHandleMouseIgnoresNonLeftPress(t *testing.T) {
	t.Parallel()

	m, parentID, _ := newMouseTestModel()
	m.selectIssueByID(parentID)
	before := m.selectedCol

	next, _ := m.handleMouse(tea.MouseMsg{
		X:      5,
		Y:      5,
		Action: tea.MouseActionRelease,
		Button: tea.MouseButtonLeft,
	})
	got := next.(model)

	if got.selectedCol != before {
		t.Fatalf("expected selectedCol unchanged for release event, got %d", got.selectedCol)
	}
	if got.currentIssue() == nil || got.currentIssue().ID != parentID {
		t.Fatalf("expected same selected issue %q, got %+v", parentID, got.currentIssue())
	}
}

func TestHandleMouseIgnoresClicksOutsideBoard(t *testing.T) {
	t.Parallel()

	m, parentID, _ := newMouseTestModel()
	m.selectIssueByID(parentID)

	next, _ := m.handleMouse(tea.MouseMsg{
		X:      0,
		Y:      0,
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
	})
	got := next.(model)
	if got.currentIssue() == nil || got.currentIssue().ID != parentID {
		t.Fatalf("expected selection unchanged when clicking outside board, got %+v", got.currentIssue())
	}
}

func TestHandleMouseIgnoredOutsideBoardMode(t *testing.T) {
	t.Parallel()

	m, parentID, _ := newMouseTestModel()
	m.mode = ModeDetails
	m.selectIssueByID(parentID)

	next, _ := m.handleMouse(tea.MouseMsg{
		X:      10,
		Y:      5,
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
	})
	got := next.(model)
	if got.currentIssue() == nil || got.currentIssue().ID != parentID {
		t.Fatalf("expected selection unchanged outside board mode, got %+v", got.currentIssue())
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
		width:       120,
		height:      30,
		mode:        ModeBoard,
		styles:      newStyles(),
		sortMode:    SortModeStatusDateOnly,
		searchInput: search,
		filter: Filter{
			Status:   "any",
			Priority: "any",
			Type:     "any",
		},
		issues: []Issue{parent, child, another},
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
		selectedCol: statusIndex(StatusOpen),
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
		byID: map[string]*Issue{
			parent.ID:  &parent,
			child.ID:   &child,
			another.ID: &another,
		},
	}

	m.computeColumns()
	m.normalizeSelectionBounds()

	return m, parent.ID, child.ID
}

func mouseClickCoordsForRow(m model, status Status, rowIdx int) (int, int) {
	const (
		boardLeft = 1
		boardTop  = 1
	)
	availableWidth := max(20, m.width-4)
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
	availableWidth := max(20, m.width-4)
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
