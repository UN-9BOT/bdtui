package app

import (
	"strings"
	"testing"
	"time"
)

func TestNormalizeIssuesParsesDeferUntil(t *testing.T) {
	t.Parallel()

	issues := normalizeIssues([]rawIssue{
		{
			ID:         "bdtui-future",
			Title:      "future",
			Status:     "open",
			Priority:   2,
			IssueType:  "task",
			DeferUntil: "2026-12-30T21:00:00Z",
		},
		{
			ID:        "bdtui-plain",
			Title:     "plain",
			Status:    "open",
			Priority:  2,
			IssueType: "task",
		},
	})

	if len(issues) != 2 {
		t.Fatalf("expected 2 issues, got %d", len(issues))
	}

	byID := make(map[string]Issue, len(issues))
	for _, issue := range issues {
		byID[issue.ID] = issue
	}

	if got := byID["bdtui-future"].DeferUntil; got != "2026-12-30T21:00:00Z" {
		t.Fatalf("expected DeferUntil to round-trip, got %q", got)
	}
	if !byID["bdtui-future"].IsDeferred() {
		t.Fatalf("expected IsDeferred()=true")
	}
	if byID["bdtui-plain"].IsDeferred() {
		t.Fatalf("expected IsDeferred()=false for empty DeferUntil")
	}
}

func TestFormatDeferDate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"empty", "", ""},
		{"blank", "   ", ""},
		{"rfc3339", "2026-06-14T00:00:00Z", "2026-06-14"},
		{"bare date", "2026-06-14", "2026-06-14"},
		{"garbage", "not a date", "not a date"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := formatDeferDate(tc.raw)
			if tc.want == "" && strings.TrimSpace(got) != "" {
				t.Fatalf("expected empty, got %q", got)
			}
			if tc.want != "" && !strings.HasPrefix(got, tc.want) {
				t.Fatalf("expected prefix %q, got %q", tc.want, got)
			}
		})
	}
}

func TestIssueDeferBadgeLabel(t *testing.T) {
	t.Parallel()

	future := Issue{DeferUntil: "2999-12-30T21:00:00Z"}
	got := future.deferBadgeLabel()
	if !strings.HasPrefix(got, "⏱") {
		t.Fatalf("expected badge to start with ⏱, got %q", got)
	}

	past := Issue{DeferUntil: "2000-01-01T00:00:00Z"}
	if got := past.deferBadgeLabel(); got != "⏸ past" {
		t.Fatalf("expected ⏸ past, got %q", got)
	}

	today := Issue{DeferUntil: time.Now().UTC().Add(time.Hour).Format(time.RFC3339)}
	if got := today.deferBadgeLabel(); got != "⏱ now" {
		t.Fatalf("expected ⏱ now, got %q", got)
	}

	none := Issue{}
	if got := none.deferBadgeLabel(); got != "" {
		t.Fatalf("expected empty badge for non-deferred issue, got %q", got)
	}
}

func TestIssueDeferState(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 6, 8, 12, 0, 0, 0, time.UTC)
	future := Issue{DeferUntil: "2026-06-10T12:00:00Z"}
	past := Issue{DeferUntil: "2026-06-01T12:00:00Z"}
	none := Issue{}

	if got := future.deferState(now); got != deferStatePending {
		t.Fatalf("expected pending for future, got %d", got)
	}
	if got := past.deferState(now); got != deferStatePast {
		t.Fatalf("expected past for past, got %d", got)
	}
	if got := none.deferState(now); got != deferStateNone {
		t.Fatalf("expected none for empty, got %d", got)
	}
}

func TestCanonicalIssuesHashChangesWhenDeferUntilChanges(t *testing.T) {
	t.Parallel()

	a := normalizeIssues([]rawIssue{
		{ID: "bdtui-x", Title: "x", Status: "open", Priority: 2, IssueType: "task"},
	})
	b := normalizeIssues([]rawIssue{
		{ID: "bdtui-x", Title: "x", Status: "open", Priority: 2, IssueType: "task", DeferUntil: "2026-06-14T00:00:00Z"},
	})

	if canonicalIssuesHash(a) == canonicalIssuesHash(b) {
		t.Fatalf("expected hash to differ when DeferUntil changes")
	}
}
