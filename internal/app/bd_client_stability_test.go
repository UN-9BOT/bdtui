package app

import "testing"

func TestNormalizeIssuesSortsRelationsByID(t *testing.T) {
	t.Parallel()

	issues := normalizeIssues([]rawIssue{
		{
			ID:        "blocker",
			Title:     "blocker",
			Status:    "open",
			Priority:  2,
			IssueType: "task",
		},
		{
			ID:        "parent",
			Title:     "parent",
			Status:    "open",
			Priority:  2,
			IssueType: "epic",
		},
		{
			ID:        "parent.10",
			Title:     "child 10",
			Status:    "open",
			Priority:  2,
			IssueType: "task",
			Dependencies: []rawDependency{
				{DependsOnID: "parent", Type: "parent-child"},
				{DependsOnID: "blocker", Type: "blocks"},
			},
		},
		{
			ID:        "parent.2",
			Title:     "child 2",
			Status:    "open",
			Priority:  2,
			IssueType: "task",
			Dependencies: []rawDependency{
				{DependsOnID: "blocker", Type: "blocks"},
				{DependsOnID: "parent", Type: "parent-child"},
			},
		},
		{
			ID:        "parent.1",
			Title:     "child 1",
			Status:    "open",
			Priority:  2,
			IssueType: "task",
			Dependencies: []rawDependency{
				{DependsOnID: "parent", Type: "parent-child"},
			},
		},
	})

	byID := make(map[string]Issue, len(issues))
	for _, issue := range issues {
		byID[issue.ID] = issue
	}

	parent := byID["parent"]
	if got, want := parent.Children, []string{"parent.1", "parent.10", "parent.2"}; len(got) != len(want) ||
		got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("expected parent children %v, got %v", want, got)
	}

	blocker := byID["blocker"]
	if got, want := blocker.Blocks, []string{"parent.10", "parent.2"}; len(got) != len(want) ||
		got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("expected blocker blocks %v, got %v", want, got)
	}
}

func TestCanonicalIssuesHashIgnoresRawOrderNoise(t *testing.T) {
	t.Parallel()

	left := normalizeIssues([]rawIssue{
		{
			ID:        "parent.2",
			Title:     "child 2",
			Status:    "open",
			Priority:  2,
			IssueType: "task",
			Dependencies: []rawDependency{
				{DependsOnID: "blocker", Type: "blocks"},
				{DependsOnID: "parent", Type: "parent-child"},
			},
		},
		{
			ID:        "parent",
			Title:     "parent",
			Status:    "open",
			Priority:  2,
			IssueType: "epic",
		},
		{
			ID:        "blocker",
			Title:     "blocker",
			Status:    "open",
			Priority:  2,
			IssueType: "task",
		},
		{
			ID:        "parent.1",
			Title:     "child 1",
			Status:    "open",
			Priority:  2,
			IssueType: "task",
			Dependencies: []rawDependency{
				{DependsOnID: "parent", Type: "parent-child"},
			},
		},
	})

	right := normalizeIssues([]rawIssue{
		{
			ID:        "blocker",
			Title:     "blocker",
			Status:    "open",
			Priority:  2,
			IssueType: "task",
		},
		{
			ID:        "parent.1",
			Title:     "child 1",
			Status:    "open",
			Priority:  2,
			IssueType: "task",
			Dependencies: []rawDependency{
				{DependsOnID: "parent", Type: "parent-child"},
			},
		},
		{
			ID:        "parent.2",
			Title:     "child 2",
			Status:    "open",
			Priority:  2,
			IssueType: "task",
			Dependencies: []rawDependency{
				{DependsOnID: "parent", Type: "parent-child"},
				{DependsOnID: "blocker", Type: "blocks"},
			},
		},
		{
			ID:        "parent",
			Title:     "parent",
			Status:    "open",
			Priority:  2,
			IssueType: "epic",
		},
	})

	leftHash := canonicalIssuesHash(left)
	rightHash := canonicalIssuesHash(right)
	if leftHash != rightHash {
		t.Fatalf("expected canonical hash to match, got %q vs %q", leftHash, rightHash)
	}
}
