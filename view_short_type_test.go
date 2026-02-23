package main

import (
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestShortTypeDashboardMapping(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in   string
		want string
	}{
		{in: "epic", want: "E"},
		{in: "feature", want: "F"},
		{in: "task", want: "T"},
		{in: "bug", want: "B"},
		{in: "chore", want: "C"},
		{in: "decision", want: "D"},
		{in: "unknown", want: "?"},
	}

	for _, tc := range cases {
		got := shortTypeDashboard(tc.in)
		if got != tc.want {
			t.Fatalf("shortTypeDashboard(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestRenderIssueRowUsesOneLetterType(t *testing.T) {
	t.Parallel()

	item := Issue{
		ID:        "bdtui-1",
		Title:     "demo",
		Priority:  2,
		IssueType: "task",
	}

	row := renderIssueRow(item, 120, 0)
	plain := stripANSI(row)
	if !strings.Contains(plain, " T ") {
		t.Fatalf("expected one-letter type marker ' T ' in row: %q", plain)
	}
	if strings.Contains(plain, " TS ") {
		t.Fatalf("did not expect two-letter type marker ' TS ' in row: %q", plain)
	}
}

func TestRenderIssueRowPinsIDToRightEdge(t *testing.T) {
	t.Parallel()

	item := Issue{
		ID:        "bdtui-56i.10",
		Title:     "move id to right edge",
		Priority:  2,
		IssueType: "task",
	}

	row := renderIssueRow(item, 40, 0)
	plain := stripANSI(row)
	if !strings.HasSuffix(plain, item.ID) {
		t.Fatalf("expected row to end with id %q, got %q", item.ID, plain)
	}
}

func TestRenderIssueRowGhostPinsIDToRightEdge(t *testing.T) {
	t.Parallel()

	item := Issue{
		ID:        "bdtui-56i.10",
		Title:     "ghost row",
		Priority:  2,
		IssueType: "task",
	}

	row := renderIssueRowGhostPlain(item, 40, 1)
	if !strings.HasSuffix(row, item.ID) {
		t.Fatalf("expected ghost row to end with id %q, got %q", item.ID, row)
	}
}

func TestRenderIssueRowSelectedPlainHidesID(t *testing.T) {
	t.Parallel()

	item := Issue{
		ID:        "bdtui-56i.10",
		Title:     "selected row",
		Priority:  2,
		IssueType: "task",
	}

	row := renderIssueRowSelectedPlain(item, 40, 0)
	if strings.Contains(row, item.ID) {
		t.Fatalf("selected row should not include id %q: %q", item.ID, row)
	}
}

func TestDashboardEpicAccentStyleEpicIsBoldWithoutBackground(t *testing.T) {
	t.Parallel()

	style, enabled := dashboardEpicAccentStyle("epic")
	if !enabled {
		t.Fatalf("expected epic accent style to be enabled")
	}
	if !style.GetBold() {
		t.Fatalf("expected epic accent style to be bold")
	}
	if _, ok := style.GetBackground().(lipgloss.NoColor); !ok {
		t.Fatalf("expected epic accent style to not set background, got %T", style.GetBackground())
	}
}

func TestDashboardEpicAccentStyleNonEpicDisabled(t *testing.T) {
	t.Parallel()

	_, enabled := dashboardEpicAccentStyle("task")
	if enabled {
		t.Fatalf("expected non-epic accent style to be disabled")
	}
}

func TestShortTypeUnchangedForNonDashboard(t *testing.T) {
	t.Parallel()

	if got := shortType("task"); got != "TS" {
		t.Fatalf("shortType(task) = %q, want TS", got)
	}
	if got := shortType("epic"); got != "EP" {
		t.Fatalf("shortType(epic) = %q, want EP", got)
	}
}

func stripANSI(s string) string {
	re := regexp.MustCompile(`\x1b\[[0-9;]*m`)
	return re.ReplaceAllString(s, "")
}
