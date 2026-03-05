package bdtui_test

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func TestRenderBoardHighlightsBlockedByRowsForSelectedIssue(t *testing.T) {
	prevProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() {
		lipgloss.SetColorProfile(prevProfile)
	})

	blocker := Issue{
		ID:        "bdtui-1",
		Title:     "blocker",
		Priority:  1,
		IssueType: "task",
		Display:   StatusOpen,
		Status:    StatusOpen,
	}
	other := Issue{
		ID:        "bdtui-2",
		Title:     "other",
		Priority:  2,
		IssueType: "task",
		Display:   StatusOpen,
		Status:    StatusOpen,
	}
	selected := Issue{
		ID:        "bdtui-3",
		Title:     "selected",
		Priority:  2,
		IssueType: "task",
		Display:   StatusBlocked,
		Status:    StatusBlocked,
		BlockedBy: []string{blocker.ID},
	}

	m := model{
		Width:  120,
		Height: 30,
		Mode:   ModeBoard,
		Styles: newStyles(),
		Columns: map[Status][]Issue{
			StatusOpen:       {blocker, other},
			StatusInProgress: {},
			StatusBlocked:    {selected},
			StatusClosed:     {},
		},
		ColumnDepths: map[Status]map[string]int{
			StatusOpen:       {blocker.ID: 0, other.ID: 0},
			StatusInProgress: {},
			StatusBlocked:    {selected.ID: 0},
			StatusClosed:     {},
		},
		ByID: map[string]*Issue{
			blocker.ID:  &blocker,
			other.ID:    &other,
			selected.ID: &selected,
		},
		SelectedCol: 2,
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
	blockerLine := lineContaining(out, blocker.ID)
	if blockerLine == "" {
		t.Fatalf("expected blocker row in board output, got %q", out)
	}
	if !strings.Contains(blockerLine, "48;5;160m") {
		t.Fatalf("expected blocker row to be highlighted with blockedBy background, got %q", blockerLine)
	}

	otherLine := lineContaining(out, other.ID)
	if otherLine == "" {
		t.Fatalf("expected non-blocker row in board output, got %q", out)
	}
	if strings.Contains(otherLine, "48;5;160m") {
		t.Fatalf("did not expect non-blocker row to use blockedBy highlight, got %q", otherLine)
	}
}

func lineContaining(s, needle string) string {
	for _, line := range strings.Split(s, "\n") {
		if strings.Contains(line, needle) {
			return line
		}
	}
	return ""
}
