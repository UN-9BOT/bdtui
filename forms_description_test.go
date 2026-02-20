package main

import "testing"

func TestIssueFormDescriptionPreservesMultilineValue(t *testing.T) {
	t.Parallel()

	form := newIssueFormCreate(nil)
	form.Cursor = 1 // description
	form.loadInputFromField()

	want := "line 1\n\nline 3\n"
	form.DescInput.SetValue(want)
	form.saveInputToField()

	if form.Description != want {
		t.Fatalf("unexpected description\nwant: %q\ngot:  %q", want, form.Description)
	}
}

func TestIssueFormInsertDescriptionNewline(t *testing.T) {
	t.Parallel()

	form := newIssueFormCreate(nil)
	form.Cursor = 1 // description
	form.loadInputFromField()

	form.DescInput.SetValue("abc")
	form.DescInput.CursorEnd()
	form.insertDescriptionNewline()

	want := "abc\n"
	if form.Description != want {
		t.Fatalf("unexpected description\nwant: %q\ngot:  %q", want, form.Description)
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
