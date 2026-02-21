package main

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.Type == tea.KeyCtrlC {
		m.setToast("warning", "Ctrl+C отключен, используйте q")
		return m, nil
	}

	switch m.mode {
	case ModeHelp:
		return m.handleHelpKey(msg)
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
	default:
		return m.handleBoardKey(msg)
	}
}

func (m model) handleHelpKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if key == "?" || key == "esc" || key == "q" {
		m.mode = ModeBoard
	}
	return m, nil
}

func (m model) handleSearchKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	switch key {
	case "esc":
		m.mode = ModeBoard
		return m, nil
	case "enter":
		m.searchQuery = strings.TrimSpace(m.searchInput.Value())
		m.mode = ModeBoard
		m.computeColumns()
		m.normalizeSelectionBounds()
		m.setToast("success", "поиск применен")
		return m, nil
	}

	var cmd tea.Cmd
	m.searchInput, cmd = m.searchInput.Update(msg)
	return m, cmd
}

func (m model) handleFilterKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.filterForm == nil {
		m.filterForm = newFilterForm(m.filter)
	}

	key := msg.String()
	switch key {
	case "esc":
		m.mode = ModeBoard
		m.filterForm = nil
		return m, nil
	case "enter":
		m.filter = m.filterForm.toFilter()
		if m.filter.Status == "" {
			m.filter.Status = "any"
		}
		if m.filter.Priority == "" {
			m.filter.Priority = "any"
		}
		m.computeColumns()
		m.normalizeSelectionBounds()
		m.mode = ModeBoard
		m.filterForm = nil
		m.setToast("success", "фильтры применены")
		return m, nil
	case "tab":
		m.filterForm.nextField()
		return m, nil
	case "shift+tab":
		m.filterForm.prevField()
		return m, nil
	case "up":
		if !m.filterForm.isTextField(m.filterForm.currentField()) {
			m.filterForm.cycleEnum(-1)
			return m, nil
		}
	case "down":
		if !m.filterForm.isTextField(m.filterForm.currentField()) {
			m.filterForm.cycleEnum(1)
			return m, nil
		}
	case "c":
		m.filterForm.Assignee = ""
		m.filterForm.Label = ""
		m.filterForm.Status = "any"
		m.filterForm.Priority = "any"
		m.filterForm.loadInput()
		return m, nil
	}

	if m.filterForm.isTextField(m.filterForm.currentField()) {
		var cmd tea.Cmd
		m.filterForm.Input, cmd = m.filterForm.Input.Update(msg)
		m.filterForm.saveInput()
		return m, cmd
	}

	return m, nil
}

func (m model) handleFormKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.form == nil {
		m.mode = ModeBoard
		return m, nil
	}

	key := msg.String()

	switch key {
	case "esc", "enter", "ctrl+s":
		// Esc intentionally saves as a safe default to avoid accidental data loss.
		if err := m.form.Validate(); err != nil {
			m.setToast("error", err.Error())
			return m, nil
		}
		return m.submitForm()
	case "up":
		m.form.prevField()
		return m, nil
	case "down":
		m.form.nextField()
		return m, nil
	case "tab":
		if !m.form.isTextField(m.form.currentField()) {
			m.form.cycleEnum(1)
		} else {
			m.form.nextField()
		}
		return m, nil
	case "shift+tab":
		if !m.form.isTextField(m.form.currentField()) {
			m.form.cycleEnum(-1)
		} else {
			m.form.prevField()
		}
		return m, nil
	case "ctrl+x":
		m.form.saveInputToField()
		cmd, err := m.openFormInEditorCmd()
		if err != nil {
			m.setToast("error", err.Error())
			return m, nil
		}
		return m, cmd
	}

	if m.form.isTextField(m.form.currentField()) {
		var cmd tea.Cmd
		m.form.Input, cmd = m.form.Input.Update(msg)
		m.form.saveInputToField()
		return m, cmd
	}

	return m, nil
}

func (m model) handlePromptKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.prompt == nil {
		m.mode = ModeBoard
		return m, nil
	}

	key := msg.String()
	switch key {
	case "esc":
		m.mode = ModeBoard
		m.prompt = nil
		return m, nil
	case "enter":
		value := strings.TrimSpace(m.prompt.Input.Value())
		issueID := m.prompt.TargetIssue
		action := m.prompt.Action
		m.mode = ModeBoard
		m.prompt = nil
		return m, m.submitPrompt(issueID, action, value)
	}

	var cmd tea.Cmd
	m.prompt.Input, cmd = m.prompt.Input.Update(msg)
	return m, cmd
}

