package app

import (
	"strings"
	"time"
)

// IsDeferred reports whether the issue has a non-empty defer_until timestamp.
func (i Issue) IsDeferred() bool {
	return strings.TrimSpace(i.DeferUntil) != ""
}

// parseDeferTime parses the bd defer_until timestamp into a UTC time.
// bd stores defer_until as RFC3339 UTC (e.g. "2026-12-30T21:00:00Z").
// Returns false if the value is empty or unparseable.
func parseDeferTime(raw string) (time.Time, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, false
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t.UTC(), true
	}
	if t, err := time.Parse("2006-01-02", raw); err == nil {
		return t.UTC(), true
	}
	return time.Time{}, false
}

// formatDeferDate returns a short human-readable date for the defer_until
// value. The value is rendered in the user's local time. Falls back to the
// raw string when the timestamp cannot be parsed.
func formatDeferDate(raw string) string {
	t, ok := parseDeferTime(raw)
	if !ok {
		return strings.TrimSpace(raw)
	}
	return t.Local().Format("2006-01-02")
}

// deferStatus describes the temporal state of a deferred issue.
type deferStatus int

const (
	deferStateNone deferStatus = iota
	deferStatePending
	deferStateActive
	deferStatePast
)

// deferBadgeLabel returns the short label rendered on board rows.
// Examples: "⏱ 2026-06-14", "⏱ now", "⏸ past".
func (i Issue) deferBadgeLabel() string {
	if !i.IsDeferred() {
		return ""
	}
	switch i.deferState(time.Now()) {
	case deferStateActive:
		return "⏱ now"
	case deferStatePast:
		return "⏸ past"
	}
	return "⏱ " + formatDeferDate(i.DeferUntil)
}

func (i Issue) deferState(now time.Time) deferStatus {
	if !i.IsDeferred() {
		return deferStateNone
	}
	t, ok := parseDeferTime(i.DeferUntil)
	if !ok {
		return deferStatePending
	}
	local := t.Local()
	if now.Before(local) {
		// within the same calendar day already counts as "now"
		y1, m1, d1 := now.Date()
		y2, m2, d2 := local.Date()
		if y1 == y2 && m1 == m2 && d1 == d2 {
			return deferStateActive
		}
		return deferStatePending
	}
	return deferStatePast
}
