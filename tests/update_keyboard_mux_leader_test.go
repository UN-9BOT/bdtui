package bdtui_test

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func muxLeaderTestModel(issue Issue) model {
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
	m := muxLeaderTestModel(issue)
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

func TestHandleBoardKeyTStartsMuxLeader(t *testing.T) {
	t.Parallel()

	issue := Issue{ID: "bdtui-dm0", Title: "herdr", Status: StatusOpen, Display: StatusOpen}
	m := muxLeaderTestModel(issue)

	next, cmd := m.HandleBoardKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	got := next.(model)
	if cmd != nil {
		t.Fatalf("expected nil cmd")
	}
	if !got.Leader || got.LeaderPrefix != "t" {
		t.Fatalf("expected active mux leader, got Leader=%v prefix=%q", got.Leader, got.LeaderPrefix)
	}
	if !strings.Contains(got.Toast, "Leader: t") {
		t.Fatalf("expected mux leader toast, got %q", got.Toast)
	}
}

func TestHandleBoardKeyYNoLongerTriggersMux(t *testing.T) {
	t.Parallel()

	issue := Issue{ID: "bdtui-dm0", Title: "herdr", Status: StatusOpen, Display: StatusOpen}
	m := muxLeaderTestModel(issue)

	next, cmd := m.HandleBoardKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'Y'}})
	got := next.(model)
	if cmd != nil {
		t.Fatalf("expected nil cmd")
	}
	if got.Mode != ModeBoard {
		t.Fatalf("expected mode=%s, got %s", ModeBoard, got.Mode)
	}
	if got.MuxPicker != nil {
		t.Fatalf("expected no mux picker on removed Y shortcut")
	}
}

func TestHandleKeyTsWithoutAttachedTargetOpensMuxPicker(t *testing.T) {
	t.Parallel()

	issue := Issue{ID: "bdtui-dm0", Title: "herdr", Status: StatusOpen, Display: StatusOpen}
	runner := &fakeHerdrRunner{results: map[string]fakeHerdrResult{
		"workspace\x1flist": {out: `{"result":{"workspaces":[{"workspace_id":"ws-1","label":"bdtui"}]}}`},
		"tab\x1flist":       {out: `{"result":{"tabs":[{"tab_id":"tab-1","workspace_id":"ws-1","label":"editor","number":1}]}}`},
		"pane\x1flist":      {out: `{"result":{"panes":[{"pane_id":"pane-2","workspace_id":"ws-1","tab_id":"tab-1","cwd":"/repo","foreground_cwd":"/repo","focused":false,"agent":"codex"}]}}`},
	}}
	plugin := newHerdrPlugin(true, runner)

	m := muxLeaderTestModel(issue)
	m.Leader = true
	m.LeaderPrefix = "t"
	m.Plugins = PluginRegistry{HerdrPlugin: plugin}

	next, cmd := m.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	got := next.(model)
	if got.Mode != ModeMuxPicker {
		t.Fatalf("expected mode=%s, got %s", ModeMuxPicker, got.Mode)
	}
	if got.MuxPicker == nil || got.MuxPicker.IssueID != issue.ID {
		t.Fatalf("expected picker with issue id %q, got %#v", issue.ID, got.MuxPicker)
	}
	if cmd != nil {
		t.Fatalf("expected no preview cmd")
	}
}

func TestHandleMuxLeaderForceSendAlwaysOpensPicker(t *testing.T) {
	t.Parallel()

	issue := Issue{ID: "bdtui-dm0", Title: "herdr", Status: StatusOpen, Display: StatusOpen}
	runner := &fakeHerdrRunner{results: map[string]fakeHerdrResult{
		"workspace\x1flist": {out: `{"result":{"workspaces":[{"workspace_id":"ws-1","label":"bdtui"}]}}`},
		"tab\x1flist":       {out: `{"result":{"tabs":[{"tab_id":"tab-1","workspace_id":"ws-1","label":"editor","number":1}]}}`},
		"pane\x1flist":      {out: `{"result":{"panes":[{"pane_id":"pane-2","workspace_id":"ws-1","tab_id":"tab-1","cwd":"/repo","foreground_cwd":"/repo","focused":false,"agent":"codex"}]}}`},
	}}
	plugin := newHerdrPlugin(true, runner)
	plugin.SetTarget(MuxTarget{PaneID: "pane-old", TabID: "tab-1", WorkspaceID: "ws-1"})

	m := muxLeaderTestModel(issue)
	m.Plugins = PluginRegistry{HerdrPlugin: plugin}

	next, cmd := m.HandleMuxLeaderCombo("S")
	got := next.(model)
	if got.Mode != ModeMuxPicker {
		t.Fatalf("expected forced picker mode, got %s", got.Mode)
	}
	if got.MuxPicker == nil || got.MuxPicker.IssueID != issue.ID {
		t.Fatalf("expected picker with issue id %q, got %#v", issue.ID, got.MuxPicker)
	}
	if cmd != nil {
		t.Fatalf("expected no preview cmd")
	}
}

