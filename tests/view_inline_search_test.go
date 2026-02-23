package bdtui_test

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/textinput"
)

func TestRenderInlineSearchBlockHidesFiltersWhenUnfocusedAndEmpty(t *testing.T) {
	t.Parallel()

	m := newInlineSearchTestModel()
	m.Mode = ModeBoard
	m.SearchExpanded = false
	m.Filter = Filter{Status: "any", Priority: "any", Type: "any"}

	out := m.RenderInlineSearchBlock()
	if strings.Contains(out, "filters:") {
		t.Fatalf("expected filters to stay hidden when empty and unfocused, got %q", out)
	}
}

func TestRenderInlineSearchBlockShowsFiltersWhenUnfocusedAndActive(t *testing.T) {
	t.Parallel()

	m := newInlineSearchTestModel()
	m.Mode = ModeBoard
	m.SearchExpanded = false
	m.Filter = Filter{
		Assignee: "alice",
		Status:   "open",
		Priority: "2",
		Type:     "task",
	}

	out := m.RenderInlineSearchBlock()
	if !strings.Contains(out, "filters:") {
		t.Fatalf("expected filters to be visible when active and unfocused, got %q", out)
	}
}

func TestRenderInlineSearchBlockShowsExpandedFiltersWhenFocused(t *testing.T) {
	t.Parallel()

	m := newInlineSearchTestModel()
	m.Mode = ModeSearch
	m.SearchExpanded = true
	m.FilterForm = newFilterForm(Filter{Status: "any", Priority: "any", Type: "any"})

	out := m.RenderInlineSearchBlock()
	if !strings.Contains(out, "filters:") {
		t.Fatalf("expected filters line in expanded search mode, got %q", out)
	}
}

func TestRenderInlineSearchBlockDimsWhenNotFocused(t *testing.T) {
	t.Parallel()

	m := newInlineSearchTestModel()
	m.Mode = ModeBoard
	m.SearchExpanded = false

	out := m.RenderInlineSearchBlock()
	if !strings.Contains(out, "search: -") {
		t.Fatalf("expected search line to stay visible when not focused, got %q", out)
	}
}

func TestViewPlacesInlineSearchAfterInspector(t *testing.T) {
	t.Parallel()

	m := newInlineSearchTestModel()
	m.Mode = ModeBoard
	m.SearchQuery = "alpha"
	m.SearchInput.SetValue("alpha")

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
	m.Mode = ModeSearch
	m.SearchExpanded = true
	m.FilterForm = newFilterForm(Filter{
		Assignee: "alice",
		Label:    "alpha",
		Status:   "open",
		Priority: "2",
		Type:     "task",
	})

	out := ansiSGRRegexp.ReplaceAllString(m.RenderInlineSearchBlock(), "")
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
		Width:  120,
		Height: 36,
		Mode:   ModeBoard,
		Styles: newStyles(),
		SearchInput: func() textinput.Model {
			in := textinput.New()
			in.Prompt = "search> "
			in.SetValue("")
			in.CursorEnd()
			return in
		}(),
		Filter: Filter{Status: "any", Priority: "any", Type: "any"},
		Issues: []Issue{
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
		Columns: map[Status][]Issue{
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
		ColumnDepths: map[Status]map[string]int{
			StatusOpen:       {"bdtui-56i.19": 0},
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
		ScrollOffset: map[Status]int{
			StatusOpen:       0,
			StatusInProgress: 0,
			StatusBlocked:    0,
			StatusClosed:     0,
		},
		ByID: map[string]*Issue{
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
