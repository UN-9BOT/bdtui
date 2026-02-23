package bdtui_test

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func TestHandleConfirmClosedParentCreateKeyCancel(t *testing.T) {
	t.Parallel()

	m := model{
		Mode: ModeConfirmClosedParentCreate,
		ConfirmClosedParentCreate: &ConfirmClosedParentCreate{
			ParentID:     "bdtui-56i.21",
			ParentTitle:  "closed parent",
			TargetStatus: StatusInProgress,
		},
	}

	next, cmd := m.HandleConfirmClosedParentCreateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	got := next.(model)

	if got.Mode != ModeBoard {
		t.Fatalf("expected mode %s, got %s", ModeBoard, got.Mode)
	}
	if got.ConfirmClosedParentCreate != nil {
		t.Fatalf("expected confirm state to be cleared")
	}
	if got.Toast != "task creation canceled" {
		t.Fatalf("expected cancel toast, got %q", got.Toast)
	}
	if cmd != nil {
		t.Fatalf("expected no cmd on cancel")
	}
}

func TestHandleConfirmClosedParentCreateKeyConfirmReturnsCmd(t *testing.T) {
	t.Parallel()

	m := model{
		Mode:   ModeConfirmClosedParentCreate,
		Client: NewBdClient("."),
		ConfirmClosedParentCreate: &ConfirmClosedParentCreate{
			ParentID:     "bdtui-56i.21",
			ParentTitle:  "closed parent",
			TargetStatus: StatusInProgress,
		},
	}

	next, cmd := m.HandleConfirmClosedParentCreateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	got := next.(model)

	if got.Mode != ModeBoard {
		t.Fatalf("expected mode %s, got %s", ModeBoard, got.Mode)
	}
	if got.ConfirmClosedParentCreate != nil {
		t.Fatalf("expected confirm state to be cleared")
	}
	if cmd == nil {
		t.Fatalf("expected cmd on confirm")
	}
}

func TestUpdateReopenParentForCreateMsgSuccessOpensCreate(t *testing.T) {
	t.Parallel()

	issue := Issue{
		ID:      "bdtui-56i.21",
		Title:   "closed parent",
		Status:  StatusClosed,
		Display: StatusClosed,
	}
	m := model{
		Mode:   ModeBoard,
		Issues: []Issue{issue},
		ByID:   map[string]*Issue{issue.ID: &issue},
		Columns: map[Status][]Issue{
			StatusOpen:       {},
			StatusInProgress: {},
			StatusBlocked:    {},
			StatusClosed:     {issue},
		},
		ColumnDepths: map[Status]map[string]int{
			StatusOpen:       {},
			StatusInProgress: {},
			StatusBlocked:    {},
			StatusClosed:     {},
		},
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
		Filter: Filter{Status: "any", Priority: "any"},
		Client: NewBdClient("."),
	}

	nextModel, cmd := m.Update(reopenParentForCreateMsg{ParentID: issue.ID})
	got := nextModel.(model)

	if got.Mode != ModeCreate {
		t.Fatalf("expected mode %s, got %s", ModeCreate, got.Mode)
	}
	if got.Form == nil {
		t.Fatalf("expected create form")
	}
	if got.Form.Parent != issue.ID {
		t.Fatalf("expected parent %q, got %q", issue.ID, got.Form.Parent)
	}
	if got.Toast != "parent moved to in_progress" {
		t.Fatalf("unexpected Toast: %q", got.Toast)
	}
	if cmd == nil {
		t.Fatalf("expected mutation reload cmd")
	}
}

func TestUpdateReopenParentForCreateMsgError(t *testing.T) {
	t.Parallel()

	m := model{Mode: ModeConfirmClosedParentCreate}
	nextModel, cmd := m.Update(reopenParentForCreateMsg{
		ParentID: "bdtui-56i.21",
		Err:      errors.New("update failed"),
	})
	got := nextModel.(model)

	if got.Mode != ModeBoard {
		t.Fatalf("expected mode %s, got %s", ModeBoard, got.Mode)
	}
	if got.Form != nil {
		t.Fatalf("did not expect create form on error")
	}
	if got.Toast == "" || got.ToastKind != "error" {
		t.Fatalf("expected error toast, got kind=%q msg=%q", got.ToastKind, got.Toast)
	}
	if cmd != nil {
		t.Fatalf("expected no cmd on error")
	}
}

func TestRenderConfirmClosedParentCreateModal(t *testing.T) {
	t.Parallel()

	m := model{
		ConfirmClosedParentCreate: &ConfirmClosedParentCreate{
			ParentID:     "bdtui-56i.21",
			ParentTitle:  "closed parent",
			TargetStatus: StatusInProgress,
		},
	}

	out := m.RenderConfirmClosedParentCreateModal()
	if !strings.Contains(out, "Cannot create issue with closed parent") {
		t.Fatalf("expected warning text, got %q", out)
	}
	if !strings.Contains(out, "y confirm | n/Esc cancel") {
		t.Fatalf("expected controls text, got %q", out)
	}
}

func TestRenderConfirmClosedParentCreateModalUsesColors(t *testing.T) {
	prevProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(prevProfile)

	m := model{
		ConfirmClosedParentCreate: &ConfirmClosedParentCreate{
			ParentID:     "bdtui-56i.22",
			ParentTitle:  "closed parent",
			TargetStatus: StatusInProgress,
		},
	}

	out := m.RenderConfirmClosedParentCreateModal()
	if !strings.Contains(out, "38;5;") {
		t.Fatalf("expected ansi256 colors in modal, got %q", out)
	}

	titleStyled := lipgloss.NewStyle().Foreground(lipgloss.Color("117")).Bold(true).Render("Create From Closed Parent")
	if !strings.Contains(out, titleStyled) {
		t.Fatalf("expected styled title, got %q", out)
	}

	warnStyled := lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true).Render("Cannot create issue with closed parent:")
	if !strings.Contains(out, warnStyled) {
		t.Fatalf("expected styled warning line, got %q", out)
	}

	if !strings.Contains(out, statusHeaderStyle(StatusInProgress).Render(string(StatusInProgress))) {
		t.Fatalf("expected styled target status in_progress, got %q", out)
	}
}
