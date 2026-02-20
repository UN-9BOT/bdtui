package main

import "testing"

func TestIssueFormDescriptionExcludedFromEditableFields(t *testing.T) {
	t.Parallel()

	form := newIssueFormCreate(nil)
	for _, field := range form.fields() {
		if field == "description" {
			t.Fatalf("description must not be editable field")
		}
	}
}

func TestIssueFormDescriptionIsNotTextField(t *testing.T) {
	t.Parallel()

	form := newIssueFormCreate(nil)
	if form.isTextField("description") {
		t.Fatalf("description must not be text field")
	}
}

func TestIssueFormTitleStillTrimmed(t *testing.T) {
	t.Parallel()

	form := newIssueFormCreate(nil)
	form.Cursor = 0 // title
	form.loadInputFromField()
	form.Input.SetValue("  hello  ")
	form.saveInputToField()

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
