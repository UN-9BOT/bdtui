package bdtui_test

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestHandleBoardKeyBCreatesBlockedIssueFromSelected(t *testing.T) {
	t.Parallel()

	issue := Issue{
		ID:      "bdtui-56i.10",
		Title:   "selected",
		Status:  StatusInProgress,
		Display: StatusInProgress,
	}

	m := model{
		Mode:   ModeBoard,
		Issues: []Issue{issue},
		ByID:   map[string]*Issue{issue.ID: &issue},
		Columns: map[Status][]Issue{
			StatusOpen:       {},
			StatusInProgress: {issue},
			StatusBlocked:    {},
			StatusClosed:     {},
		},
		SelectedCol: 1,
		SelectedIdx: map[Status]int{
			StatusOpen:       0,
			StatusInProgress: 0,
			StatusBlocked:    0,
			StatusClosed:     0,
		},
	}

	next, _ := m.HandleBoardKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")})
	got := next.(model)

	if got.Mode != ModeCreate {
		t.Fatalf("expected mode %s, got %s", ModeCreate, got.Mode)
	}
	if got.Form == nil {
		t.Fatalf("expected create form")
	}
	if got.Form.Status != StatusBlocked {
		t.Fatalf("expected create form status %s, got %s", StatusBlocked, got.Form.Status)
	}
	if got.CreateBlockerID != issue.ID {
		t.Fatalf("expected CreateBlockerID %q, got %q", issue.ID, got.CreateBlockerID)
	}
}

func TestLeaderGbRemoved(t *testing.T) {
	t.Parallel()

	issue := Issue{
		ID:      "bdtui-56i.11",
		Title:   "selected",
		Status:  StatusOpen,
		Display: StatusOpen,
	}

	m := model{
		Mode:   ModeBoard,
		Leader: true,
		Issues: []Issue{issue},
		ByID:   map[string]*Issue{issue.ID: &issue},
		Columns: map[Status][]Issue{
			StatusOpen:       {issue},
			StatusInProgress: {},
			StatusBlocked:    {},
			StatusClosed:     {},
		},
		SelectedIdx: map[Status]int{
			StatusOpen:       0,
			StatusInProgress: 0,
			StatusBlocked:    0,
			StatusClosed:     0,
		},
	}

	next, _ := m.HandleBoardKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")})
	got := next.(model)

	if got.Mode != ModeBoard {
		t.Fatalf("expected mode %s, got %s", ModeBoard, got.Mode)
	}
	if got.Prompt != nil {
		t.Fatalf("expected no prompt for removed g b combo")
	}
	if !strings.Contains(strings.ToLower(got.Toast), "unknown leader combo") {
		t.Fatalf("expected unknown leader combo toast, got %q", got.Toast)
	}
}

func TestDefaultKeymapDoesNotAdvertiseLeaderGb(t *testing.T) {
	t.Parallel()

	keymap := defaultKeymap()
	global := strings.Join(keymap.Global, "\n")
	leader := strings.Join(keymap.Leader, "\n")
	if strings.Contains(leader, "g b") {
		t.Fatalf("expected leader keymap to remove g b, got %q", leader)
	}
	if !strings.Contains(leader, "g B") {
		t.Fatalf("expected leader keymap to keep g B, got %q", leader)
	}
	if strings.Contains(global, "Y:") {
		t.Fatalf("expected global keymap to remove Y tmux shortcut, got %q", global)
	}
	if !strings.Contains(global, "z: toggle hide/show children") {
		t.Fatalf("expected global keymap to advertise z toggle, got %q", global)
	}
	if !strings.Contains(global, "t: tmux leader combos") {
		t.Fatalf("expected global keymap to advertise t leader, got %q", global)
	}
	for _, expected := range []string{"t s:", "t S:", "t a:", "t d:"} {
		if !strings.Contains(leader, expected) {
			t.Fatalf("expected leader keymap to include %q, got %q", expected, leader)
		}
	}
}

