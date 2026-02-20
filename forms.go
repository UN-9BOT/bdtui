package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
)

var issueTypes = []string{"task", "epic", "bug", "feature", "chore", "decision"}

func newIssueFormCreate() *IssueForm {
	in := textinput.New()
	in.Prompt = "> "
	in.CharLimit = 500
	in.Focus()

	f := &IssueForm{
		Create:    true,
		Cursor:    0,
		Priority:  2,
		IssueType: "task",
		Status:    StatusOpen,
		Assignee:  defaultAssigneeFromEnv(),
		Input:     in,
	}
	f.loadInputFromField()
	return f
}

func newIssueFormEdit(issue *Issue) *IssueForm {
	in := textinput.New()
	in.Prompt = "> "
	in.CharLimit = 500
	in.Focus()

	labels := ""
	if len(issue.Labels) > 0 {
		labels = strings.Join(issue.Labels, ", ")
	}

	f := &IssueForm{
		Create:      false,
		IssueID:     issue.ID,
		Original:    issue,
		Cursor:      0,
		Title:       issue.Title,
		Description: issue.Description,
		Status:      issue.Status,
		Priority:    issue.Priority,
		IssueType:   issue.IssueType,
		Assignee:    issue.Assignee,
		Labels:      labels,
		Parent:      issue.Parent,
		Input:       in,
	}
	f.loadInputFromField()
	return f
}

func (f *IssueForm) fields() []string {
	if f.Create {
		return []string{"title", "description", "priority", "type", "assignee", "labels", "parent"}
	}
	return []string{"title", "description", "status", "priority", "type", "assignee", "labels", "parent"}
}

func (f *IssueForm) currentField() string {
	fields := f.fields()
	if f.Cursor < 0 {
		f.Cursor = 0
	}
	if f.Cursor >= len(fields) {
		f.Cursor = len(fields) - 1
	}
	return fields[f.Cursor]
}

func (f *IssueForm) nextField() {
	f.saveInputToField()
	fields := f.fields()
	if f.Cursor < len(fields)-1 {
		f.Cursor++
	} else {
		f.Cursor = 0
	}
	f.loadInputFromField()
}

func (f *IssueForm) prevField() {
	f.saveInputToField()
	fields := f.fields()
	if f.Cursor > 0 {
		f.Cursor--
	} else {
		f.Cursor = len(fields) - 1
	}
	f.loadInputFromField()
}

func (f *IssueForm) isTextField(field string) bool {
	switch field {
	case "title", "description", "assignee", "labels", "parent":
		return true
	default:
		return false
	}
}

func (f *IssueForm) loadInputFromField() {
	field := f.currentField()
	if !f.isTextField(field) {
		f.Input.Blur()
		f.Input.SetValue("")
		return
	}
	f.Input.Focus()
	switch field {
	case "title":
		f.Input.SetValue(f.Title)
	case "description":
		f.Input.SetValue(f.Description)
	case "assignee":
		f.Input.SetValue(f.Assignee)
	case "labels":
		f.Input.SetValue(f.Labels)
	case "parent":
		f.Input.SetValue(f.Parent)
	}
	f.Input.CursorEnd()
}

func (f *IssueForm) saveInputToField() {
	field := f.currentField()
	if !f.isTextField(field) {
		return
	}
	v := strings.TrimSpace(f.Input.Value())
	switch field {
	case "title":
		f.Title = v
	case "description":
		f.Description = v
	case "assignee":
		f.Assignee = v
	case "labels":
		f.Labels = v
	case "parent":
		f.Parent = v
	}
}

func (f *IssueForm) cycleEnum(delta int) {
	field := f.currentField()
	switch field {
	case "priority":
		f.Priority += delta
		if f.Priority < 0 {
			f.Priority = 4
		}
		if f.Priority > 4 {
			f.Priority = 0
		}
	case "status":
		statuses := []Status{StatusOpen, StatusInProgress, StatusBlocked, StatusClosed}
		idx := 0
		for i, s := range statuses {
			if s == f.Status {
				idx = i
				break
			}
		}
		idx += delta
		if idx < 0 {
			idx = len(statuses) - 1
		}
		if idx >= len(statuses) {
			idx = 0
		}
		f.Status = statuses[idx]
	case "type":
		idx := 0
		for i, t := range issueTypes {
			if t == f.IssueType {
				idx = i
				break
			}
		}
		idx += delta
		if idx < 0 {
			idx = len(issueTypes) - 1
		}
		if idx >= len(issueTypes) {
			idx = 0
		}
		f.IssueType = issueTypes[idx]
	}
}

