package beads

import (
	"path/filepath"
	"strings"

	"github.com/fsnotify/fsnotify"
)

func WatchTargets(root string) []string {
	return []string{
		filepath.Join(root, "last-touched"),
		filepath.Join(root, "issues.jsonl"),
		root,
	}
}

func IsWatchEventRelevant(ev fsnotify.Event) bool {
	if ev.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename|fsnotify.Remove) == 0 {
		return false
	}
	base := strings.TrimSpace(filepath.Base(ev.Name))
	return base == "last-touched" || base == "issues.jsonl"
}
