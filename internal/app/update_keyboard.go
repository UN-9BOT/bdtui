package app

import (
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.Type == tea.KeyCtrlC {
		m.clearSearchAndFilters()
		m.setToast("success", "search and filters cleared")
		return m, nil
	}

	switch m.Mode {
	case ModeHelp:
		return m.handleHelpKey(msg)
	case ModeDetails:
		return m.handleDetailsKey(msg)
	case ModeSearch:
		return m.handleSearchKey(msg)
	case ModeFilter:
		return m.handleFilterKey(msg)
	case ModeCreate, ModeEdit:
		return m.handleFormKey(msg)
	case ModePrompt:
		return m.handlePromptKey(msg)
	case ModeParentPicker:
		return m.handleParentPickerKey(msg)
	case ModeTmuxPicker:
		return m.handleTmuxPickerKey(msg)
	case ModeDepList:
		return m.handleDepListKey(msg)
	case ModeConfirmDelete:
		return m.handleDeleteConfirmKey(msg)
	case ModeConfirmClosedParentCreate:
		return m.handleConfirmClosedParentCreateKey(msg)
	default:
		return m.handleBoardKey(msg)
	}
}

func (m model) handleHelpKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	switch key {
	case "?", "esc", "q":
		m.Mode = ModeBoard
		m.HelpScroll = 0
		m.HelpQuery = ""
		return m, nil
	case "down":
		maxOffset := m.helpMaxScroll()
		if m.HelpScroll < maxOffset {
			m.HelpScroll++
		}
		return m, nil
	case "up":
		if m.HelpScroll > 0 {
			m.HelpScroll--
		}
		return m, nil
	case "backspace":
		if m.HelpQuery == "" {
			return m, nil
		}
		queryRunes := []rune(m.HelpQuery)
		m.HelpQuery = string(queryRunes[:len(queryRunes)-1])
		m.HelpScroll = 0
		return m, nil
	case "ctrl+u":
		m.HelpQuery = ""
		m.HelpScroll = 0
		return m, nil
	}

	if msg.Type == tea.KeyRunes && len(msg.Runes) > 0 {
		m.HelpQuery += string(msg.Runes)
		m.HelpScroll = 0
		return m, nil
	}

	return m, nil
}

func (m model) handleDetailsKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	switch key {
	case "q":
		return m, tea.Quit
	case "esc", "enter", " ":
		m.ShowDetails = false
		m.Mode = ModeBoard
		m.DetailsScroll = 0
		m.DetailsIssueID = ""
		return m, nil
	case "j", "down":
		issue := m.currentIssue()
		maxOffset := m.detailsMaxScroll(issue)
		if m.DetailsScroll < maxOffset {
			m.DetailsScroll++
		}
		return m, nil
	case "k", "up":
		if m.DetailsScroll > 0 {
			m.DetailsScroll--
		}
		return m, nil
	case "ctrl+x":
		if !m.activateEditForCurrentIssue() {
			return m, nil
		}
		m.ResumeDetailsAfterEditor = true
		cmd, err := m.openFormInEditor()
		if err != nil {
			m.ResumeDetailsAfterEditor = false
			m.Mode = ModeDetails
			m.Form = nil
			m.setToast("error", err.Error())
			return m, nil
		}
		return m, cmd
	}
	return m, nil
}