func (m model) handleParentPickerKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.parentPicker == nil {
		m.mode = ModeBoard
		return m, nil
	}

	key := msg.String()
	switch key {
	case "esc", "q":
		m.parentPicker = nil
		m.mode = ModeBoard
		return m, nil
	case "j", "down":
		if len(m.parentPicker.Options) > 0 {
			m.parentPicker.Index = (m.parentPicker.Index + 1) % len(m.parentPicker.Options)
		}
		return m, nil
	case "k", "up":
		if len(m.parentPicker.Options) > 0 {
			m.parentPicker.Index--
			if m.parentPicker.Index < 0 {
				m.parentPicker.Index = len(m.parentPicker.Options) - 1
			}
		}
		return m, nil
	case "enter":
		if len(m.parentPicker.Options) == 0 {
			m.setToast("warning", "нет доступных parent-кандидатов")
			m.parentPicker = nil
			m.mode = ModeBoard
			return m, nil
		}
		targetID := m.parentPicker.TargetIssueID
		selected := m.parentPicker.Options[m.parentPicker.Index]
		parent := strings.TrimSpace(selected.ID)
		m.parentPicker = nil
		m.mode = ModeBoard
		return m, opCmd("parent updated", func() error {
			return m.client.UpdateIssue(UpdateParams{ID: targetID, Parent: &parent})
		})
	}
	return m, nil
}

func (m model) handleDepListKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.depList == nil {
		m.mode = ModeBoard
		return m, nil
	}

	key := msg.String()
	switch key {
	case "esc", "q":
		m.depList = nil
		m.mode = ModeBoard
	case "j", "down":
		if m.depList.Scroll < len(m.depList.Lines)-1 {
			m.depList.Scroll++
		}
	case "k", "up":
		if m.depList.Scroll > 0 {
			m.depList.Scroll--
		}
	}
	return m, nil
}

func (m model) handleTmuxPickerKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.tmuxPicker == nil {
		m.mode = ModeBoard
		return m, nil
	}

	key := msg.String()
	switch key {
	case "esc", "q":
		m.tmuxPicker = nil
		m.mode = ModeBoard
		return m, nil
	case "j", "down":
		if len(m.tmuxPicker.Targets) > 0 {
			m.tmuxPicker.Index = (m.tmuxPicker.Index + 1) % len(m.tmuxPicker.Targets)
		}
		return m, nil
	case "k", "up":
		if len(m.tmuxPicker.Targets) > 0 {
			m.tmuxPicker.Index--
			if m.tmuxPicker.Index < 0 {
				m.tmuxPicker.Index = len(m.tmuxPicker.Targets) - 1
			}
		}
		return m, nil
	case "enter":
		if len(m.tmuxPicker.Targets) == 0 {
			m.tmuxPicker = nil
			m.mode = ModeBoard
			m.setToast("warning", "нет tmux-целей")
			return m, nil
		}

		selected := m.tmuxPicker.Targets[m.tmuxPicker.Index]
		issueID := strings.TrimSpace(m.tmuxPicker.IssueID)
		m.tmuxPicker = nil
		m.mode = ModeBoard

		tmuxPlugin := m.plugins.Tmux()
		if tmuxPlugin == nil || !tmuxPlugin.Enabled() {
			m.setToast("warning", "tmux plugin disabled")
			return m, nil
		}
		tmuxPlugin.SetTarget(selected)

		if issueID == "" {
			m.setToast("success", "tmux target выбран: "+selected.Label())
			return m, nil
		}

		return m, pluginCmd("tmux buffer updated: "+issueID, func() error {
			return tmuxPlugin.SendIssueIDToBuffer(issueID)
		})
	}

	return m, nil
}

func (m model) handleDeleteConfirmKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.confirmDelete == nil {
		m.mode = ModeBoard
		return m, nil
	}

	key := msg.String()
	switch key {
	case "esc", "n":
		m.confirmDelete = nil
		m.mode = ModeBoard
		return m, nil
	case "1":
		m.confirmDelete.Mode = DeleteModeForce
		m.confirmDelete.Selected = 0
		return m, nil
	case "2":
		m.confirmDelete.Mode = DeleteModeCascade
		m.confirmDelete.Selected = 1
		return m, nil
	case "y", "enter":
		issueID := m.confirmDelete.IssueID
		mode := m.confirmDelete.Mode
		m.confirmDelete = nil
		m.mode = ModeBoard
		return m, opCmd("задача удалена", func() error {
			return m.client.DeleteIssue(issueID, mode)
		})
	}

	return m, nil
}

