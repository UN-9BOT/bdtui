package main

import (
	"fmt"
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
		m.clampDetailsScroll()
		if m.helpScroll < 0 {
			m.helpScroll = 0
		}
		maxHelpOffset := m.helpMaxScroll()
		if m.helpScroll > maxHelpOffset {
			m.helpScroll = maxHelpOffset
		}
		return m, nil

	case tickMsg:
		m.now = time.Time(msg)

		var cmds []tea.Cmd
		if !m.cfg.NoWatch {
			cmds = append(cmds, tickCmd())
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

		if (msg.source == "tick" || msg.source == "watch") && msg.hash == m.lastHash {
			return m, nil
		}

		m.applyLoadedIssues(msg.issues, msg.hash)
		if msg.source == "manual" || msg.source == "mutation" {
			m.setToast("success", "data refreshed")
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
		if strings.TrimSpace(msg.warning) != "" {
			m.setToast("warning", msg.warning)
		}
		return m, nil

	case sortModePersistMsg:
		if msg.err != nil {
			m.setToast("warning", "sort mode changed but not saved: "+msg.err.Error())
			return m, nil
		}
		m.setToast("success", fmt.Sprintf("sort mode: %s", msg.mode.Label()))
		return m, nil

	case tmuxMarkCleanupMsg:
		if msg.token != m.tmuxMark.token {
			return m, nil
		}
		if strings.TrimSpace(msg.paneID) == "" || msg.paneID != m.tmuxMark.paneID {
			return m, nil
		}
		tmuxPlugin := m.plugins.Tmux()
		if tmuxPlugin != nil && tmuxPlugin.Enabled() {
			if err := tmuxPlugin.ClearMarkPane(msg.paneID); err != nil {
				m.setToast("warning", "tmux mark cleanup failed: "+err.Error())
				return m, nil
			}
		}
		m.tmuxMark.paneID = ""
		return m, nil

	case tea.FocusMsg:
		m.uiFocused = true
		return m, nil

	case tea.BlurMsg:
		m.uiFocused = false
		return m, nil

	case beadsChangedMsg:
		if m.cfg.NoWatch {
			return m, nil
		}
		return m, tea.Batch(
			m.loadCmd("watch"),
			watchBeadsChangesCmd(m.beadsDir),
		)

	case beadsWatchErrMsg:
		if msg.err != nil {
			m.setToast("warning", "beads watch: "+msg.err.Error())
		}
		if m.cfg.NoWatch {
			return m, nil
		}
		return m, beadsWatchRetryCmd(2 * time.Second)

	case beadsWatchRetryMsg:
		if m.cfg.NoWatch {
			return m, nil
		}
		return m, watchBeadsChangesCmd(m.beadsDir)

	case tea.KeyMsg:
		m.uiFocused = true
		return m.handleKey(msg)

	case tea.MouseMsg:
		m.uiFocused = true
		return m.handleMouse(msg)

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

	case reopenParentForCreateMsg:
		if msg.err != nil {
			m.setToast("error", msg.err.Error())
			m.mode = ModeBoard
			return m, nil
		}

		m.setIssueStatusLocal(msg.parentID, StatusInProgress)
		m.form = newIssueFormCreateWithParent(m.issues, msg.parentID)
		m.mode = ModeCreate
		m.setToast("success", "parent moved to in_progress")
		return m, m.loadCmd("mutation")

	case formEditorMsg:
		if msg.err != nil {
			if m.resumeDetailsAfterEditor {
				m.resumeDetailsAfterEditor = false
				m.mode = ModeDetails
				m.form = nil
			}
			m.setToast("error", msg.err.Error())
			return m, nil
		}
		if m.form == nil {
			m.resumeDetailsAfterEditor = false
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
		if m.resumeDetailsAfterEditor {
			issueID := strings.TrimSpace(m.form.IssueID)
			m.resumeDetailsAfterEditor = false
			next, cmd := m.submitForm()
			updated := next.(model)
			updated.mode = ModeDetails
			updated.showDetails = true
			if issueID != "" {
				updated.detailsIssueID = issueID
			}
			return updated, cmd
		}
		m.setToast("success", "fields updated from editor")
		return m, nil

	}

	return m, nil
}