func TestHandleMuxLeaderAttachOpensPickerWithoutIssueID(t *testing.T) {
	t.Parallel()

	issue := Issue{ID: "bdtui-dm0", Title: "herdr", Status: StatusOpen, Display: StatusOpen}
	runner := &fakeHerdrRunner{results: map[string]fakeHerdrResult{
		"workspace\x1flist": {out: `{"result":{"workspaces":[{"workspace_id":"ws-1","label":"bdtui"}]}}`},
		"tab\x1flist":       {out: `{"result":{"tabs":[{"tab_id":"tab-1","workspace_id":"ws-1","label":"editor","number":1}]}}`},
		"pane\x1flist":      {out: `{"result":{"panes":[{"pane_id":"pane-2","workspace_id":"ws-1","tab_id":"tab-1","cwd":"/repo","foreground_cwd":"/repo","focused":false,"agent":"codex"}]}}`},
	}}
	plugin := newHerdrPlugin(true, runner)
	plugin.SetTarget(MuxTarget{PaneID: "pane-old", TabID: "tab-1", WorkspaceID: "ws-1"})

	m := muxLeaderTestModel(issue)
	m.Plugins = PluginRegistry{HerdrPlugin: plugin}

	next, _ := m.HandleMuxLeaderCombo("a")
	got := next.(model)
	if got.Mode != ModeMuxPicker {
		t.Fatalf("expected mode=%s, got %s", ModeMuxPicker, got.Mode)
	}
	if got.MuxPicker == nil || got.MuxPicker.IssueID != "" {
		t.Fatalf("expected attach picker without issue id, got %#v", got.MuxPicker)
	}
}

func TestHandleMuxLeaderDetachClearsCurrentTarget(t *testing.T) {
	t.Parallel()

	issue := Issue{ID: "bdtui-dm0", Title: "herdr", Status: StatusOpen, Display: StatusOpen}
	plugin := newHerdrPlugin(true, &fakeHerdrRunner{})
	plugin.SetTarget(MuxTarget{PaneID: "pane-2", TabID: "tab-1", WorkspaceID: "ws-1"})

	m := muxLeaderTestModel(issue)
	m.Plugins = PluginRegistry{HerdrPlugin: plugin}

	next, cmd := m.HandleMuxLeaderCombo("d")
	got := next.(model)
	if cmd != nil {
		t.Fatalf("expected nil cmd")
	}
	if plugin.CurrentTarget() != nil {
		t.Fatalf("expected mux target to be detached")
	}
	if !strings.Contains(got.Toast, "detached") {
		t.Fatalf("expected detach toast, got %q", got.Toast)
	}
}

func TestHandleMuxLeaderDetachWarnsWhenNoTarget(t *testing.T) {
	t.Parallel()

	issue := Issue{ID: "bdtui-dm0", Title: "herdr", Status: StatusOpen, Display: StatusOpen}
	plugin := newHerdrPlugin(true, &fakeHerdrRunner{})

	m := muxLeaderTestModel(issue)
	m.Plugins = PluginRegistry{HerdrPlugin: plugin}

	next, cmd := m.HandleMuxLeaderCombo("d")
	got := next.(model)
	if cmd != nil {
		t.Fatalf("expected nil cmd")
	}
	if got.Toast != "no herdr target attached" {
		t.Fatalf("expected no-target warning, got %q", got.Toast)
	}
}

func TestRenderBoardFooterMentionsHerdrLeaderAndZ(t *testing.T) {
	t.Parallel()

	m := model{
		Mode:   ModeBoard,
		Width:  120,
		Height: 30,
		Styles: newStyles(),
	}

	out := m.RenderFooter()
	if !strings.Contains(out, "z toggle children") || !strings.Contains(out, "t + key herdr") {
		t.Fatalf("expected footer to mention z/t herdr controls, got %q", out)
	}
	if strings.Contains(out, "Y paste to tmux") {
		t.Fatalf("expected footer to remove Y tmux shortcut, got %q", out)
	}
}

func TestRenderTitleShowsMuxLeaderPrefix(t *testing.T) {
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
		t.Fatalf("expected title to mention mux leader, got %q", out)
	}
}

func TestRenderMuxPickerModalDoesNotMentionY(t *testing.T) {
	t.Parallel()

	m := model{
		Mode:   ModeMuxPicker,
		Styles: newStyles(),
		MuxPicker: &MuxPickerState{
			Targets: []MuxTarget{{WorkspaceLabel: "bdtui", TabLabel: "editor", PaneID: "pane-2", Agent: "codex"}},
		},
	}

	out := m.RenderModal()
	if strings.Contains(out, "(Y)") {
		t.Fatalf("expected mux picker modal to remove Y label, got %q", out)
	}
}

func TestRenderMuxPickerModalShowsWorkspaceAndTabLabels(t *testing.T) {
	t.Parallel()

	m := model{
		Mode:   ModeMuxPicker,
		Styles: newStyles(),
		MuxPicker: &MuxPickerState{
			Targets: []MuxTarget{
				{WorkspaceLabel: "bdtui", TabLabel: "editor", PaneID: "pane-2", Agent: "codex", ForegroundCwd: "/repo"},
				{WorkspaceLabel: "configs", TabLabel: "notes", PaneID: "pane-4", ForegroundCwd: "/home/unbot/.config/nvim"},
			},
		},
	}

	out := m.RenderModal()
	if !strings.Contains(out, "bdtui/editor") {
		t.Fatalf("expected workspace/tab label in picker, got %q", out)
	}
	if !strings.Contains(out, "configs/notes") {
		t.Fatalf("expected second workspace/tab label in picker, got %q", out)
	}
}

func TestRenderMuxPickerModalOmitsSeparatorWhenTabLabelEmpty(t *testing.T) {
	t.Parallel()

	m := model{
		Mode:   ModeMuxPicker,
		Styles: newStyles(),
		MuxPicker: &MuxPickerState{
			Targets: []MuxTarget{
				{WorkspaceLabel: "bdtui", PaneID: "pane-2", Agent: "codex"},
			},
		},
	}

	out := m.RenderModal()
	if strings.Contains(out, "bdtui/") {
		t.Fatalf("expected picker to omit trailing tab separator, got %q", out)
	}
}
