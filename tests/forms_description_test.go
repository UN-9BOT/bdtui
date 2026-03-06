package bdtui_test

import (
	"strings"
	"testing"
)

func TestIssueFormDescriptionExcludedFromEditableFields(t *testing.T) {
	t.Parallel()

	form := newIssueFormCreate(nil)
	for _, field := range form.Fields() {
		if field == "description" {
			t.Fatalf("description must not be editable field")
		}
	}
}

func TestIssueFormDescriptionIsNotTextField(t *testing.T) {
	t.Parallel()

	form := newIssueFormCreate(nil)
	if form.IsTextField("description") {
		t.Fatalf("description must not be text field")
	}
}

func TestIssueFormTitleStillTrimmed(t *testing.T) {
	t.Parallel()

	form := newIssueFormCreate(nil)
	form.Cursor = 0 // title
	form.LoadInputFromField()
	form.Input.SetValue("  hello  ")
	form.SaveInputToField()

	if form.Title != "hello" {
		t.Fatalf("unexpected title: %q", form.Title)
	}
}

func TestFirstNDescriptionLines(t *testing.T) {
	t.Parallel()

	lines, clipped := firstNDescriptionLines("1\n2\n3\n4\n5\n6", 5, 20)
	if !clipped {
		t.Fatalf("expected clipped=true")
	}
	if len(lines) != 5 {
		t.Fatalf("expected 5 lines, got %d", len(lines))
	}
	if lines[0] != "1" || lines[4] != "5" {
		t.Fatalf("unexpected lines: %#v", lines)
	}
}

func TestFirstNDescriptionLinesKeepsPlainMarkdownText(t *testing.T) {
	t.Parallel()

	lines, clipped := firstNDescriptionLines("# H1\n- one\n`code`", 5, 40)
	if clipped {
		t.Fatalf("expected clipped=false")
	}
	if strings.Contains(strings.Join(lines, "\n"), "\x1b[") {
		t.Fatalf("expected plain text preview without ANSI markdown styling, got %#v", lines)
	}
	if lines[0] != "# H1" {
		t.Fatalf("expected raw markdown in form preview, got %#v", lines)
	}
}