func (f *IssueForm) Validate() error {
	f.saveInputToField()
	if strings.TrimSpace(f.Title) == "" {
		return fmt.Errorf("title is required")
	}
	if f.Priority < 0 || f.Priority > 4 {
		return fmt.Errorf("priority must be 0..4")
	}
	if f.IssueType == "" {
		return fmt.Errorf("issue type is required")
	}
	return nil
}

func newFilterForm(base Filter) *FilterForm {
	in := textinput.New()
	in.Prompt = "> "
	in.CharLimit = 200
	in.Focus()

	if base.Status == "" {
		base.Status = "any"
	}
	if base.Priority == "" {
		base.Priority = "any"
	}

	f := &FilterForm{
		Cursor:   0,
		Assignee: base.Assignee,
		Label:    base.Label,
		Status:   base.Status,
		Priority: base.Priority,
		Input:    in,
	}
	f.loadInput()
	return f
}

func (f *FilterForm) fields() []string {
	return []string{"assignee", "label", "status", "priority"}
}

func (f *FilterForm) currentField() string {
	fields := f.fields()
	if f.Cursor < 0 {
		f.Cursor = 0
	}
	if f.Cursor >= len(fields) {
		f.Cursor = len(fields) - 1
	}
	return fields[f.Cursor]
}

func (f *FilterForm) nextField() {
	f.saveInput()
	f.Cursor = (f.Cursor + 1) % len(f.fields())
	f.loadInput()
}

func (f *FilterForm) prevField() {
	f.saveInput()
	f.Cursor--
	if f.Cursor < 0 {
		f.Cursor = len(f.fields()) - 1
	}
	f.loadInput()
}

func (f *FilterForm) isTextField(field string) bool {
	return field == "assignee" || field == "label"
}

func (f *FilterForm) loadInput() {
	field := f.currentField()
	if !f.isTextField(field) {
		f.Input.Blur()
		f.Input.SetValue("")
		return
	}
	f.Input.Focus()
	if field == "assignee" {
		f.Input.SetValue(f.Assignee)
	} else {
		f.Input.SetValue(f.Label)
	}
	f.Input.CursorEnd()
}

func (f *FilterForm) saveInput() {
	field := f.currentField()
	if !f.isTextField(field) {
		return
	}
	val := strings.TrimSpace(f.Input.Value())
	if field == "assignee" {
		f.Assignee = val
	} else {
		f.Label = val
	}
}

func (f *FilterForm) cycleEnum(delta int) {
	field := f.currentField()
	switch field {
	case "status":
		opts := []string{"any", "open", "in_progress", "blocked", "closed"}
		idx := 0
		for i, v := range opts {
			if v == f.Status {
				idx = i
				break
			}
		}
		idx += delta
		if idx < 0 {
			idx = len(opts) - 1
		}
		if idx >= len(opts) {
			idx = 0
		}
		f.Status = opts[idx]
	case "priority":
		opts := []string{"any", "0", "1", "2", "3", "4"}
		idx := 0
		for i, v := range opts {
			if v == f.Priority {
				idx = i
				break
			}
		}
		idx += delta
		if idx < 0 {
			idx = len(opts) - 1
		}
		if idx >= len(opts) {
			idx = 0
		}
		f.Priority = opts[idx]
	}
}

func (f *FilterForm) toFilter() Filter {
	f.saveInput()
	return Filter{
		Assignee: strings.TrimSpace(f.Assignee),
		Label:    strings.TrimSpace(f.Label),
		Status:   strings.TrimSpace(f.Status),
		Priority: strings.TrimSpace(f.Priority),
	}
}

func newPrompt(mode Mode, title, description, issueID string, action PromptAction, initial string) *PromptState {
	in := textinput.New()
	in.Prompt = "> "
	in.CharLimit = 240
	in.Focus()
	in.SetValue(initial)
	in.CursorEnd()
	return &PromptState{
		Title:       title,
		Description: description,
		Action:      action,
		TargetIssue: issueID,
		Input:       in,
	}
}
