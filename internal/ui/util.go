package ui

import (
	"fmt"
	"os"
	"strings"
)

func Truncate(s string, width int) string {
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

func JoinNonEmpty(parts ...string) string {
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, ", ")
}

func ParseLabels(value string) []string {
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

func ParsePriority(value string) (int, error) {
	switch strings.TrimSpace(value) {
	case "0", "1", "2", "3", "4":
		return int(value[0] - 0), nil
	default:
		return 0, fmt.Errorf("invalid priority: %q", value)
	}
}

func CyclePriority(current int) int {
	return (current + 1) % 5
}

func CyclePriorityBackward(current int) int {
	return (current - 1 + 5) % 5
}

func DefaultAssigneeFromEnv() string {
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
