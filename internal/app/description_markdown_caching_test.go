package app

import (
	"sync/atomic"
	"testing"
	"time"
)

// TestRenderMarkdownCachesRenderer ensures we don't re-create the glamour
// renderer for every call, which would re-trigger termenv's synchronous
// CSI 11;? background-color probe on stdout and hang the TUI on terminals
// that do not respond to the query.
func TestRenderMarkdownCachesRenderer(t *testing.T) {
	

	resetGlamourRendererCache()

	var creations atomic.Int32
	prevFactory := glamourRendererFactory
	glamourRendererFactory = func(width int) (*glamourTermRenderer, error) {
		creations.Add(1)
		return prevFactory(width)
	}
	t.Cleanup(func() {
		glamourRendererFactory = prevFactory
		resetGlamourRendererCache()
	})

	for i := 0; i < 50; i++ {
		if _, err := glamourRenderer(40); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	if got := creations.Load(); got != 1 {
		t.Fatalf("expected renderer to be cached and created once, got %d creations", got)
	}
}

// TestRenderMarkdownCompletesWithoutTTY guards against a regression where
// glamour's WithAutoStyle() blocks on a synchronous terminal probe of stdout.
// We bound the call with a timeout; on a hung renderer the test fails.
func TestRenderMarkdownCompletesWithoutTTY(t *testing.T) {
	

	resetGlamourRendererCache()

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = glamourRenderer(40)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("renderMarkdownWithGlamour blocked (likely a synchronous TTY probe)")
	}
}