func (m model) handleSearchKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.FilterForm == nil {
		m.FilterForm = newFilterForm(m.Filter)
	}

	key := msg.String()
	switch key {
	case "esc":
		m.applyFilterForm()
		m.SearchExpanded = false
		m.SearchInput.Blur()
		m.Mode = ModeBoard
		return m, nil
	case "enter":
		m.applyFilterForm()
		m.SearchExpanded = false
		m.SearchInput.Blur()
		m.Mode = ModeBoard
		return m, nil
	case "ctrl+f":
		m.SearchExpanded = true
		return m, nil
	case "up":
		if m.SearchExpanded {
			m.shiftSearchFilterField(-1)
			return m, nil
		}
	case "down":
		if m.SearchExpanded {
			m.shiftSearchFilterField(1)
			return m, nil
		}
	case "tab", "ctrl+i":
		if m.SearchExpanded {
			m.cycleSearchFilterValue(1)
			return m, nil
		}
	case "shift+tab":
		if m.SearchExpanded {
			m.cycleSearchFilterValue(-1)
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.SearchInput, cmd = m.SearchInput.Update(msg)
	nextQuery := strings.TrimSpace(m.SearchInput.Value())
	if nextQuery != m.SearchQuery {
		m.SearchQuery = nextQuery
		m.computeColumns()
		m.normalizeSelectionBounds()
	}
	return m, cmd
}

func (m *model) clearSearchAndFilters() {
	m.SearchQuery = ""
	m.SearchInput.SetValue("")
	m.SearchInput.CursorStart()
	m.SearchExpanded = false
	m.Filter = Filter{
		Status:   "any",
		Priority: "any",
		Type:     "any",
	}
	if m.FilterForm != nil {
		m.FilterForm = newFilterForm(m.Filter)
	}
	m.computeColumns()
	m.normalizeSelectionBounds()
}

func (m *model) applyFilterForm() {
	if m.FilterForm == nil {
		m.FilterForm = newFilterForm(m.Filter)
	}
	m.Filter = m.FilterForm.toFilter()
	if m.Filter.Status == "" {
		m.Filter.Status = "any"
	}
	if m.Filter.Priority == "" {
		m.Filter.Priority = "any"
	}
	if m.Filter.Type == "" {
		m.Filter.Type = "any"
	}
	m.computeColumns()
	m.normalizeSelectionBounds()
}

func (m *model) shiftSearchFilterField(delta int) {
	if m.FilterForm == nil {
		m.FilterForm = newFilterForm(m.Filter)
	}
	fields := m.FilterForm.fields()
	if len(fields) == 0 {
		return
	}
	m.FilterForm.Cursor += delta
	if m.FilterForm.Cursor < 0 {
		m.FilterForm.Cursor = len(fields) - 1
	}
	if m.FilterForm.Cursor >= len(fields) {
		m.FilterForm.Cursor = 0
	}
}

func (m *model) cycleSearchFilterValue(delta int) {
	if m.FilterForm == nil {
		m.FilterForm = newFilterForm(m.Filter)
	}
	field := m.FilterForm.currentField()
	options := m.searchFilterOptions(field)
	if len(options) == 0 {
		return
	}

	current := m.searchFilterFieldValue(field)
	idx := 0
	for i, option := range options {
		if strings.EqualFold(option, current) {
			idx = i
			break
		}
	}
	idx += delta
	if idx < 0 {
		idx = len(options) - 1
	}
	if idx >= len(options) {
		idx = 0
	}

	m.setSearchFilterFieldValue(field, options[idx])
	m.applyFilterForm()
}

func (m model) searchFilterFieldValue(field string) string {
	if m.FilterForm == nil {
		return "any"
	}
	switch field {
	case "assignee":
		return m.FilterForm.Assignee
	case "label":
		return m.FilterForm.Label
	case "status":
		return m.FilterForm.Status
	case "priority":
		return m.FilterForm.Priority
	case "type":
		return m.FilterForm.Type
	default:
		return "any"
	}
}

func (m *model) setSearchFilterFieldValue(field string, value string) {
	if m.FilterForm == nil {
		return
	}
	switch field {
	case "assignee":
		m.FilterForm.Assignee = value
		m.FilterForm.Input.SetValue(value)
	case "label":
		m.FilterForm.Label = value
		m.FilterForm.Input.SetValue(value)
	case "status":
		m.FilterForm.Status = value
	case "priority":
		m.FilterForm.Priority = value
	case "type":
		m.FilterForm.Type = value
	}
}

func (m model) searchFilterOptions(field string) []string {
	switch field {
	case "assignee":
		out := []string{"any"}
		seen := map[string]bool{}
		for _, issue := range m.Issues {
			value := strings.TrimSpace(issue.Assignee)
			if value == "" {
				continue
			}
			key := strings.ToLower(value)
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, value)
		}
		sort.Slice(out[1:], func(i, j int) bool {
			return strings.ToLower(out[i+1]) < strings.ToLower(out[j+1])
		})
		return out
	case "label":
		out := []string{"any"}
		seen := map[string]bool{}
		for _, issue := range m.Issues {
			for _, raw := range issue.Labels {
				value := strings.TrimSpace(raw)
				if value == "" {
					continue
				}
				key := strings.ToLower(value)
				if seen[key] {
					continue
				}
				seen[key] = true
				out = append(out, value)
			}
		}
		sort.Slice(out[1:], func(i, j int) bool {
			return strings.ToLower(out[i+1]) < strings.ToLower(out[j+1])
		})
		return out
	case "status":
		return []string{"any", "open", "in_progress", "blocked", "closed"}
	case "priority":
		return []string{"any", "0", "1", "2", "3", "4"}
	case "type":
		return []string{"any", "task", "epic", "bug", "feature", "chore", "decision"}
	default:
		return nil
	}
}

func (m model) handleFilterKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.FilterForm == nil {
		m.FilterForm = newFilterForm(m.Filter)
	}

	key := msg.String()
	switch key {
	case "esc":
		m.Mode = ModeBoard
		m.FilterForm = nil
		return m, nil
	case "enter":
		m.Filter = m.FilterForm.toFilter()
		if m.Filter.Status == "" {
			m.Filter.Status = "any"
		}
		if m.Filter.Priority == "" {
			m.Filter.Priority = "any"
		}
		if m.Filter.Type == "" {
			m.Filter.Type = "any"
		}
		m.computeColumns()
		m.normalizeSelectionBounds()
		m.Mode = ModeBoard
		m.FilterForm = nil
		m.setToast("success", "filters applied")
		return m, nil
	case "tab":
		m.FilterForm.nextField()
		return m, nil
	case "shift+tab":
		m.FilterForm.prevField()
		return m, nil
	case "up":
		if !m.FilterForm.isTextField(m.FilterForm.currentField()) {
			m.FilterForm.cycleEnum(-1)
			return m, nil
		}
	case "down":
		if !m.FilterForm.isTextField(m.FilterForm.currentField()) {
			m.FilterForm.cycleEnum(1)
			return m, nil
		}
	case "c":
		m.FilterForm.Assignee = ""
		m.FilterForm.Label = ""
		m.FilterForm.Status = "any"
		m.FilterForm.Priority = "any"
		m.FilterForm.Type = "any"
		m.FilterForm.loadInput()
		return m, nil
	}

	if m.FilterForm.isTextField(m.FilterForm.currentField()) {
		var cmd tea.Cmd
		m.FilterForm.Input, cmd = m.FilterForm.Input.Update(msg)
		m.FilterForm.saveInput()
		return m, cmd
	}

	return m, nil
}

