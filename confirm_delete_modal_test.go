package main

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func TestRenderDeleteModalShowsBasics(t *testing.T) {
	t.Parallel()

	m := model{
		confirmDelete: &ConfirmDelete{
			IssueID: "bdtui-56i.23",
			Mode:    DeleteModeForce,
			Preview: "line1\nline2",
		},
	}

	out := m.renderDeleteModal()
	if !strings.Contains(out, "Delete Issue") {
		t.Fatalf("expected title, got %q", out)
	}
	if !strings.Contains(out, "bdtui-56i.23") {
		t.Fatalf("expected issue id, got %q", out)
	}
	if !strings.Contains(out, "line1") || !strings.Contains(out, "line2") {
		t.Fatalf("expected preview lines, got %q", out)
	}
}

func TestRenderDeleteModalUsesColors(t *testing.T) {
	prevProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(prevProfile)

	m := model{
		confirmDelete: &ConfirmDelete{
			IssueID: "bdtui-56i.23",
			Mode:    DeleteModeCascade,
			Preview: "preview",
		},
	}

	out := m.renderDeleteModal()
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
}
