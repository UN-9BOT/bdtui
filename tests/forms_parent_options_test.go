package bdtui_test

import "testing"

func TestBuildParentOptionsEpicBeforeTaskWithinSameStatus(t *testing.T) {
	t.Parallel()

	issues := []Issue{
		{
			ID:        "bdtui-56i.101",
			Title:     "task parent",
			Status:    StatusOpen,
			Display:   StatusOpen,
			Priority:  1,
			IssueType: "task",
		},
		{
			ID:        "bdtui-56i.102",
			Title:     "epic parent",
			Status:    StatusOpen,
			Display:   StatusOpen,
			Priority:  3,
			IssueType: "epic",
		},
	}

	opts, _ := buildParentOptions(issues, "", "")
	ids := optionIDs(opts)
	want := []string{"", "bdtui-56i.102", "bdtui-56i.101"}
	assertIDsEqual(t, ids, want)
}

func TestBuildParentOptionsKeepsStatusPrecedenceAcrossTypes(t *testing.T) {
	t.Parallel()

	issues := []Issue{
		{
			ID:        "bdtui-56i.201",
			Title:     "open task",
			Status:    StatusOpen,
			Display:   StatusOpen,
			Priority:  2,
			IssueType: "task",
		},
		{
			ID:        "bdtui-56i.202",
			Title:     "in progress epic",
			Status:    StatusInProgress,
			Display:   StatusInProgress,
			Priority:  0,
			IssueType: "epic",
		},
	}

	opts, _ := buildParentOptions(issues, "", "")
	ids := optionIDs(opts)
	want := []string{"", "bdtui-56i.201", "bdtui-56i.202"}
	assertIDsEqual(t, ids, want)
}

func TestBuildParentOptionsExcludesClosedIssues(t *testing.T) {
	t.Parallel()

	issues := []Issue{
		{
			ID:        "bdtui-56i.301",
			Title:     "closed epic",
			Status:    StatusClosed,
			Display:   StatusClosed,
			Priority:  0,
			IssueType: "epic",
		},
		{
			ID:        "bdtui-56i.302",
			Title:     "open task",
			Status:    StatusOpen,
			Display:   StatusOpen,
			Priority:  1,
			IssueType: "task",
		},
	}

	opts, _ := buildParentOptions(issues, "", "")
	ids := optionIDs(opts)
	want := []string{"", "bdtui-56i.302"}
	assertIDsEqual(t, ids, want)
}

func optionIDs(opts []ParentOption) []string {
	out := make([]string, 0, len(opts))
	for _, opt := range opts {
		out = append(out, opt.ID)
	}
	return out
}

func assertIDsEqual(t *testing.T, got []string, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("len(got) = %d, want %d; got=%v want=%v", len(got), len(want), got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got[%d] = %q, want %q; got=%v want=%v", i, got[i], want[i], got, want)
		}
	}
}