func (m model) handleFormKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.Form == nil {
		m.Mode = ModeBoard
		return m, nil
	}

	key := msg.String()

	switch key {
	case "esc":
		m.Form.saveInputToField()
		if m.Mode == ModeCreate && strings.TrimSpace(m.Form.Title) == "" {
			m.Mode = ModeBoard
			m.Form = nil
			m.CreateBlockerID = ""
			m.setToast("info", "creation canceled")
			return m, nil
		}
		if err := m.Form.Validate(); err != nil {
			m.setToast("error", err.Error())
			return m, nil
		}
		return m.submitForm()
	case "enter", "ctrl+s":
		if err := m.Form.Validate(); err != nil {
			m.setToast("error", err.Error())
			return m, nil
		}
		return m.submitForm()
	case "up":
		m.Form.prevField()
		return m, nil
	case "down":
		m.Form.nextField()
		return m, nil
	case "tab":
		if !m.Form.isTextField(m.Form.currentField()) {
			m.Form.cycleEnum(1)
		} else {
			m.Form.nextField()
		}
		return m, nil
	case "shift+tab":
		if !m.Form.isTextField(m.Form.currentField()) {
			m.Form.cycleEnum(-1)
		} else {
			m.Form.prevField()
		}
		return m, nil
	case "ctrl+x":
		m.Form.saveInputToField()
		m.ResumeDetailsAfterEditor = false
		cmd, err := m.openFormInEditor()
		if err != nil {
			m.setToast("error", err.Error())
			return m, nil
		}
		return m, cmd
	}

	if m.Form.isTextField(m.Form.currentField()) {
		var cmd tea.Cmd
		m.Form.Input, cmd = m.Form.Input.Update(msg)
		m.Form.saveInputToField()
		return m, cmd
	}

	return m, nil
}

func (m model) handlePromptKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.Prompt == nil {
		m.Mode = ModeBoard
		return m, nil
	}

	key := msg.String()
	switch key {
	case "esc":
		m.Mode = ModeBoard
		m.Prompt = nil
		return m, nil
	case "enter":
		value := strings.TrimSpace(m.Prompt.Input.Value())
		issueID := m.Prompt.TargetIssue
		action := m.Prompt.Action
		m.Mode = ModeBoard
		m.Prompt = nil
		return m, m.submitPrompt(issueID, action, value)
	}

	var cmd tea.Cmd
	m.Prompt.Input, cmd = m.Prompt.Input.Update(msg)
	return m, cmd
}

func (m model) handleParentPickerKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.ParentPicker == nil {
		m.Mode = ModeBoard
		return m, nil
	}

	key := msg.String()
	switch key {
	case "esc", "q":
		m.ParentPicker = nil
		m.Mode = ModeBoard
		return m, nil
	case "j", "down":
		if len(m.ParentPicker.Options) > 0 {
			m.ParentPicker.Index = (m.ParentPicker.Index + 1) % len(m.ParentPicker.Options)
		}
		return m, nil
	case "k", "up":
		if len(m.ParentPicker.Options) > 0 {
			m.ParentPicker.Index--
			if m.ParentPicker.Index < 0 {
				m.ParentPicker.Index = len(m.ParentPicker.Options) - 1
			}
		}
		return m, nil
	case "enter":
		if len(m.ParentPicker.Options) == 0 {
			m.setToast("warning", "no parent candidates available")
			m.ParentPicker = nil
			m.Mode = ModeBoard
			return m, nil
		}
		targetID := m.ParentPicker.TargetIssueID
		selected := m.ParentPicker.Options[m.ParentPicker.Index]
		parent := strings.TrimSpace(selected.ID)
		m.ParentPicker = nil
		m.Mode = ModeBoard
		return m, opCmd("parent updated", func() error {
			return m.Client.UpdateIssue(UpdateParams{ID: targetID, Parent: &parent})
		})
	}
	return m, nil
}

func (m model) handleDepListKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.DepList == nil {
		m.Mode = ModeBoard
		return m, nil
	}

	key := msg.String()
	switch key {
	case "esc", "q":
		m.DepList = nil
		m.Mode = ModeBoard
	case "j", "down":
		if m.DepList.Scroll < len(m.DepList.Lines)-1 {
			m.DepList.Scroll++
		}
	case "k", "up":
		if m.DepList.Scroll > 0 {
			m.DepList.Scroll--
		}
	}
	return m, nil
}

