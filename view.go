package main

import (
	"fmt"
	"strings"
	"unicode/utf8"

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
	inspector := m.renderInspector()
	footer := m.renderFooter()

	parts := []string{title, board, inspector, footer}

	base := strings.Join(parts, "\n")
	modal := m.renderModal()
	if modal == "" {
		return m.styles.App.Render(base)
	}

	wrappedBase := m.styles.App.Render(base)
	modalStyle := m.styles.HelpBox.MaxWidth(max(30, m.width-4))
	overlay := lipgloss.Place(
		m.width,
		m.height,
		lipgloss.Center,
		lipgloss.Center,
		modalStyle.Render(modal),
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
	borderColor := columnBorderColor(status, active)
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Width(width)

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

	header := truncate(fmt.Sprintf("%s (%d)", status.Label(), len(col)), maxTextWidth)
	lines := []string{statusHeaderStyle(status).Render(header)}

	itemsPerPage := max(1, innerHeight-2)

	if len(col) == 0 {
		lines = append(lines, m.styles.Dim.Render(truncate("(empty)", maxTextWidth)))
	} else {
		end := min(len(col), offset+itemsPerPage)
		for i := offset; i < end; i++ {
			item := col[i]
			row := renderIssueRow(item, maxTextWidth)
			if i == idx && active {
				selectedPlain := truncate(fmt.Sprintf("P%d %s %s", item.Priority, item.ID, item.Title), maxTextWidth)
				row = m.styles.Selected.Render(selectedPlain)
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

func (m model) renderInspector() string {
	issue := m.currentIssue()
	if issue == nil {
		return m.styles.Border.Width(max(20, m.width-4)).Render("No selected issue")
	}

	w := max(20, m.width-4)
	inner := max(4, w-4)
	innerHeight := m.inspectorInnerHeight()

	lines := []string{
		statusHeaderStyle(issue.Display).Render(
			truncate(
				fmt.Sprintf(
					"Selected: %s | %s | %s | %s",
					issue.ID, issue.IssueType, renderPriorityLabel(issue.Priority), issue.Status,
				),
				inner,
			),
		),
		truncate("Title: "+issue.Title, inner),
		truncate(
			"Assignee: "+defaultString(issue.Assignee, "-")+" | Labels: "+defaultString(strings.Join(issue.Labels, ", "), "-"),
			inner,
		),
	}

	if m.showDetails {
		lines = append(lines,
			truncate(
				"Parent: "+defaultString(issue.Parent, "-")+" | blockedBy: "+defaultString(strings.Join(issue.BlockedBy, ","), "-")+" | blocks: "+defaultString(strings.Join(issue.Blocks, ","), "-"),
				inner,
			),
		)
		lines = append(lines, truncate("Description: "+defaultString(issue.Description, "-"), inner))
	}

	for len(lines) < innerHeight {
		lines = append(lines, "")
	}
	if len(lines) > innerHeight {
		lines = lines[:innerHeight]
	}

	return m.styles.Border.Width(w).Render(strings.Join(lines, "\n"))
}

func (m model) renderFooter() string {
	left := "j/k move | h/l col | Enter/Space expand info | y copy id | n new | e edit | d delete | g + key deps | ? help | q quit"
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
			return m.form.parentDisplay()
		}
		return ""
	}

	lines := []string{title, ""}
	maxLineWidth := max(24, min(120, m.width-14))
	for _, f := range m.form.fields() {
		prefix := fmt.Sprintf("%s %s: ", mark(f), f)
		rawValue := defaultString(valueFor(f), "-")
		segments := wrapPlainText(rawValue, max(8, maxLineWidth-lipgloss.Width(prefix)))
		if len(segments) == 0 {
			segments = []string{"-"}
		}
		lines = append(lines, prefix+segments[0])
		indent := strings.Repeat(" ", lipgloss.Width(prefix))
		for _, seg := range segments[1:] {
			lines = append(lines, indent+seg)
		}
	}

	if m.form.isTextField(field) {
		lines = append(lines, "")
		prefix := "edit: > "
		raw := m.form.Input.Value()
		cursorPos := m.form.Input.Position()
		display := injectCursorMarker(raw, cursorPos)
		if strings.TrimSpace(raw) == "" {
			display = "|"
		}
		segments := wrapPlainText(display, max(8, maxLineWidth-lipgloss.Width(prefix)))
		lines = append(lines, prefix+segments[0])
		indent := strings.Repeat(" ", lipgloss.Width(prefix))
		for _, seg := range segments[1:] {
			lines = append(lines, indent+seg)
		}
		lines = append(lines, fmt.Sprintf("cursor: %d/%d", cursorPos, utf8.RuneCountInString(raw)))
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
		case "parent":
			enumValues = "values: " + strings.Join(m.form.parentHints(7), " | ")
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

func wrapPlainText(s string, width int) []string {
	if width <= 0 {
		return []string{s}
	}

	var out []string
	for _, rawLine := range strings.Split(s, "\n") {
		rawLine = strings.TrimSpace(rawLine)
		if rawLine == "" {
			out = append(out, "")
			continue
		}

		words := strings.Fields(rawLine)
		if len(words) == 0 {
			out = append(out, "")
			continue
		}

		cur := ""
		for _, w := range words {
			if lipgloss.Width(w) > width {
				if cur != "" {
					out = append(out, cur)
					cur = ""
				}
				out = append(out, splitLongToken(w, width)...)
				continue
			}

			if cur == "" {
				cur = w
				continue
			}

			candidate := cur + " " + w
			if lipgloss.Width(candidate) <= width {
				cur = candidate
			} else {
				out = append(out, cur)
				cur = w
			}
		}

		if cur != "" {
			out = append(out, cur)
		}
	}

	if len(out) == 0 {
		return []string{""}
	}
	return out
}

func splitLongToken(s string, width int) []string {
	if width <= 0 {
		return []string{s}
	}

	runes := []rune(s)
	var out []string
	cur := make([]rune, 0, width)

	for _, r := range runes {
		test := append(cur, r)
		if lipgloss.Width(string(test)) > width {
			if len(cur) > 0 {
				out = append(out, string(cur))
				cur = cur[:0]
			}
		}
		cur = append(cur, r)
	}

	if len(cur) > 0 {
		out = append(out, string(cur))
	}
	if len(out) == 0 {
		return []string{s}
	}
	return out
}

func injectCursorMarker(value string, pos int) string {
	runes := []rune(value)
	if pos < 0 {
		pos = 0
	}
	if pos > len(runes) {
		pos = len(runes)
	}

	out := make([]rune, 0, len(runes)+1)
	out = append(out, runes[:pos]...)
	out = append(out, '|')
	out = append(out, runes[pos:]...)
	return string(out)
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

func columnBorderColor(status Status, active bool) lipgloss.Color {
	if !active {
		switch status {
		case StatusOpen:
			return lipgloss.Color("31")
		case StatusInProgress:
			return lipgloss.Color("136")
		case StatusBlocked:
			return lipgloss.Color("88")
		case StatusClosed:
			return lipgloss.Color("238")
		default:
			return lipgloss.Color("241")
		}
	}

	switch status {
	case StatusOpen:
		return lipgloss.Color("45")
	case StatusInProgress:
		return lipgloss.Color("220")
	case StatusBlocked:
		return lipgloss.Color("203")
	case StatusClosed:
		return lipgloss.Color("246")
	default:
		return lipgloss.Color("39")
	}
}

func statusHeaderStyle(status Status) lipgloss.Style {
	switch status {
	case StatusOpen:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("45")).Bold(true)
	case StatusInProgress:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Bold(true)
	case StatusBlocked:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Bold(true)
	case StatusClosed:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("246")).Bold(true)
	default:
		return lipgloss.NewStyle().Bold(true)
	}
}

func priorityStyle(priority int) lipgloss.Style {
	switch priority {
	case 0:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Bold(true)
	case 1:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("208")).Bold(true)
	case 2:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Bold(true)
	case 3:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("81")).Bold(true)
	default:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Bold(true)
	}
}

func issueTypeStyle(issueType string) lipgloss.Style {
	switch strings.ToLower(strings.TrimSpace(issueType)) {
	case "epic":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("207")).Bold(true)
	case "feature":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true)
	case "task":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("114")).Bold(true)
	case "bug":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Bold(true)
	case "chore":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Bold(true)
	case "decision":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("213")).Bold(true)
	default:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("250"))
	}
}

func shortType(issueType string) string {
	switch strings.ToLower(strings.TrimSpace(issueType)) {
	case "epic":
		return "EP"
	case "feature":
		return "FE"
	case "task":
		return "TS"
	case "bug":
		return "BG"
	case "chore":
		return "CH"
	case "decision":
		return "DC"
	default:
		return "??"
	}
}

func renderPriorityLabel(priority int) string {
	return fmt.Sprintf("P%d", priority)
}

func renderIssueRow(item Issue, maxTextWidth int) string {
	priority := renderPriorityLabel(item.Priority)
	issueType := shortType(item.IssueType)
	id := truncate(item.ID, 14)

	fixedWidth := lipgloss.Width(priority) + 1 + lipgloss.Width(issueType) + 1 + lipgloss.Width(id) + 1
	titleWidth := max(1, maxTextWidth-fixedWidth)
	title := truncate(item.Title, titleWidth)

	return priorityStyle(item.Priority).Render(priority) +
		" " + issueTypeStyle(item.IssueType).Render(issueType) +
		" " + lipgloss.NewStyle().Foreground(lipgloss.Color("246")).Render(id) +
		" " + title
}
