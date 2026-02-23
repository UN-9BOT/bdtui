package main

import "testing"

func TestHandleLeaderComboUSelectsVisibleParent(t *testing.T) {
	t.Parallel()

	m, parentID, childID := newMouseTestModel()
	m.selectIssueByID(childID)

	next, cmd := m.handleLeaderCombo("u")
	got := next.(model)

	if cmd != nil {
		t.Fatalf("expected nil cmd, got %v", cmd)
	}
	if got.currentIssue() == nil || got.currentIssue().ID != parentID {
		t.Fatalf("expected selected parent %q, got %+v", parentID, got.currentIssue())
	}
	if got.selectedCol != statusIndex(StatusOpen) {
		t.Fatalf("expected selectedCol=%d, got %d", statusIndex(StatusOpen), got.selectedCol)
	}
}

func TestHandleLeaderComboUClearsFiltersAndSelectsHiddenParent(t *testing.T) {
	t.Parallel()

	m, parentID, childID := newMouseTestModel()
	m.searchQuery = "child"
	m.searchInput.SetValue("child")
	m.filter.Status = "blocked"
	m.computeColumns()
	m.normalizeSelectionBounds()
	m.selectIssueByID(childID)

	next, _ := m.handleLeaderCombo("u")
	got := next.(model)

	if got.searchQuery != "" {
		t.Fatalf("expected searchQuery cleared, got %q", got.searchQuery)
	}
	if got.searchInput.Value() != "" {
		t.Fatalf("expected search input cleared, got %q", got.searchInput.Value())
	}
	if got.filter.Assignee != "" || got.filter.Label != "" || got.filter.Status != "any" || got.filter.Priority != "any" || got.filter.Type != "any" {
		t.Fatalf("expected filters cleared, got %+v", got.filter)
	}
	if got.currentIssue() == nil || got.currentIssue().ID != parentID {
		t.Fatalf("expected selected parent %q, got %+v", parentID, got.currentIssue())
	}
}

func TestHandleLeaderComboUWarnsWhenIssueHasNoParent(t *testing.T) {
	t.Parallel()

	m, parentID, _ := newMouseTestModel()
	m.selectIssueByID(parentID)

	next, _ := m.handleLeaderCombo("u")
	got := next.(model)

	if got.currentIssue() == nil || got.currentIssue().ID != parentID {
		t.Fatalf("expected selection unchanged %q, got %+v", parentID, got.currentIssue())
	}
	if got.toastKind != "warning" {
		t.Fatalf("expected warning toast, got %q", got.toastKind)
	}
	if got.toast != "issue has no parent" {
		t.Fatalf("expected no-parent warning toast, got %q", got.toast)
	}
}

func TestHandleLeaderComboUWarnsWhenParentNotFound(t *testing.T) {
	t.Parallel()

	m, _, childID := newMouseTestModel()
	m.searchQuery = "child"
	m.searchInput.SetValue("child")
	m.filter.Status = "blocked"

	for i := range m.issues {
		if m.issues[i].ID == childID {
			m.issues[i].Parent = "bdtui-missing-parent"
		}
	}
	m.byID = make(map[string]*Issue, len(m.issues))
	for i := range m.issues {
		m.byID[m.issues[i].ID] = &m.issues[i]
	}
	m.computeColumns()
	m.normalizeSelectionBounds()
	m.selectIssueByID(childID)

	next, _ := m.handleLeaderCombo("u")
	got := next.(model)

	if got.searchQuery != "" {
		t.Fatalf("expected searchQuery cleared, got %q", got.searchQuery)
	}
	if got.filter.Assignee != "" || got.filter.Label != "" || got.filter.Status != "any" || got.filter.Priority != "any" || got.filter.Type != "any" {
		t.Fatalf("expected filters cleared, got %+v", got.filter)
	}
	if got.currentIssue() == nil || got.currentIssue().ID != childID {
		t.Fatalf("expected selection unchanged on child %q, got %+v", childID, got.currentIssue())
	}
	if got.toastKind != "warning" {
		t.Fatalf("expected warning toast, got %q", got.toastKind)
	}
	if got.toast != "parent not found: bdtui-missing-parent" {
		t.Fatalf("unexpected toast: %q", got.toast)
	}
}