func (m model) handleTmuxPickerKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.TmuxPicker == nil {
		m.Mode = ModeBoard
		return m, nil
	}

	key := msg.String()
	switch key {
	case "esc", "q":
		cleanupCmd := m.scheduleTmuxMarkCleanup(5 * time.Second)
		m.TmuxPicker = nil
		m.Mode = ModeBoard
		return m, cleanupCmd
	case "j", "down":
		if len(m.TmuxPicker.Targets) > 0 {
			m.TmuxPicker.Index = (m.TmuxPicker.Index + 1) % len(m.TmuxPicker.Targets)
		}
		if err := m.markTmuxPickerSelection(); err != nil {
			m.setToast("warning", err.Error())
			return m, nil
		}
		return m, m.blinkTmuxPaneCmd(m.currentTmuxPickerPaneID())
	case "k", "up":
		if len(m.TmuxPicker.Targets) > 0 {
			m.TmuxPicker.Index--
			if m.TmuxPicker.Index < 0 {
				m.TmuxPicker.Index = len(m.TmuxPicker.Targets) - 1
			}
		}
		if err := m.markTmuxPickerSelection(); err != nil {
			m.setToast("warning", err.Error())
			return m, nil
		}
		return m, m.blinkTmuxPaneCmd(m.currentTmuxPickerPaneID())
	case "enter":
		if len(m.TmuxPicker.Targets) == 0 {
			cleanupCmd := m.scheduleTmuxMarkCleanup(5 * time.Second)
			m.TmuxPicker = nil
			m.Mode = ModeBoard
			m.setToast("warning", "no tmux targets")
			return m, cleanupCmd
		}

		selected := m.TmuxPicker.Targets[m.TmuxPicker.Index]
		issueID := strings.TrimSpace(m.TmuxPicker.IssueID)
		cleanupCmd := m.scheduleTmuxMarkCleanup(5 * time.Second)
		m.TmuxPicker = nil
		m.Mode = ModeBoard

		tmuxPlugin := m.Plugins.Tmux()
		if tmuxPlugin == nil || !tmuxPlugin.Enabled() {
			m.setToast("warning", "tmux plugin disabled")
			return m, cleanupCmd
		}
		tmuxPlugin.SetTarget(selected)

		if issueID == "" {
			m.setToast("success", "tmux target selected: "+selected.Label())
			return m, cleanupCmd
		}
		payload := m.formatBeadsStartTaskCommand(issueID)

		return m, tea.Batch(
			cleanupCmd,
			func() tea.Msg {
				if err := tmuxPlugin.SendTextToBuffer(payload); err != nil {
					return pluginMsg{info: "tmux command pasted", err: err}
				}
				if err := tmuxPlugin.FocusPane(selected.PaneID); err != nil {
					return pluginMsg{
						info:    "tmux command pasted",
						warning: "focus pane failed: " + err.Error(),
					}
				}
				return pluginMsg{info: "tmux command pasted"}
			},
		)
	}

	return m, nil
}

func (m model) handleDeleteConfirmKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.ConfirmDelete == nil {
		m.Mode = ModeBoard
		return m, nil
	}

	key := msg.String()
	switch key {
	case "esc", "n":
		m.ConfirmDelete = nil
		m.Mode = ModeBoard
		return m, nil
	case "1":
		m.ConfirmDelete.Mode = DeleteModeForce
		m.ConfirmDelete.Selected = 0
		return m, nil
	case "2":
		m.ConfirmDelete.Mode = DeleteModeCascade
		m.ConfirmDelete.Selected = 1
		return m, nil
	case "y", "enter":
		issueID := m.ConfirmDelete.IssueID
		mode := m.ConfirmDelete.Mode
		m.ConfirmDelete = nil
		m.Mode = ModeBoard
		return m, opCmd("issue deleted", func() error {
			return m.Client.DeleteIssue(issueID, mode)
		})
	}

	return m, nil
}

func (m model) handleConfirmClosedParentCreateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.ConfirmClosedParentCreate == nil {
		m.Mode = ModeBoard
		return m, nil
	}

	key := msg.String()
	switch key {
	case "esc", "n":
		m.ConfirmClosedParentCreate = nil
		m.Mode = ModeBoard
		m.setToast("warning", "task creation canceled")
		return m, nil
	case "y", "enter":
		parentID := m.ConfirmClosedParentCreate.ParentID
		targetStatus := m.ConfirmClosedParentCreate.TargetStatus
		m.ConfirmClosedParentCreate = nil
		m.Mode = ModeBoard
		return m, reopenParentForCreateCmd(m.Client, parentID, targetStatus)
	}

	return m, nil
}