func TestHandleLeaderComboBOpensBlockerPicker(t *testing.T) {
	t.Parallel()

	issue := Issue{
		ID:      "bdtui-56i.21",
		Title:   "selected",
		Status:  StatusOpen,
		Display: StatusOpen,
	}
	other := Issue{
		ID:      "bdtui-56i.22",
		Title:   "candidate",
		Status:  StatusOpen,
		Display: StatusOpen,
	}

	m := model{
		Mode:   ModeBoard,
		Issues: []Issue{issue, other},
		ByID:   map[string]*Issue{issue.ID: &issue, other.ID: &other},
		Columns: map[Status][]Issue{
			StatusOpen:       {issue, other},
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
	}

	next, cmd := m.HandleLeaderCombo("B")
	got := next.(model)

	if cmd != nil {
		t.Fatalf("expected nil cmd when opening blocker picker")
	}
	if got.Mode != ModeBlockerPicker {
		t.Fatalf("expected mode %s, got %s", ModeBlockerPicker, got.Mode)
	}
	if got.BlockerPicker == nil {
		t.Fatalf("expected blocker picker state")
	}
	if got.BlockerPicker.TargetIssueID != issue.ID {
		t.Fatalf("expected target %q, got %q", issue.ID, got.BlockerPicker.TargetIssueID)
	}
	if len(got.BlockerPicker.Columns[StatusOpen]) != 1 {
		t.Fatalf("expected exactly one open candidate, got %d", len(got.BlockerPicker.Columns[StatusOpen]))
	}
	if got.BlockerPicker.Columns[StatusOpen][0].ID != other.ID {
		t.Fatalf("expected candidate %q, got %q", other.ID, got.BlockerPicker.Columns[StatusOpen][0].ID)
	}
}

func TestBlockerPickerSpaceTogglesSelectedBlocker(t *testing.T) {
	t.Parallel()

	issue := Issue{ID: "bdtui-56i.21", Title: "selected", Status: StatusOpen, Display: StatusOpen}
	blocker := Issue{ID: "bdtui-56i.22", Title: "candidate", Status: StatusOpen, Display: StatusOpen}

	m := model{
		Mode:   ModeBoard,
		Issues: []Issue{issue, blocker},
		ByID:   map[string]*Issue{issue.ID: &issue, blocker.ID: &blocker},
		Columns: map[Status][]Issue{
			StatusOpen:       {issue, blocker},
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
	}

	next, _ := m.HandleLeaderCombo("B")
	got := next.(model)
	if got.BlockerPicker == nil {
		t.Fatalf("expected blocker picker state")
	}
	if got.BlockerPicker.Selected[blocker.ID] {
		t.Fatalf("did not expect blocker selected before toggle")
	}

	next, _ = got.HandleKey(tea.KeyMsg{Type: tea.KeySpace})
	got = next.(model)
	if got.BlockerPicker == nil {
		t.Fatalf("expected blocker picker state after toggle")
	}
	if !got.BlockerPicker.Selected[blocker.ID] {
		t.Fatalf("expected blocker %q selected after space toggle", blocker.ID)
	}
}

func TestBlockerPickerEnterAppliesAndCloses(t *testing.T) {
	t.Parallel()

	issue := Issue{
		ID:        "bdtui-56i.21",
		Title:     "selected",
		Status:    StatusOpen,
		Display:   StatusOpen,
		BlockedBy: []string{"bdtui-56i.22"},
	}
	blocker := Issue{ID: "bdtui-56i.22", Title: "candidate", Status: StatusOpen, Display: StatusOpen}

	m := model{
		Mode:   ModeBoard,
		Client: NewBdClient(t.TempDir()),
		Issues: []Issue{issue, blocker},
		ByID:   map[string]*Issue{issue.ID: &issue, blocker.ID: &blocker},
		Columns: map[Status][]Issue{
			StatusOpen:       {issue, blocker},
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
	}

	next, _ := m.HandleLeaderCombo("B")
	got := next.(model)
	next, _ = got.HandleKey(tea.KeyMsg{Type: tea.KeySpace})
	got = next.(model)

	next, cmd := got.HandleKey(tea.KeyMsg{Type: tea.KeyEnter})
	got = next.(model)
	if cmd == nil {
		t.Fatalf("expected apply cmd on enter")
	}
	if got.Mode != ModeBoard {
		t.Fatalf("expected mode %s, got %s", ModeBoard, got.Mode)
	}
	if got.BlockerPicker != nil {
		t.Fatalf("expected blocker picker to close")
	}
}

func TestBlockerPickerEscAppliesAndCloses(t *testing.T) {
	t.Parallel()

	issue := Issue{ID: "bdtui-56i.21", Title: "selected", Status: StatusOpen, Display: StatusOpen}
	blocker := Issue{ID: "bdtui-56i.22", Title: "candidate", Status: StatusOpen, Display: StatusOpen}

	m := model{
		Mode:   ModeBoard,
		Client: NewBdClient(t.TempDir()),
		Issues: []Issue{issue, blocker},
		ByID:   map[string]*Issue{issue.ID: &issue, blocker.ID: &blocker},
		Columns: map[Status][]Issue{
			StatusOpen:       {issue, blocker},
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
	}

	next, _ := m.HandleLeaderCombo("B")
	got := next.(model)
	next, _ = got.HandleKey(tea.KeyMsg{Type: tea.KeySpace})
	got = next.(model)

	next, cmd := got.HandleKey(tea.KeyMsg{Type: tea.KeyEsc})
	got = next.(model)
	if cmd == nil {
		t.Fatalf("expected apply cmd on esc")
	}
	if got.Mode != ModeBoard {
		t.Fatalf("expected mode %s, got %s", ModeBoard, got.Mode)
	}
	if got.BlockerPicker != nil {
		t.Fatalf("expected blocker picker to close")
	}
}
