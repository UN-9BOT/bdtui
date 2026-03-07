package bdtui_test

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func deleteModalIssueIndex(issues []Issue) map[string]*Issue {
	byID := make(map[string]*Issue, len(issues))
	for i := range issues {
		byID[issues[i].ID] = &issues[i]
	}
	return byID
}

func TestRenderDeleteModalShowsBasics(t *testing.T) {
	t.Parallel()

	m := model{
		ConfirmDelete: &ConfirmDelete{
			IssueID: "bdtui-56i.23",
			Mode:    DeleteModeForce,
			Preview: "line1\nline2",
		},
	}

	out := m.RenderDeleteModal()
	if !strings.Contains(out, "Delete Issue") {
		t.Fatalf("expected title, got %q", out)
	}
	if !strings.Contains(out, "bdtui-56i.23") {
		t.Fatalf("expected issue id, got %q", out)
	}
	if !strings.Contains(out, "line1") || !strings.Contains(out, "line2") {
		t.Fatalf("expected preview lines, got %q", out)
	}
	if !strings.Contains(out, "bd delete bdtui-56i.23 --force") {
		t.Fatalf("expected force command, got %q", out)
	}
}

func TestRenderDeleteModalUsesColors(t *testing.T) {
	prevProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(prevProfile)

	m := model{
		ConfirmDelete: &ConfirmDelete{
			IssueID: "bdtui-56i.23",
			Mode:    DeleteModeCascade,
			Preview: "preview",
		},
	}

	out := m.RenderDeleteModal()
	if !strings.Contains(out, "38;5;") {
		t.Fatalf("expected ansi256 colors in modal, got %q", out)
	}

	titleStyled := lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Bold(true).Render("Delete Issue")
	if !strings.Contains(out, titleStyled) {
		t.Fatalf("expected styled title, got %q", out)
	}

	cascadeStyled := lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true).Render("2 cascade")
	if !strings.Contains(out, cascadeStyled) {
		t.Fatalf("expected styled cascade option, got %q", out)
	}
	if !strings.Contains(out, "bd delete bdtui-56i.23 --force --cascade") {
		t.Fatalf("expected cascade command, got %q", out)
	}
}

func TestRenderDeleteModalShowsCascadeTargetsSection(t *testing.T) {
	t.Parallel()

	issues := []Issue{
		{
			ID:       "bdtui-56i",
			Title:    "Epic",
			Children: []string{"bdtui-56i.33", "bdtui-56i.34"},
		},
		{
			ID:       "bdtui-56i.33",
			Title:    "Child task",
			Parent:   "bdtui-56i",
			Children: []string{"bdtui-56i.33.1"},
		},
		{
			ID:     "bdtui-56i.34",
			Title:  "Sibling task",
			Parent: "bdtui-56i",
		},
		{
			ID:     "bdtui-56i.33.1",
			Title:  "Grandchild task",
			Parent: "bdtui-56i.33",
		},
	}

	m := model{
		Issues: issues,
		ByID:   deleteModalIssueIndex(issues),
		ConfirmDelete: &ConfirmDelete{
			IssueID: "bdtui-56i",
			Mode:    DeleteModeCascade,
			Preview: "preview",
		},
	}

	out := m.RenderDeleteModal()
	if !strings.Contains(out, "Cascade delete targets (3):") {
		t.Fatalf("expected cascade targets section, got %q", out)
	}
	for _, expected := range []string{
		"bdtui-56i.33: Child task",
		"bdtui-56i.34: Sibling task",
		"bdtui-56i.33.1: Grandchild task",
	} {
		if !strings.Contains(out, expected) {
			t.Fatalf("expected cascade target %q, got %q", expected, out)
		}
	}
}

func TestRenderDeleteModalHidesCascadeTargetsOutsideCascadeMode(t *testing.T) {
	t.Parallel()

	issues := []Issue{
		{
			ID:       "bdtui-56i",
			Title:    "Epic",
			Children: []string{"bdtui-56i.33"},
		},
		{
			ID:     "bdtui-56i.33",
			Title:  "Child task",
			Parent: "bdtui-56i",
		},
	}

	m := model{
		Issues: issues,
		ByID:   deleteModalIssueIndex(issues),
		ConfirmDelete: &ConfirmDelete{
			IssueID: "bdtui-56i",
			Mode:    DeleteModeForce,
			Preview: "preview",
		},
	}

	out := m.RenderDeleteModal()
	if strings.Contains(out, "Cascade delete targets") {
		t.Fatalf("expected no cascade targets section in force mode, got %q", out)
	}
}
