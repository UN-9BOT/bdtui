package main

import (
	"fmt"
	"strconv"
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

type boardRow struct {
	issue Issue
	depth int
	ghost bool
}

func (m model) issueBaseDepth(status Status, issueID string) int {
	if m.columnDepths == nil {
		return 0
	}
	statusDepths, ok := m.columnDepths[status]
	if !ok || statusDepths == nil {
		return 0
	}
	return statusDepths[issueID]
}

func (m model) crossStatusParentChain(item Issue, status Status) []Issue {
	parentID := strings.TrimSpace(item.Parent)
	if parentID == "" {
		return nil
	}

	visited := map[string]bool{
		strings.TrimSpace(item.ID): true,
	}
	nearestToRoot := make([]Issue, 0, 4)

	for strings.TrimSpace(parentID) != "" {
		pid := strings.TrimSpace(parentID)
		if visited[pid] {
			break
		}
		visited[pid] = true

		parent := m.byID[pid]
		if parent == nil {
			break
		}
		if parent.Display != status {
			nearestToRoot = append(nearestToRoot, *parent)
		}
		parentID = strings.TrimSpace(parent.Parent)
	}

	if len(nearestToRoot) == 0 {
		return nil
	}

	out := make([]Issue, 0, len(nearestToRoot))
	for i := len(nearestToRoot) - 1; i >= 0; i-- {
		out = append(out, nearestToRoot[i])
	}
	return out
}

func (m model) buildColumnRows(status Status) ([]boardRow, map[string]int) {
	col := m.columns[status]
	if len(col) == 0 {
		return nil, map[string]int{}
	}

	rows := make([]boardRow, 0, len(col))
	issueRowIndex := make(map[string]int, len(col))
	prevChainIDs := make([]string, 0, 4)
	prevBaseDepth := -1

	commonPrefixLen := func(a, b []string) int {
		n := min(len(a), len(b))
		i := 0
		for i < n && a[i] == b[i] {
			i++
		}
		return i
	}

	for _, item := range col {
		baseDepth := m.issueBaseDepth(status, item.ID)
		ghostChain := m.crossStatusParentChain(item, status)
		chainIDs := make([]string, 0, len(ghostChain))
		for _, ghostIssue := range ghostChain {
			chainIDs = append(chainIDs, ghostIssue.ID)
		}

		start := 0
		if baseDepth == prevBaseDepth {
			start = commonPrefixLen(prevChainIDs, chainIDs)
		}

		for i := start; i < len(ghostChain); i++ {
			ghostIssue := ghostChain[i]
			rows = append(rows, boardRow{
				issue: ghostIssue,
				depth: baseDepth + i,
				ghost: true,
			})
		}

		rows = append(rows, boardRow{
			issue: item,
			depth: baseDepth + len(ghostChain),
			ghost: false,
		})
		issueRowIndex[item.ID] = len(rows) - 1
		prevChainIDs = chainIDs
		prevBaseDepth = baseDepth
	}

	return rows, issueRowIndex
}

func (m model) renderColumn(status Status, width int, innerHeight int, active bool) string {
	borderColor := columnBorderColor(status, active)
	border := columnBorderStyle(active)
	grayBoard := m.mode == ModeDetails
	if grayBoard {
		borderColor = lipgloss.Color("241")
	}
	style := lipgloss.NewStyle().
		Border(border).
		BorderForeground(borderColor).
		Width(width)

	col := m.columns[status]
	rows, issueRowIndex := m.buildColumnRows(status)
	idx := m.selectedIdx[status]
	if idx < 0 {
		idx = 0
	}
	selectedRowIdx := -1
	if len(col) > 0 && idx >= 0 && idx < len(col) {
		if rowIdx, ok := issueRowIndex[col[idx].ID]; ok {
			selectedRowIdx = rowIdx
		}
	}

	offset := m.scrollOffset[status]
	if offset < 0 {
		offset = 0
	}

	maxTextWidth := max(1, width-4)

	header := truncate(fmt.Sprintf("%s (%d)", status.Label(), len(col)), maxTextWidth)
	headerLine := statusHeaderStyle(status).Render(header)
	if grayBoard {
		headerLine = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245")).
			Bold(true).
			Render(header)
	}
	lines := []string{headerLine}

	itemsPerPage := max(1, innerHeight-2)

	if len(rows) == 0 {
		emptyLine := truncate("(empty)", maxTextWidth)
		if grayBoard {
			lines = append(lines, lipgloss.NewStyle().Foreground(lipgloss.Color("242")).Render(emptyLine))
		} else {
			lines = append(lines, m.styles.Dim.Render(emptyLine))
		}
	} else {
		if offset >= len(rows) {
			offset = len(rows) - 1
		}
		if offset < 0 {
			offset = 0
		}
		end := min(len(rows), offset+itemsPerPage)
		for i := offset; i < end; i++ {
			rowItem := rows[i]
			row := renderIssueRow(rowItem.issue, maxTextWidth, rowItem.depth)
			if rowItem.ghost {
				row = lipgloss.NewStyle().
					Foreground(lipgloss.Color("242")).
					Faint(true).
					Render(renderIssueRowGhostPlain(rowItem.issue, maxTextWidth, rowItem.depth))
			}
			if grayBoard && !rowItem.ghost {
				row = lipgloss.NewStyle().
					Foreground(lipgloss.Color("243")).
					Render(renderIssueRowGhostPlain(rowItem.issue, maxTextWidth, rowItem.depth))
			}
			if i == selectedRowIdx && active && !rowItem.ghost && !grayBoard {
				row = m.styles.Selected.Render(renderIssueRowSelectedPlain(rowItem.issue, maxTextWidth, rowItem.depth))
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
	containerStyle := m.styles.Border
	if m.mode == ModeDetails {
		containerStyle = m.styles.Active
	}

	issue := m.currentIssue()
	if issue == nil {
		return containerStyle.Width(max(20, m.width-4)).Render("No selected issue")
	}

	w := max(20, m.width-4)
	inner := m.inspectorInnerWidth()
	innerHeight := m.inspectorInnerHeight()

	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Bold(true)
	sepStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	idStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("81")).Bold(true)
	titleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("229"))
	assigneeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("117")).Bold(true)
	labelsStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("114")).Bold(true)

	selectedPrefix := "Selected: "
	typeText := defaultString(issue.IssueType, "-")
	prioText := renderPriorityLabel(issue.Priority)
	statusText := defaultString(string(issue.Status), "-")
	idFixed := lipgloss.Width(selectedPrefix) + lipgloss.Width(" | ")*3 + lipgloss.Width(typeText) + lipgloss.Width(prioText) + lipgloss.Width(statusText)
	idWidth := max(1, inner-idFixed)
	idText := truncate(defaultString(issue.ID, "-"), idWidth)

	titlePrefix := "Title: "
	titleText := truncate(defaultString(issue.Title, "-"), max(1, inner-lipgloss.Width(titlePrefix)))

	assigneePrefix := "Assignee: "
	labelsPrefix := " | Labels: "
	assigneeRaw := defaultString(issue.Assignee, "-")
	labelsRaw := defaultString(strings.Join(issue.Labels, ", "), "-")
	valueBudget := max(2, inner-lipgloss.Width(assigneePrefix)-lipgloss.Width(labelsPrefix))
	assigneeWidth := max(1, min(valueBudget/3, valueBudget-1))
	labelsWidth := max(1, valueBudget-assigneeWidth)
	assigneeText := truncate(assigneeRaw, assigneeWidth)
	labelsText := truncate(labelsRaw, labelsWidth)

	sep := sepStyle.Render(" | ")
	lines := []string{
		labelStyle.Render(selectedPrefix) +
			idStyle.Render(idText) +
			sep +
			issueTypeStyle(typeText).Render(typeText) +
			sep +
			priorityStyle(issue.Priority).Render(prioText) +
			sep +
			statusHeaderStyle(issue.Display).Render(statusText),
		labelStyle.Render(titlePrefix) + titleStyle.Render(titleText),
		labelStyle.Render(assigneePrefix) +
			assigneeStyle.Render(assigneeText) +
			labelStyle.Render(labelsPrefix) +
			labelsStyle.Render(labelsText),
	}

	if m.showDetails {
		details := detailLines(issue, inner)
		height := m.detailsViewportHeight()
		if height > 0 && len(details) > 0 {
			maxOffset := len(details) - height
			if maxOffset < 0 {
				maxOffset = 0
			}
			start := m.detailsScroll
			if start < 0 {
				start = 0
			}
			if start > maxOffset {
				start = maxOffset
			}
			end := start + height
			if end > len(details) {
				end = len(details)
			}
			lines = append(lines, details[start:end]...)
		}
	}

	for len(lines) < innerHeight {
		lines = append(lines, "")
	}
	if len(lines) > innerHeight {
		lines = lines[:innerHeight]
	}

	return containerStyle.Width(w).Render(strings.Join(lines, "\n"))
}

