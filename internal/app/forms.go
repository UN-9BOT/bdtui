package app

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
)

var issueTypes = []string{"task", "epic", "bug", "feature", "chore", "decision"}

func newIssueFormCreate(issues []Issue) *IssueForm {
	return newIssueFormCreateWithParent(issues, "")
}

func newIssueFormCreateWithParent(issues []Issue, selectedParent string) *IssueForm {
	in := textinput.New()
	in.Prompt = "> "
	in.CharLimit = 500
	in.Focus()

	parentOpts, parentIdx := buildParentOptions(issues, "", selectedParent)

	f := &IssueForm{
		Create:      true,
		Cursor:      0,
		Priority:    2,
		IssueType:   "task",
		Status:      StatusOpen,
		Assignee:    defaultAssigneeFromEnv(),
		Parent:      parentOpts[parentIdx].ID,
		ParentIndex: parentIdx,
		ParentOpts:  parentOpts,
		Input:       in,
	}
	f.loadInputFromField()
	return f
}

func newIssueFormEdit(issue *Issue, issues []Issue) *IssueForm {
	in := textinput.New()
	in.Prompt = "> "
	in.CharLimit = 500
	in.Focus()

	labels := ""
	if len(issue.Labels) > 0 {
		labels = strings.Join(issue.Labels, ", ")
	}

	parentOpts, parentIdx := buildParentOptions(issues, issue.ID, issue.Parent)

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
		ParentIndex: parentIdx,
		ParentOpts:  parentOpts,
		Input:       in,
	}
	f.loadInputFromField()
	return f
}

func (f *IssueForm) fields() []string {
	if f.Create {
		return []string{"title", "status", "priority", "type", "assignee", "labels", "parent"}
	}
	return []string{"title", "status", "priority", "type", "assignee", "labels", "parent"}
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
	case "title", "assignee", "labels":
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
	case "assignee":
		f.Input.SetValue(f.Assignee)
	case "labels":
		f.Input.SetValue(f.Labels)
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
	case "assignee":
		f.Assignee = v
	case "labels":
		f.Labels = v
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
	case "parent":
		if len(f.ParentOpts) == 0 {
			return
		}
		f.ParentIndex += delta
		if f.ParentIndex < 0 {
			f.ParentIndex = len(f.ParentOpts) - 1
		}
		if f.ParentIndex >= len(f.ParentOpts) {
			f.ParentIndex = 0
		}
		f.Parent = f.ParentOpts[f.ParentIndex].ID
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

func buildParentOptions(issues []Issue, selfID string, selectedParent string) ([]ParentOption, int) {
	opts := []ParentOption{{ID: "", Title: "(none)", IssueType: "none", Priority: 0}}

	for _, issue := range issues {
		if issue.ID == "" || issue.ID == selfID {
			continue
		}
		if issue.Status == StatusClosed {
			continue
		}
		opts = append(opts, ParentOption{
			ID:        issue.ID,
			Title:     issue.Title,
			IssueType: issue.IssueType,
			Priority:  issue.Priority,
			Display:   issue.Display,
		})
	}

	sort.SliceStable(opts[1:], func(i, j int) bool {
		left := opts[i+1]
		right := opts[j+1]
		ls := statusRank(left.Display)
		rs := statusRank(right.Display)
		if ls != rs {
			return ls < rs
		}
		lr := issueTypeRank(left.IssueType)
		rr := issueTypeRank(right.IssueType)
		if lr != rr {
			return lr < rr
		}
		if left.Priority != right.Priority {
			return left.Priority < right.Priority
		}
		if left.ID != right.ID {
			return left.ID < right.ID
		}
		return left.Title < right.Title
	})

	idx := 0
	for i, opt := range opts {
		if opt.ID != "" && strings.EqualFold(opt.ID, selectedParent) {
			idx = i
			break
		}
	}
	return opts, idx
}

func statusRank(s Status) int {
	switch s {
	case StatusOpen:
		return 0
	case StatusInProgress:
		return 1
	case StatusBlocked:
		return 2
	case StatusClosed:
		return 3
	default:
		return 99
	}
}

func issueTypeRank(t string) int {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "epic":
		return 0
	case "feature":
		return 1
	case "task":
		return 2
	case "bug":
		return 3
	case "chore":
		return 4
	case "decision":
		return 5
	default:
		return 99
	}
}

