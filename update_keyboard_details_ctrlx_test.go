package main

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
		mode:   ModeDetails,
		issues: []Issue{issue},
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
		openFormInEditorOverride: func(_ model) (tea.Cmd, error) {
			return func() tea.Msg { return nil }, nil
		},
	}

	next, cmd := m.handleDetailsKey(tea.KeyMsg{Type: tea.KeyCtrlX})
	got := next.(model)

	if got.mode != ModeEdit {
		t.Fatalf("expected mode=%s, got %s", ModeEdit, got.mode)
	}
	if got.form == nil {
		t.Fatalf("expected edit form to be initialized")
	}
	if got.form.IssueID != issue.ID {
		t.Fatalf("expected form issue id %q, got %q", issue.ID, got.form.IssueID)
	}
	if cmd == nil {
		t.Fatalf("expected external editor cmd")
	}
}

func TestHandleDetailsKeyCtrlXNoIssueShowsWarning(t *testing.T) {
	t.Parallel()

	m := model{
		mode: ModeDetails,
		columns: map[Status][]Issue{
			StatusOpen:       {},
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
	}

	next, cmd := m.handleDetailsKey(tea.KeyMsg{Type: tea.KeyCtrlX})
	got := next.(model)

	if got.mode != ModeDetails {
		t.Fatalf("expected mode to stay details, got %s", got.mode)
	}
	if got.toast != "no issue selected" {
		t.Fatalf("expected warning toast, got %q", got.toast)
	}
	if cmd != nil {
		t.Fatalf("expected nil cmd when no issue is selected")
	}
}

func TestRenderFooterDetailsMentionsCtrlX(t *testing.T) {
	t.Parallel()

	m := model{
		mode:   ModeDetails,
		width:  120,
		height: 30,
		styles: newStyles(),
	}

	out := m.renderFooter()
	if !strings.Contains(out, "Ctrl+X ext edit") {
		t.Fatalf("expected details footer to mention Ctrl+X, got %q", out)
	}
}
