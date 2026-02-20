package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m model) View() string {
	if m.width <= 0 || m.height <= 0 {
		return "loading terminal size..."
	}

	if m.loading {
		return m.styles.App.Render("Loading beads data...")
	}

	title := m.renderTitle()
	board := m.renderBoard()
	details := ""
	if m.showDetails {
		details = m.renderDetails()
	}
	footer := m.renderFooter()

	parts := []string{title, board}
	if details != "" {
		parts = append(parts, details)
	}
	parts = append(parts, footer)

	base := strings.Join(parts, "\n")
	modal := m.renderModal()
	if modal == "" {
		return m.styles.App.Render(base)
	}

	wrappedBase := m.styles.App.Render(base)
	overlay := lipgloss.Place(
		m.width,
		m.height,
		lipgloss.Center,
		lipgloss.Center,
		m.styles.HelpBox.Render(modal),
	)
	return wrappedBase + "\n" + overlay
}

func (m model) renderTitle() string {
	filterHint := ""
	if !m.filter.IsEmpty() {
		filterHint = fmt.Sprintf(" | filter: a=%s l=%s s=%s p=%s", defaultString(m.filter.Assignee, "-"), defaultString(m.filter.Label, "-"), defaultString(m.filter.Status, "any"), defaultString(m.filter.Priority, "any"))
	}
	searchHint := ""
	if strings.TrimSpace(m.searchQuery) != "" {
		searchHint = " | search: " + m.searchQuery
	}
	leaderHint := ""
	if m.leader {
		leaderHint = " | leader: g ..."
	}

	line := truncate(buildTitle(m)+searchHint+filterHint+leaderHint, max(10, m.width-4))
	return m.styles.Title.Render(line)
}

func (m model) renderBoard() string {
	availableWidth := max(20, m.width-4)
	gap := 1
	totalGap := gap * (len(statusOrder) - 1)
	panelWidth := (availableWidth - totalGap) / len(statusOrder)
	if panelWidth < 20 {
		panelWidth = 20
	}

	innerHeight := m.boardInnerHeight()

	cols := make([]string, 0, len(statusOrder))
	for idx, status := range statusOrder {
		cols = append(cols, m.renderColumn(status, panelWidth, innerHeight, idx == m.selectedCol))
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, cols...)
}

func (m model) renderColumn(status Status, width int, innerHeight int, active bool) string {
	style := m.styles.Border.Width(width)
	if active {
		style = m.styles.Active.Width(width)
	}

	col := m.columns[status]
	idx := m.selectedIdx[status]
	if idx < 0 {
		idx = 0
	}

	offset := m.scrollOffset[status]
	if offset < 0 {
		offset = 0
	}

	maxTextWidth := max(1, width-4)

	lines := []string{
		truncate(fmt.Sprintf("%s (%d)", status.Label(), len(col)), maxTextWidth),
	}

	itemsPerPage := max(1, innerHeight-2)

	if len(col) == 0 {
		lines = append(lines, m.styles.Dim.Render(truncate("(empty)", maxTextWidth)))
	} else {
		end := min(len(col), offset+itemsPerPage)
		for i := offset; i < end; i++ {
			item := col[i]
			row := fmt.Sprintf("P%d %s %s", item.Priority, item.ID, item.Title)
			row = truncate(row, maxTextWidth)
			if i == idx && active {
				row = m.styles.Selected.Render(row)
			}
			lines = append(lines, row)
		}
	}

	for len(lines) < innerHeight {
		lines = append(lines, "")
	}

	if len(lines) > innerHeight {
		lines = lines[:innerHeight]
	}

	return style.Render(strings.Join(lines, "\n"))
}

func (m model) renderDetails() string {
	issue := m.currentIssue()
	if issue == nil {
		return m.styles.Border.Width(max(20, m.width-4)).Render("No selected issue")
	}

	w := max(20, m.width-4)
	inner := max(4, w-4)

	lines := []string{
		truncate(fmt.Sprintf("%s | %s | P%d | %s", issue.ID, issue.IssueType, issue.Priority, issue.Status), inner),
		truncate("Title: "+issue.Title, inner),
		truncate("Assignee: "+defaultString(issue.Assignee, "-"), inner),
		truncate("Labels: "+defaultString(strings.Join(issue.Labels, ", "), "-"), inner),
		truncate("Parent: "+defaultString(issue.Parent, "-")+" | blockedBy: "+defaultString(strings.Join(issue.BlockedBy, ","), "-")+" | blocks: "+defaultString(strings.Join(issue.Blocks, ","), "-"), inner),
		truncate("Description: "+defaultString(issue.Description, "-"), inner),
	}

	return m.styles.Border.Width(w).Render(strings.Join(lines, "\n"))
}

