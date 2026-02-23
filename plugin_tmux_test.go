package main

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

type fakeTmuxResult struct {
	out string
	err error
}

type fakeTmuxRunner struct {
	results map[string]fakeTmuxResult
	calls   [][]string
}

func (f *fakeTmuxRunner) Run(args ...string) (string, error) {
	cloned := append([]string(nil), args...)
	f.calls = append(f.calls, cloned)
	res, ok := f.results[strings.Join(args, "\x1f")]
	if !ok {
		return "", fmt.Errorf("unexpected call: %v", args)
	}
	if res.err != nil {
		return "", res.err
	}
	return res.out, nil
}

func TestParseTmuxClientSessions(t *testing.T) {
	raw := "$1:111\n$2:222\ninvalid\n"
	got := parseTmuxClientSessions(raw)
	if !got["$1"] || !got["$2"] {
		t.Fatalf("unexpected sessions: %#v", got)
	}
	if got[""] {
		t.Fatalf("empty session should not exist: %#v", got)
	}
}

func TestParseTmuxTargets(t *testing.T) {
	raw := "dev\t$1\t%3\tbash\tmain\nwork\t$2\t%9\tcodex\tcodex pane\n"
	targets := parseTmuxTargets(raw)
	if len(targets) != 2 {
		t.Fatalf("expected 2 targets, got %d", len(targets))
	}
	if targets[1].PaneID != "%9" {
		t.Fatalf("unexpected pane id: %#v", targets[1])
	}
}

func TestTmuxPlugin_ListTargetsSortsCodexFirst(t *testing.T) {
	runner := &fakeTmuxRunner{results: map[string]fakeTmuxResult{
		"list-clients\x1f-F\x1f#{session_id}:#{client_pid}": {
			out: "$2:111\n",
		},
		"list-panes\x1f-a\x1f-F\x1f#{session_name}\t#{session_id}\t#{pane_id}\t#{pane_current_command}\t#{pane_title}": {
			out: "dev\t$1\t%1\tbash\tlocal\nwork\t$2\t%2\tcodex\tCodex Main\n",
		},
	}}

	plugin := newTmuxPlugin(true, runner)
	targets, err := plugin.ListTargets()
	if err != nil {
		t.Fatalf("ListTargets() error = %v", err)
	}
	if len(targets) != 2 {
		t.Fatalf("expected 2 targets, got %d", len(targets))
	}
	if targets[0].PaneID != "%2" {
		t.Fatalf("expected codex pane first, got %+v", targets)
	}
	if !targets[0].HasClient {
		t.Fatalf("expected first target with active client")
	}
}

func TestTmuxPlugin_SendTextToBuffer(t *testing.T) {
	runner := &fakeTmuxRunner{results: map[string]fakeTmuxResult{
		"display-message\x1f-p\x1f-t\x1f%2\x1f#{pane_id}":       {out: "%2"},
		"set-buffer\x1f--\x1fskill $beads start task bdtui-123": {out: ""},
		"paste-buffer\x1f-t\x1f%2":                              {out: ""},
	}}

	plugin := newTmuxPlugin(true, runner)
	plugin.SetTarget(TmuxTarget{SessionName: "work", SessionID: "$2", PaneID: "%2"})

	if err := plugin.SendTextToBuffer("skill $beads start task bdtui-123"); err != nil {
		t.Fatalf("SendTextToBuffer() error = %v", err)
	}
	if len(runner.calls) != 3 {
		t.Fatalf("expected 3 calls, got %d", len(runner.calls))
	}
}

func TestTmuxPlugin_SendTextToBufferRequiresTarget(t *testing.T) {
	plugin := newTmuxPlugin(true, &fakeTmuxRunner{})
	err := plugin.SendTextToBuffer("skill $beads start task bdtui-123")
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "target") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTmuxPlugin_FocusPane(t *testing.T) {
	runner := &fakeTmuxRunner{results: map[string]fakeTmuxResult{
		"select-pane\x1f-t\x1f%2": {out: ""},
	}}
	plugin := newTmuxPlugin(true, runner)
	if err := plugin.FocusPane("%2"); err != nil {
		t.Fatalf("FocusPane() error = %v", err)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(runner.calls))
	}
}

