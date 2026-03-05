package app

import "testing"

func TestNormalizeIssuesPreservesTombstoneStatus(t *testing.T) {
	t.Parallel()

	issues := normalizeIssues([]rawIssue{
		{
			ID:        "bdtui-1",
			Title:     "deleted",
			Status:    "tombstone",
			Priority:  2,
			IssueType: "task",
		},
	})

	if len(issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(issues))
	}
	if issues[0].Status != StatusTombstone {
		t.Fatalf("expected status %q, got %q", StatusTombstone, issues[0].Status)
	}
	if issues[0].Display != StatusTombstone {
		t.Fatalf("expected display %q, got %q", StatusTombstone, issues[0].Display)
	}
}

func TestComputeColumnsSkipsTombstones(t *testing.T) {
	t.Parallel()

	m := model{
		Issues: []Issue{
			{
				ID:       "bdtui-live",
				Title:    "live",
				Status:   StatusOpen,
				Display:  StatusOpen,
				Priority: 2,
			},
			{
				ID:       "bdtui-deleted",
				Title:    "deleted",
				Status:   StatusTombstone,
				Display:  StatusTombstone,
				Priority: 2,
			},
		},
		Collapsed: make(map[string]bool),
		Filter: Filter{
			Status:   "any",
			Priority: "any",
			Type:     "any",
		},
	}

	m.computeColumns()

	if len(m.Columns[StatusOpen]) != 1 {
		t.Fatalf("expected 1 open issue, got %d", len(m.Columns[StatusOpen]))
	}
	if got := m.Columns[StatusOpen][0].ID; got != "bdtui-live" {
		t.Fatalf("expected live issue in open column, got %q", got)
	}
	for _, status := range statusOrder {
		for _, issue := range m.Columns[status] {
			if issue.Status == StatusTombstone || issue.Display == StatusTombstone {
				t.Fatalf("did not expect tombstone issue in column %q: %+v", status, issue)
			}
		}
	}
}
