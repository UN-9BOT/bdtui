package bdtui_test

import (
	"strings"
	"testing"
)

func TestRenderFooterShowsCapIndicatorAtLoadedLimit(t *testing.T) {
	t.Parallel()

	issues := make([]Issue, 200)
	m := model{
		Mode:        ModeBoard,
		Width:       400,
		Height:      30,
		Issues:      issues,
		LoadedLimit: 200,
		Styles:      newStyles(),
	}

	if got := m.RenderFooter(); !strings.Contains(got, "loaded 200 (capped at 200)") {
		t.Fatalf("expected cap indicator, got %q", got)
	}
}

func TestRenderFooterHidesCapIndicatorBelowLoadedLimit(t *testing.T) {
	t.Parallel()

	m := model{
		Mode:        ModeBoard,
		Width:       400,
		Height:      30,
		Issues:      make([]Issue, 199),
		LoadedLimit: 200,
		Styles:      newStyles(),
	}

	if got := m.RenderFooter(); strings.Contains(got, "capped at") {
		t.Fatalf("unexpected cap indicator, got %q", got)
	}
}
