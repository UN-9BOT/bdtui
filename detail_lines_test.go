package main

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
	if len(lines) < 2 {
		t.Fatalf("expected at least 2 detail lines, got %d", len(lines))
	}

	keyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	for _, want := range []string{
		keyStyle.Render("Parent:"),
		keyStyle.Render("blockedBy:"),
		keyStyle.Render("blocks:"),
	} {
		if !strings.Contains(lines[0], want) {
			t.Fatalf("expected meta line to contain styled key %q, got %q", want, lines[0])
		}
	}

	if !strings.Contains(lines[1], keyStyle.Render("Description:")) {
		t.Fatalf("expected description line to contain styled key, got %q", lines[1])
	}
}
