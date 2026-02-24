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
	leader := strings.Join(keymap.Leader, "\n")
	if strings.Contains(leader, "g b") {
		t.Fatalf("expected leader keymap to remove g b, got %q", leader)
	}
	if !strings.Contains(leader, "g B") {
		t.Fatalf("expected leader keymap to keep g B, got %q", leader)
	}
}
