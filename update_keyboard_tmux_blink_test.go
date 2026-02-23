package main

import (
	"testing"
	"time"
)

func TestMarkTmuxPickerSelectionBlinksSelectedPane(t *testing.T) {
	runner := &fakeTmuxRunner{results: map[string]fakeTmuxResult{
		"select-pane\x1f-m\x1f-t\x1f%14":                                                  {out: ""},
		"display-message\x1f-p\x1f-t\x1f%14\x1f#{window_id}":                              {out: "@10"},
		"show-options\x1f-w\x1f-v\x1f-t\x1f@10\x1fwindow-active-style":                    {out: "default"},
		"set-option\x1f-w\x1f-t\x1f@10\x1fwindow-active-style\x1ffg=default,bg=colour160": {out: ""},
		"set-option\x1f-w\x1f-t\x1f@10\x1fwindow-active-style\x1ffg=default,bg=default":   {out: ""},
		"set-option\x1f-w\x1f-t\x1f@10\x1fwindow-active-style\x1fdefault":                 {out: ""},
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
	if len(runner.calls) != 8 {
		t.Fatalf("expected 8 tmux calls, got %d", len(runner.calls))
	}
}
