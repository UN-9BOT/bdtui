package bdtui_test

import (
	"strings"
	"testing"
)

func TestRenderBoardDetailsModeDimsKanban(t *testing.T) {
	t.Parallel()

	m := model{
		Width:  120,
		Height: 30,
		Mode:   ModeBoard,
		Styles: newStyles(),
		Columns: map[Status][]Issue{
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

	board := m.RenderBoard()
	if strings.Contains(board, "bdtui-ppr") {
		t.Fatalf("expected selected row in board mode to hide id, got %q", board)
	}

	m.Mode = ModeDetails
	detailsBoard := m.RenderBoard()
	if !strings.Contains(detailsBoard, "bdtui-ppr") {
		t.Fatalf("expected details mode row to render id via gray/plain row style, got %q", detailsBoard)
	}
}
