package bdtui_test

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestHandleMuxPickerKeyDownMovesSelectionWithoutPreviewCmd(t *testing.T) {
	m := model{
		Mode:    ModeMuxPicker,
		Plugins: PluginRegistry{HerdrPlugin: newHerdrPlugin(true, &fakeHerdrRunner{})},
		MuxPicker: &MuxPickerState{
			Targets: []MuxTarget{{PaneID: "pane-1"}, {PaneID: "pane-2"}},
			Index:   0,
		},
	}

	next, cmd := m.HandleMuxPickerKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	updated := next.(model)
	if cmd != nil {
		t.Fatalf("expected no preview cmd")
	}
	if updated.MuxPicker.Index != 1 {
		t.Fatalf("expected index 1, got %d", updated.MuxPicker.Index)
	}
}

func TestHandleMuxPickerEscapeClosesWithoutCleanupCmd(t *testing.T) {
	m := model{
		Mode: ModeMuxPicker,
		MuxPicker: &MuxPickerState{
			Targets: []MuxTarget{{PaneID: "pane-1"}},
		},
	}

	next, cmd := m.HandleMuxPickerKey(tea.KeyMsg{Type: tea.KeyEsc})
	updated := next.(model)
	if cmd != nil {
		t.Fatalf("expected no cleanup cmd")
	}
	if updated.Mode != ModeBoard {
		t.Fatalf("expected mode %s, got %s", ModeBoard, updated.Mode)
	}
	if updated.MuxPicker != nil {
		t.Fatalf("expected picker to close")
	}
}
