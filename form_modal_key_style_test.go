package main

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func TestRenderFormModalCreateHighlightsKeysInGray(t *testing.T) {
	prevProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(prevProfile)

	m := model{
		mode:   ModeCreate,
		width:  120,
		height: 30,
		styles: newStyles(),
		form:   newIssueFormCreate(nil),
	}

	out := m.renderFormModal()
	assertFormModalHasGrayKeys(t, out)
}

func TestRenderFormModalEditHighlightsKeysInGray(t *testing.T) {
	prevProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(prevProfile)

	issue := Issue{
		ID:        "bdtui-56i.12",
		Title:     "demo",
		Status:    StatusInProgress,
		Priority:  2,
		IssueType: "task",
		Assignee:  "unbot",
		Labels:    []string{"ui"},
	}
	m := model{
		mode:   ModeEdit,
		width:  120,
		height: 30,
		styles: newStyles(),
		form:   newIssueFormEdit(&issue, nil),
	}

	out := m.renderFormModal()
	assertFormModalHasGrayKeys(t, out)
}

func assertFormModalHasGrayKeys(t *testing.T, out string) {
	t.Helper()

	keyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	for _, key := range []string{
		"title:",
		"status:",
		"priority:",
		"type:",
		"assignee:",
		"labels:",
		"parent:",
		"description:",
	} {
		if !strings.Contains(out, keyStyle.Render(key)) {
			t.Fatalf("expected form modal to contain gray key %q, got %q", key, out)
		}
	}
}
