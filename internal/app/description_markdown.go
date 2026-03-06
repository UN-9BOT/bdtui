package app

import (
	"strings"

	"github.com/charmbracelet/glamour"
)

type markdownRenderFn func(content string, width int) (string, error)

var renderMarkdown markdownRenderFn = renderMarkdownWithGlamour

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
	if width <= 0 {
		width = 1
	}

	r, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(width),
	)
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
