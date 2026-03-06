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

func TestDetailLinesShowsPlainDescriptionPreview(t *testing.T) {
	issue := &Issue{
		Description: "# Header\n\n- one\n- two\n\n`code`",
	}

	lines := detailLines(issue, 80)
	if len(lines) < 2 {
		t.Fatalf("expected multiline details, got %#v", lines)
	}

	keyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	if !strings.Contains(lines[0], keyStyle.Render("Description:")) {
		t.Fatalf("expected styled Description prefix, got %q", lines[0])
	}

	joined := strings.Join(lines, "\n")
	if strings.Contains(joined, "• one") || strings.Contains(joined, "• two") {
		t.Fatalf("expected plain preview in details, got %q", joined)
	}
	if !strings.Contains(joined, "- one") || !strings.Contains(joined, "- two") {
		t.Fatalf("expected raw markdown markers in preview, got %q", joined)
	}
}

func TestDetailLinesShowsEllipsisWhenPreviewClipped(t *testing.T) {
	issue := &Issue{
		Description: "1\n2\n3\n4\n5\n6",
	}

	lines := detailLines(issue, 80)
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "...") {
		t.Fatalf("expected clipped preview hint, got %q", joined)
	}
}
