package bdtui_test

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func TestRenderWorkflowPickerModalListsOriginTags(t *testing.T) {
	prevProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() {
		lipgloss.SetColorProfile(prevProfile)
	})

	m := model{
		Width:  120,
		Height: 30,
		Mode:   ModeWorkflowPicker,
		Styles: newStyles(),
		WorkflowPicker: &WorkflowPickerState{
			TargetIssueID: "bdtui-100",
			Index:         1,
			Options: []WorkflowOption{
				{Name: "fix-bug", Origin: "project"},
				{Name: "ship", Origin: "global"},
			},
		},
	}

	out := m.RenderWorkflowPickerModal()
	if !strings.Contains(out, "Workflow Picker") {
		t.Fatalf("missing header, got %q", out)
	}
	if !strings.Contains(out, "[project]") {
		t.Fatalf("expected [project] tag, got %q", out)
	}
	if !strings.Contains(out, "[global]") {
		t.Fatalf("expected [global] tag, got %q", out)
	}
	if !strings.Contains(out, "fix-bug") || !strings.Contains(out, "ship") {
		t.Fatalf("missing workflow names, got %q", out)
	}
	if !strings.Contains(out, "bdtui-100") {
		t.Fatalf("missing target task id, got %q", out)
	}
}

func TestRenderWorkflowPickerModalEmpty(t *testing.T) {
	m := model{
		Width:  120,
		Height: 30,
		Mode:   ModeWorkflowPicker,
		Styles: newStyles(),
		WorkflowPicker: &WorkflowPickerState{
			TargetIssueID: "bdtui-100",
		},
	}

	out := m.RenderWorkflowPickerModal()
	if !strings.Contains(out, "No workflows available") {
		t.Fatalf("expected empty-state message, got %q", out)
	}
}