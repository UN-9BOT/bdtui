package app

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.Width = msg.Width
		m.Height = msg.Height
		for _, st := range statusOrder {
			m.ensureSelectionVisible(st)
		}
		m.clampDetailsScroll()
		if m.HelpScroll < 0 {
			m.HelpScroll = 0
		}
		maxHelpOffset := m.helpMaxScroll()
		if m.HelpScroll > maxHelpOffset {
			m.HelpScroll = maxHelpOffset
		}
		return m, nil

	case tickMsg:
		m.Now = time.Time(msg)

		var cmds []tea.Cmd
		if !m.Cfg.NoWatch {
			cmds = append(cmds, tickCmd())
		}
		if !m.ToastUntil.IsZero() && m.Now.After(m.ToastUntil) {
			m.Toast = ""
			m.ToastKind = ""
			m.ToastUntil = time.Time{}
		}
		return m, tea.Batch(cmds...)

	case loadedMsg:
		if msg.err != nil {
			if msg.source != "tick" {
				m.setToast("error", msg.err.Error())
			}
			m.Loading = false
			return m, nil
		}

		if (msg.source == "tick" || msg.source == "watch") && msg.hash == m.LastHash {
			return m, nil
		}

		m.applyLoadedIssues(msg.Issues, msg.hash)
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
		m.setToast("success", fmt.Sprintf("sort mode: %s", msg.Mode.Label()))
		return m, nil

	case FormEditorMsg:
		return m.Update(msg.toInternal())

	case ReopenParentForCreateMsg:
		return m.Update(reopenParentForCreateMsg{
			parentID: msg.ParentID,
			err:      msg.Err,
		})

	case tmuxMarkCleanupMsg:
		if msg.Token != m.TmuxMark.Token {
			return m, nil
		}
		if strings.TrimSpace(msg.PaneID) == "" || msg.PaneID != m.TmuxMark.PaneID {
			return m, nil
		}
		tmuxPlugin := m.Plugins.Tmux()
		if tmuxPlugin != nil && tmuxPlugin.Enabled() {
			if err := tmuxPlugin.ClearMarkPane(msg.PaneID); err != nil {
				m.setToast("warning", "tmux mark cleanup failed: "+err.Error())
				return m, nil
			}
		}
		m.TmuxMark.PaneID = ""
		return m, nil

	case tea.FocusMsg:
		m.UIFocused = true
		return m, nil

	case tea.BlurMsg:
		m.UIFocused = false
		return m, nil

	case beadsChangedMsg:
		if m.Cfg.NoWatch {
			return m, nil
		}
		return m, tea.Batch(
			m.loadCmd("watch"),
			watchBeadsChangesCmd(m.BeadsDir),
		)

	case beadsWatchErrMsg:
		if msg.err != nil {
			m.setToast("warning", "beads watch: "+msg.err.Error())
		}
		if m.Cfg.NoWatch {
			return m, nil
		}
		return m, beadsWatchRetryCmd(2 * time.Second)

	case beadsWatchRetryMsg:
		if m.Cfg.NoWatch {
			return m, nil
		}
		return m, watchBeadsChangesCmd(m.BeadsDir)

	case tea.KeyMsg:
		m.UIFocused = true
		return m.handleKey(msg)

	case tea.MouseMsg:
		m.UIFocused = true
		return m.handleMouse(msg)

	case depListMsg:
		if msg.err != nil {
			m.setToast("error", msg.err.Error())
			m.Mode = ModeBoard
			return m, nil
		}
		lines := strings.Split(msg.text, "\n")
		m.DepList = &DepListState{IssueID: msg.issueID, Lines: lines}
		m.Mode = ModeDepList
		return m, nil

	case deletePreviewMsg:
		if msg.err != nil {
			m.setToast("error", msg.err.Error())
			m.Mode = ModeBoard
			return m, nil
		}
		m.ConfirmDelete = &ConfirmDelete{
			IssueID: msg.issueID,
			Mode:    DeleteModeForce,
			Preview: msg.text,
		}
		m.Mode = ModeConfirmDelete
		return m, nil

	case reopenParentForCreateMsg:
		if msg.err != nil {
			m.setToast("error", msg.err.Error())
			m.Mode = ModeBoard
			return m, nil
		}

		m.setIssueStatusLocal(msg.parentID, StatusInProgress)
		m.Form = newIssueFormCreateWithParent(m.Issues, msg.parentID)
		m.Mode = ModeCreate
		m.setToast("success", "parent moved to in_progress")
		return m, m.loadCmd("mutation")

	case formEditorMsg:
		if msg.err != nil {
			if m.ResumeDetailsAfterEditor {
				m.ResumeDetailsAfterEditor = false
				m.Mode = ModeDetails
				m.Form = nil
			}
			m.setToast("error", msg.err.Error())
			return m, nil
		}
		if m.Form == nil {
			m.ResumeDetailsAfterEditor = false
			return m, nil
		}

		m.Form.Title = msg.payload.Title
		m.Form.Description = msg.payload.Description
		if parsed, ok := statusFromString(msg.payload.Status); ok {
			m.Form.Status = parsed
		}
		m.Form.Priority = clampPriority(msg.payload.Priority)
		m.Form.IssueType = strings.TrimSpace(msg.payload.IssueType)
		m.Form.Assignee = strings.TrimSpace(msg.payload.Assignee)
		m.Form.Labels = strings.TrimSpace(msg.payload.Labels)
		m.Form.Parent = strings.TrimSpace(msg.payload.Parent)
		m.Form.loadInputFromField()
		if m.ResumeDetailsAfterEditor {
			issueID := strings.TrimSpace(m.Form.IssueID)
			m.ResumeDetailsAfterEditor = false
			next, cmd := m.submitForm()
			updated := next.(model)
			updated.Mode = ModeDetails
			updated.ShowDetails = true
			if issueID != "" {
				updated.DetailsIssueID = issueID
			}
			return updated, cmd
		}
		m.setToast("success", "fields updated from editor")
		return m, nil

	}

	return m, nil
}
