package bdtui_test

import (
	"fmt"
	"strings"
	"testing"
)

type fakeHerdrResult struct {
	out string
	err error
}

type fakeHerdrRunner struct {
	results map[string]fakeHerdrResult
	calls   [][]string
}

func (f *fakeHerdrRunner) Run(args ...string) (string, error) {
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

func TestParseHerdrTargets(t *testing.T) {
	workspaces := `{"result":{"workspaces":[{"workspace_id":"ws-1","label":"bdtui"}]}}`
	tabs := `{"result":{"tabs":[{"tab_id":"tab-1","workspace_id":"ws-1","label":"editor","number":1}]}}`
	panes := `{"result":{"panes":[{"pane_id":"pane-2","workspace_id":"ws-1","tab_id":"tab-1","cwd":"/repo","foreground_cwd":"/repo/app","focused":false,"agent":"codex"}]}}`

	targets := parseHerdrTargets(panes, tabs, workspaces)
	if len(targets) != 1 {
		t.Fatalf("expected 1 target, got %d", len(targets))
	}
	if targets[0].PaneID != "pane-2" {
		t.Fatalf("unexpected pane id: %#v", targets[0])
	}
	if targets[0].WorkspaceLabel != "bdtui" || targets[0].TabLabel != "editor" {
		t.Fatalf("expected labels from workspace/tab maps, got %#v", targets[0])
	}
	if targets[0].ForegroundCwd != "/repo/app" {
		t.Fatalf("unexpected foreground cwd: %#v", targets[0])
	}
}

func TestHerdrPluginListTargetsSortsSameWorkspaceAndAgentFirst(t *testing.T) {
	runner := &fakeHerdrRunner{results: map[string]fakeHerdrResult{
		"workspace\x1flist": {out: `{"result":{"workspaces":[{"workspace_id":"ws-1","label":"bdtui"},{"workspace_id":"ws-2","label":"misc"}]}}`},
		"tab\x1flist":       {out: `{"result":{"tabs":[{"tab_id":"tab-1","workspace_id":"ws-1","label":"editor","number":1},{"tab_id":"tab-2","workspace_id":"ws-2","label":"shell","number":1}]}}`},
		"pane\x1flist":      {out: `{"result":{"panes":[{"pane_id":"pane-self","workspace_id":"ws-1","tab_id":"tab-1","cwd":"/repo","foreground_cwd":"/repo","focused":true},{"pane_id":"pane-agent","workspace_id":"ws-1","tab_id":"tab-1","cwd":"/repo","foreground_cwd":"/repo","focused":false,"agent":"codex"},{"pane_id":"pane-shell","workspace_id":"ws-2","tab_id":"tab-2","cwd":"/tmp","focused":false}]}}`},
	}}

	plugin := newHerdrPlugin(true, runner)
	targets, err := plugin.ListTargets()
	if err != nil {
		t.Fatalf("ListTargets() error = %v", err)
	}
	if len(targets) != 2 {
		t.Fatalf("expected 2 targets after filtering focused pane, got %d", len(targets))
	}
	if targets[0].PaneID != "pane-agent" {
		t.Fatalf("expected same-workspace agent pane first, got %+v", targets)
	}
	for _, target := range targets {
		if target.PaneID == "pane-self" {
			t.Fatalf("expected focused pane to be filtered out: %+v", targets)
		}
	}
}

func TestHerdrPluginListTargetsDisabled(t *testing.T) {
	plugin := newHerdrPlugin(false, &fakeHerdrRunner{})
	_, err := plugin.ListTargets()
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "disabled") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHerdrPluginSendTextToTarget(t *testing.T) {
	runner := &fakeHerdrRunner{results: map[string]fakeHerdrResult{
		"pane\x1fget\x1fpane-2": {out: `{"result":{"pane":{"pane_id":"pane-2"}}}`},
		"pane\x1fsend-text\x1fpane-2\x1fskill $beads start implement task bdtui-dm0": {out: ""},
	}}

	plugin := newHerdrPlugin(true, runner)
	plugin.SetTarget(MuxTarget{PaneID: "pane-2", TabID: "tab-1", WorkspaceID: "ws-1"})

	if err := plugin.SendTextToTarget("skill $beads start implement task bdtui-dm0"); err != nil {
		t.Fatalf("SendTextToTarget() error = %v", err)
	}
	if len(runner.calls) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(runner.calls))
	}
}

func TestHerdrPluginSendTextToTargetRequiresTarget(t *testing.T) {
	plugin := newHerdrPlugin(true, &fakeHerdrRunner{})
	err := plugin.SendTextToTarget("skill $beads start implement task bdtui-dm0")
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "target") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHerdrPluginFocusTargetFallsBackToTabFocus(t *testing.T) {
	runner := &fakeHerdrRunner{results: map[string]fakeHerdrResult{
		"tab\x1ffocus\x1ftab-1": {out: ""},
	}}
	plugin := newHerdrPlugin(true, runner)

	if err := plugin.FocusTarget(MuxTarget{PaneID: "pane-2", TabID: "tab-1"}); err != nil {
		t.Fatalf("FocusTarget() error = %v", err)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(runner.calls))
	}
}

func TestMuxTargetLabelIncludesWorkspaceAndTab(t *testing.T) {
	target := MuxTarget{
		WorkspaceLabel: "bdtui",
		TabLabel:       "editor",
		PaneID:         "pane-2",
		Agent:          "codex",
		ForegroundCwd:  "/repo",
	}

	label := target.Label()
	if !strings.Contains(label, "bdtui/editor") {
		t.Fatalf("expected workspace/tab prefix, got %q", label)
	}
	if !strings.Contains(label, "codex") {
		t.Fatalf("expected agent label, got %q", label)
	}
	if !strings.Contains(label, "/repo") {
		t.Fatalf("expected cwd label, got %q", label)
	}
}
