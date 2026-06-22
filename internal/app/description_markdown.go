package app

import (
	"strings"
	"sync"

	"github.com/charmbracelet/glamour"
	"github.com/muesli/termenv"
)

type markdownRenderFn func(content string, width int) (string, error)

var renderMarkdown markdownRenderFn = renderMarkdownWithGlamour

// glamourTermRenderer is a thin alias so tests can swap the factory without
// importing glamour.
type glamourTermRenderer = glamour.TermRenderer

// glamourRendererFactory builds a renderer for a given word-wrap width.
// Indirected through a var so tests can count invocations.
var glamourRendererFactory = newGlamourRenderer

func newGlamourRenderer(width int) (*glamourTermRenderer, error) {
	if width <= 0 {
		width = 1
	}
	return glamour.NewTermRenderer(
		glamour.WithStandardStyle("notty"),
		glamour.WithColorProfile(termenv.ANSI256),
		glamour.WithWordWrap(width),
	)
}

// glamourRendererCache memoises glamour renderers keyed by word-wrap width.
// glamour.NewTermRenderer with WithAutoStyle() calls termenv.HasDarkBackground,
// which sends a CSI 11;? query and performs a blocking read on stdout. Doing
// that on every View() frame (or, worse, once per detail render) hangs the TUI
// on terminals that do not answer the query, so we pin a color profile and
// reuse a single renderer per width.
var glamourRendererCache sync.Map // map[int]*glamour.TermRenderer

func resetGlamourRendererCache() {
	glamourRendererCache.Range(func(k, _ any) bool {
		glamourRendererCache.Delete(k)
		return true
	})
}

func glamourRenderer(width int) (*glamourTermRenderer, error) {
	if width <= 0 {
		width = 1
	}
	if v, ok := glamourRendererCache.Load(width); ok {
		return v.(*glamourTermRenderer), nil
	}
	r, err := glamourRendererFactory(width)
	if err != nil {
		return nil, err
	}
	glamourRendererCache.Store(width, r)
	return r, nil
}

func renderDescriptionLines(description string, width int) []string {
	text := strings.ReplaceAll(description, "\r\n", "\n")
	if strings.TrimSpace(text) == "" {
		return []string{"-"}
	}

	rendered, err := renderMarkdown(text, width)
	if err != nil {
		out := wrapPlainText(text, width)
		if len(out) == 0 {
			return []string{"-"}
		}
		return out
	}

	lines := splitRenderedMarkdownLines(rendered)
	if len(lines) == 0 {
		return []string{"-"}
	}
	return lines
}

func renderMarkdownWithGlamour(content string, width int) (string, error) {
	r, err := glamourRenderer(width)
	if err != nil {
		return "", err
	}
	out, err := r.Render(content)
	if err != nil {
		return "", err
	}
	return strings.TrimRight(out, "\n"), nil
}

func splitRenderedMarkdownLines(rendered string) []string {
	text := strings.ReplaceAll(rendered, "\r\n", "\n")
	lines := strings.Split(text, "\n")
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}