func detailLines(issue *Issue, inner int) []string {
	if issue == nil || inner <= 0 {
		return nil
	}

	keyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))

	metaPlain := truncate(
		"Parent: "+defaultString(issue.Parent, "-")+" | blockedBy: "+defaultString(strings.Join(issue.BlockedBy, ","), "-")+" | blocks: "+defaultString(strings.Join(issue.Blocks, ","), "-"),
		inner,
	)
	meta := styleDetailMetaLine(metaPlain, keyStyle)

	lines := []string{meta}
	prefix := "Description: "
	available := inner - len(prefix)
	if available < 1 {
		available = 1
	}

	descRaw := defaultString(issue.Description, "-")
	descLines := wrapPlainText(descRaw, available)
	if len(descLines) == 0 {
		descLines = []string{"-"}
	}
	lines = append(lines, keyStyle.Render(prefix)+descLines[0])
	indent := strings.Repeat(" ", len(prefix))
	for _, line := range descLines[1:] {
		lines = append(lines, indent+line)
	}
	return lines
}

func styleDetailMetaLine(line string, keyStyle lipgloss.Style) string {
	if strings.TrimSpace(line) == "" {
		return line
	}
	replacer := strings.NewReplacer(
		"Parent:", keyStyle.Render("Parent:"),
		"blockedBy:", keyStyle.Render("blockedBy:"),
		"blocks:", keyStyle.Render("blocks:"),
	)
	return replacer.Replace(line)
}