func (m model) handleBoardKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	if key == "q" {
		return m, tea.Quit
	}
	if key == "?" {
		m.HelpScroll = 0
		m.HelpQuery = ""
		m.Mode = ModeHelp
		return m, nil
	}

	if m.Leader {
		m.Leader = false
		return m.handleLeaderCombo(key)
	}

	switch key {
	case "left", "h":
		m.moveColumn(-1)
		return m, nil
	case "right", "l":
		m.moveColumn(1)
		return m, nil
	case "up", "k":
		m.moveSelection(-1)
		return m, nil
	case "down", "j":
		m.moveSelection(1)
		return m, nil
	case "0":
		st := m.currentStatus()
		m.SelectedIdx[st] = 0
		m.ensureSelectionVisible(st)
		return m, nil
	case "G":
		st := m.currentStatus()
		col := m.Columns[st]
		if len(col) > 0 {
			m.SelectedIdx[st] = len(col) - 1
			m.ensureSelectionVisible(st)
		}
		return m, nil
	case "enter", " ":
		issue := m.currentIssue()
		if issue == nil {
			m.setToast("warning", "no issue selected")
			return m, nil
		}
		m.ShowDetails = true
		m.Mode = ModeDetails
		m.DetailsScroll = 0
		m.DetailsIssueID = issue.ID
		return m, nil
	case "/":
		m.SearchInput.SetValue(m.SearchQuery)
		m.SearchInput.CursorEnd()
		m.SearchInput.Focus()
		m.SearchExpanded = false
		m.FilterForm = newFilterForm(m.Filter)
		m.Mode = ModeSearch
		return m, nil
	case "f":
		m.SearchInput.SetValue(m.SearchQuery)
		m.SearchInput.CursorEnd()
		m.SearchInput.Focus()
		m.SearchExpanded = false
		m.FilterForm = newFilterForm(m.Filter)
		m.Mode = ModeSearch
		return m, nil
	case "c":
		m.clearSearchAndFilters()
		m.setToast("success", "search and filters cleared")
		return m, nil
	case "r":
		return m, m.loadCmd("manual")
	case "n":
		m.CreateBlockerID = ""
		m.Form = newIssueFormCreate(m.Issues)
		m.Mode = ModeCreate
		return m, nil
	case "N":
		m.CreateBlockerID = ""
		issue := m.currentIssue()
		if issue == nil {
			m.setToast("warning", "no issue selected")
			return m, nil
		}
		if issue.Status == StatusClosed {
			m.ConfirmClosedParentCreate = &ConfirmClosedParentCreate{
				ParentID:     issue.ID,
				ParentTitle:  issue.Title,
				TargetStatus: StatusInProgress,
			}
			m.Mode = ModeConfirmClosedParentCreate
			return m, nil
		}
		m.Form = newIssueFormCreateWithParent(m.Issues, issue.ID)
		m.Mode = ModeCreate
		return m, nil
	case "b":
		issue := m.currentIssue()
		if issue == nil {
			m.setToast("warning", "no issue selected")
			return m, nil
		}
		m.CreateBlockerID = strings.TrimSpace(issue.ID)
		m.Form = newIssueFormCreate(m.Issues)
		m.Form.Status = StatusBlocked
		m.Mode = ModeCreate
		m.setToast("info", "create blocked issue")
		return m, nil
	case "e":
		if !m.activateEditForCurrentIssue() {
			return m, nil
		}
		return m, nil
	case "ctrl+x":
		if !m.activateEditForCurrentIssue() {
			return m, nil
		}
		m.ResumeDetailsAfterEditor = false
		cmd, err := m.openFormInEditor()
		if err != nil {
			m.setToast("error", err.Error())
			return m, nil
		}
		return m, cmd
	case "a":
		issue := m.currentIssue()
		if issue == nil {
			m.setToast("warning", "no issue selected")
			return m, nil
		}
		m.Prompt = newPrompt(ModePrompt, "Quick Assignee", "Enter assignee (empty = unassign)", issue.ID, PromptAssignee, issue.Assignee)
		m.Mode = ModePrompt
		return m, nil
	case "t":
		issue := m.currentIssue()
		if issue == nil {
			m.setToast("warning", "no issue selected")
			return m, nil
		}
		m.Collapsed[issue.ID] = !m.Collapsed[issue.ID]
		m.computeColumns()
		m.normalizeSelectionBounds()
		if m.Collapsed[issue.ID] {
			m.setToast("success", issue.ID+" children hidden")
		} else {
			m.setToast("success", issue.ID+" children shown")
		}
		return m, nil
	case "y":
		issue := m.currentIssue()
		if issue == nil {
			m.setToast("warning", "no issue selected")
			return m, nil
		}
		if err := copyToClipboard(issue.ID); err != nil {
			m.setToast("error", err.Error())
			return m, nil
		}
		m.setToast("success", "copied id: "+issue.ID)
		return m, nil
	case "Y":
		return m.handleTmuxSendIssueID()
	case "L":
		issue := m.currentIssue()
		if issue == nil {
			m.setToast("warning", "no issue selected")
			return m, nil
		}
		m.Prompt = newPrompt(ModePrompt, "Quick Labels", "Enter labels separated by commas", issue.ID, PromptLabels, strings.Join(issue.Labels, ", "))
		m.Mode = ModePrompt
		return m, nil
	case "p":
		issue := m.currentIssue()
		if issue == nil {
			m.setToast("warning", "no issue selected")
			return m, nil
		}
		id := issue.ID
		next := cyclePriority(issue.Priority)
		return m, opCmd(fmt.Sprintf("%s: priority -> P%d", id, next), func() error {
			p := next
			return m.Client.UpdateIssue(UpdateParams{ID: id, Priority: &p})
		})
	case "P":
		issue := m.currentIssue()
		if issue == nil {
			m.setToast("warning", "no issue selected")
			return m, nil
		}
		id := issue.ID
		next := cyclePriorityBackward(issue.Priority)
		return m, opCmd(fmt.Sprintf("%s: priority -> P%d", id, next), func() error {
			p := next
			return m.Client.UpdateIssue(UpdateParams{ID: id, Priority: &p})
		})
	case "s":
		issue := m.currentIssue()
		if issue == nil {
			m.setToast("warning", "no issue selected")
			return m, nil
		}
		id := issue.ID
		next := cycleStatus(issue.Status)
		return m, opCmd(fmt.Sprintf("%s: status -> %s", id, next), func() error {
			st := next
			return m.Client.UpdateIssue(UpdateParams{ID: id, Status: &st})
		})
	case "S":
		issue := m.currentIssue()
		if issue == nil {
			m.setToast("warning", "no issue selected")
			return m, nil
		}
		id := issue.ID
		next := cycleStatusBackward(issue.Status)
		return m, opCmd(fmt.Sprintf("%s: status -> %s", id, next), func() error {
			st := next
			return m.Client.UpdateIssue(UpdateParams{ID: id, Status: &st})
		})
	case "x":
		issue := m.currentIssue()
		if issue == nil {
			m.setToast("warning", "no issue selected")
			return m, nil
		}
		id := issue.ID
		if issue.Status == StatusClosed {
			return m, opCmd(fmt.Sprintf("%s reopened", id), func() error { return m.Client.ReopenIssue(id) })
		}
		return m, opCmd(fmt.Sprintf("%s closed", id), func() error { return m.Client.CloseIssue(id) })
	case "d":
		issue := m.currentIssue()
		if issue == nil {
			m.setToast("warning", "no issue selected")
			return m, nil
		}
		m.setToast("warning", "loading delete preview...")
		return m, deletePreviewCmd(m.Client, issue.ID)
	case "g":
		m.Leader = true
		m.setToast("info", "Leader: g …")
		return m, nil
	}

	return m, nil
}

