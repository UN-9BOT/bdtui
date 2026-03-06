package bdtui_test

import "testing"

func TestInspectorInnerHeightCollapsed(t *testing.T) {
	t.Parallel()

	m := model{Height: 40, ShowDetails: false}
	if got := m.InspectorInnerHeight(); got != 5 {
		t.Fatalf("expected collapsed inspector inner height 5, got %d", got)
	}
}

func TestInspectorInnerHeightExpandedUsesFortyPercent(t *testing.T) {
	t.Parallel()

	issue := Issue{
		ID:          "bdtui-56i.29",
		Title:       "long details",
		Status:      StatusOpen,
		Display:     StatusOpen,
		Priority:    2,
		IssueType:   "task",
		Description: "1\n2\n3\n4\n5\n6",
		Notes:       "a\nb\nc\nd\ne\nf",
	}
	m := model{
		Width:       100,
		Height:      40,
		ShowDetails: true,
		Columns: map[Status][]Issue{
			StatusOpen:       {issue},
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
	}

	if got := m.InspectorOuterHeight(); got != 15 {
		t.Fatalf("expected expanded inspector outer height 15, got %d", got)
	}
	if got := m.InspectorInnerHeight(); got != 13 {
		t.Fatalf("expected expanded inspector inner height 13, got %d", got)
	}
	if got := m.DetailsViewportHeight(); got != 10 {
		t.Fatalf("expected details viewport height 10, got %d", got)
	}
}

func TestInspectorInnerHeightExpandedStaysFixedForShortDetails(t *testing.T) {
	t.Parallel()

	issue := Issue{
		ID:          "bdtui-56i.29",
		Title:       "short details",
		Status:      StatusOpen,
		Display:     StatusOpen,
		Priority:    2,
		IssueType:   "task",
		Description: "desc",
		Notes:       "note",
	}
	m := model{
		Width:       100,
		Height:      40,
		ShowDetails: true,
		Columns: map[Status][]Issue{
			StatusOpen:       {issue},
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
	}

	if got := m.InspectorInnerHeight(); got != 13 {
		t.Fatalf("expected short expanded inspector inner height 13, got %d", got)
	}
	if got := m.InspectorOuterHeight(); got != 15 {
		t.Fatalf("expected short expanded inspector outer height 15, got %d", got)
	}
}

func TestInspectorExpandedKeepsBoardUsableOnShortScreens(t *testing.T) {
	t.Parallel()

	m := model{Height: 15, ShowDetails: true}

	if got := m.InspectorOuterHeight(); got != 7 {
		t.Fatalf("expected clamped inspector outer height 7, got %d", got)
	}
	if got := m.BoardInnerHeight(); got != 6 {
		t.Fatalf("expected board inner height 6, got %d", got)
	}
}