func (m model) handleBoardKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	if key == "q" {
		return m, tea.Quit
	}
	if key == "?" {
		m.mode = ModeHelp
		return m, nil
	}

	if m.leader {
		m.leader = false
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
		m.selectedIdx[st] = 0
		m.ensureSelectionVisible(st)
		return m, nil
	case "G":
		st := m.currentStatus()
		col := m.columns[st]
		if len(col) > 0 {
			m.selectedIdx[st] = len(col) - 1
			m.ensureSelectionVisible(st)
		}
		return m, nil
	case "enter", " ":
		m.showDetails = !m.showDetails
		return m, nil
	case "/":
		m.searchInput.SetValue(m.searchQuery)
		m.searchInput.CursorEnd()
		m.searchInput.Focus()
		m.mode = ModeSearch
		return m, nil
	case "f":
		m.filterForm = newFilterForm(m.filter)
		m.mode = ModeFilter
		return m, nil
	case "c":
		m.searchQuery = ""
		m.searchInput.SetValue("")
		m.filter = Filter{Status: "any", Priority: "any"}
		m.computeColumns()
		m.normalizeSelectionBounds()
		m.setToast("success", "поиск и фильтры очищены")
		return m, nil
	case "r":
		return m, m.loadCmd("manual")
	case "n":
		m.form = newIssueFormCreate(m.issues)
		m.mode = ModeCreate
		return m, nil
	case "e":
		issue := m.currentIssue()
		if issue == nil {
			m.setToast("warning", "нет выбранной задачи")
			return m, nil
		}
		clone := *issue
		m.form = newIssueFormEdit(&clone, m.issues)
		m.mode = ModeEdit
		return m, nil
	case "ctrl+x":
		issue := m.currentIssue()
		if issue == nil {
			m.setToast("warning", "нет выбранной задачи")
			return m, nil
		}
		clone := *issue
		m.form = newIssueFormEdit(&clone, m.issues)
		m.mode = ModeEdit
		cmd, err := m.openFormInEditorCmd()
		if err != nil {
			m.setToast("error", err.Error())
			return m, nil
		}
		return m, cmd
	case "a":
		issue := m.currentIssue()
		if issue == nil {
			m.setToast("warning", "нет выбранной задачи")
			return m, nil
		}
		m.prompt = newPrompt(ModePrompt, "Quick Assignee", "Введите assignee (пусто = unassign)", issue.ID, PromptAssignee, issue.Assignee)
		m.mode = ModePrompt
		return m, nil
	case "y":
		issue := m.currentIssue()
		if issue == nil {
			m.setToast("warning", "нет выбранной задачи")
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
			m.setToast("warning", "нет выбранной задачи")
			return m, nil
		}
		m.prompt = newPrompt(ModePrompt, "Quick Labels", "Введите labels через запятую", issue.ID, PromptLabels, strings.Join(issue.Labels, ", "))
		m.mode = ModePrompt
		return m, nil
	case "p":
		issue := m.currentIssue()
		if issue == nil {
			m.setToast("warning", "нет выбранной задачи")
			return m, nil
		}
		id := issue.ID
		next := cyclePriority(issue.Priority)
		return m, opCmd(fmt.Sprintf("%s: priority -> P%d", id, next), func() error {
			p := next
			return m.client.UpdateIssue(UpdateParams{ID: id, Priority: &p})
		})
	case "s":
		issue := m.currentIssue()
		if issue == nil {
			m.setToast("warning", "нет выбранной задачи")
			return m, nil
		}
		id := issue.ID
		next := cycleStatus(issue.Status)
		return m, opCmd(fmt.Sprintf("%s: status -> %s", id, next), func() error {
			st := next
			return m.client.UpdateIssue(UpdateParams{ID: id, Status: &st})
		})
	case "x":
		issue := m.currentIssue()
		if issue == nil {
			m.setToast("warning", "нет выбранной задачи")
			return m, nil
		}
		id := issue.ID
		if issue.Status == StatusClosed {
			return m, opCmd(fmt.Sprintf("%s reopened", id), func() error { return m.client.ReopenIssue(id) })
		}
		return m, opCmd(fmt.Sprintf("%s closed", id), func() error { return m.client.CloseIssue(id) })
	case "d":
		issue := m.currentIssue()
		if issue == nil {
			m.setToast("warning", "нет выбранной задачи")
			return m, nil
		}
		m.setToast("warning", "получаю preview удаления...")
		return m, deletePreviewCmd(m.client, issue.ID)
	case "g":
		m.leader = true
		m.setToast("info", "leader: g …")
		return m, nil
	}

	return m, nil
}