func (m model) handleLeaderCombo(key string) (tea.Model, tea.Cmd) {
	if key == "o" {
		m.SortMode = m.SortMode.Toggle()
		m.computeColumns()
		m.normalizeSelectionBounds()
		cmd := persistSortModeCmd(m.Client, m.SortMode)
		if cmd == nil {
			m.setToast("warning", "sort mode changed: "+m.SortMode.Label()+" (not persisted)")
			return m, nil
		}
		return m, cmd
	}

	issue := m.currentIssue()
	if issue == nil {
		m.setToast("warning", "no issue selected")
		return m, nil
	}

	switch key {
	case "B":
		m.Prompt = newPrompt(ModePrompt, "Remove Blocker", "Enter blocker issue ID to remove", issue.ID, PromptDepRemove, "")
		m.Mode = ModePrompt
		return m, nil
	case "p":
		m.ParentPicker = newParentPickerState(m.Issues, issue.ID, issue.Parent)
		m.Mode = ModeParentPicker
		return m, nil
	case "u":
		parentID := strings.TrimSpace(issue.Parent)
		if parentID == "" {
			m.setToast("warning", "issue has no parent")
			return m, nil
		}

		if m.selectIssueByID(parentID) {
			return m, nil
		}

		m.clearSearchAndFilters()
		if m.selectIssueByID(parentID) {
			return m, nil
		}

		m.setToast("warning", fmt.Sprintf("parent not found: %s", parentID))
		return m, nil
	case "P":
		id := issue.ID
		return m, opCmd(fmt.Sprintf("%s parent cleared", id), func() error {
			empty := ""
			return m.Client.UpdateIssue(UpdateParams{ID: id, Parent: &empty})
		})
	case "d":
		m.setToast("info", "loading dependencies...")
		return m, depListCmd(m.Client, issue.ID)
	default:
		m.setToast("warning", "unknown leader combo")
		return m, nil
	}
}

