package main

import (
	"time"

	"github.com/charmbracelet/bubbles/textinput"
)

type Status string

const (
	StatusOpen       Status = "open"
	StatusInProgress Status = "in_progress"
	StatusBlocked    Status = "blocked"
	StatusClosed     Status = "closed"
)

var statusOrder = []Status{StatusOpen, StatusInProgress, StatusBlocked, StatusClosed}

func (s Status) Label() string {
	switch s {
	case StatusOpen:
		return "Open"
	case StatusInProgress:
		return "In Progress"
	case StatusBlocked:
		return "Blocked"
	case StatusClosed:
		return "Closed"
	default:
		return string(s)
	}
}

type Issue struct {
	ID          string
	Title       string
	Description string
	Status      Status
	Display     Status
	Priority    int
	IssueType   string
	Assignee    string
	Labels      []string
	CreatedAt   string
	UpdatedAt   string
	ClosedAt    string

	Parent    string
	Children  []string
	BlockedBy []string
	Blocks    []string
}

type Filter struct {
	Assignee string
	Label    string
	Status   string
	Priority string
}

func (f Filter) IsEmpty() bool {
	return f.Assignee == "" && f.Label == "" && f.Status == "any" && f.Priority == "any"
}

type Mode string

const (
	ModeBoard         Mode = "board"
	ModeHelp          Mode = "help"
	ModeSearch        Mode = "search"
	ModeFilter        Mode = "filter"
	ModeCreate        Mode = "create"
	ModeEdit          Mode = "edit"
	ModePrompt        Mode = "prompt"
	ModeParentPicker  Mode = "parent_picker"
	ModeTmuxPicker    Mode = "tmux_picker"
	ModeDepList       Mode = "dep_list"
	ModeConfirmDelete Mode = "confirm_delete"
)

type PromptAction string

const (
	PromptAssignee  PromptAction = "assignee"
	PromptLabels    PromptAction = "labels"
	PromptDepAdd    PromptAction = "dep_add"
	PromptDepRemove PromptAction = "dep_remove"
	PromptParentSet PromptAction = "parent_set"
)

type PromptState struct {
	Title       string
	Description string
	Action      PromptAction
	TargetIssue string
	Input       textinput.Model
}

type IssueForm struct {
	Create   bool
	IssueID  string
	Original *Issue
	Cursor   int

	Title       string
	Description string
	Status      Status
	Priority    int
	IssueType   string
	Assignee    string
	Labels      string
	Parent      string
	ParentIndex int
	ParentOpts  []ParentOption

	Input textinput.Model
}

type ParentOption struct {
	ID        string
	Title     string
	IssueType string
	Priority  int
	Display   Status
}

type ParentPickerState struct {
	TargetIssueID string
	Options       []ParentOption
	Index         int
}

type TmuxPickerState struct {
	IssueID string
	Targets []TmuxTarget
	Index   int
}

type FilterForm struct {
	Cursor   int
	Assignee string
	Label    string
	Status   string
	Priority string
	Input    textinput.Model
}

type DeleteMode string

const (
	DeleteModeForce   DeleteMode = "force"
	DeleteModeCascade DeleteMode = "cascade"
)

type ConfirmDelete struct {
	IssueID  string
	Mode     DeleteMode
	Preview  string
	Selected int
}

type DepListState struct {
	IssueID string
	Lines   []string
	Scroll  int
}

type loadedMsg struct {
	issues []Issue
	hash   string
	err    error
	source string
}

type opMsg struct {
	info string
	err  error
}

type depListMsg struct {
	issueID string
	text    string
	err     error
}

type deletePreviewMsg struct {
	issueID string
	text    string
	err     error
}

type pluginMsg struct {
	info string
	err  error
}

type tickMsg time.Time
