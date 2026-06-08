package bdtui_test

import (
	"strings"
	"testing"
	"time"
)

func TestIssueIsDeferredTrueOnlyWhenDeferUntilSet(t *testing.T) {
	t.Parallel()

	deferred := Issue{DeferUntil: "2026-06-14T00:00:00Z"}
	if !deferred.IsDeferred() {
		t.Fatalf("expected IsDeferred()=true when DeferUntil is set")
	}

	plain := Issue{}
	if plain.IsDeferred() {
		t.Fatalf("expected IsDeferred()=false when DeferUntil is empty")
	}

	whitespace := Issue{DeferUntil: "   "}
	if whitespace.IsDeferred() {
		t.Fatalf("expected IsDeferred()=false for whitespace-only DeferUntil")
	}
}

func TestRenderIssueRowIncludesDeferBadgeForDeferredIssue(t *testing.T) {
	t.Parallel()

	item := Issue{
		ID:         "bdtui-future",
		Title:      "future work",
		Priority:   2,
		IssueType:  "task",
		DeferUntil: "2999-12-30T12:00:00Z",
	}

	row := renderIssueRow(item, 120, 0)
	plain := stripANSI(row)
	if !strings.Contains(plain, "⏱") {
		t.Fatalf("expected defer badge in row, got %q", plain)
	}
	if !strings.Contains(plain, "2999-12-30") && !strings.Contains(plain, "2999-12-31") {
		t.Fatalf("expected defer date 2999-12-30/31 in row, got %q", plain)
	}
	if !strings.Contains(plain, "future work") {
		t.Fatalf("expected title to remain visible, got %q", plain)
	}
}

func TestRenderIssueRowOmitsDeferBadgeForNonDeferredIssue(t *testing.T) {
	t.Parallel()

	item := Issue{
		ID:        "bdtui-plain",
		Title:     "plain work",
		Priority:  2,
		IssueType: "task",
	}

	row := renderIssueRow(item, 120, 0)
	plain := stripANSI(row)
	if strings.Contains(plain, "⏱") {
		t.Fatalf("did not expect defer badge in non-deferred row, got %q", plain)
	}
	if strings.Contains(plain, "⏸") {
		t.Fatalf("did not expect past badge in non-deferred row, got %q", plain)
	}
}

func TestRenderIssueRowShowsPastBadgeForExpiredDefer(t *testing.T) {
	t.Parallel()

	item := Issue{
		ID:         "bdtui-old",
		Title:      "old work",
		Priority:   2,
		IssueType:  "task",
		DeferUntil: "2000-01-01T00:00:00Z",
	}

	row := renderIssueRow(item, 120, 0)
	plain := stripANSI(row)
	if !strings.Contains(plain, "⏸ past") {
		t.Fatalf("expected past badge in row, got %q", plain)
	}
}

func TestRenderIssueRowShowsNowBadgeForTodayDefer(t *testing.T) {
	t.Parallel()

	today := time.Now().UTC().Add(time.Hour).Format(time.RFC3339)
	item := Issue{
		ID:         "bdtui-today",
		Title:      "today",
		Priority:   2,
		IssueType:  "task",
		DeferUntil: today,
	}

	row := renderIssueRow(item, 120, 0)
	plain := stripANSI(row)
	if !strings.Contains(plain, "⏱ now") {
		t.Fatalf("expected now badge in row, got %q", plain)
	}
}

func newInspectorModel(issue Issue) model {
	items := map[Status][]Issue{}
	for _, s := range statusOrder {
		items[s] = []Issue{}
	}
	items[issue.Status] = []Issue{issue}

	return model{
		Width:       120,
		Height:      30,
		Mode:        ModeBoard,
		Styles:      newStyles(),
		Columns:     items,
		ColumnDepths: map[Status]map[string]int{
			StatusOpen:       {issue.ID: 0},
			StatusInProgress: {},
			StatusBlocked:    {},
			StatusClosed:     {},
		},
		ByID: map[string]*Issue{issue.ID: &issue},
		SelectedCol: 0,
		SelectedIdx: map[Status]int{
			StatusOpen:       0,
			StatusInProgress: 0,
			StatusBlocked:    0,
			StatusClosed:     0,
		},
		ScrollOffset: map[Status]int{
			StatusOpen:       0,
			StatusInProgress: 0,
			StatusBlocked:    0,
			StatusClosed:     0,
		},
	}
}

func TestRenderInspectorShowsDeferredDateInMetaLine(t *testing.T) {
	t.Parallel()

	issue := Issue{
		ID:         "bdtui-future",
		Title:      "future",
		Status:     StatusOpen,
		Display:    StatusOpen,
		Priority:   2,
		IssueType:  "task",
		DeferUntil: "2026-06-14T00:00:00Z",
	}
	m := newInspectorModel(issue)

	inspector := m.RenderInspector()
	plain := stripANSI(inspector)

	if !strings.Contains(plain, "deferred:") {
		t.Fatalf("expected 'deferred:' label in inspector, got %q", plain)
	}
	if !strings.Contains(plain, "2026-06-14") {
		t.Fatalf("expected defer date 2026-06-14 in inspector, got %q", plain)
	}
}

func TestRenderInspectorShowsDashForNonDeferredIssue(t *testing.T) {
	t.Parallel()

	issue := Issue{
		ID:        "bdtui-plain",
		Title:     "plain",
		Status:    StatusOpen,
		Display:   StatusOpen,
		Priority:  2,
		IssueType: "task",
	}
	m := newInspectorModel(issue)

	inspector := m.RenderInspector()
	plain := stripANSI(inspector)

	if !strings.Contains(plain, "deferred: -") {
		t.Fatalf("expected 'deferred: -' for non-deferred issue, got %q", plain)
	}
}

func TestRenderIssueRowSelectedPlainIncludesDeferBadge(t *testing.T) {
	t.Parallel()

	item := Issue{
		ID:         "bdtui-future",
		Title:      "future work",
		Priority:   2,
		IssueType:  "task",
		DeferUntil: "2999-12-30T12:00:00Z",
	}

	row := renderIssueRowSelectedPlain(item, 120, 0)
	if !strings.Contains(row, "⏱") {
		t.Fatalf("expected defer badge in selected plain row, got %q", row)
	}
	if !strings.Contains(row, "future work") {
		t.Fatalf("expected title to remain visible, got %q", row)
	}
}
