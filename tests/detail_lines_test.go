package bdtui_test

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestDetailLinesHighlightsMetaKeys(t *testing.T) {
	issue := &Issue{
		Parent:      "bdtui-56i",
		BlockedBy:   []string{"bdtui-1", "bdtui-2"},
		Blocks:      []string{"bdtui-3"},
		Description: "desc",
	}

	lines := detailLines(issue, 120)
	if len(lines) < 1 {
		t.Fatalf("expected at least 1 detail line, got %d", len(lines))
	}

	keyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	for _, unwanted := range []string{"Parent:", "blockedBy:", "blocks:"} {
		if strings.Contains(lines[0], unwanted) {
			t.Fatalf("expected detail lines to omit %q meta key, got %q", unwanted, lines[0])
		}
	}
	if !strings.Contains(lines[0], keyStyle.Render("Description:")) {
		t.Fatalf("expected description line to contain styled key, got %q", lines[0])
	}
}
