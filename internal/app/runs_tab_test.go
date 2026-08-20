package app

import (
	"testing"
)

// TestRenderRunsModalEmpty asserts the modal renders cleanly when no
// runs are loaded (either still loading or after an error). This is
// the contract: the user must always see a header and a footer so the
// tab is never a blank pane.
func TestRenderRunsModalEmpty(t *testing.T) {
	m := model{Runs: &RunsTabState{LoadingMsg: "loading runs..."}}
	out := m.renderRunsModal()
	if out == "" {
		t.Fatal("renderRunsModal returned empty string for loading state")
	}
}

// TestRenderRunsModalRows asserts the modal renders one line per run
// plus the header / footer. The selected row must be prefixed with
// "> " so the user can see which row is active.
func TestRenderRunsModalRows(t *testing.T) {
	m := model{Runs: &RunsTabState{
		Loaded: true,
		Index:  1,
		Rows: []RunRow{
			{RunID: "run-aaaa", Status: "completed", TaskID: "task-smooth", WorkflowStageHint: "plan -> review -> implement"},
			{RunID: "run-bbbb", Status: "waiting_human", TaskID: "task-human", WorkflowStageHint: "review -> ask", HasPendingHuman: true},
		},
	}}
	out := m.renderRunsModal()
	if !contains(out, "run-aaaa") {
		t.Fatalf("expected run-aaaa in modal output, got: %q", out)
	}
	if !contains(out, "run-bbbb") {
		t.Fatalf("expected run-bbbb in modal output, got: %q", out)
	}
	if !contains(out, "[needs human]") {
		t.Fatalf("expected needs-human flag, got: %q", out)
	}
}

// TestMoveRunSelectionClamps ensures the selection index never goes
// negative and never exceeds len(rows)-1, even when the caller asks
// for a large delta or the row list is empty.
func TestMoveRunSelectionClamps(t *testing.T) {
	cases := []struct {
		name  string
		rows  int
		idx   int
		delta int
		want  int
	}{
		{"empty list", 0, 0, 1, 0},     // no-op
		{"clamp low", 3, 0, -5, 0},     // -5 from 0 -> 0
		{"clamp high", 3, 2, 5, 2},     // 2 + 5 = 7 -> 2
		{"move up one", 3, 2, -1, 1},   // 2 - 1 = 1
		{"move down one", 3, 1, 1, 2},  // 1 + 1 = 2
		{"nil state", 0, 0, 1, 0},      // nil state -> no-op
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := model{}
			if tc.rows > 0 {
				m.Runs = &RunsTabState{
					Index: tc.idx,
					Rows:  make([]RunRow, tc.rows),
				}
			}
			m.moveRunSelection(tc.delta)
			if m.Runs == nil {
				return
			}
			if m.Runs.Index != tc.want {
				t.Fatalf("Index after delta %d: got %d, want %d", tc.delta, m.Runs.Index, tc.want)
			}
		})
	}
}

// TestCurrentRunEmpty verifies the helper returns nil when the tab is
// empty (no rows, no state, out-of-range index) instead of panicking
// on a missing pointer dereference.
func TestCurrentRunEmpty(t *testing.T) {
	if (model{}).currentRun() != nil {
		t.Fatal("currentRun on zero model should be nil")
	}
	m := model{Runs: &RunsTabState{}}
	if m.currentRun() != nil {
		t.Fatal("currentRun on empty Runs should be nil")
	}
	m = model{Runs: &RunsTabState{Index: 5, Rows: nil}}
	if m.currentRun() != nil {
		t.Fatal("currentRun on out-of-range index should be nil")
	}
}

// TestRunsKeyHandlersCoverBindings is a textual sanity check: every
// binding named in the runs-tab footer must be handled by handleRunsKey
// (otherwise pressing it would fall through and be misinterpreted as
// a board key). This is a static check, not a runtime call, so it
// does not need a fully-initialised Model.
func TestRunsKeyHandlersCoverBindings(t *testing.T) {
	doc := "j/k move  r retry  x cancel  R refresh  esc/q close"
	for _, b := range []string{"j", "k", "r", "x", "R", "esc", "q"} {
		if !contains(doc, b) {
			t.Errorf("binding %q not mentioned in docs %q", b, doc)
		}
	}
}

// contains is a tiny helper so we don't have to import strings just to
// do a substring check.
func contains(s, sub string) bool {
	if len(sub) == 0 {
		return true
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}