func (m model) renderFooter() string {
	left := "j/k move | h/l col | n new | e edit | d delete | g + key deps | ? help | q quit"
	if m.mode != ModeBoard {
		left = "mode: " + string(m.mode) + " | Esc cancel"
	}

	right := ""
	if m.toast != "" {
		switch m.toastKind {
		case "error":
			right = m.styles.Error.Render(m.toast)
		case "warning":
			right = m.styles.Warning.Render(m.toast)
		case "success":
			right = m.styles.Success.Render(m.toast)
		default:
			right = m.toast
		}
	}

	line := truncate(left+"  "+right, max(10, m.width-4))
	return m.styles.Footer.Render(line)
}

func (m model) renderModal() string {
	switch m.mode {
	case ModeHelp:
		return m.renderHelpModal()
	case ModeSearch:
		return m.renderSearchModal()
	case ModeFilter:
		return m.renderFilterModal()
	case ModeCreate, ModeEdit:
		return m.renderFormModal()
	case ModePrompt:
		return m.renderPromptModal()
	case ModeDepList:
		return m.renderDepListModal()
	case ModeConfirmDelete:
		return m.renderDeleteModal()
	default:
		return ""
	}
}

func (m model) renderHelpModal() string {
	lines := []string{"Hotkeys"}
	lines = append(lines, "")
	lines = append(lines, "Global:")
	lines = append(lines, m.keymap.Global...)
	lines = append(lines, "")
	lines = append(lines, "Leader (g):")
	lines = append(lines, m.keymap.Leader...)
	lines = append(lines, "")
	lines = append(lines, "Forms:")
	lines = append(lines, m.keymap.Form...)
	lines = append(lines, "", "Press ? or Esc to close")
	return strings.Join(lines, "\n")
}

func (m model) renderSearchModal() string {
	return strings.Join([]string{
		"Search",
		"",
		"Ищет по id/title/description/assignee/labels",
		m.searchInput.View(),
		"",
		"Enter: apply | Esc: cancel",
	}, "\n")
}

func (m model) renderFilterModal() string {
	if m.filterForm == nil {
		return "Filter\n\nloading..."
	}

	field := m.filterForm.currentField()
	mark := func(name string) string {
		if name == field {
			return "▶"
		}
		return " "
	}

	assignee := m.filterForm.Assignee
	label := m.filterForm.Label
	if field == "assignee" || field == "label" {
		if field == "assignee" {
			assignee = m.filterForm.Input.Value()
		} else {
			label = m.filterForm.Input.Value()
		}
	}

	lines := []string{
		"Filters",
		"",
		fmt.Sprintf("%s assignee: %s", mark("assignee"), defaultString(assignee, "any")),
		fmt.Sprintf("%s label:    %s", mark("label"), defaultString(label, "any")),
		fmt.Sprintf("%s status:   %s", mark("status"), defaultString(m.filterForm.Status, "any")),
		fmt.Sprintf("%s priority: %s", mark("priority"), defaultString(m.filterForm.Priority, "any")),
		"",
	}

	if field == "assignee" || field == "label" {
		lines = append(lines, "edit: "+m.filterForm.Input.View())
	} else {
		lines = append(lines, "use ↑/↓ to cycle enum")
	}

	lines = append(lines, "", "Tab/Shift+Tab | Enter apply | c clear | Esc cancel")
	return strings.Join(lines, "\n")
}