func (m model) renderFooter() string {
	left := "j/k move | h/l col | Enter/Space focus details | y copy id | Y paste to tmux | n new | e edit | Ctrl+X ext edit | d delete | g + key deps | ? help | q quit"
	if m.mode != ModeBoard {
		if m.mode == ModeDetails {
			left = "mode: details | j/k scroll | Esc close"
		} else if m.mode == ModeCreate {
			left = "mode: create | Enter save | Esc close if title is empty"
		} else if m.mode == ModeEdit {
			left = "mode: edit | Enter/Esc save"
		} else {
			left = "mode: " + string(m.mode) + " | Esc cancel"
		}
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
	case ModeParentPicker:
		return m.renderParentPickerModal()
	case ModeTmuxPicker:
		return m.renderTmuxPickerModal()
	case ModeDepList:
		return m.renderDepListModal()
	case ModeConfirmDelete:
		return m.renderDeleteModal()
	default:
		return ""
	}
}

func (m model) renderHelpModal() string {
	content := m.helpContentLines()
	viewport := m.helpViewportContentLines()
	filterLinesCount := m.helpFilterLinesCount(viewport)
	contentViewport := max(1, viewport-filterLinesCount)

	maxOffset := m.helpMaxScroll()
	offset := m.helpScroll
	if offset < 0 {
		offset = 0
	}
	if offset > maxOffset {
		offset = maxOffset
	}

	end := offset + contentViewport
	if end > len(content) {
		end = len(content)
	}
	if end < offset {
		end = offset
	}

	innerWidth := m.helpModalInnerWidth()
	emptyLine := strings.Repeat(" ", innerWidth)
	lines := make([]string, 0, viewport+2)
	for _, line := range m.helpFilterBoxLines(innerWidth, filterLinesCount) {
		padded := padRightToWidth(line, innerWidth)
		lines = append(lines, m.styleHelpFilterBoxLine(line, padded))
	}
	for _, line := range content[offset:end] {
		padded := padRightToWidth(line, innerWidth)
		lines = append(lines, m.styleHelpContentLine(line, padded))
	}
	for len(lines) < filterLinesCount+contentViewport {
		lines = append(lines, emptyLine)
	}

	if maxOffset > 0 {
		lines = append(lines, emptyLine, m.styleHelpFooterLine(padRightToWidth(m.helpFooterLine(offset, maxOffset), innerWidth)))
	} else {
		lines = append(lines, emptyLine, m.styleHelpFooterLine(padRightToWidth(m.helpControlsHintLine()+" | ?/Esc close", innerWidth)))
	}
	return strings.Join(lines, "\n")
}

func (m model) helpContentLines() []string {
	lines := []string{"Hotkeys"}
	query := strings.TrimSpace(strings.ToLower(m.helpQuery))
	global := m.filterHelpEntries(m.keymap.Global, query)
	leader := m.filterHelpEntries(m.keymap.Leader, query)
	form := m.filterHelpEntries(m.keymap.Form, query)

	lines = append(lines, "")
	if len(global) > 0 {
		lines = append(lines, "Global:")
		lines = append(lines, global...)
		lines = append(lines, "")
	}
	if len(leader) > 0 {
		lines = append(lines, "Leader (g):")
		lines = append(lines, leader...)
		lines = append(lines, "")
	}
	if len(form) > 0 {
		lines = append(lines, "Forms:")
		lines = append(lines, form...)
	}
	if len(global) == 0 && len(leader) == 0 && len(form) == 0 {
		lines = append(lines, "No matches")
	}
	return lines
}

