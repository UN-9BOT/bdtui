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
		Notes:       "note",
	}

	lines := detailLines(issue, 120, false)
	if len(lines) < 2 {
		t.Fatalf("expected two detail lines, got %d", len(lines))
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
	if !strings.Contains(lines[1], keyStyle.Render("Notes:")) {
		t.Fatalf("expected notes line to contain styled key, got %q", lines[1])
	}
}

func TestDetailLinesCollapsedShowsSingleLinePreviewForDescriptionAndNotes(t *testing.T) {
	issue := &Issue{
		Description: "\n  # Header\n\n- one\n- two\n\n`code`",
		Notes:       "\n\n  first note line\nsecond note line",
	}

	lines := detailLines(issue, 80, false)
	if len(lines) < 2 {
		t.Fatalf("expected compact detail lines, got %#v", lines)
	}

	keyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	if !strings.Contains(lines[0], keyStyle.Render("Description:")) {
		t.Fatalf("expected styled Description prefix, got %q", lines[0])
	}
	if !strings.Contains(lines[1], keyStyle.Render("Notes:")) {
		t.Fatalf("expected styled Notes prefix, got %q", lines[1])
	}

	joined := strings.Join(lines, "\n")
	if strings.Contains(joined, "\n\n") {
		t.Fatalf("expected single-line compact preview, got %q", joined)
	}
	if !strings.Contains(lines[0], "# Header - one - two `code`") {
		t.Fatalf("expected collapsed plain description preview, got %q", lines[0])
	}
	if !strings.Contains(lines[1], "first note line second note line") {
		t.Fatalf("expected collapsed plain notes preview, got %q", lines[1])
	}
}

func TestDetailLinesShowsEllipsisWhenPreviewClipped(t *testing.T) {
	issue := &Issue{
		Description: "\n\n  12345678901234567890",
		Notes:       "\n  abcdefghijklmnopqrst",
	}

	lines := detailLines(issue, 20, false)
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "…") {
		t.Fatalf("expected clipped preview hint, got %q", joined)
	}
}

func TestDetailLinesShowsDashWhenPreviewIsEmptyAfterLeadingTrim(t *testing.T) {
	issue := &Issue{
		Description: " \n\t  ",
		Notes:       "\n\n",
	}

	lines := detailLines(issue, 40, false)
	if !strings.Contains(lines[0], "Description: -") {
		t.Fatalf("expected empty description preview to render dash, got %q", lines[0])
	}
	if !strings.Contains(lines[1], "Notes: -") {
		t.Fatalf("expected empty notes preview to render dash, got %q", lines[1])
	}
}

func TestDetailLinesExpandedShowsFiveLinePreviewForDescriptionAndNotes(t *testing.T) {
	issue := &Issue{
		Description: "- one\n- two\n- three\n- four\n- five\n- six",
		Notes:       "- alpha\n- beta\n- gamma\n- delta\n- omega\n- zeta",
	}

	lines := detailLines(issue, 40, true)
	joined := strings.Join(lines, "\n")
	if !strings.Contains(lines[0], "Description:") || !strings.Contains(lines[0], "• one") {
		t.Fatalf("expected first expanded description line, got %q", lines[0])
	}
	if !strings.Contains(joined, "• four") {
		t.Fatalf("expected markdown-rendered description list item, got %q", joined)
	}
	if !strings.Contains(joined, "• alpha") || !strings.Contains(joined, "• delta") {
		t.Fatalf("expected markdown-rendered notes list item, got %q", joined)
	}
	if strings.Contains(joined, "Description: - one") || strings.Contains(joined, "Notes: - alpha") {
		t.Fatalf("expected markdown markers instead of raw list syntax, got %q", joined)
	}
	if strings.Contains(joined, "\n...") {
		t.Fatalf("did not expect separate ellipsis line in expanded preview, got %q", joined)
	}
	if !strings.Contains(joined, "• five") || !strings.Contains(joined, "• omega") || !strings.Contains(joined, "…") {
		t.Fatalf("expected inline clipped hint on fifth line, got %q", joined)
	}
}
