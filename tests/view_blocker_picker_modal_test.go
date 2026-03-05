package bdtui_test

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func TestRenderBlockerPickerModalNoGhostOrParentTree(t *testing.T) {
	prevProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() {
		lipgloss.SetColorProfile(prevProfile)
	})

	target := Issue{ID: "bdtui-100", Title: "target", Display: StatusOpen, Status: StatusOpen}
	parent := Issue{ID: "bdtui-200", Title: "parent", Display: StatusOpen, Status: StatusOpen}
	child := Issue{ID: "bdtui-201", Title: "child", Parent: parent.ID, Display: StatusOpen, Status: StatusOpen}

	m := model{
		Width:  120,
		Height: 36,
		Mode:   ModeBlockerPicker,
		Styles: newStyles(),
		BlockerPicker: &BlockerPickerState{
			TargetIssueID: target.ID,
			Columns: map[Status][]Issue{
				StatusOpen:       {parent, child},
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
			ScrollOffset: map[Status]int{
				StatusOpen:       0,
				StatusInProgress: 0,
				StatusBlocked:    0,
				StatusClosed:     0,
			},
			Original: map[string]bool{},
			Selected: map[string]bool{
				parent.ID: true,
			},
		},
	}

	out := m.RenderBlockerPickerModal()
	if strings.Contains(out, "↳") {
		t.Fatalf("expected blocker picker modal to render without tree prefixes, got %q", out)
	}

	parentLine := lineContainingBlockerPicker(out, parent.ID)
	if parentLine == "" {
		t.Fatalf("expected marked blocker row with id %q, got %q", parent.ID, out)
	}
	if !strings.Contains(parentLine, "[x]") {
		t.Fatalf("expected checked checkbox for marked blocker, got %q", parentLine)
	}
	if !strings.Contains(parentLine, "48;5;31m") {
		t.Fatalf("expected selected cursor highlight on current row, got %q", parentLine)
	}

	childLine := lineContainingBlockerPicker(out, child.ID)
	if childLine == "" {
		t.Fatalf("expected unmarked blocker row with id %q, got %q", child.ID, out)
	}
	if !strings.Contains(childLine, "[ ]") {
		t.Fatalf("expected unchecked checkbox for unmarked blocker, got %q", childLine)
	}
	if strings.Contains(childLine, "48;5;67m") {
		t.Fatalf("did not expect old marked background style, got %q", childLine)
	}
}

func lineContainingBlockerPicker(s, needle string) string {
	for _, line := range strings.Split(s, "\n") {
		if strings.Contains(line, needle) {
			return line
		}
	}
	return ""
}
