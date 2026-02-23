package app

import (
	beadsadapter "bdtui/internal/adapters/beads"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/fsnotify/fsnotify"
)

const (
	beadsWatchDebounce = 220 * time.Millisecond
	beadsRetryDelay    = 2 * time.Second
)

func watchBeadsChangesCmd(beadsDir string) tea.Cmd {
	root := strings.TrimSpace(beadsDir)
	return func() tea.Msg {
		if root == "" {
			return beadsWatchErrMsg{err: fmt.Errorf("beads dir is empty")}
		}

		watcher, err := fsnotify.NewWatcher()
		if err != nil {
			return beadsWatchErrMsg{err: err}
		}
		defer watcher.Close()

		watched := 0
		for _, path := range beadsWatchTargets(root) {
			if err := watcher.Add(path); err == nil {
				watched++
			}
		}
		if watched == 0 {
			if err := watcher.Add(root); err != nil {
				return beadsWatchErrMsg{err: err}
			}
		}

		var debounce *time.Timer
		var debounceCh <-chan time.Time

		for {
			select {
			case err, ok := <-watcher.Errors:
				if !ok {
					return beadsWatchErrMsg{err: fmt.Errorf("beads watcher stopped")}
				}
				return beadsWatchErrMsg{err: err}

			case ev, ok := <-watcher.Events:
				if !ok {
					return beadsWatchErrMsg{err: fmt.Errorf("beads watcher stopped")}
				}
				if !isBeadsWatchEventRelevant(ev) {
					continue
				}

				if debounce == nil {
					debounce = time.NewTimer(beadsWatchDebounce)
					debounceCh = debounce.C
					continue
				}

				if !debounce.Stop() {
					select {
					case <-debounce.C:
					default:
					}
				}
				debounce.Reset(beadsWatchDebounce)

			case <-debounceCh:
				return beadsChangedMsg{}
			}
		}
	}
}

func beadsWatchRetryCmd(delay time.Duration) tea.Cmd {
	if delay <= 0 {
		delay = beadsRetryDelay
	}
	return tea.Tick(delay, func(time.Time) tea.Msg {
		return beadsWatchRetryMsg{}
	})
}

func beadsWatchTargets(root string) []string {
	return beadsadapter.WatchTargets(root)
}

func isBeadsWatchEventRelevant(ev fsnotify.Event) bool {
	return beadsadapter.IsWatchEventRelevant(ev)
}
