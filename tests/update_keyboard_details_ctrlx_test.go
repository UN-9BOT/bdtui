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
	if !strings.Contains(out, "d open description") {
		t.Fatalf("expected details footer to mention d open description, got %q", out)
	}
	if !strings.Contains(out, "n open notes") {
		t.Fatalf("expected details footer to mention n open notes, got %q", out)
	}
	if !strings.Contains(out, "Ctrl+X ext edit") {
		t.Fatalf("expected details footer to mention Ctrl+X, got %q", out)
	}
	if strings.Contains(out, "j/k select item") || strings.Contains(out, "Enter/Space open description") {
		t.Fatalf("expected details footer to omit removed details hotkeys, got %q", out)
	}
}

func TestHandleDetailsKeyJKDoNotMoveItemFocus(t *testing.T) {
	t.Parallel()

	m := model{
		Mode:        ModeDetails,
		ShowDetails: true,
		DetailsItem: 3,
	}

	next, _ := m.HandleDetailsKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	got := next.(model)
	if got.DetailsItem != 3 {
		t.Fatalf("expected details item to stay on description, got %d", got.DetailsItem)
	}

	next, _ = got.HandleDetailsKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	got = next.(model)
	if got.DetailsItem != 3 {
		t.Fatalf("expected details item to stay on description, got %d", got.DetailsItem)
	}
}

func TestHandleDetailsKeyQDoesNothing(t *testing.T) {
	t.Parallel()

	m := model{
		Mode:        ModeDetails,
		ShowDetails: true,
		DetailsItem: 3,
	}

	next, cmd := m.HandleDetailsKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	got := next.(model)
	if got.Mode != ModeDetails {
		t.Fatalf("expected mode=%s, got %s", ModeDetails, got.Mode)
	}
	if cmd != nil {
		t.Fatalf("expected nil cmd for q in details")
	}
}

func TestHandleBoardKeyEnterSetsDetailsFocusToDescription(t *testing.T) {
	t.Parallel()

	issue := Issue{
		ID:        "bdtui-56i.30",
		Title:     "details focus on description",
		Status:    StatusOpen,
		Display:   StatusOpen,
		Priority:  2,
		IssueType: "task",
	}

	m := model{
		Mode:   ModeBoard,
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
	}

	next, cmd := m.HandleKey(tea.KeyMsg{Type: tea.KeyEnter})
	got := next.(model)
	if got.Mode != ModeDetails {
		t.Fatalf("expected mode=%s, got %s", ModeDetails, got.Mode)
	}
	if got.DetailsItem != 3 {
		t.Fatalf("expected details item=3, got %d", got.DetailsItem)
	}
	if cmd != nil {
		t.Fatalf("expected nil cmd")
	}
}
