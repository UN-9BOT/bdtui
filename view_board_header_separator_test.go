package main

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
		width:  120,
		height: 30,
		mode:   ModeBoard,
		styles: newStyles(),
		columns: map[Status][]Issue{
			StatusOpen:       {issue},
			StatusInProgress: {},
			StatusBlocked:    {},
			StatusClosed:     {},
		},
		selectedCol: 0,
		selectedIdx: map[Status]int{
			StatusOpen:       0,
			StatusInProgress: 0,
			StatusBlocked:    0,
			StatusClosed:     0,
		},
		scrollOffset: map[Status]int{
			StatusOpen:       0,
			StatusInProgress: 0,
			StatusBlocked:    0,
			StatusClosed:     0,
		},
	}

	out := m.renderBoard()
	if !strings.Contains(out, "Open (1)") {
		t.Fatalf("expected open header in board output")
	}
	if !strings.Contains(out, "────") {
		t.Fatalf("expected divider line below header, got %q", out)
	}
}
