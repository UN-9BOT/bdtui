package bdtui_test

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestHandleDetailsKeyDOpensDescriptionPreview(t *testing.T) {
	t.Parallel()

	issue := Issue{
		ID:        "bdtui-56i.26",
		Title:     "details with description",
		Status:    StatusOpen,
		Display:   StatusOpen,
		Priority:  2,
		IssueType: "task",
	}

	m := model{
		Mode:        ModeDetails,
		ShowDetails: true,
		DetailsItem: 4,
		Issues:      []Issue{issue},
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

	next, cmd := m.HandleDetailsKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	got := next.(model)

	if got.Mode != ModeDescriptionPreview {
		t.Fatalf("expected mode=%s, got %s", ModeDescriptionPreview, got.Mode)
	}
	if got.DescriptionPreview == nil {
		t.Fatalf("expected description preview state")
	}
	if got.DescriptionPreview.IssueID != issue.ID {
		t.Fatalf("expected issue id=%q, got %q", issue.ID, got.DescriptionPreview.IssueID)
	}
	if got.DescriptionPreview.Scroll != 0 {
		t.Fatalf("expected scroll=0, got %d", got.DescriptionPreview.Scroll)
	}
	if cmd != nil {
		t.Fatalf("expected nil cmd")
	}
}

func TestHandleDetailsKeyEnterAndSpaceDoNothing(t *testing.T) {
	t.Parallel()

	m := model{
		Mode:        ModeDetails,
		ShowDetails: true,
		DetailsItem: 4,
	}

	next, cmd := m.HandleDetailsKey(tea.KeyMsg{Type: tea.KeyEnter})
	got := next.(model)
	if got.Mode != ModeDetails {
		t.Fatalf("expected mode=%s after enter, got %s", ModeDetails, got.Mode)
	}
	if got.DescriptionPreview != nil {
		t.Fatalf("expected enter to keep description preview closed")
	}
	if cmd != nil {
		t.Fatalf("expected nil cmd for enter")
	}

	next, cmd = got.HandleDetailsKey(tea.KeyMsg{Type: tea.KeySpace})
	got = next.(model)
	if got.Mode != ModeDetails {
		t.Fatalf("expected mode=%s after space, got %s", ModeDetails, got.Mode)
	}
	if got.DescriptionPreview != nil {
		t.Fatalf("expected space to keep description preview closed")
	}
	if cmd != nil {
		t.Fatalf("expected nil cmd for space")
	}
}

func TestHandleKeyDescriptionPreviewEscReturnsToDetails(t *testing.T) {
	t.Parallel()

	m := model{
		Mode:               ModeDescriptionPreview,
		ShowDetails:        true,
		DetailsItem:        4,
		DescriptionPreview: &DescriptionPreviewState{IssueID: "bdtui-56i.26", Scroll: 2},
	}

	next, cmd := m.HandleKey(tea.KeyMsg{Type: tea.KeyEsc})
	got := next.(model)

	if got.Mode != ModeDetails {
		t.Fatalf("expected mode=%s, got %s", ModeDetails, got.Mode)
	}
	if cmd != nil {
		t.Fatalf("expected nil cmd")
	}
}

func TestHandleKeyDescriptionPreviewCtrlXSetsResumeFlag(t *testing.T) {
	t.Parallel()

	issue := Issue{
		ID:        "bdtui-56i.26",
		Title:     "details with description",
		Status:    StatusOpen,
		Display:   StatusOpen,
		Priority:  2,
		IssueType: "task",
	}

	m := model{
		Mode:               ModeDescriptionPreview,
		ShowDetails:        true,
		DetailsIssueID:     issue.ID,
		DetailsItem:        4,
		DescriptionPreview: &DescriptionPreviewState{IssueID: issue.ID, Scroll: 3},
		Issues:             []Issue{issue},
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

	next, cmd := m.HandleKey(tea.KeyMsg{Type: tea.KeyCtrlX})
	got := next.(model)

	if got.Mode != ModeEdit {
		t.Fatalf("expected mode=%s, got %s", ModeEdit, got.Mode)
	}
	if !got.ResumeDescriptionAfterEditor {
		t.Fatalf("expected ResumeDescriptionAfterEditor=true")
	}
	if got.ResumeDescriptionScroll != 3 {
		t.Fatalf("expected ResumeDescriptionScroll=3, got %d", got.ResumeDescriptionScroll)
	}
	if cmd == nil {
		t.Fatalf("expected external editor cmd")
	}
}