func (m model) filterHelpEntries(entries []string, query string) []string {
	if query == "" {
		return entries
	}

	filtered := make([]string, 0, len(entries))
	for _, entry := range entries {
		text := entry
		if idx := strings.Index(text, ":"); idx >= 0 && idx < len(text)-1 {
			text = text[idx+1:]
		}
		if strings.Contains(strings.ToLower(strings.TrimSpace(text)), query) {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}

func (m model) helpViewportContentLines() int {
	// HelpBox has top/bottom border + padding = 4 lines. Keep 2 lines for footer/hints.
	available := m.height - 6
	content := available - 2
	if content < 1 {
		return 1
	}
	return content
}

func (m model) helpMaxScroll() int {
	contentLen := len(m.helpContentLines())
	viewport := max(1, m.helpViewportContentLines()-m.helpFilterLinesCount(m.helpViewportContentLines()))
	maxOffset := contentLen - viewport
	if maxOffset < 0 {
		return 0
	}
	return maxOffset
}

func (m model) helpFilterLinesCount(viewport int) int {
	switch {
	case viewport >= 4:
		return 3
	case viewport == 3:
		return 2
	case viewport == 2:
		return 1
	default:
		return 0
	}
}

func (m model) helpFilterInputWithCursor() string {
	return m.helpQuery + "▏"
}

func (m model) helpControlsHintLine() string {
	return "Backspace delete | Ctrl+U clear"
}

func (m model) helpFooterLine(offset int, maxOffset int) string {
	total := maxOffset + 1
	digits := len(strconv.Itoa(total))
	return fmt.Sprintf("↑/↓ scroll (%*d/%d) | %s | ?/Esc close", digits, offset+1, total, m.helpControlsHintLine())
}

func (m model) helpFilterBoxLines(innerWidth int, linesCount int) []string {
	if innerWidth < 1 || linesCount <= 0 {
		return nil
	}

	input := m.helpFilterInputWithCursor()
	inputInner := max(1, innerWidth-4)
	input = truncate(input, inputInner)

	switch linesCount {
	case 1:
		line := "[Filter: " + input + "]"
		return []string{truncate(line, innerWidth)}
	case 2:
		topInner := max(1, innerWidth-2)
		topTitle := " Filter "
		top := "┌" + truncate(topTitle+strings.Repeat("─", topInner), topInner) + "┐"
		bottomInner := max(1, innerWidth-4)
		bottom := "└ " + padRightToWidth(truncate(input, bottomInner), bottomInner) + " ┘"
		return []string{top, bottom}
	default:
		topInner := max(1, innerWidth-2)
		topTitle := " Filter "
		top := "┌" + truncate(topTitle+strings.Repeat("─", topInner), topInner) + "┐"
		middle := "│ " + padRightToWidth(input, inputInner) + " │"
		bottom := "└" + strings.Repeat("─", topInner) + "┘"
		return []string{top, middle, bottom}
	}
}

func (m model) styleHelpFilterInput(input string) string {
	inputStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("230"))
	cursorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("81")).Bold(true)
	before, after, found := strings.Cut(input, "▏")
	if !found {
		return inputStyle.Render(input)
	}
	return inputStyle.Render(before) + cursorStyle.Render("▏") + inputStyle.Render(after)
}

func (m model) styleHelpFilterBoxLine(raw string, padded string) string {
	borderStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("63")).Bold(true)
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("117")).Bold(true)

	if strings.HasPrefix(raw, "┌") || strings.HasPrefix(raw, "└") {
		if strings.Contains(raw, " Filter ") {
			before, after, _ := strings.Cut(raw, " Filter ")
			styled := borderStyle.Render(before) + labelStyle.Render(" Filter ") + borderStyle.Render(after)
			return padRightToWidth(styled, lipgloss.Width(padded))
		}
		return borderStyle.Render(padded)
	}

	if strings.HasPrefix(raw, "│ ") && strings.HasSuffix(raw, " │") {
		inner := strings.TrimSuffix(strings.TrimPrefix(raw, "│ "), " │")
		styled := borderStyle.Render("│ ") + m.styleHelpFilterInput(inner) + borderStyle.Render(" │")
		return padRightToWidth(styled, lipgloss.Width(padded))
	}

	if strings.HasPrefix(raw, "└ ") && strings.HasSuffix(raw, " ┘") {
		inner := strings.TrimSuffix(strings.TrimPrefix(raw, "└ "), " ┘")
		styled := borderStyle.Render("└ ") + m.styleHelpFilterInput(inner) + borderStyle.Render(" ┘")
		return padRightToWidth(styled, lipgloss.Width(padded))
	}

	if strings.HasPrefix(raw, "[Filter: ") && strings.HasSuffix(raw, "]") {
		inner := strings.TrimSuffix(strings.TrimPrefix(raw, "[Filter: "), "]")
		styled := borderStyle.Render("[") + labelStyle.Render("Filter: ") + m.styleHelpFilterInput(inner) + borderStyle.Render("]")
		return padRightToWidth(styled, lipgloss.Width(padded))
	}

	return borderStyle.Render(padded)
}

func (m model) styleHelpContentLine(raw string, padded string) string {
	switch raw {
	case "":
		return padded
	case "Hotkeys":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("117")).Bold(true).Render(padded)
	case "Global:", "Leader (g):", "Forms:":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("111")).Bold(true).Render(padded)
	case "No matches":
		return m.styles.Dim.Render(padded)
	}

	if idx := strings.Index(raw, ":"); idx > 0 && idx < len(raw)-1 {
		keyPart := strings.TrimSpace(raw[:idx+1])
		descPart := strings.TrimSpace(raw[idx+1:])
		keyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("81")).Bold(true)
		descStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("250"))
		styled := keyStyle.Render(keyPart) + " " + descStyle.Render(descPart)
		return padRightToWidth(styled, lipgloss.Width(padded))
	}

	return lipgloss.NewStyle().Foreground(lipgloss.Color("250")).Render(padded)
}

