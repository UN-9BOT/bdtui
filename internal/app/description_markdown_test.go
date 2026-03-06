package app

import (
	"errors"
	"strings"
	"testing"
)

func TestRenderDescriptionLinesMarkdown(t *testing.T) {
	t.Parallel()

	got := renderDescriptionLines("# Head\n\n- one\n- two\n\n`code`", 40)
	if len(got) < 3 {
		t.Fatalf("expected multiline markdown output, got %#v", got)
	}

	joined := strings.Join(got, "\n")
	if !strings.Contains(joined, "• one") || !strings.Contains(joined, "• two") {
		t.Fatalf("expected markdown list rendering, got %q", joined)
	}
	if strings.Contains(joined, "- one") || strings.Contains(joined, "- two") {
		t.Fatalf("expected list markers transformed by markdown renderer, got %q", joined)
	}
}

func TestRenderDescriptionLinesEmpty(t *testing.T) {
	t.Parallel()

	got := renderDescriptionLines(" \n\t", 40)
	if len(got) != 1 || got[0] != "-" {
		t.Fatalf("expected fallback dash for empty description, got %#v", got)
	}
}

func TestRenderDescriptionLinesFallbackToPlain(t *testing.T) {
	t.Parallel()

	previous := renderMarkdown
	t.Cleanup(func() { renderMarkdown = previous })
	renderMarkdown = func(_ string, _ int) (string, error) {
		return "", errors.New("boom")
	}

	got := renderDescriptionLines("alpha beta gamma", 8)
	if len(got) < 2 {
		t.Fatalf("expected wrapped plain text on renderer error, got %#v", got)
	}
	if got[0] != "alpha" || got[1] != "beta" {
		t.Fatalf("unexpected plain fallback content: %#v", got)
	}
}
