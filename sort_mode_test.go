package main

import "testing"

func TestSortIssuesByModeStatusDateOnly(t *testing.T) {
	t.Parallel()

	items := []Issue{
		{ID: "c", Priority: 0, UpdatedAt: "2026-02-21T10:00:00Z"},
		{ID: "a", Priority: 4, UpdatedAt: "2026-02-21T12:00:00Z"},
		{ID: "b", Priority: 2, UpdatedAt: "2026-02-21T12:00:00Z"},
	}

	sortIssuesByMode(items, SortModeStatusDateOnly)

	got := []string{items[0].ID, items[1].ID, items[2].ID}
	want := []string{"a", "b", "c"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("status_date_only unexpected order: got=%v want=%v", got, want)
		}
	}
}

func TestSortIssuesByModePriorityThenStatusDate(t *testing.T) {
	t.Parallel()

	items := []Issue{
		{ID: "x", Priority: 2, UpdatedAt: "2026-02-21T10:00:00Z"},
		{ID: "y", Priority: 1, UpdatedAt: "2026-02-21T09:00:00Z"},
		{ID: "z", Priority: 1, UpdatedAt: "2026-02-21T11:00:00Z"},
	}

	sortIssuesByMode(items, SortModePriorityThenStatusDate)

	got := []string{items[0].ID, items[1].ID, items[2].ID}
	want := []string{"z", "y", "x"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("priority_then_status_date unexpected order: got=%v want=%v", got, want)
		}
	}
}

func TestHandleLeaderComboToggleSortModeWithoutSelection(t *testing.T) {
	t.Parallel()

	m := model{
		sortMode: SortModeStatusDateOnly,
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

	next, _ := m.handleLeaderCombo("o")
	got := next.(model)
	if got.sortMode != SortModePriorityThenStatusDate {
		t.Fatalf("expected toggled sort mode, got %s", got.sortMode)
	}
}

func TestParseSortMode(t *testing.T) {
	t.Parallel()

	mode, ok := parseSortMode(" priority_then_status_date ")
	if !ok || mode != SortModePriorityThenStatusDate {
		t.Fatalf("unexpected parse result: ok=%v mode=%s", ok, mode)
	}
	if _, ok := parseSortMode("unknown"); ok {
		t.Fatalf("expected unknown mode parse to fail")
	}
}
