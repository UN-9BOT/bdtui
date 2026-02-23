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

	payload := formEditorPayload{
		Title:       "test",
		Status:      "open",
		Priority:    2,
		IssueType:   "task",
		Assignee:    "unbot",
		Labels:      "",
		Parent:      "",
		Description: "tail-without-newline",
	}

	raw, err := marshalEditorContent(payload)
	if err != nil {
		t.Fatalf("marshalEditorContent returned error: %v", err)
	}

	parsed, err := parseEditorContent(raw)
	if err != nil {
		t.Fatalf("parseEditorContent returned error: %v", err)
	}

	if parsed.Description != payload.Description {
		t.Fatalf("unexpected description\nexpected: %q\nactual:   %q", payload.Description, parsed.Description)
	}
}

func TestMarshalEditorContentAddsEnumHints(t *testing.T) {
	t.Parallel()

	payload := formEditorPayload{
		Title:       "test",
		Status:      "open",
		Priority:    2,
		IssueType:   "task",
		Assignee:    "unbot",
		Labels:      "",
		Parent:      "",
		Description: "desc",
	}

	raw, err := marshalEditorContent(payload)
	if err != nil {
		t.Fatalf("marshalEditorContent returned error: %v", err)
	}

	text := string(raw)
	if !strings.Contains(text, "status: open # open | in_progress | blocked | closed") {
		t.Fatalf("expected status hint, got: %q", text)
	}
	if !strings.Contains(text, "priority: 2 # 0 | 1 | 2 | 3 | 4") {
		t.Fatalf("expected priority hint, got: %q", text)
	}
	if !strings.Contains(text, "type: task # task | epic | bug | feature | chore | decision") {
		t.Fatalf("expected type hint, got: %q", text)
	}
}

func TestParseEditorContentWithInlineHints(t *testing.T) {
	t.Parallel()

	raw := strings.Join([]string{
		"---",
		"title: test",
		"status: in_progress # open | in_progress | blocked | closed",
		"priority: 3 # 0 | 1 | 2 | 3 | 4",
		"type: feature # task | epic | bug | feature | chore | decision",
		"assignee: unbot",
		"labels: \"\"",
		"parent: \"\"",
		"---",
		"description",
	}, "\n")

	parsed, err := parseEditorContent([]byte(raw))
	if err != nil {
		t.Fatalf("parseEditorContent returned error: %v", err)
	}
	if parsed.Status != "in_progress" {
		t.Fatalf("unexpected status: %q", parsed.Status)
	}
	if parsed.Priority != 3 {
		t.Fatalf("unexpected priority: %d", parsed.Priority)
	}
	if parsed.IssueType != "feature" {
		t.Fatalf("unexpected type: %q", parsed.IssueType)
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
