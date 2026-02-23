package app

import ui "bdtui/internal/ui"

func truncate(s string, width int) string {
	return ui.Truncate(s, width)
}

func joinNonEmpty(parts ...string) string {
	return ui.JoinNonEmpty(parts...)
}

func parseLabels(value string) []string {
	return ui.ParseLabels(value)
}

func parsePriority(value string) (int, error) {
	return ui.ParsePriority(value)
}

func statusFromString(v string) (Status, bool) {
	switch v {
	case string(StatusOpen):
		return StatusOpen, true
	case string(StatusInProgress):
		return StatusInProgress, true
	case string(StatusBlocked):
		return StatusBlocked, true
	case string(StatusClosed):
		return StatusClosed, true
	default:
		return "", false
	}
}

func cycleStatus(current Status) Status {
	order := []Status{StatusOpen, StatusInProgress, StatusBlocked, StatusClosed}
	idx := 0
	for i, s := range order {
		if s == current {
			idx = i
			break
		}
	}
	return order[(idx+1)%len(order)]
}

func cycleStatusBackward(current Status) Status {
	order := []Status{StatusOpen, StatusInProgress, StatusBlocked, StatusClosed}
	idx := 0
	for i, s := range order {
		if s == current {
			idx = i
			break
		}
	}
	return order[(idx-1+len(order))%len(order)]
}

func cyclePriority(current int) int {
	return ui.CyclePriority(current)
}

func cyclePriorityBackward(current int) int {
	return ui.CyclePriorityBackward(current)
}

func defaultAssigneeFromEnv() string {
	return ui.DefaultAssigneeFromEnv()
}
