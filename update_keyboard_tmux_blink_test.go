package main

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestMarkTmuxPickerSelectionDoesNotBlinkSelectedPane(t *testing.T) {
	runner := &fakeTmuxRunner{results: map[string]fakeTmuxResult{
		"select-pane\x1f-m\x1f-t\x1f%14": {out: ""},
	}}
	plugin := newTmuxPlugin(true, runner)
	plugin.sleepFn = func(_ time.Duration) {}

	m := model{
		plugins: PluginRegistry{tmux: plugin},
		tmuxPicker: &TmuxPickerState{
			Targets: []TmuxTarget{{PaneID: "%14"}},
			Index:   0,
		},
	}

	if err := m.markTmuxPickerSelection(); err != nil {
		t.Fatalf("markTmuxPickerSelection() error = %v", err)
	}
	if m.tmuxPicker.MarkedPaneID != "%14" {
		t.Fatalf("expected marked pane %%14, got %q", m.tmuxPicker.MarkedPaneID)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("expected only mark call, got %d", len(runner.calls))
	}
}

func TestHandleTmuxPickerKeyDownSchedulesParallelBlinkCmd(t *testing.T) {
	runner := &fakeTmuxRunner{results: map[string]fakeTmuxResult{
		"select-pane\x1f-m\x1f-t\x1f%15":                                                  {out: ""},
		"display-message\x1f-p\x1f-t\x1f%15\x1f#{window_id}":                              {out: "@10"},
		"list-panes\x1f-t\x1f@10\x1f-F\x1f#{?pane_active,#{pane_id},}":                    {out: "%14\n"},
		"show-options\x1f-w\x1f-v\x1f-t\x1f@10\x1fwindow-active-style":                    {out: "default"},
		"select-pane\x1f-t\x1f%15":                                                        {out: ""},
		"set-option\x1f-w\x1f-t\x1f@10\x1fwindow-active-style\x1ffg=default,bg=colour160": {out: ""},
		"set-option\x1f-w\x1f-t\x1f@10\x1fwindow-active-style\x1ffg=default,bg=default":   {out: ""},
		"select-pane\x1f-t\x1f%14":                                                        {out: ""},
		"set-option\x1f-w\x1f-t\x1f@10\x1fwindow-active-style\x1fdefault":                 {out: ""},
	}}
	plugin := newTmuxPlugin(true, runner)
	plugin.sleepFn = func(_ time.Duration) {}

	m := model{
		mode:    ModeTmuxPicker,
		plugins: PluginRegistry{tmux: plugin},
		tmuxPicker: &TmuxPickerState{
			Targets: []TmuxTarget{{PaneID: "%14"}, {PaneID: "%15"}},
			Index:   0,
		},
	}

	next, cmd := m.handleTmuxPickerKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	updated := next.(model)
	if cmd == nil {
		t.Fatalf("expected blink cmd")
	}
	if updated.tmuxPicker.Index != 1 {
		t.Fatalf("expected index 1, got %d", updated.tmuxPicker.Index)
	}
	if updated.tmuxPicker.MarkedPaneID != "%15" {
		t.Fatalf("expected marked pane %%15, got %q", updated.tmuxPicker.MarkedPaneID)
	}

	msg := cmd()
	if msg != nil {
		t.Fatalf("expected nil msg on successful async blink, got %#v", msg)
	}
	if len(runner.calls) != 11 {
		t.Fatalf("expected 11 tmux calls (mark + blink), got %d", len(runner.calls))
	}
}
