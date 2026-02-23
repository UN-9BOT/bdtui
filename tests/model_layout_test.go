package bdtui_test

import "testing"

func TestInspectorInnerHeightCollapsed(t *testing.T) {
	t.Parallel()

	m := model{Height: 40, ShowDetails: false}
	if got := m.InspectorInnerHeight(); got != 3 {
		t.Fatalf("expected collapsed inspector inner height 3, got %d", got)
	}
}

func TestInspectorInnerHeightExpandedUsesFortyPercent(t *testing.T) {
	t.Parallel()

	m := model{Height: 40, ShowDetails: true}

	if got := m.InspectorOuterHeight(); got != 16 {
		t.Fatalf("expected expanded inspector outer height 16 (40%%), got %d", got)
	}
	if got := m.InspectorInnerHeight(); got != 14 {
		t.Fatalf("expected expanded inspector inner height 14, got %d", got)
	}
	if got := m.DetailsViewportHeight(); got != 11 {
		t.Fatalf("expected details viewport height 11, got %d", got)
	}
}

func TestInspectorExpandedKeepsBoardUsableOnShortScreens(t *testing.T) {
	t.Parallel()

	m := model{Height: 15, ShowDetails: true}

	if got := m.InspectorOuterHeight(); got != 5 {
		t.Fatalf("expected clamped inspector outer height 5, got %d", got)
	}
	if got := m.BoardInnerHeight(); got != 6 {
		t.Fatalf("expected board inner height 6, got %d", got)
	}
}