func (f *IssueForm) currentParentOption() ParentOption {
	if len(f.ParentOpts) == 0 {
		return ParentOption{ID: f.Parent}
	}
	if f.ParentIndex >= 0 && f.ParentIndex < len(f.ParentOpts) {
		opt := f.ParentOpts[f.ParentIndex]
		if strings.EqualFold(opt.ID, f.Parent) {
			return opt
		}
	}
	for i, opt := range f.ParentOpts {
		if strings.EqualFold(opt.ID, f.Parent) {
			f.ParentIndex = i
			return opt
		}
	}
	return ParentOption{ID: f.Parent}
}

func (f *IssueForm) parentDisplay() string {
	opt := f.currentParentOption()
	if strings.TrimSpace(opt.ID) == "" {
		return "-"
	}
	meta := fmt.Sprintf("[%s P%d]", opt.IssueType, opt.Priority)
	title := strings.TrimSpace(opt.Title)
	if title == "" {
		return fmt.Sprintf("%s %s", opt.ID, meta)
	}
	return fmt.Sprintf("%s %s %s", opt.ID, title, meta)
}

func (f *IssueForm) parentHints(limit int) []string {
	if len(f.ParentOpts) == 0 {
		return []string{"(none)"}
	}
	if limit <= 0 {
		limit = 5
	}

	center := f.ParentIndex
	if center < 0 || center >= len(f.ParentOpts) {
		center = 0
	}

	start := center - (limit / 2)
	if start < 0 {
		start = 0
	}
	end := start + limit
	if end > len(f.ParentOpts) {
		end = len(f.ParentOpts)
		start = max(0, end-limit)
	}

	out := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		opt := f.ParentOpts[i]
		label := "(none)"
		if opt.ID != "" {
			label = opt.ID
		}
		if i == center {
			label = "[" + label + "]"
		}
		out = append(out, label)
	}
	return out
}

func newParentPickerState(issues []Issue, targetIssueID string, selectedParent string) *ParentPickerState {
	opts, idx := buildParentOptions(issues, targetIssueID, selectedParent)
	return &ParentPickerState{
		TargetIssueID: targetIssueID,
		Options:       opts,
		Index:         idx,
	}
}

func newFilterForm(base Filter) *FilterForm {
	in := textinput.New()
	in.Prompt = "> "
	in.CharLimit = 200
	in.Focus()

	if base.Assignee == "" {
		base.Assignee = "any"
	}
	if base.Label == "" {
		base.Label = "any"
	}
	if base.Status == "" {
		base.Status = "any"
	}
	if base.Priority == "" {
		base.Priority = "any"
	}
	if base.Type == "" {
		base.Type = "any"
	}

	f := &FilterForm{
		Cursor:   0,
		Assignee: base.Assignee,
		Label:    base.Label,
		Status:   base.Status,
		Priority: base.Priority,
		Type:     base.Type,
		Input:    in,
	}
	f.loadInput()
	return f
}

func (f *FilterForm) fields() []string {
	return []string{"assignee", "label", "status", "priority", "type"}
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
	case "type":
		opts := []string{"any", "task", "epic", "bug", "feature", "chore", "decision"}
		idx := 0
		for i, v := range opts {
			if v == f.Type {
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
		f.Type = opts[idx]
	}
}

func (f *FilterForm) toFilter() Filter {
	f.saveInput()
	assignee := strings.TrimSpace(f.Assignee)
	if strings.EqualFold(assignee, "any") {
		assignee = ""
	}
	label := strings.TrimSpace(f.Label)
	if strings.EqualFold(label, "any") {
		label = ""
	}
	return Filter{
		Assignee: assignee,
		Label:    label,
		Status:   strings.TrimSpace(f.Status),
		Priority: strings.TrimSpace(f.Priority),
		Type:     strings.TrimSpace(f.Type),
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