func (m model) styleHelpFooterLine(padded string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("109")).Render(padded)
}

func (m model) helpModalInnerWidth() int {
	width := 1
	for _, line := range m.helpContentLines() {
		if w := lipgloss.Width(line); w > width {
			width = w
		}
	}

	if w := lipgloss.Width("Filter: "+m.helpFilterInputWithCursor()) + 4; w > width {
		width = w
	}

	maxOffset := m.helpMaxScroll()
	if maxOffset > 0 {
		if w := lipgloss.Width(m.helpFooterLine(maxOffset, maxOffset)); w > width {
			width = w
		}
	} else {
		if w := lipgloss.Width(m.helpControlsHintLine() + " | ?/Esc close"); w > width {
			width = w
		}
	}
	return width
}

func padRightToWidth(s string, width int) string {
	if width <= 0 {
		return s
	}
	w := lipgloss.Width(s)
	if w >= width {
		return s
	}
	return s + strings.Repeat(" ", width-w)
}

func (m model) renderSearchModal() string {
	return strings.Join([]string{
		"Search",
		"",
		"Searches by id/title/description/assignee/labels (interactive)",
		m.searchInput.View(),
		"",
		"Type to filter | Enter: done | Esc: cancel",
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
		inlineTextValue := func(activeField string, saved string) string {
			if field != activeField {
				return saved
			}
			raw := m.form.Input.Value()
			display := injectCursorMarker(raw, m.form.Input.Position())
			if strings.TrimSpace(raw) == "" {
				return "|"
			}
			return display
		}

		switch name {
		case "title":
			return inlineTextValue("title", m.form.Title)
		case "status":
			return string(m.form.Status)
		case "priority":
			return fmt.Sprintf("%d", m.form.Priority)
		case "type":
			return m.form.IssueType
		case "assignee":
			return inlineTextValue("assignee", m.form.Assignee)
		case "labels":
			return inlineTextValue("labels", m.form.Labels)
		case "parent":
			return m.form.parentDisplay()
		}
		return ""
	}

	showParentSide := field == "parent" && len(m.form.ParentOpts) > 0
	modalContentWidth := max(40, min(170, m.width-14))
	leftPaneWidth := modalContentWidth
	rightPaneWidth := 0
	keyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	if showParentSide {
		rightPaneWidth = max(36, min(56, (modalContentWidth*40)/100))
		leftPaneWidth = max(36, modalContentWidth-rightPaneWidth-2)
	}

	lines := []string{title, ""}
	maxLineWidth := leftPaneWidth
	for _, f := range m.form.fields() {
		prefixPlain := fmt.Sprintf("%s %s: ", mark(f), f)
		prefix := fmt.Sprintf("%s %s ", mark(f), keyStyle.Render(f+":"))
		rawValue := defaultString(valueFor(f), "-")
		segments := wrapPlainText(rawValue, max(8, maxLineWidth-lipgloss.Width(prefixPlain)))
		if len(segments) == 0 {
			segments = []string{"-"}
		}
		lines = append(lines, prefix+styleFormFieldSegment(f, segments[0]))
		indent := strings.Repeat(" ", lipgloss.Width(prefixPlain))
		for _, seg := range segments[1:] {
			lines = append(lines, indent+styleFormFieldSegment(f, seg))
		}
	}

	lines = append(lines, "")
	prefixPlain := "description: "
	prefix := keyStyle.Render("description:") + " "
	previewWidth := max(8, maxLineWidth-lipgloss.Width(prefixPlain))
	previewLines, wasClipped := firstNDescriptionLines(m.form.Description, 5, previewWidth)
	lines = append(lines, prefix+previewLines[0])
	indent := strings.Repeat(" ", lipgloss.Width(prefixPlain))
	for _, line := range previewLines[1:] {
		lines = append(lines, indent+line)
	}
	if wasClipped {
		lines = append(lines, indent+m.styles.Dim.Render("..."))
	}

	if !m.form.isTextField(field) {
		enumValues := "values: -"
		cycleHint := "use Tab/Shift+Tab to cycle enum"
		switch field {
		case "status":
			enumValues = "values: " + renderEnumValuesStyled(
				[]string{"open", "in_progress", "blocked", "closed"},
				string(m.form.Status),
				m.styles.Selected,
				enumStyleStatus,
			)
		case "type":
			enumValues = "values: " + renderEnumValuesStyled(
				[]string{"task", "epic", "bug", "feature", "chore", "decision"},
				m.form.IssueType,
				m.styles.Selected,
				enumStyleIssueType,
			)
		case "priority":
			enumValues = "values: " + renderEnumValuesStyled(
				[]string{"0", "1", "2", "3", "4"},
				fmt.Sprintf("%d", m.form.Priority),
				m.styles.Selected,
				enumStylePriority,
			)
		case "parent":
			enumValues = "values: " + strings.Join(m.form.parentHints(7), " | ")
		}
		lines = append(lines, "", cycleHint, enumValues)
	}

	helpLine := "↑/↓ move fields | Tab/Shift+Tab cycle enum fields | Ctrl+X edit description in $EDITOR | Enter save | Esc close if empty title"
	if !m.form.Create {
		helpLine = "↑/↓ move fields | Tab/Shift+Tab cycle enum fields | Ctrl+X edit description in $EDITOR | Enter/Esc save"
	}
	lines = append(lines, "", helpLine)
	left := lipgloss.NewStyle().Width(leftPaneWidth).Render(strings.Join(lines, "\n"))
	if !showParentSide {
		return left
	}

	right := lipgloss.NewStyle().Width(rightPaneWidth).Render(m.renderParentOptionsSidebar(rightPaneWidth))
	return lipgloss.JoinHorizontal(lipgloss.Top, left, "  ", right)
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

func (m model) renderParentPickerModal() string {
	if m.parentPicker == nil {
		return "Parent Picker\n\nloading..."
	}

	total := len(m.parentPicker.Options)
	if total == 0 {
		return "Parent Picker\n\nNo parent candidates available\n\nEsc close"
	}

	lines := []string{
		statusHeaderStyle(StatusInProgress).Render("Parent Picker (g p)"),
		"",
		m.styles.Warning.Render("Choose parent: ↑/↓ (or j/k), Enter apply, Esc cancel"),
		"",
	}

	center := m.parentPicker.Index
	if center < 0 || center >= total {
		center = 0
	}
	start := max(0, center-4)
	end := min(total, start+9)
	if end-start < 9 {
		start = max(0, end-9)
	}

	for i := start; i < end; i++ {
		opt := m.parentPicker.Options[i]
		prefix := "  "
		if i == center {
			prefix = m.styles.Warning.Render("▶ ")
		}

		line := m.styles.Dim.Render("(none)")
		linePlain := "(none)"
		if opt.ID != "" {
			statusText := string(opt.Display)
			prioText := renderPriorityLabel(opt.Priority)
			typeText := shortType(opt.IssueType)
			idText := opt.ID
			titleText := truncate(opt.Title, 40)

			linePlain = fmt.Sprintf("%s %s %s %s %s", statusText, prioText, typeText, idText, titleText)
			line = fmt.Sprintf(
				"%s %s %s %s %s",
				statusHeaderStyle(opt.Display).Render(statusText),
				priorityStyle(opt.Priority).Render(prioText),
				issueTypeStyle(opt.IssueType).Render(typeText),
				lipgloss.NewStyle().Foreground(lipgloss.Color("246")).Render(idText),
				titleText,
			)
		}

		if i == center {
			line = m.styles.Selected.Render(linePlain)
		}
		lines = append(lines, prefix+line)
	}

	lines = append(lines, "", m.styles.Dim.Render(fmt.Sprintf("option %d/%d", center+1, total)))
	return strings.Join(lines, "\n")
}

func (m model) renderTmuxPickerModal() string {
	if m.tmuxPicker == nil {
		return "Tmux Picker\n\nloading..."
	}

	total := len(m.tmuxPicker.Targets)
	if total == 0 {
		return "Tmux Picker\n\nNo tmux targets available\n\nEsc close"
	}

	lines := []string{
		statusHeaderStyle(StatusInProgress).Render("Tmux Target Picker (Y)"),
		"",
		m.styles.Warning.Render("Choose target: ↑/↓ (or j/k), Enter select, Esc cancel"),
		m.styles.Dim.Render("current pane is marked in tmux, mark clears 5s after picker exit"),
		"",
	}

	center := m.tmuxPicker.Index
	if center < 0 || center >= total {
		center = 0
	}
	start := max(0, center-5)
	end := min(total, start+11)
	if end-start < 11 {
		start = max(0, end-11)
	}

	for i := start; i < end; i++ {
		target := m.tmuxPicker.Targets[i]
		prefix := "  "
		if i == center {
			prefix = m.styles.Warning.Render("▶ ")
		}

		codexMark := "  "
		if isLikelyCodexTarget(target) {
			codexMark = m.styles.Success.Render("C ")
		}
		clientMark := "  "
		if target.HasClient {
			clientMark = m.styles.Success.Render("A ")
		}
		markMark := "  "
		if strings.TrimSpace(target.PaneID) != "" && target.PaneID == m.tmuxPicker.MarkedPaneID {
			markMark = m.styles.Warning.Render("M ")
		}
		markFlag := "-"
		if strings.TrimSpace(target.PaneID) != "" && target.PaneID == m.tmuxPicker.MarkedPaneID {
			markFlag = "M"
		}
		codexFlag := "-"
		if isLikelyCodexTarget(target) {
			codexFlag = "C"
		}
		clientFlag := "-"
		if target.HasClient {
			clientFlag = "A"
		}

		linePlain := fmt.Sprintf(
			"[%s%s%s] %s %s %s %s %s",
			markFlag,
			codexFlag,
			clientFlag,
			defaultString(target.SessionName, "?"),
			defaultString(target.PaneID, "?"),
			defaultString(target.Command, "-"),
			defaultString(target.Title, "-"),
			map[bool]string{true: "client", false: "no-client"}[target.HasClient],
		)

		line := fmt.Sprintf(
			"%s%s%s %s %s %s",
			markMark,
			codexMark,
			clientMark,
			lipgloss.NewStyle().Foreground(lipgloss.Color("246")).Render(defaultString(target.SessionName, "?")),
			lipgloss.NewStyle().Foreground(lipgloss.Color("111")).Render(defaultString(target.PaneID, "?")),
			truncate(defaultString(target.Command, "-")+" | "+defaultString(target.Title, "-"), 70),
		)
		if i == center {
			line = m.styles.Selected.Render(linePlain)
		}
		lines = append(lines, prefix+line)
	}

	legend := "M=marked, C=codex-like, A=attached-client"
	lines = append(lines, "", m.styles.Dim.Render(legend), m.styles.Dim.Render(fmt.Sprintf("option %d/%d", center+1, total)))
	return strings.Join(lines, "\n")
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

func firstNDescriptionLines(description string, maxSourceLines int, width int) ([]string, bool) {
	if maxSourceLines <= 0 {
		maxSourceLines = 5
	}
	text := strings.ReplaceAll(description, "\r\n", "\n")
	srcLines := strings.Split(text, "\n")
	if len(srcLines) == 0 {
		return []string{"-"}, false
	}
	if len(srcLines) == 1 && strings.TrimSpace(srcLines[0]) == "" {
		return []string{"-"}, false
	}

	clipped := len(srcLines) > maxSourceLines
	if clipped {
		srcLines = srcLines[:maxSourceLines]
	}

	out := make([]string, 0, len(srcLines))
	for _, line := range srcLines {
		segments := wrapPlainText(line, width)
		if len(segments) == 0 {
			out = append(out, "")
			continue
		}
		out = append(out, segments...)
	}
	if len(out) == 0 {
		return []string{"-"}, clipped
	}
	return out, clipped
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
	return renderEnumValuesStyled(values, current, style, nil)
}

func renderEnumValuesStyled(values []string, current string, selected lipgloss.Style, enumStyle func(string) lipgloss.Style) string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		if v == current {
			out = append(out, selected.Render(v))
			continue
		}
		if enumStyle != nil {
			out = append(out, enumStyle(v).Render(v))
			continue
		}
		out = append(out, v)
	}
	return strings.Join(out, " | ")
}

func styleFormFieldSegment(field string, segment string) string {
	switch field {
	case "status":
		return enumStyleStatus(segment).Render(segment)
	case "priority":
		return enumStylePriority(segment).Render(segment)
	case "type":
		return enumStyleIssueType(segment).Render(segment)
	default:
		return segment
	}
}

func enumStyleStatus(raw string) lipgloss.Style {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case string(StatusOpen):
		return statusHeaderStyle(StatusOpen)
	case string(StatusInProgress):
		return statusHeaderStyle(StatusInProgress)
	case string(StatusBlocked):
		return statusHeaderStyle(StatusBlocked)
	case string(StatusClosed):
		return statusHeaderStyle(StatusClosed)
	default:
		return lipgloss.NewStyle()
	}
}

