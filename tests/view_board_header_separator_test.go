package bdtui_test

import (
	"strings"
	"testing"
)

func TestRenderBoardAddsHeaderSeparator(t *testing.T) {
	t.Parallel()

	issue := Issue{
		ID:        "bdtui-cmo.1",
		Title:     "header divider",
		Priority:  2,
		IssueType: "task",
		Display:   StatusOpen,
	}

	m := model{
		Width:  120,
		Height: 30,
		Mode:   ModeBoard,
		Styles: newStyles(),
		Columns: map[Status][]Issue{
			StatusOpen:       {issue},
			StatusInProgress: {},
			StatusBlocked:    {},
			StatusClosed:     {},
		},
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

	out := m.RenderBoard()
	if !strings.Contains(out, "Open (1)") {
		t.Fatalf("expected open header in board output")
	}
	if !strings.Contains(out, "────") {
		t.Fatalf("expected divider line below header, got %q", out)
	}
}
