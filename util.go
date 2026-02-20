package main

import (
	"fmt"
	"os"
	"strings"
)

func truncate(s string, width int) string {
	if width <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= width {
		return s
	}
	if width == 1 {
		return "…"
	}
	return string(r[:width-1]) + "…"
}

func joinNonEmpty(parts ...string) string {
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, ", ")
}

func parseLabels(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	raw := strings.Split(value, ",")
	out := make([]string, 0, len(raw))
	seen := map[string]bool{}
	for _, label := range raw {
		trimmed := strings.TrimSpace(label)
		if trimmed == "" || seen[trimmed] {
			continue
		}
		seen[trimmed] = true
		out = append(out, trimmed)
	}
	return out
}

func parsePriority(value string) (int, error) {
	switch strings.TrimSpace(value) {
	case "0", "1", "2", "3", "4":
		return int(value[0] - 0), nil
	default:
		return 0, fmt.Errorf("invalid priority: %q", value)
	}
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

func cyclePriority(current int) int {
	return (current + 1) % 5
}

func defaultAssigneeFromEnv() string {
	candidates := []string{
		os.Getenv("BD_ACTOR"),
		os.Getenv("USER"),
		os.Getenv("USERNAME"),
	}
	for _, c := range candidates {
		c = strings.TrimSpace(c)
		if c != "" {
			return c
		}
	}
	return ""
}
