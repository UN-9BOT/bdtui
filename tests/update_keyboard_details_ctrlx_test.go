package bdtui_test

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestHandleDetailsKeyCtrlXOpensEditorAndSwitchesToEdit(t *testing.T) {
	t.Parallel()

	issue := Issue{
		ID:        "bdtui-56i.16",
		Title:     "details editor hotkey",
		Status:    StatusOpen,
		Display:   StatusOpen,
		Priority:  2,
		IssueType: "task",
		Assignee:  "unbot",
	}

	m := model{
		Mode:   ModeDetails,
		Issues: []Issue{issue},
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
		OpenFormInEditorOverride: func(_ model) (tea.Cmd, error) {
			return func() tea.Msg { return nil }, nil
		},
	}

	next, cmd := m.HandleDetailsKey(tea.KeyMsg{Type: tea.KeyCtrlX})
	got := next.(model)

	if got.Mode != ModeEdit {
		t.Fatalf("expected mode=%s, got %s", ModeEdit, got.Mode)
	}
	if got.Form == nil {
		t.Fatalf("expected edit form to be initialized")
	}
	if got.Form.IssueID != issue.ID {
		t.Fatalf("expected form issue id %q, got %q", issue.ID, got.Form.IssueID)
	}
	if cmd == nil {
		t.Fatalf("expected external editor cmd")
	}
}

func TestHandleDetailsKeyCtrlXNoIssueShowsWarning(t *testing.T) {
	t.Parallel()

	m := model{
		Mode: ModeDetails,
		Columns: map[Status][]Issue{
			StatusOpen:       {},
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

	next, cmd := m.HandleDetailsKey(tea.KeyMsg{Type: tea.KeyCtrlX})
	got := next.(model)

	if got.Mode != ModeDetails {
		t.Fatalf("expected mode to stay details, got %s", got.Mode)
	}
	if got.Toast != "no issue selected" {
		t.Fatalf("expected warning toast, got %q", got.Toast)
	}
	if cmd != nil {
		t.Fatalf("expected nil cmd when no issue is selected")
	}
}

func TestRenderFooterDetailsMentionsCtrlX(t *testing.T) {
	t.Parallel()

	m := model{
		Mode:   ModeDetails,
		Width:  120,
		Height: 30,
		Styles: newStyles(),
	}

	out := m.RenderFooter()
	if !strings.Contains(out, "Ctrl+X ext edit") {
		t.Fatalf("expected details footer to mention Ctrl+X, got %q", out)
	}
}
