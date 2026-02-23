package bdtui_test

import "testing"

func emptyColumns() map[Status][]Issue {
	return map[Status][]Issue{
		StatusOpen:       {},
		StatusInProgress: {},
		StatusBlocked:    {},
		StatusClosed:     {},
	}
}

func emptyDepths() map[Status]map[string]int {
	return map[Status]map[string]int{
		StatusOpen:       {},
		StatusInProgress: {},
		StatusBlocked:    {},
		StatusClosed:     {},
	}
}

func TestBuildColumnRowsAddsGhostParentChainAcrossStatuses(t *testing.T) {
	t.Parallel()

	epic := Issue{ID: "epic-1", Title: "Epic", Display: StatusOpen, Status: StatusOpen}
	task := Issue{ID: "task-1", Title: "Task", Parent: epic.ID, Display: StatusInProgress, Status: StatusInProgress}
	subtask := Issue{ID: "task-2", Title: "Subtask", Parent: task.ID, Display: StatusBlocked, Status: StatusBlocked}

	cols := emptyColumns()
	cols[StatusBlocked] = []Issue{subtask}
	depths := emptyDepths()
	depths[StatusBlocked][subtask.ID] = 0

	m := model{
		ByID: map[string]*Issue{
			epic.ID:    &epic,
			task.ID:    &task,
			subtask.ID: &subtask,
		},
		Columns:      cols,
		ColumnDepths: depths,
	}

	rows, issueRowIndex := m.BuildColumnRows(StatusBlocked)
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}
	if rows[0].Issue.ID != epic.ID || !rows[0].Ghost || rows[0].Depth != 0 {
		t.Fatalf("unexpected row[0]: %+v", rows[0])
	}
	if rows[1].Issue.ID != task.ID || !rows[1].Ghost || rows[1].Depth != 1 {
		t.Fatalf("unexpected row[1]: %+v", rows[1])
	}
	if rows[2].Issue.ID != subtask.ID || rows[2].Ghost || rows[2].Depth != 2 {
		t.Fatalf("unexpected row[2]: %+v", rows[2])
	}
	if issueRowIndex[subtask.ID] != 2 {
		t.Fatalf("expected selected row index 2, got %d", issueRowIndex[subtask.ID])
	}
}

func TestBuildColumnRowsNoGhostWhenParentInSameStatus(t *testing.T) {
	t.Parallel()

	parent := Issue{ID: "task-1", Title: "Parent", Display: StatusBlocked, Status: StatusBlocked}
	child := Issue{ID: "task-2", Title: "Child", Parent: parent.ID, Display: StatusBlocked, Status: StatusBlocked}

	cols := emptyColumns()
	cols[StatusBlocked] = []Issue{parent, child}
	depths := emptyDepths()
	depths[StatusBlocked][parent.ID] = 0
	depths[StatusBlocked][child.ID] = 1

	m := model{
		ByID: map[string]*Issue{
			parent.ID: &parent,
			child.ID:  &child,
		},
		Columns:      cols,
		ColumnDepths: depths,
	}

	rows, _ := m.BuildColumnRows(StatusBlocked)
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if rows[0].Ghost || rows[1].Ghost {
		t.Fatalf("expected no ghost rows, got %+v", rows)
	}
	if rows[0].Depth != 0 || rows[1].Depth != 1 {
		t.Fatalf("unexpected depths: parent=%d child=%d", rows[0].Depth, rows[1].Depth)
	}
}

func TestBuildColumnRowsGroupsSiblingsUnderSingleGhostParent(t *testing.T) {
	t.Parallel()

	parent := Issue{ID: "epic-1", Title: "Epic", Display: StatusOpen, Status: StatusOpen}
	child1 := Issue{ID: "task-1", Title: "Child 1", Parent: parent.ID, Display: StatusBlocked, Status: StatusBlocked}
	child2 := Issue{ID: "task-2", Title: "Child 2", Parent: parent.ID, Display: StatusBlocked, Status: StatusBlocked}

	cols := emptyColumns()
	cols[StatusBlocked] = []Issue{child1, child2}
	depths := emptyDepths()
	depths[StatusBlocked][child1.ID] = 0
	depths[StatusBlocked][child2.ID] = 0

	m := model{
		ByID: map[string]*Issue{
			parent.ID: &parent,
			child1.ID: &child1,
			child2.ID: &child2,
		},
		Columns:      cols,
		ColumnDepths: depths,
	}

	rows, issueRowIndex := m.BuildColumnRows(StatusBlocked)
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows (1 ghost + 2 issues), got %d", len(rows))
	}
	if rows[0].Issue.ID != parent.ID || !rows[0].Ghost {
		t.Fatalf("expected row[0] to be ghost parent, got %+v", rows[0])
	}
	if rows[1].Issue.ID != child1.ID || rows[1].Ghost {
		t.Fatalf("expected row[1] child1 issue, got %+v", rows[1])
	}
	if rows[2].Issue.ID != child2.ID || rows[2].Ghost {
		t.Fatalf("expected row[2] child2 issue, got %+v", rows[2])
	}
	if rows[1].Depth != 1 || rows[2].Depth != 1 {
		t.Fatalf("expected both children depth=1, got row1=%d row2=%d", rows[1].Depth, rows[2].Depth)
	}
	if issueRowIndex[child1.ID] != 1 || issueRowIndex[child2.ID] != 2 {
		t.Fatalf("unexpected issueRowIndex: %#v", issueRowIndex)
	}
}

