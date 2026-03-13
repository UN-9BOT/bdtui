package app

import "testing"

func TestMoveColumnWrapsForward(t *testing.T) {
	t.Parallel()

	m := model{
		SelectedCol:  len(statusOrder) - 1,
		Columns:      make(map[Status][]Issue),
		SelectedIdx:  make(map[Status]int),
		ScrollOffset: make(map[Status]int),
	}

	for _, st := range statusOrder {
		m.Columns[st] = []Issue{{ID: "test-" + string(st), Status: st, Display: st}}
		m.SelectedIdx[st] = 0
		m.ScrollOffset[st] = 0
	}

	m.moveColumn(1)

	if m.SelectedCol != 0 {
		t.Fatalf("expected SelectedCol=0 (wrap to open), got %d", m.SelectedCol)
	}
}

func TestMoveColumnWrapsBackward(t *testing.T) {
	t.Parallel()

	m := model{
		SelectedCol:  0,
		Columns:      make(map[Status][]Issue),
		SelectedIdx:  make(map[Status]int),
		ScrollOffset: make(map[Status]int),
	}

	for _, st := range statusOrder {
		m.Columns[st] = []Issue{{ID: "test-" + string(st), Status: st, Display: st}}
		m.SelectedIdx[st] = 0
		m.ScrollOffset[st] = 0
	}

	m.moveColumn(-1)

	if m.SelectedCol != len(statusOrder)-1 {
		t.Fatalf("expected SelectedCol=%d (wrap to closed), got %d", len(statusOrder)-1, m.SelectedCol)
	}
}

func TestMoveColumnNoWrapInMiddle(t *testing.T) {
	t.Parallel()

	m := model{
		SelectedCol:  1,
		Columns:      make(map[Status][]Issue),
		SelectedIdx:  make(map[Status]int),
		ScrollOffset: make(map[Status]int),
	}

	for _, st := range statusOrder {
		m.Columns[st] = []Issue{{ID: "test-" + string(st), Status: st, Display: st}}
		m.SelectedIdx[st] = 0
		m.ScrollOffset[st] = 0
	}

	m.moveColumn(1)
	if m.SelectedCol != 2 {
		t.Fatalf("expected SelectedCol=2, got %d", m.SelectedCol)
	}

	m.moveColumn(-1)
	if m.SelectedCol != 1 {
		t.Fatalf("expected SelectedCol=1, got %d", m.SelectedCol)
	}
}