func enumStylePriority(raw string) lipgloss.Style {
	p, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return lipgloss.NewStyle()
	}
	return priorityStyle(p)
}

func enumStyleIssueType(raw string) lipgloss.Style {
	return issueTypeStyle(raw)
}

func (m model) renderParentOptionsSidebar(width int) string {
	if m.form == nil || len(m.form.ParentOpts) == 0 {
		return "Parent candidates\n\n(no options)"
	}

	lines := []string{
		"Parent candidates",
		"use Tab/Shift+Tab to cycle",
		"",
	}

	total := len(m.form.ParentOpts)
	center := m.form.ParentIndex
	if center < 0 || center >= total {
		center = 0
	}

	visible := min(9, total)
	start := max(0, center-(visible/2))
	end := min(total, start+visible)
	if end-start < visible {
		start = max(0, end-visible)
	}

	maxText := max(12, width-3)
	for i := start; i < end; i++ {
		opt := m.form.ParentOpts[i]
		prefix := "  "
		if i == center {
			prefix = "▶ "
		}

		linePlain := ""
		var line string
		if opt.ID == "" {
			linePlain = "(none)"
			line = linePlain
		} else {
			statusText := string(opt.Display)
			prioText := renderPriorityLabel(opt.Priority)
			typeText := shortType(opt.IssueType)
			idText := opt.ID
			fixed := lipgloss.Width(statusText) + 1 + lipgloss.Width(prioText) + 1 + lipgloss.Width(typeText) + 1 + lipgloss.Width(idText) + 1
			titleWidth := max(8, maxText-fixed)
			titleText := truncate(opt.Title, titleWidth)

			linePlain = fmt.Sprintf("%s %s %s %s %s", statusText, prioText, typeText, idText, titleText)

			statusPart := statusHeaderStyle(opt.Display).Render(statusText)
			typePart := issueTypeStyle(opt.IssueType).Render(typeText)
			prioPart := priorityStyle(opt.Priority).Render(prioText)
			idPart := lipgloss.NewStyle().Foreground(lipgloss.Color("246")).Render(idText)
			line = fmt.Sprintf("%s %s %s %s %s", statusPart, prioPart, typePart, idPart, titleText)
		}

		if i == center {
			line = m.styles.Selected.Render(truncate(linePlain, maxText))
		}
		lines = append(lines, prefix+line)
	}

	lines = append(lines, "", fmt.Sprintf("%d/%d", center+1, total))
	return strings.Join(lines, "\n")
}