func (m model) submitForm() (tea.Model, tea.Cmd) {
	form := m.Form
	if form == nil {
		m.Mode = ModeBoard
		return m, nil
	}

	form.saveInputToField()
	m.Mode = ModeBoard
	m.Form = nil

	if form.Create {
		params := CreateParams{
			Title:       form.Title,
			Description: form.Description,
			Priority:    form.Priority,
			IssueType:   form.IssueType,
			Assignee:    form.Assignee,
			Labels:      parseLabels(form.Labels),
			Parent:      form.Parent,
		}
		blockerID := strings.TrimSpace(m.CreateBlockerID)
		m.CreateBlockerID = ""

		return m, func() tea.Msg {
			id, err := m.Client.CreateIssue(params)
			if err != nil {
				return opMsg{err: err}
			}

			status := form.Status
			if blockerID != "" {
				status = StatusBlocked
			}
			if status == "" {
				status = StatusOpen
			}
			if status != StatusOpen {
				if strings.TrimSpace(id) == "" {
					return opMsg{err: fmt.Errorf("created issue id is empty, cannot set status to %s", status)}
				}
				if err := m.Client.UpdateIssue(UpdateParams{
					ID:     id,
					Status: &status,
				}); err != nil {
					return opMsg{err: err}
				}
			}

			if blockerID != "" {
				if strings.TrimSpace(id) == "" {
					return opMsg{err: fmt.Errorf("created issue id is empty, cannot add blocker")}
				}
				if err := m.Client.DepAdd(id, blockerID); err != nil {
					return opMsg{err: err}
				}
			}

			if id == "" {
				return opMsg{info: "issue created"}
			}
			if blockerID != "" {
				return opMsg{info: fmt.Sprintf("created %s (blocked by %s)", id, blockerID)}
			}
			return opMsg{info: "created " + id}
		}
	}

	if form.Original == nil {
		return m, opCmd("", func() error { return fmt.Errorf("missing original issue data") })
	}

	upd := UpdateParams{ID: form.IssueID}
	changed := 0

	if form.Title != form.Original.Title {
		v := form.Title
		upd.Title = &v
		changed++
	}
	if form.Description != form.Original.Description {
		v := form.Description
		upd.Description = &v
		changed++
	}
	if form.Status != form.Original.Status {
		v := form.Status
		upd.Status = &v
		changed++
	}
	if form.Priority != form.Original.Priority {
		v := form.Priority
		upd.Priority = &v
		changed++
	}
	if form.IssueType != form.Original.IssueType {
		v := form.IssueType
		upd.IssueType = &v
		changed++
	}
	if strings.TrimSpace(form.Assignee) != strings.TrimSpace(form.Original.Assignee) {
		v := strings.TrimSpace(form.Assignee)
		upd.Assignee = &v
		changed++
	}

	currentLabels := strings.Join(form.Original.Labels, ",")
	newLabels := strings.Join(parseLabels(form.Labels), ",")
	if currentLabels != newLabels {
		labels := parseLabels(form.Labels)
		upd.Labels = &labels
		changed++
	}

	if strings.TrimSpace(form.Parent) != strings.TrimSpace(form.Original.Parent) {
		v := strings.TrimSpace(form.Parent)
		upd.Parent = &v
		changed++
	}

	if changed == 0 {
		m.setToast("info", "no changes")
		return m, nil
	}

	return m, opCmd(fmt.Sprintf("%s updated", form.IssueID), func() error {
		return m.Client.UpdateIssue(upd)
	})
}

func (m model) handleTmuxSendIssueID() (tea.Model, tea.Cmd) {
	issue := m.currentIssue()
	if issue == nil {
		m.setToast("warning", "no issue selected")
		return m, nil
	}

	tmuxPlugin := m.Plugins.Tmux()
	if tmuxPlugin == nil || !tmuxPlugin.Enabled() {
		m.setToast("warning", "tmux plugin disabled (--plugins=tmux)")
		return m, nil
	}

	if tmuxPlugin.CurrentTarget() == nil {
		targets, err := tmuxPlugin.ListTargets()
		if err != nil {
			m.setToast("error", err.Error())
			return m, nil
		}
		if len(targets) == 0 {
			m.setToast("warning", "no tmux targets available")
			return m, nil
		}
		m.cancelTmuxMarkCleanup()
		m.TmuxPicker = &TmuxPickerState{
			IssueID: issue.ID,
			Targets: targets,
			Index:   0,
		}
		m.Mode = ModeTmuxPicker
		if err := m.markTmuxPickerSelection(); err != nil {
			m.setToast("warning", err.Error())
			return m, nil
		}
		return m, m.blinkTmuxPaneCmd(m.currentTmuxPickerPaneID())
	}

	payload := m.formatBeadsStartTaskCommand(issue.ID)
	target := tmuxPlugin.CurrentTarget()
	targetPane := ""
	if target != nil {
		targetPane = target.PaneID
	}
	return m, func() tea.Msg {
		if err := tmuxPlugin.SendTextToBuffer(payload); err != nil {
			return pluginMsg{info: "tmux command pasted", err: err}
		}
		if err := tmuxPlugin.FocusPane(targetPane); err != nil {
			return pluginMsg{
				info:    "tmux command pasted",
				warning: "focus pane failed: " + err.Error(),
			}
		}
		return pluginMsg{info: "tmux command pasted"}
	}
}

func (m *model) activateEditForCurrentIssue() bool {
	issue := m.currentIssue()
	if issue == nil {
		m.setToast("warning", "no issue selected")
		return false
	}
	clone := *issue
	m.Form = newIssueFormEdit(&clone, m.Issues)
	m.Mode = ModeEdit
	return true
}

