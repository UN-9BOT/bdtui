package main

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/textinput"
)

func TestRenderInlineSearchBlockHidesFiltersWhenUnfocusedAndEmpty(t *testing.T) {
	t.Parallel()

	m := newInlineSearchTestModel()
	m.mode = ModeBoard
	m.searchExpanded = false
	m.filter = Filter{Status: "any", Priority: "any", Type: "any"}

	out := m.renderInlineSearchBlock()
	if strings.Contains(out, "filters:") {
		t.Fatalf("expected filters to stay hidden when empty and unfocused, got %q", out)
	}
}

func TestRenderInlineSearchBlockShowsFiltersWhenUnfocusedAndActive(t *testing.T) {
	t.Parallel()

	m := newInlineSearchTestModel()
	m.mode = ModeBoard
	m.searchExpanded = false
	m.filter = Filter{
		Assignee: "alice",
		Status:   "open",
		Priority: "2",
		Type:     "task",
	}

	out := m.renderInlineSearchBlock()
	if !strings.Contains(out, "filters:") {
		t.Fatalf("expected filters to be visible when active and unfocused, got %q", out)
	}
}

func TestRenderInlineSearchBlockShowsExpandedFiltersWhenFocused(t *testing.T) {
	t.Parallel()

	m := newInlineSearchTestModel()
	m.mode = ModeSearch
	m.searchExpanded = true
	m.filterForm = newFilterForm(Filter{Status: "any", Priority: "any", Type: "any"})

	out := m.renderInlineSearchBlock()
	if !strings.Contains(out, "filters:") {
		t.Fatalf("expected filters line in expanded search mode, got %q", out)
	}
}

func TestRenderInlineSearchBlockDimsWhenNotFocused(t *testing.T) {
	t.Parallel()

	m := newInlineSearchTestModel()
	m.mode = ModeBoard
	m.searchExpanded = false

	out := m.renderInlineSearchBlock()
	if !strings.Contains(out, "search: -") {
		t.Fatalf("expected search line to stay visible when not focused, got %q", out)
	}
}

func TestViewPlacesInlineSearchAfterInspector(t *testing.T) {
	t.Parallel()

	m := newInlineSearchTestModel()
	m.mode = ModeBoard
	m.searchQuery = "alpha"
	m.searchInput.SetValue("alpha")

	out := ansiSGRRegexp.ReplaceAllString(m.View(), "")
	descPos := strings.Index(out, "Selected:")
	searchPos := strings.Index(out, "search:")
	if descPos == -1 || searchPos == -1 {
		t.Fatalf("expected both inspector and search lines, got %q", out)
	}
	if searchPos <= descPos {
		t.Fatalf("expected search block after inspector block, got selectedPos=%d searchPos=%d", descPos, searchPos)
	}
}

func TestRenderInlineSearchBlockExpandedShowsAllFilterValues(t *testing.T) {
	t.Parallel()

	m := newInlineSearchTestModel()
	m.mode = ModeSearch
	m.searchExpanded = true
	m.filterForm = newFilterForm(Filter{
		Assignee: "alice",
		Label:    "alpha",
		Status:   "open",
		Priority: "2",
		Type:     "task",
	})

	out := ansiSGRRegexp.ReplaceAllString(m.renderInlineSearchBlock(), "")
	if !strings.Contains(out, "assignee: any | alice") {
		t.Fatalf("expected assignee values list, got %q", out)
	}
	if !strings.Contains(out, "label: any | alpha") {
		t.Fatalf("expected label values list, got %q", out)
	}
	if !strings.Contains(out, "status: any | open | in_progress | blocked | closed") {
		t.Fatalf("expected status values list, got %q", out)
	}
	if !strings.Contains(out, "priority: any | 0 | 1 | 2 | 3 | 4") {
		t.Fatalf("expected priority values list, got %q", out)
	}
	if !strings.Contains(out, "type: any | task | epic | bug | feature | chore | decision") {
		t.Fatalf("expected type values list, got %q", out)
	}
}

func newInlineSearchTestModel() model {
	return model{
		width:  120,
		height: 36,
		mode:   ModeBoard,
		styles: newStyles(),
		searchInput: func() textinput.Model {
			in := textinput.New()
			in.Prompt = "search> "
			in.SetValue("")
			in.CursorEnd()
			return in
		}(),
		filter: Filter{Status: "any", Priority: "any", Type: "any"},
		issues: []Issue{
			{
				ID:          "bdtui-56i.19",
				Title:       "inline search block",
				Description: "description body",
				Status:      StatusOpen,
				Display:     StatusOpen,
				Priority:    2,
				IssueType:   "task",
				Assignee:    "alice",
				Labels:      []string{"alpha"},
			},
		},
		columns: map[Status][]Issue{
			StatusOpen: {
				{
					ID:          "bdtui-56i.19",
					Title:       "inline search block",
					Description: "description body",
					Status:      StatusOpen,
					Display:     StatusOpen,
					Priority:    2,
					IssueType:   "task",
					Assignee:    "alice",
					Labels:      []string{"alpha"},
				},
			},
			StatusInProgress: {},
			StatusBlocked:    {},
			StatusClosed:     {},
		},
		columnDepths: map[Status]map[string]int{
			StatusOpen:       {"bdtui-56i.19": 0},
			StatusInProgress: {},
			StatusBlocked:    {},
			StatusClosed:     {},
		},
		selectedCol: 0,
		selectedIdx: map[Status]int{
			StatusOpen:       0,
			StatusInProgress: 0,
			StatusBlocked:    0,
			StatusClosed:     0,
		},
		scrollOffset: map[Status]int{
			StatusOpen:       0,
			StatusInProgress: 0,
			StatusBlocked:    0,
			StatusClosed:     0,
		},
		byID: map[string]*Issue{
			"bdtui-56i.19": &Issue{
				ID:          "bdtui-56i.19",
				Title:       "inline search block",
				Description: "description body",
				Status:      StatusOpen,
				Display:     StatusOpen,
				Priority:    2,
				IssueType:   "task",
				Assignee:    "alice",
				Labels:      []string{"alpha"},
			},
		},
	}
}