func (m model) handleLeaderCombo(key string) (tea.Model, tea.Cmd) {
	issue := m.currentIssue()
	if issue == nil {
		m.setToast("warning", "нет выбранной задачи")
		return m, nil
	}

	switch key {
	case "b":
		m.prompt = newPrompt(ModePrompt, "Add Blocker", "Введите ID blocker issue", issue.ID, PromptDepAdd, "")
		m.mode = ModePrompt
		return m, nil
	case "B":
		m.prompt = newPrompt(ModePrompt, "Remove Blocker", "Введите ID blocker issue для удаления", issue.ID, PromptDepRemove, "")
		m.mode = ModePrompt
		return m, nil
	case "p":
		m.parentPicker = newParentPickerState(m.issues, issue.ID, issue.Parent)
		m.mode = ModeParentPicker
		return m, nil
	case "P":
		id := issue.ID
		return m, opCmd(fmt.Sprintf("%s parent cleared", id), func() error {
			empty := ""
			return m.client.UpdateIssue(UpdateParams{ID: id, Parent: &empty})
		})
	case "d":
		m.setToast("info", "загружаю зависимости...")
		return m, depListCmd(m.client, issue.ID)
	default:
		m.setToast("warning", "неизвестный leader combo")
		return m, nil
	}
}

func (m model) submitForm() (tea.Model, tea.Cmd) {
	form := m.form
	if form == nil {
		m.mode = ModeBoard
		return m, nil
	}

	form.saveInputToField()
	m.mode = ModeBoard
	m.form = nil

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

		return m, func() tea.Msg {
			id, err := m.client.CreateIssue(params)
			if err != nil {
				return opMsg{err: err}
			}

			status := form.Status
			if status == "" {
				status = StatusOpen
			}
			if status != StatusOpen {
				if strings.TrimSpace(id) == "" {
					return opMsg{err: fmt.Errorf("created issue id is empty, cannot set status to %s", status)}
				}
				if err := m.client.UpdateIssue(UpdateParams{
					ID:     id,
					Status: &status,
				}); err != nil {
					return opMsg{err: err}
				}
			}

			if id == "" {
				return opMsg{info: "задача создана"}
			}
			return opMsg{info: "создана " + id}
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
		m.setToast("info", "изменений нет")
		return m, nil
	}

	return m, opCmd(fmt.Sprintf("%s updated", form.IssueID), func() error {
		return m.client.UpdateIssue(upd)
	})
}

func (m model) handleTmuxSendIssueID() (tea.Model, tea.Cmd) {
	issue := m.currentIssue()
	if issue == nil {
		m.setToast("warning", "нет выбранной задачи")
		return m, nil
	}

	tmuxPlugin := m.plugins.Tmux()
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
			m.setToast("warning", "нет доступных tmux-целей")
			return m, nil
		}
		m.tmuxPicker = &TmuxPickerState{
			IssueID: issue.ID,
			Targets: targets,
			Index:   0,
		}
		m.mode = ModeTmuxPicker
		return m, nil
	}

	return m, pluginCmd("tmux buffer updated: "+issue.ID, func() error {
		return tmuxPlugin.SendIssueIDToBuffer(issue.ID)
	})
}

func (m model) submitPrompt(issueID string, action PromptAction, value string) tea.Cmd {
	switch action {
	case PromptAssignee:
		return opCmd("assignee updated", func() error {
			v := value
			return m.client.UpdateIssue(UpdateParams{ID: issueID, Assignee: &v})
		})
	case PromptLabels:
		labels := parseLabels(value)
		return opCmd("labels updated", func() error {
			return m.client.UpdateIssue(UpdateParams{ID: issueID, Labels: &labels})
		})
	case PromptDepAdd:
		if value == "" {
			return opCmd("", func() error { return fmt.Errorf("blocker id is required") })
		}
		return opCmd("blocker added", func() error {
			return m.client.DepAdd(issueID, value)
		})
	case PromptDepRemove:
		if value == "" {
			return opCmd("", func() error { return fmt.Errorf("blocker id is required") })
		}
		return opCmd("blocker removed", func() error {
			return m.client.DepRemove(issueID, value)
		})
	case PromptParentSet:
		return opCmd("parent updated", func() error {
			v := value
			return m.client.UpdateIssue(UpdateParams{ID: issueID, Parent: &v})
		})
	default:
		return opCmd("", func() error { return fmt.Errorf("unknown prompt action") })
	}
}