func reopenParentForCreateCmd(client *BdClient, parentID string, targetStatus Status) tea.Cmd {
	return func() tea.Msg {
		id := strings.TrimSpace(parentID)
		if id == "" {
			return reopenParentForCreateMsg{err: fmt.Errorf("parent id is empty")}
		}
		if client == nil {
			return reopenParentForCreateMsg{parentID: id, err: fmt.Errorf("bd client is not configured")}
		}
		status := targetStatus
		err := client.UpdateIssue(UpdateParams{ID: id, Status: &status})
		return reopenParentForCreateMsg{parentID: id, err: err}
	}
}

func (m model) formatBeadsStartTaskCommand(issueID string) string {
	id := strings.TrimSpace(issueID)
	if id == "" {
		return ""
	}

	base := fmt.Sprintf("skill $beads start implement task %s", id)
	issue := m.ByID[id]
	if issue == nil {
		return base
	}

	parentID := strings.TrimSpace(issue.Parent)
	if parentID == "" {
		return base
	}
	parent := m.ByID[parentID]
	if parent == nil {
		return base
	}
	if strings.EqualFold(strings.TrimSpace(parent.IssueType), "epic") {
		return fmt.Sprintf("%s (epic %s)", base, parent.ID)
	}

	return base
}

func (m *model) markTmuxPickerSelection() error {
	if m.TmuxPicker == nil || len(m.TmuxPicker.Targets) == 0 {
		return nil
	}

	tmuxPlugin := m.Plugins.Tmux()
	if tmuxPlugin == nil || !tmuxPlugin.Enabled() {
		return fmt.Errorf("tmux plugin disabled")
	}

	if m.TmuxPicker.Index < 0 {
		m.TmuxPicker.Index = 0
	}
	if m.TmuxPicker.Index >= len(m.TmuxPicker.Targets) {
		m.TmuxPicker.Index = len(m.TmuxPicker.Targets) - 1
	}

	targetPane := strings.TrimSpace(m.TmuxPicker.Targets[m.TmuxPicker.Index].PaneID)
	if targetPane == "" {
		return fmt.Errorf("tmux pane id is empty")
	}

	prevPane := strings.TrimSpace(m.TmuxMark.PaneID)
	if prevPane != "" && prevPane != targetPane {
		if err := tmuxPlugin.ClearMarkPane(prevPane); err != nil {
			return fmt.Errorf("clear previous mark failed: %w", err)
		}
	}

	if prevPane == targetPane {
		if marked, err := tmuxPlugin.IsPaneMarked(targetPane); err == nil && marked {
			m.TmuxPicker.MarkedPaneID = targetPane
			return nil
		}
	}

	if err := tmuxPlugin.MarkPane(targetPane); err != nil {
		return fmt.Errorf("mark pane failed: %w", err)
	}

	m.TmuxMark.PaneID = targetPane
	m.TmuxPicker.MarkedPaneID = targetPane
	return nil
}

func (m model) currentTmuxPickerPaneID() string {
	if m.TmuxPicker == nil || len(m.TmuxPicker.Targets) == 0 {
		return ""
	}
	idx := m.TmuxPicker.Index
	if idx < 0 {
		idx = 0
	}
	if idx >= len(m.TmuxPicker.Targets) {
		idx = len(m.TmuxPicker.Targets) - 1
	}
	return strings.TrimSpace(m.TmuxPicker.Targets[idx].PaneID)
}

func (m model) blinkTmuxPaneCmd(paneID string) tea.Cmd {
	targetPane := strings.TrimSpace(paneID)
	if targetPane == "" {
		return nil
	}

	tmuxPlugin := m.Plugins.Tmux()
	if tmuxPlugin == nil || !tmuxPlugin.Enabled() {
		return nil
	}

	return func() tea.Msg {
		if err := tmuxPlugin.BlinkPaneWindow(targetPane); err != nil {
			return pluginMsg{warning: "tmux blink failed: " + err.Error()}
		}
		return nil
	}
}

func (m model) submitPrompt(issueID string, action PromptAction, value string) tea.Cmd {
	switch action {
	case PromptAssignee:
		return opCmd("assignee updated", func() error {
			v := value
			return m.Client.UpdateIssue(UpdateParams{ID: issueID, Assignee: &v})
		})
	case PromptLabels:
		labels := parseLabels(value)
		return opCmd("labels updated", func() error {
			return m.Client.UpdateIssue(UpdateParams{ID: issueID, Labels: &labels})
		})
	case PromptDepAdd:
		if value == "" {
			return opCmd("", func() error { return fmt.Errorf("blocker id is required") })
		}
		return opCmd("blocker added", func() error {
			return m.Client.DepAdd(issueID, value)
		})
	case PromptDepRemove:
		if value == "" {
			return opCmd("", func() error { return fmt.Errorf("blocker id is required") })
		}
		return opCmd("blocker removed", func() error {
			return m.Client.DepRemove(issueID, value)
		})
	case PromptParentSet:
		return opCmd("parent updated", func() error {
			v := value
			return m.Client.UpdateIssue(UpdateParams{ID: issueID, Parent: &v})
		})
	default:
		return opCmd("", func() error { return fmt.Errorf("unknown prompt action") })
	}
}
