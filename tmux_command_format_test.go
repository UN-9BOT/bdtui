package main

import "testing"

func TestFormatBeadsStartTaskCommand_NoParent(t *testing.T) {
	m := model{
		byID: map[string]*Issue{
			"bdtui-pa0.1": {ID: "bdtui-pa0.1", IssueType: "task"},
		},
	}
	got := m.formatBeadsStartTaskCommand("bdtui-pa0.1")
	want := "skill $beads start task bdtui-pa0.1"
	if got != want {
		t.Fatalf("unexpected command\nwant: %q\ngot:  %q", want, got)
	}
}

func TestFormatBeadsStartTaskCommand_WithEpicParent(t *testing.T) {
	m := model{
		byID: map[string]*Issue{
			"bdtui-pa0.1": {ID: "bdtui-pa0.1", IssueType: "task", Parent: "bdtui-pa0"},
			"bdtui-pa0":   {ID: "bdtui-pa0", IssueType: "epic"},
		},
	}
	got := m.formatBeadsStartTaskCommand("bdtui-pa0.1")
	want := "skill $beads start task bdtui-pa0.1 (epic bdtui-pa0)"
	if got != want {
		t.Fatalf("unexpected command\nwant: %q\ngot:  %q", want, got)
	}
}

func TestFormatBeadsStartTaskCommand_ParentNotEpic(t *testing.T) {
	m := model{
		byID: map[string]*Issue{
			"bdtui-pa0.1": {ID: "bdtui-pa0.1", IssueType: "task", Parent: "bdtui-pa0"},
			"bdtui-pa0":   {ID: "bdtui-pa0", IssueType: "feature"},
		},
	}
	got := m.formatBeadsStartTaskCommand("bdtui-pa0.1")
	want := "skill $beads start task bdtui-pa0.1"
	if got != want {
		t.Fatalf("unexpected command\nwant: %q\ngot:  %q", want, got)
	}
}
