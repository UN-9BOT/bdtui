package main

import (
	"regexp"
	"strings"
	"testing"
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