func columnBorderStyle(active bool) lipgloss.Border {
	if active {
		return lipgloss.ThickBorder()
	}
	return lipgloss.RoundedBorder()
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

func shortTypeDashboard(issueType string) string {
	switch strings.ToLower(strings.TrimSpace(issueType)) {
	case "epic":
		return "E"
	case "feature":
		return "F"
	case "task":
		return "T"
	case "bug":
		return "B"
	case "chore":
		return "C"
	case "decision":
		return "D"
	default:
		return "?"
	}
}

func renderPriorityLabel(priority int) string {
	return fmt.Sprintf("P%d", priority)
}

func renderIssueRow(item Issue, maxTextWidth int, depth int) string {
	priority := renderPriorityLabel(item.Priority)
	issueType := shortTypeDashboard(item.IssueType)
	prefix := treePrefix(depth)
	title, id, gap := layoutDashboardRowWithRightID(maxTextWidth, prefix, priority, issueType, item.Title, item.ID)

	return prefix +
		priorityStyle(item.Priority).Render(priority) +
		" " + issueTypeStyle(item.IssueType).Render(issueType) +
		" " + title +
		gap + lipgloss.NewStyle().Foreground(lipgloss.Color("246")).Render(id)
}

func renderIssueRowSelectedPlain(item Issue, maxTextWidth int, depth int) string {
	priority := renderPriorityLabel(item.Priority)
	issueType := shortTypeDashboard(item.IssueType)
	prefix := treePrefix(depth)

	fixedWidth := lipgloss.Width(prefix) + lipgloss.Width(priority) + 1 + lipgloss.Width(issueType) + 1
	titleWidth := max(1, maxTextWidth-fixedWidth)
	title := truncate(item.Title, titleWidth)

	return prefix + priority + " " + issueType + " " + title
}

func renderIssueRowGhostPlain(item Issue, maxTextWidth int, depth int) string {
	priority := renderPriorityLabel(item.Priority)
	issueType := shortTypeDashboard(item.IssueType)
	prefix := treePrefix(depth)
	title, id, gap := layoutDashboardRowWithRightID(maxTextWidth, prefix, priority, issueType, item.Title, item.ID)

	return prefix + priority + " " + issueType + " " + title + gap + id
}

func layoutDashboardRowWithRightID(maxTextWidth int, prefix string, priority string, issueType string, titleRaw string, idRaw string) (title string, id string, gap string) {
	if maxTextWidth < 1 {
		maxTextWidth = 1
	}

	fixedPrefixWidth := lipgloss.Width(prefix) + lipgloss.Width(priority) + 1 + lipgloss.Width(issueType) + 1
	maxIDWidth := maxTextWidth - fixedPrefixWidth - 2
	if maxIDWidth < 1 {
		maxIDWidth = 1
	}
	id = truncate(idRaw, min(14, maxIDWidth))

	titleWidth := maxTextWidth - fixedPrefixWidth - 1 - lipgloss.Width(id)
	if titleWidth < 0 {
		titleWidth = 0
	}
	title = truncate(titleRaw, titleWidth)

	leftPlain := prefix + priority + " " + issueType + " " + title
	gapWidth := maxTextWidth - lipgloss.Width(leftPlain) - lipgloss.Width(id)
	if gapWidth < 1 {
		gapWidth = 1
	}
	gap = strings.Repeat(" ", gapWidth)
	return title, id, gap
}

func treePrefix(depth int) string {
	if depth <= 0 {
		return ""
	}
	if depth == 1 {
		return "↳ "
	}
	return strings.Repeat("  ", depth-1) + "↳ "
}
