package bdtui_test

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func TestRenderInspectorDetailsModeHighlightsBorder(t *testing.T) {
	prevProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(prevProfile)

	m := model{
		Width:  100,
		Height: 30,
		Mode:   ModeBoard,
		Styles: newStyles(),
	}

	boardInspector := m.RenderInspector()
	if !strings.Contains(boardInspector, "38;5;241m") {
		t.Fatalf("expected board mode inspector to use neutral border color, got %q", boardInspector)
	}
	if strings.Contains(boardInspector, "38;5;39m") {
		t.Fatalf("did not expect active border color in board mode, got %q", boardInspector)
	}

	m.Mode = ModeDetails
	detailsInspector := m.RenderInspector()
	if !strings.Contains(detailsInspector, "38;5;39m") {
		t.Fatalf("expected details mode inspector to use active border color, got %q", detailsInspector)
	}
}

func TestRenderInspectorDetailsModeDoesNotHighlightDescriptionCursor(t *testing.T) {
	prevProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(prevProfile)

	issue := Issue{
		ID:          "bdtui-56i.30",
		Title:       "details without cursor",
		Status:      StatusOpen,
		Display:     StatusOpen,
		Priority:    2,
		IssueType:   "task",
		Description: "line 1\nline 2",
	}

	m := model{
		Width:       100,
		Height:      30,
		Mode:        ModeDetails,
		ShowDetails: true,
		DetailsItem: 4,
		Styles:      newStyles(),
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
	}

	out := m.RenderInspector()
	if strings.Contains(out, "48;5;31m") {
		t.Fatalf("expected details inspector to render without selected-line cursor, got %q", out)
	}
}
