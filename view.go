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

func (m model) renderColumn(status Status, width int, innerHeight int, active bool) string {
	borderColor := columnBorderColor(status, active)
	border := columnBorderStyle(active)
	style := lipgloss.NewStyle().
		Border(border).
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
			depth := 0
			if m.columnDepths != nil {
				if statusDepths, ok := m.columnDepths[status]; ok {
					depth = statusDepths[item.ID]
				}
			}
			row := renderIssueRow(item, maxTextWidth, depth)
			if i == idx && active {
				row = m.styles.Selected.Render(renderIssueRowSelectedPlain(item, maxTextWidth, depth))
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

	return m.styles.Border.Width(w).Render(strings.Join(lines, "\n"))
}

func detailLines(issue *Issue, inner int) []string {
	if issue == nil || inner <= 0 {
		return nil
	}

	meta := truncate(
		"Parent: "+defaultString(issue.Parent, "-")+" | blockedBy: "+defaultString(strings.Join(issue.BlockedBy, ","), "-")+" | blocks: "+defaultString(strings.Join(issue.Blocks, ","), "-"),
		inner,
	)

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
	lines = append(lines, prefix+descLines[0])
	indent := strings.Repeat(" ", len(prefix))
	for _, line := range descLines[1:] {
		lines = append(lines, indent+line)
	}
	return lines
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
		"Searches by id/title/description/assignee/labels",
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
	if showParentSide {
		rightPaneWidth = max(36, min(56, (modalContentWidth*40)/100))
		leftPaneWidth = max(36, modalContentWidth-rightPaneWidth-2)
	}

	lines := []string{title, ""}
	maxLineWidth := leftPaneWidth
	for _, f := range m.form.fields() {
		prefix := fmt.Sprintf("%s %s: ", mark(f), f)
		rawValue := defaultString(valueFor(f), "-")
		segments := wrapPlainText(rawValue, max(8, maxLineWidth-lipgloss.Width(prefix)))
		if len(segments) == 0 {
			segments = []string{"-"}
		}
		lines = append(lines, prefix+styleFormFieldSegment(f, segments[0]))
		indent := strings.Repeat(" ", lipgloss.Width(prefix))
		for _, seg := range segments[1:] {
			lines = append(lines, indent+styleFormFieldSegment(f, seg))
		}
	}

	lines = append(lines, "")
	prefix := "description: "
	previewWidth := max(8, maxLineWidth-lipgloss.Width(prefix))
	previewLines, wasClipped := firstNDescriptionLines(m.form.Description, 5, previewWidth)
	lines = append(lines, prefix+previewLines[0])
	indent := strings.Repeat(" ", lipgloss.Width(prefix))
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

func renderPriorityLabel(priority int) string {
	return fmt.Sprintf("P%d", priority)
}

func renderIssueRow(item Issue, maxTextWidth int, depth int) string {
	priority := renderPriorityLabel(item.Priority)
	issueType := shortType(item.IssueType)
	id := truncate(item.ID, 14)
	prefix := treePrefix(depth)

	fixedWidth := lipgloss.Width(prefix) + lipgloss.Width(priority) + 1 + lipgloss.Width(issueType) + 1 + lipgloss.Width(id) + 1
	titleWidth := max(1, maxTextWidth-fixedWidth)
	title := truncate(item.Title, titleWidth)

	return prefix +
		priorityStyle(item.Priority).Render(priority) +
		" " + issueTypeStyle(item.IssueType).Render(issueType) +
		" " + lipgloss.NewStyle().Foreground(lipgloss.Color("246")).Render(id) +
		" " + title
}

func renderIssueRowSelectedPlain(item Issue, maxTextWidth int, depth int) string {
	priority := renderPriorityLabel(item.Priority)
	issueType := shortType(item.IssueType)
	prefix := treePrefix(depth)

	fixedWidth := lipgloss.Width(prefix) + lipgloss.Width(priority) + 1 + lipgloss.Width(issueType) + 1
	titleWidth := max(1, maxTextWidth-fixedWidth)
	title := truncate(item.Title, titleWidth)

	return prefix + priority + " " + issueType + " " + title
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
