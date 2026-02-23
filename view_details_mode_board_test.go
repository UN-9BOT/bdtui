package main

import (
	"strings"
	"testing"
)

func TestRenderBoardDetailsModeDimsKanban(t *testing.T) {
	t.Parallel()

	m := model{
		width:  120,
		height: 30,
		mode:   ModeBoard,
		styles: newStyles(),
		columns: map[Status][]Issue{
			StatusOpen: {
				{
					ID:        "bdtui-ppr",
					Title:     "dim board in details mode",
					Priority:  2,
					IssueType: "task",
					Display:   StatusOpen,
				},
			},
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

	board := m.renderBoard()
	if strings.Contains(board, "bdtui-ppr") {
		t.Fatalf("expected selected row in board mode to hide id, got %q", board)
	}

	m.mode = ModeDetails
	detailsBoard := m.renderBoard()
	if !strings.Contains(detailsBoard, "bdtui-ppr") {
		t.Fatalf("expected details mode row to render id via gray/plain row style, got %q", detailsBoard)
	}
}
