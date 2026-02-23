package bdtui_test

import "testing"

func TestHandleLeaderComboUSelectsVisibleParent(t *testing.T) {
	t.Parallel()

	m, parentID, childID := newMouseTestModel()
	m.SelectIssueByID(childID)

	next, cmd := m.HandleLeaderCombo("u")
	got := next.(model)

	if cmd != nil {
		t.Fatalf("expected nil cmd, got %v", cmd)
	}
	if got.CurrentIssue() == nil || got.CurrentIssue().ID != parentID {
		t.Fatalf("expected selected parent %q, got %+v", parentID, got.CurrentIssue())
	}
	if got.SelectedCol != statusIndex(StatusOpen) {
		t.Fatalf("expected selectedCol=%d, got %d", statusIndex(StatusOpen), got.SelectedCol)
	}
}

func TestHandleLeaderComboUClearsFiltersAndSelectsHiddenParent(t *testing.T) {
	t.Parallel()

	m, parentID, childID := newMouseTestModel()
	m.SearchQuery = "child"
	m.SearchInput.SetValue("child")
	m.Filter.Status = "blocked"
	m.ComputeColumns()
	m.NormalizeSelectionBounds()
	m.SelectIssueByID(childID)

	next, _ := m.HandleLeaderCombo("u")
	got := next.(model)

	if got.SearchQuery != "" {
		t.Fatalf("expected searchQuery cleared, got %q", got.SearchQuery)
	}
	if got.SearchInput.Value() != "" {
		t.Fatalf("expected search input cleared, got %q", got.SearchInput.Value())
	}
	if got.Filter.Assignee != "" || got.Filter.Label != "" || got.Filter.Status != "any" || got.Filter.Priority != "any" || got.Filter.Type != "any" {
		t.Fatalf("expected filters cleared, got %+v", got.Filter)
	}
	if got.CurrentIssue() == nil || got.CurrentIssue().ID != parentID {
		t.Fatalf("expected selected parent %q, got %+v", parentID, got.CurrentIssue())
	}
}

func TestHandleLeaderComboUWarnsWhenIssueHasNoParent(t *testing.T) {
	t.Parallel()

	m, parentID, _ := newMouseTestModel()
	m.SelectIssueByID(parentID)

	next, _ := m.HandleLeaderCombo("u")
	got := next.(model)

	if got.CurrentIssue() == nil || got.CurrentIssue().ID != parentID {
		t.Fatalf("expected selection unchanged %q, got %+v", parentID, got.CurrentIssue())
	}
	if got.ToastKind != "warning" {
		t.Fatalf("expected warning toast, got %q", got.ToastKind)
	}
	if got.Toast != "issue has no parent" {
		t.Fatalf("expected no-parent warning toast, got %q", got.Toast)
	}
}

func TestHandleLeaderComboUWarnsWhenParentNotFound(t *testing.T) {
	t.Parallel()

	m, _, childID := newMouseTestModel()
	m.SearchQuery = "child"
	m.SearchInput.SetValue("child")
	m.Filter.Status = "blocked"

	for i := range m.Issues {
		if m.Issues[i].ID == childID {
			m.Issues[i].Parent = "bdtui-missing-parent"
		}
	}
	m.ByID = make(map[string]*Issue, len(m.Issues))
	for i := range m.Issues {
		m.ByID[m.Issues[i].ID] = &m.Issues[i]
	}
	m.ComputeColumns()
	m.NormalizeSelectionBounds()
	m.SelectIssueByID(childID)

	next, _ := m.HandleLeaderCombo("u")
	got := next.(model)

	if got.SearchQuery != "" {
		t.Fatalf("expected searchQuery cleared, got %q", got.SearchQuery)
	}
	if got.Filter.Assignee != "" || got.Filter.Label != "" || got.Filter.Status != "any" || got.Filter.Priority != "any" || got.Filter.Type != "any" {
		t.Fatalf("expected filters cleared, got %+v", got.Filter)
	}
	if got.CurrentIssue() == nil || got.CurrentIssue().ID != childID {
		t.Fatalf("expected selection unchanged on child %q, got %+v", childID, got.CurrentIssue())
	}
	if got.ToastKind != "warning" {
		t.Fatalf("expected warning toast, got %q", got.ToastKind)
	}
	if got.Toast != "parent not found: bdtui-missing-parent" {
		t.Fatalf("unexpected Toast: %q", got.Toast)
	}
}
