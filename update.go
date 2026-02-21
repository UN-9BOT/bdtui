package main

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		for _, st := range statusOrder {
			m.ensureSelectionVisible(st)
		}
		return m, nil

	case tickMsg:
		m.now = time.Time(msg)

		var cmds []tea.Cmd
		if !m.cfg.NoWatch {
			cmds = append(cmds, tickCmd())
			cmds = append(cmds, m.loadCmd("tick"))
		}
		if !m.toastUntil.IsZero() && m.now.After(m.toastUntil) {
			m.toast = ""
			m.toastKind = ""
			m.toastUntil = time.Time{}
		}
		return m, tea.Batch(cmds...)

	case loadedMsg:
		if msg.err != nil {
			if msg.source != "tick" {
				m.setToast("error", msg.err.Error())
			}
			m.loading = false
			return m, nil
		}

		if msg.source == "tick" && msg.hash == m.lastHash {
			return m, nil
		}

		m.applyLoadedIssues(msg.issues, msg.hash)
		if msg.source != "tick" {
			m.setToast("success", "данные обновлены")
		}
		return m, nil

	case opMsg:
		if msg.err != nil {
			m.setToast("error", msg.err.Error())
			return m, nil
		}
		if strings.TrimSpace(msg.info) != "" {
			m.setToast("success", msg.info)
		}
		return m, m.loadCmd("mutation")

	case pluginMsg:
		if msg.err != nil {
			m.setToast("error", msg.err.Error())
			return m, nil
		}
		if strings.TrimSpace(msg.info) != "" {
			m.setToast("success", msg.info)
		}
		return m, nil

	case depListMsg:
		if msg.err != nil {
			m.setToast("error", msg.err.Error())
			m.mode = ModeBoard
			return m, nil
		}
		lines := strings.Split(msg.text, "\n")
		m.depList = &DepListState{IssueID: msg.issueID, Lines: lines}
		m.mode = ModeDepList
		return m, nil

	case deletePreviewMsg:
		if msg.err != nil {
			m.setToast("error", msg.err.Error())
			m.mode = ModeBoard
			return m, nil
		}
		m.confirmDelete = &ConfirmDelete{
			IssueID: msg.issueID,
			Mode:    DeleteModeForce,
			Preview: msg.text,
		}
		m.mode = ModeConfirmDelete
		return m, nil

	case formEditorMsg:
		if msg.err != nil {
			m.setToast("error", msg.err.Error())
			return m, nil
		}
		if m.form == nil {
			return m, nil
		}

		m.form.Title = msg.payload.Title
		m.form.Description = msg.payload.Description
		if parsed, ok := statusFromString(msg.payload.Status); ok {
			m.form.Status = parsed
		}
		m.form.Priority = clampPriority(msg.payload.Priority)
		m.form.IssueType = strings.TrimSpace(msg.payload.IssueType)
		m.form.Assignee = strings.TrimSpace(msg.payload.Assignee)
		m.form.Labels = strings.TrimSpace(msg.payload.Labels)
		m.form.Parent = strings.TrimSpace(msg.payload.Parent)
		m.form.loadInputFromField()
		m.setToast("success", "поля обновлены из editor")
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case tea.MouseMsg:
		return m.handleMouse(msg)
	}

	return m, nil
}
