package bdtui_test

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func tmuxLeaderTestModel(issue Issue) model {
	return model{
		Mode:   ModeBoard,
		Issues: []Issue{issue},
		ByID:   map[string]*Issue{issue.ID: &issue},
		Columns: map[Status][]Issue{
			StatusOpen:       {issue},
			StatusInProgress: {},
			StatusBlocked:    {},
			StatusClosed:     {},
		},
		SelectedCol: 0,
		SelectedIdx: map[Status]int{
			StatusOpen:       0,
			StatusInProgress: 0,
			StatusBlocked:    0,
			StatusClosed:     0,
		},
		Styles: newStyles(),
	}
}

func TestHandleBoardKeyZTogglesChildren(t *testing.T) {
	t.Parallel()

	issue := Issue{
		ID:       "bdtui-56i.31",
		Title:    "parent",
		Status:   StatusOpen,
		Display:  StatusOpen,
		Children: []string{"bdtui-56i.31.1"},
	}
	m := tmuxLeaderTestModel(issue)
	m.Collapsed = map[string]bool{}

	next, cmd := m.HandleBoardKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'z'}})
	got := next.(model)
	if cmd != nil {
		t.Fatalf("expected nil cmd")
	}
	if !got.Collapsed[issue.ID] {
		t.Fatalf("expected children to be collapsed")
	}
}

func TestHandleBoardKeyTStartsTmuxLeader(t *testing.T) {
	t.Parallel()

	issue := Issue{ID: "bdtui-56i.31.1", Title: "tmux", Status: StatusOpen, Display: StatusOpen}
	m := tmuxLeaderTestModel(issue)

	next, cmd := m.HandleBoardKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	got := next.(model)
	if cmd != nil {
		t.Fatalf("expected nil cmd")
	}
	if !got.Leader || got.LeaderPrefix != "t" {
		t.Fatalf("expected active tmux leader, got Leader=%v prefix=%q", got.Leader, got.LeaderPrefix)
	}
	if !strings.Contains(got.Toast, "Leader: t") {
		t.Fatalf("expected tmux leader toast, got %q", got.Toast)
	}
}

func TestHandleBoardKeyYNoLongerTriggersTmux(t *testing.T) {
	t.Parallel()

	issue := Issue{ID: "bdtui-56i.31.1", Title: "tmux", Status: StatusOpen, Display: StatusOpen}
	m := tmuxLeaderTestModel(issue)

	next, cmd := m.HandleBoardKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'Y'}})
	got := next.(model)
	if cmd != nil {
		t.Fatalf("expected nil cmd")
	}
	if got.Mode != ModeBoard {
		t.Fatalf("expected mode=%s, got %s", ModeBoard, got.Mode)
	}
	if got.TmuxPicker != nil {
		t.Fatalf("expected no tmux picker on removed Y shortcut")
	}
}

func TestHandleKeyTsWithoutAttachedTargetOpensTmuxPicker(t *testing.T) {
	t.Parallel()

	issue := Issue{ID: "bdtui-56i.31.1", Title: "tmux", Status: StatusOpen, Display: StatusOpen}
	runner := &fakeTmuxRunner{results: map[string]fakeTmuxResult{
		"list-clients\x1f-F\x1f#{session_id}:#{client_pid}": {out: "$1:111\n"},
		"list-panes\x1f-a\x1f-F\x1f#{session_name}\t#{session_id}\t#{pane_id}\t#{window_id}\t#{pane_current_command}\t#{pane_title}": {
			out: "work\t$1\t%7\t@1\tzsh\ttarget\n",
		},
		"select-pane\x1f-m\x1f-t\x1f%7": {out: ""},
	}}
	plugin := newTmuxPlugin(true, runner)

	m := tmuxLeaderTestModel(issue)
	m.Leader = true
	m.LeaderPrefix = "t"
	m.Plugins = PluginRegistry{TmuxPlugin: plugin}

	next, cmd := m.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	got := next.(model)
	if got.Mode != ModeTmuxPicker {
		t.Fatalf("expected mode=%s, got %s", ModeTmuxPicker, got.Mode)
	}
	if got.TmuxPicker == nil || got.TmuxPicker.IssueID != issue.ID {
		t.Fatalf("expected picker with issue id %q, got %#v", issue.ID, got.TmuxPicker)
	}
	if cmd == nil {
		t.Fatalf("expected blink cmd")
	}
}

func TestHandleTmuxLeaderForceSendAlwaysOpensPicker(t *testing.T) {
	t.Parallel()

	issue := Issue{ID: "bdtui-56i.31.1", Title: "tmux", Status: StatusOpen, Display: StatusOpen}
	runner := &fakeTmuxRunner{results: map[string]fakeTmuxResult{
		"list-clients\x1f-F\x1f#{session_id}:#{client_pid}": {out: "$1:111\n"},
		"list-panes\x1f-a\x1f-F\x1f#{session_name}\t#{session_id}\t#{pane_id}\t#{window_id}\t#{pane_current_command}\t#{pane_title}": {
			out: "work\t$1\t%7\t@1\tzsh\ttarget\n",
		},
		"select-pane\x1f-m\x1f-t\x1f%7": {out: ""},
	}}
	plugin := newTmuxPlugin(true, runner)
	plugin.SetTarget(TmuxTarget{SessionName: "work", SessionID: "$1", PaneID: "%99"})

	m := tmuxLeaderTestModel(issue)
	m.Plugins = PluginRegistry{TmuxPlugin: plugin}

	next, cmd := m.HandleTmuxLeaderCombo("S")
	got := next.(model)
	if got.Mode != ModeTmuxPicker {
		t.Fatalf("expected forced picker mode, got %s", got.Mode)
	}
	if got.TmuxPicker == nil || got.TmuxPicker.IssueID != issue.ID {
		t.Fatalf("expected picker with issue id %q, got %#v", issue.ID, got.TmuxPicker)
	}
	if cmd == nil {
		t.Fatalf("expected blink cmd")
	}
}

