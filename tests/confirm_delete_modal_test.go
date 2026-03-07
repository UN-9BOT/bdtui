package bdtui_test

import (
	"regexp"
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
		Width: 90,
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
	if !strings.Contains(out, "title: -") {
		t.Fatalf("expected empty title placeholder, got %q", out)
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
		Width: 120,
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
			Children: []string{"bdtui-56i.34", "bdtui-56i.33"},
		},
		{
			ID:       "bdtui-56i.33",
			Title:    "Child task with longer title",
			Parent:   "bdtui-56i",
			Children: []string{"bdtui-56i.33.1"},
		},
		{
			ID:     "bdtui-56i.34",
			Title:  "Sibling task with longer title",
			Parent: "bdtui-56i",
		},
		{
			ID:     "bdtui-56i.33.1",
			Title:  "Grandchild task with longer title",
			Parent: "bdtui-56i.33",
		},
	}

	m := model{
		Width:  120,
		Issues: issues,
		ByID:   deleteModalIssueIndex(issues),
		ConfirmDelete: &ConfirmDelete{
			IssueID: "bdtui-56i",
			Mode:    DeleteModeCascade,
			Preview: "preview",
		},
	}

	out := m.RenderDeleteModal()
	if !strings.Contains(out, "title: Epic") {
		t.Fatalf("expected root title line, got %q", out)
	}
	if !strings.Contains(out, "Cascade delete targets (3):") {
		t.Fatalf("expected cascade targets section, got %q", out)
	}
	for _, pattern := range []string{
		`bdtui-56i\.33\s+Child task with longer title`,
		`bdtui-56i\.33\.1\s+Grandchild task with longer title`,
		`bdtui-56i\.34\s+Sibling task with longer title`,
	} {
		if !regexp.MustCompile(pattern).MatchString(out) {
			t.Fatalf("expected cascade target pattern %q, got %q", pattern, out)
		}
	}
	idxChild := strings.Index(out, "bdtui-56i.33")
	idxGrandchild := strings.Index(out, "bdtui-56i.33.1")
	idxSibling := strings.Index(out, "bdtui-56i.34")
	if !(idxChild >= 0 && idxGrandchild > idxChild && idxSibling > idxGrandchild) {
		t.Fatalf("expected cascade targets sorted by id, got %q", out)
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
		Width:  120,
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

func TestRenderDeleteModalAddsTitlesToDependencyPreviewRows(t *testing.T) {
	t.Parallel()

	issues := []Issue{
		{
			ID:    "bdtui-l7b",
			Title: "notes support in bdtui",
		},
		{
			ID:     "bdtui-l7b.1",
			Title:  "Dashboard indicator with longer title",
			Parent: "bdtui-l7b",
		},
	}

	m := model{
		Width:  120,
		Issues: issues,
		ByID:   deleteModalIssueIndex(issues),
		ConfirmDelete: &ConfirmDelete{
			IssueID: "bdtui-l7b",
			Mode:    DeleteModeForce,
			Preview: strings.Join([]string{
				"⚠️  DELETE PREVIEW",
				"",
				"Dependency links to remove: 1",
				"  bdtui-l7b.1 -> bdtui-l7b (inbound)",
			}, "\n"),
		},
	}

	out := m.RenderDeleteModal()
	if !strings.Contains(out, "title: notes support in bdtui") {
		t.Fatalf("expected root title line, got %q", out)
	}
	if !regexp.MustCompile(`Dashboard indicator with longer tit`).MatchString(out) {
		t.Fatalf("expected dependency row title enrichment, got %q", out)
	}
}

func TestRenderDeleteModalTruncatesDependencyRowWithoutTitleOnNarrowWidth(t *testing.T) {
	t.Parallel()

	issues := []Issue{
		{
			ID:    "bdtui-l7b",
			Title: "notes support in bdtui",
		},
		{
			ID:     "bdtui-l7b.1",
			Parent: "bdtui-l7b",
		},
	}

	row := "  bdtui-l7b.1 -> bdtui-l7b (inbound)"
	m := model{
		Width:  40,
		Issues: issues,
		ByID:   deleteModalIssueIndex(issues),
		ConfirmDelete: &ConfirmDelete{
			IssueID: "bdtui-l7b",
			Mode:    DeleteModeForce,
			Preview: row,
		},
	}

	out := m.RenderDeleteModal()
	if !strings.Contains(out, "bdtui-l7b.1 -> bdtui-l7b (i…") {
		t.Fatalf("expected dependency row truncated in single line fallback, got %q", out)
	}
	if strings.Contains(out, "Dashboard indicator") {
		t.Fatalf("expected no injected title when title missing, got %q", out)
	}
}
