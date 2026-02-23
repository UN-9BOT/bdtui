package main

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

func (m model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.mode != ModeBoard {
		return m, nil
	}
	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return m, nil
	}

	status, rowIdx, ok := m.boardMouseTarget(msg.X, msg.Y)
	if !ok {
		return m, nil
	}

	rows, _ := m.buildColumnRows(status)
	if rowIdx < 0 || rowIdx >= len(rows) {
		return m, nil
	}

	target := rows[rowIdx]
	targetID := strings.TrimSpace(target.issue.ID)
	if targetID == "" {
		return m, nil
	}

	if !target.ghost {
		m.selectIssueByID(targetID)
		return m, nil
	}

	if m.selectIssueByID(targetID) {
		return m, nil
	}

	m.clearSearchAndFilters()
	m.selectIssueByID(targetID)
	return m, nil
}

func (m model) boardMouseTarget(x int, y int) (Status, int, bool) {
	const (
		boardLeft = 1
		boardTop  = 1
		taskStart = 2
	)

	if x < 0 || y < 0 {
		return "", 0, false
	}

	innerHeight := m.boardInnerHeight()
	outerHeight := innerHeight + 2
	if y < boardTop || y >= boardTop+outerHeight {
		return "", 0, false
	}

	yLocal := y - boardTop
	if yLocal == 0 || yLocal == outerHeight-1 {
		return "", 0, false
	}
	innerY := yLocal - 1
	rowInViewport := innerY - taskStart

	itemsPerPage := max(1, innerHeight-3)
	if rowInViewport < 0 || rowInViewport >= itemsPerPage {
		return "", 0, false
	}

	availableWidth := max(20, m.width-4)
	panelWidth := (availableWidth - (len(statusOrder) - 1)) / len(statusOrder)
	if panelWidth < 20 {
		panelWidth = 20
	}
	outerWidth := panelWidth + 2

	xLocal := x - boardLeft
	if xLocal < 0 {
		return "", 0, false
	}

	colIdx := xLocal / outerWidth
	if colIdx < 0 || colIdx >= len(statusOrder) {
		return "", 0, false
	}

	colStart := colIdx * outerWidth
	xInCol := xLocal - colStart
	if xInCol <= 0 || xInCol >= outerWidth-1 {
		return "", 0, false
	}

	status := statusOrder[colIdx]
	rowIdx := m.scrollOffset[status] + rowInViewport
	return status, rowIdx, true
}
