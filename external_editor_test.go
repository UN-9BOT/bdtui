package main

import (
	"strings"
	"testing"
)

func TestParseEditorContentPreservesDescriptionNewlines(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		description string
	}{
		{
			name:        "plain text",
			description: "line 1\nline 2",
		},
		{
			name:        "leading blank line",
			description: "\nline after blank",
		},
		{
			name:        "trailing blank lines",
			description: "line\n\n",
		},
		{
			name:        "only blank lines",
			description: "\n\n",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			raw := []byte(editorDocument(tc.description))
			parsed, err := parseEditorContent(raw)
			if err != nil {
				t.Fatalf("parseEditorContent returned error: %v", err)
			}
			if parsed.Description != tc.description {
				t.Fatalf("unexpected description\nexpected: %q\nactual:   %q", tc.description, parsed.Description)
			}
		})
	}
}

func TestParseEditorContentCRLFNormalization(t *testing.T) {
	t.Parallel()

	raw := editorDocument("line 1\n\nline 3\n")
	rawCRLF := strings.ReplaceAll(raw, "\n", "\r\n")

	parsed, err := parseEditorContent([]byte(rawCRLF))
	if err != nil {
		t.Fatalf("parseEditorContent returned error: %v", err)
	}

	want := "line 1\n\nline 3\n"
	if parsed.Description != want {
		t.Fatalf("unexpected description\nexpected: %q\nactual:   %q", want, parsed.Description)
	}
}

func TestParseEditorContentMissingClosingFrontmatter(t *testing.T) {
	t.Parallel()

	raw := []byte(strings.Join([]string{
		"---",
		"title: test",
		"status: open",
		"priority: 2",
		"type: task",
		"assignee: unbot",
		"labels: \"\"",
		"parent: \"\"",
		"no closing separator",
	}, "\n"))

	_, err := parseEditorContent(raw)
	if err == nil {
		t.Fatal("expected error for missing closing separator, got nil")
	}
	if !strings.Contains(err.Error(), "closing frontmatter separator") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMarshalEditorContentDoesNotAppendExtraNewline(t *testing.T) {
	t.Parallel()

	frontmatter := formEditorFrontmatter{
		Title:     "test",
		Status:    "open",
		Priority:  2,
		IssueType: "task",
		Assignee:  "unbot",
		Labels:    "",
		Parent:    "",
	}

	description := "tail-without-newline"
	raw, err := marshalEditorContent(frontmatter, description)
	if err != nil {
		t.Fatalf("marshalEditorContent returned error: %v", err)
	}

	parsed, err := parseEditorContent(raw)
	if err != nil {
		t.Fatalf("parseEditorContent returned error: %v", err)
	}

	if parsed.Description != description {
		t.Fatalf("unexpected description\nexpected: %q\nactual:   %q", description, parsed.Description)
	}
}

func editorDocument(description string) string {
	return strings.Join([]string{
		"---",
		"title: test",
		"status: open",
		"priority: 2",
		"type: task",
		"assignee: unbot",
		"labels: \"\"",
		"parent: \"\"",
		"---",
		description,
	}, "\n")
}