func TestBuildColumnRowsStopsOnParentCycle(t *testing.T) {
	t.Parallel()

	a := Issue{ID: "a", Title: "A", Parent: "b", Display: StatusBlocked, Status: StatusBlocked}
	b := Issue{ID: "b", Title: "B", Parent: "a", Display: StatusOpen, Status: StatusOpen}

	cols := emptyColumns()
	cols[StatusBlocked] = []Issue{a}
	depths := emptyDepths()
	depths[StatusBlocked][a.ID] = 0

	m := model{
		ByID: map[string]*Issue{
			a.ID: &a,
			b.ID: &b,
		},
		Columns:      cols,
		ColumnDepths: depths,
	}

	rows, _ := m.BuildColumnRows(StatusBlocked)
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if rows[0].Issue.ID != b.ID || !rows[0].Ghost {
		t.Fatalf("expected first row to be ghost parent b, got %+v", rows[0])
	}
	if rows[1].Issue.ID != a.ID || rows[1].Ghost {
		t.Fatalf("expected second row to be real issue a, got %+v", rows[1])
	}
}

func TestEnsureSelectionVisibleUsesRenderedRowsWithGhosts(t *testing.T) {
	t.Parallel()

	root := Issue{ID: "root", Title: "Root", Display: StatusBlocked, Status: StatusBlocked}
	root2 := Issue{ID: "root-2", Title: "Root 2", Display: StatusBlocked, Status: StatusBlocked}
	epic := Issue{ID: "epic-1", Title: "Epic", Display: StatusOpen, Status: StatusOpen}
	task := Issue{ID: "task-1", Title: "Task", Parent: epic.ID, Display: StatusInProgress, Status: StatusInProgress}
	child := Issue{ID: "task-2", Title: "Child", Parent: task.ID, Display: StatusBlocked, Status: StatusBlocked}

	cols := emptyColumns()
	cols[StatusBlocked] = []Issue{root, root2, child}
	depths := emptyDepths()
	depths[StatusBlocked][root.ID] = 0
	depths[StatusBlocked][root2.ID] = 0
	depths[StatusBlocked][child.ID] = 0

	m := model{
		Height: 14,
		ByID: map[string]*Issue{
			root.ID:  &root,
			root2.ID: &root2,
			epic.ID:  &epic,
			task.ID:  &task,
			child.ID: &child,
		},
		Columns:      cols,
		ColumnDepths: depths,
		SelectedIdx: map[Status]int{
			StatusOpen:       0,
			StatusInProgress: 0,
			StatusBlocked:    2,
			StatusClosed:     0,
		},
		ScrollOffset: map[Status]int{
			StatusOpen:       0,
			StatusInProgress: 0,
			StatusBlocked:    0,
			StatusClosed:     0,
		},
	}

	rows, issueRowIndex := m.BuildColumnRows(StatusBlocked)
	if len(rows) != 5 {
		t.Fatalf("expected 5 visible rows, got %d", len(rows))
	}
	if issueRowIndex[child.ID] != 4 {
		t.Fatalf("expected selected visible row 4, got %d", issueRowIndex[child.ID])
	}
	if got := m.SelectedVisibleRowIndex(StatusBlocked); got != 4 {
		t.Fatalf("expected selectedVisibleRowIndex=4, got %d", got)
	}

	m.EnsureSelectionVisible(StatusBlocked)
	if m.ScrollOffset[StatusBlocked] != 2 {
		t.Fatalf("expected scroll offset 2, got %d", m.ScrollOffset[StatusBlocked])
	}
}