func TestHandleTmuxLeaderAttachOpensPickerWithoutIssueID(t *testing.T) {
	t.Parallel()

	issue := Issue{ID: "bdtui-56i.31.1", Title: "tmux", Status: StatusOpen, Display: StatusOpen}
	runner := &fakeTmuxRunner{results: map[string]fakeTmuxResult{
		"list-clients\x1f-F\x1f#{session_id}:#{client_pid}": {out: "$1:111\n"},
		"list-panes\x1f-a\x1f-F\x1f#{session_name}\t#{session_id}\t#{pane_id}\t#{window_id}\t#{pane_current_command}\t#{pane_title}": {
			out: "work\t$1\t%7\t@1\tzsh\ttarget\n",
		},
		"select-pane\x1f-m\x1f-t\x1f%7": {out: ""},
	}}
	plugin := newTmuxPlugin(true, runner)
	plugin.SetTarget(TmuxTarget{SessionName: "work", SessionID: "$1", PaneID: "%99"})

	m := tmuxLeaderTestModel(issue)
	m.Plugins = PluginRegistry{TmuxPlugin: plugin}

	next, _ := m.HandleTmuxLeaderCombo("a")
	got := next.(model)
	if got.Mode != ModeTmuxPicker {
		t.Fatalf("expected mode=%s, got %s", ModeTmuxPicker, got.Mode)
	}
	if got.TmuxPicker == nil || got.TmuxPicker.IssueID != "" {
		t.Fatalf("expected attach picker without issue id, got %#v", got.TmuxPicker)
	}
}

func TestHandleTmuxLeaderDetachClearsCurrentTarget(t *testing.T) {
	t.Parallel()

	issue := Issue{ID: "bdtui-56i.31.1", Title: "tmux", Status: StatusOpen, Display: StatusOpen}
	plugin := newTmuxPlugin(true, &fakeTmuxRunner{})
	plugin.SetTarget(TmuxTarget{SessionName: "work", SessionID: "$1", PaneID: "%7"})

	m := tmuxLeaderTestModel(issue)
	m.Plugins = PluginRegistry{TmuxPlugin: plugin}

	next, cmd := m.HandleTmuxLeaderCombo("d")
	got := next.(model)
	if cmd != nil {
		t.Fatalf("expected nil cmd")
	}
	if plugin.CurrentTarget() != nil {
		t.Fatalf("expected tmux target to be detached")
	}
	if !strings.Contains(got.Toast, "detached") {
		t.Fatalf("expected detach toast, got %q", got.Toast)
	}
}

func TestHandleTmuxLeaderDetachWarnsWhenNoTarget(t *testing.T) {
	t.Parallel()

	issue := Issue{ID: "bdtui-56i.31.1", Title: "tmux", Status: StatusOpen, Display: StatusOpen}
	plugin := newTmuxPlugin(true, &fakeTmuxRunner{})

	m := tmuxLeaderTestModel(issue)
	m.Plugins = PluginRegistry{TmuxPlugin: plugin}

	next, cmd := m.HandleTmuxLeaderCombo("d")
	got := next.(model)
	if cmd != nil {
		t.Fatalf("expected nil cmd")
	}
	if got.Toast != "no tmux target attached" {
		t.Fatalf("expected no-target warning, got %q", got.Toast)
	}
}

func TestRenderBoardFooterMentionsTmuxLeaderAndZ(t *testing.T) {
	t.Parallel()

	m := model{
		Mode:   ModeBoard,
		Width:  120,
		Height: 30,
		Styles: newStyles(),
	}

	out := m.RenderFooter()
	if !strings.Contains(out, "z toggle children") || !strings.Contains(out, "t + key tmux") {
		t.Fatalf("expected footer to mention z/t tmux controls, got %q", out)
	}
	if strings.Contains(out, "Y paste to tmux") {
		t.Fatalf("expected footer to remove Y tmux shortcut, got %q", out)
	}
}

func TestRenderTitleShowsTmuxLeaderPrefix(t *testing.T) {
	t.Parallel()

	m := model{
		Mode:         ModeBoard,
		Width:        120,
		Height:       20,
		Leader:       true,
		LeaderPrefix: "t",
		Styles:       newStyles(),
	}

	out := m.View()
	if !strings.Contains(out, "Leader: t ...") {
		t.Fatalf("expected title to mention tmux leader, got %q", out)
	}
}

func TestRenderTmuxPickerModalDoesNotMentionY(t *testing.T) {
	t.Parallel()

	m := model{
		Mode:   ModeTmuxPicker,
		Styles: newStyles(),
		TmuxPicker: &TmuxPickerState{
			Targets: []TmuxTarget{{SessionName: "work", PaneID: "%7", Command: "zsh", Title: "target"}},
		},
	}

	out := m.RenderModal()
	if strings.Contains(out, "(Y)") {
		t.Fatalf("expected tmux picker modal to remove Y label, got %q", out)
	}
}