func (m model) renderFormModal() string {
	if m.form == nil {
		return "Form\n\nloading..."
	}

	title := "Create Issue"
	if !m.form.Create {
		title = "Edit Issue: " + m.form.IssueID
	}

	field := m.form.currentField()
	mark := func(name string) string {
		if field == name {
			return "▶"
		}
		return " "
	}

	valueFor := func(name string) string {
		switch name {
		case "title":
			if field == "title" {
				return m.form.Input.Value()
			}
			return m.form.Title
		case "description":
			if field == "description" {
				return m.form.Input.Value()
			}
			return m.form.Description
		case "status":
			return string(m.form.Status)
		case "priority":
			return fmt.Sprintf("%d", m.form.Priority)
		case "type":
			return m.form.IssueType
		case "assignee":
			if field == "assignee" {
				return m.form.Input.Value()
			}
			return m.form.Assignee
		case "labels":
			if field == "labels" {
				return m.form.Input.Value()
			}
			return m.form.Labels
		case "parent":
			if field == "parent" {
				return m.form.Input.Value()
			}
			return m.form.Parent
		}
		return ""
	}

	lines := []string{title, ""}
	for _, f := range m.form.fields() {
		lines = append(lines, fmt.Sprintf("%s %s: %s", mark(f), f, defaultString(valueFor(f), "-")))
	}

	if m.form.isTextField(field) {
		lines = append(lines, "", "edit: "+m.form.Input.View())
	} else {
		enumValues := "values: -"
		switch field {
		case "status":
			enumValues = "values: " + renderEnumValues(
				[]string{"open", "in_progress", "blocked", "closed"},
				string(m.form.Status),
				m.styles.Selected,
			)
		case "type":
			enumValues = "values: " + renderEnumValues(
				[]string{"task", "epic", "bug", "feature", "chore", "decision"},
				m.form.IssueType,
				m.styles.Selected,
			)
		case "priority":
			enumValues = "values: " + renderEnumValues(
				[]string{"0", "1", "2", "3", "4"},
				fmt.Sprintf("%d", m.form.Priority),
				m.styles.Selected,
			)
		}
		lines = append(lines, "", "use ↑/↓ to cycle enum", enumValues)
	}

	lines = append(lines, "", "Tab/Shift+Tab | Ctrl+X open in $EDITOR | Enter save | Esc cancel")
	return strings.Join(lines, "\n")
}

func (m model) renderPromptModal() string {
	if m.prompt == nil {
		return "Prompt\n\nloading..."
	}

	return strings.Join([]string{
		m.prompt.Title,
		"",
		m.prompt.Description,
		m.prompt.Input.View(),
		"",
		"Enter submit | Esc cancel",
	}, "\n")
}

func (m model) renderDepListModal() string {
	if m.depList == nil {
		return "Dependencies\n\nloading..."
	}

	maxLines := 18
	if m.height > 24 {
		maxLines = m.height - 8
	}

	start := min(max(0, m.depList.Scroll), max(0, len(m.depList.Lines)-1))
	end := min(len(m.depList.Lines), start+maxLines)
	if end < start {
		end = start
	}

	lines := []string{fmt.Sprintf("Dependencies: %s", m.depList.IssueID), ""}
	lines = append(lines, m.depList.Lines[start:end]...)
	lines = append(lines, "", "j/k scroll | Esc close")
	return strings.Join(lines, "\n")
}

func (m model) renderDeleteModal() string {
	if m.confirmDelete == nil {
		return "Delete\n\nloading..."
	}

	modeLine := "1 force"
	if m.confirmDelete.Mode == DeleteModeCascade {
		modeLine = "2 cascade"
	}

	previewLines := strings.Split(m.confirmDelete.Preview, "\n")
	if len(previewLines) > 10 {
		previewLines = previewLines[:10]
		previewLines = append(previewLines, "...")
	}

	lines := []string{
		"Delete Issue",
		"",
		"issue: " + m.confirmDelete.IssueID,
		"mode: " + modeLine,
		"",
		"Preview:",
	}
	lines = append(lines, previewLines...)
	lines = append(lines,
		"",
		"1 force | 2 cascade",
		"y/Enter confirm | n/Esc cancel",
	)
	return strings.Join(lines, "\n")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func renderEnumValues(values []string, current string, style lipgloss.Style) string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		if v == current {
			out = append(out, style.Render(v))
			continue
		}
		out = append(out, v)
	}
	return strings.Join(out, " | ")
}