func TestTmuxPlugin_FocusPaneRequiresPaneID(t *testing.T) {
	plugin := newTmuxPlugin(true, &fakeTmuxRunner{})
	err := plugin.FocusPane("   ")
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "empty pane id") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTmuxPlugin_FocusPaneDisabled(t *testing.T) {
	plugin := newTmuxPlugin(false, &fakeTmuxRunner{})
	err := plugin.FocusPane("%2")
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "disabled") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTmuxPlugin_ListTargetsDisabled(t *testing.T) {
	plugin := newTmuxPlugin(false, &fakeTmuxRunner{})
	_, err := plugin.ListTargets()
	if err == nil || !strings.Contains(err.Error(), "disabled") {
		t.Fatalf("expected disabled error, got %v", err)
	}
}

func TestTmuxPlugin_MarkPane(t *testing.T) {
	runner := &fakeTmuxRunner{results: map[string]fakeTmuxResult{
		"select-pane\x1f-m\x1f-t\x1f%7": {out: ""},
	}}
	plugin := newTmuxPlugin(true, runner)
	if err := plugin.MarkPane("%7"); err != nil {
		t.Fatalf("MarkPane() error = %v", err)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(runner.calls))
	}
}

func TestTmuxPlugin_IsPaneMarked(t *testing.T) {
	runner := &fakeTmuxRunner{results: map[string]fakeTmuxResult{
		"display-message\x1f-p\x1f-t\x1f%7\x1f#{?pane_marked,1,0}": {out: "1"},
	}}
	plugin := newTmuxPlugin(true, runner)
	marked, err := plugin.IsPaneMarked("%7")
	if err != nil {
		t.Fatalf("IsPaneMarked() error = %v", err)
	}
	if !marked {
		t.Fatalf("expected pane to be marked")
	}
}

func TestTmuxPlugin_ClearMarkPaneSkipsWhenNotMarked(t *testing.T) {
	runner := &fakeTmuxRunner{results: map[string]fakeTmuxResult{
		"display-message\x1f-p\x1f-t\x1f%7\x1f#{?pane_marked,1,0}": {out: "0"},
	}}
	plugin := newTmuxPlugin(true, runner)
	if err := plugin.ClearMarkPane("%7"); err != nil {
		t.Fatalf("ClearMarkPane() error = %v", err)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("expected only is-marked call, got %d", len(runner.calls))
	}
}

func TestTmuxPlugin_ClearMarkPaneTogglesWhenMarked(t *testing.T) {
	runner := &fakeTmuxRunner{results: map[string]fakeTmuxResult{
		"display-message\x1f-p\x1f-t\x1f%7\x1f#{?pane_marked,1,0}": {out: "1"},
		"select-pane\x1f-M\x1f-t\x1f%7":                            {out: ""},
	}}
	plugin := newTmuxPlugin(true, runner)
	if err := plugin.ClearMarkPane("%7"); err != nil {
		t.Fatalf("ClearMarkPane() error = %v", err)
	}
	if len(runner.calls) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(runner.calls))
	}
}

func TestTmuxPlugin_BlinkPaneWindow(t *testing.T) {
	runner := &fakeTmuxRunner{results: map[string]fakeTmuxResult{
		"display-message\x1f-p\x1f-t\x1f%14\x1f#{window_id}":                              {out: "@10"},
		"show-options\x1f-w\x1f-v\x1f-t\x1f@10\x1fwindow-active-style":                    {out: "default"},
		"set-option\x1f-w\x1f-t\x1f@10\x1fwindow-active-style\x1ffg=default,bg=colour160": {out: ""},
		"set-option\x1f-w\x1f-t\x1f@10\x1fwindow-active-style\x1ffg=default,bg=default":   {out: ""},
		"set-option\x1f-w\x1f-t\x1f@10\x1fwindow-active-style\x1fdefault":                 {out: ""},
	}}

	plugin := newTmuxPlugin(true, runner)
	plugin.sleepFn = func(_ time.Duration) {}

	if err := plugin.BlinkPaneWindow("%14"); err != nil {
		t.Fatalf("BlinkPaneWindow() error = %v", err)
	}
	if len(runner.calls) != 7 {
		t.Fatalf("expected 7 calls, got %d", len(runner.calls))
	}
}

func TestTmuxPlugin_BlinkPaneWindowRequiresPaneID(t *testing.T) {
	plugin := newTmuxPlugin(true, &fakeTmuxRunner{})
	if err := plugin.BlinkPaneWindow("   "); err == nil {
		t.Fatalf("expected error")
	}
}
