package bdtui_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	b "bdtui/internal/app"
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
		"parent: \"\"",
		"---",
		description,
	}, "\n")
}

func TestEditorTempDir(t *testing.T) {
	t.Parallel()

	dir := b.EditorTempDir()
	expected := filepath.Join(".idea", "tmp")

	if dir != expected {
		t.Fatalf("expected %q, got %q", expected, dir)
	}

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("directory %q should exist: %v", dir, err)
	}
	if !info.IsDir() {
		t.Fatalf("%q should be a directory", dir)
	}

	_ = os.RemoveAll(dir)
}

func TestParseEditorSectionsDescriptionOnly(t *testing.T) {
	t.Parallel()

	body := "\n---\n- DESCRIPTION\n---\n\nline 1\nline 2"
	desc, notes := parseEditorSections(body)

	if desc != "line 1\nline 2" {
		t.Fatalf("unexpected description: %q", desc)
	}
	if notes != "" {
		t.Fatalf("expected empty notes, got: %q", notes)
	}
}

func TestParseEditorSectionsNotesOnly(t *testing.T) {
	t.Parallel()

	body := "\n---\n- NOTES\n---\n\nnote line"
	desc, notes := parseEditorSections(body)

	if desc != "" {
		t.Fatalf("expected empty description, got: %q", desc)
	}
	if notes != "note line" {
		t.Fatalf("unexpected notes: %q", notes)
	}
}

func TestParseEditorSectionsBothDescriptionAndNotes(t *testing.T) {
	t.Parallel()

	body := "\n---\n- DESCRIPTION\n---\n\ndesc content\n\n---\n- NOTES\n---\n\nnotes content"
	desc, notes := parseEditorSections(body)

	if desc != "desc content" {
		t.Fatalf("unexpected description: %q", desc)
	}
	if notes != "notes content" {
		t.Fatalf("unexpected notes: %q", notes)
	}
}

func TestParseEditorSectionsNoMarkers(t *testing.T) {
	t.Parallel()

	body := "plain text without markers"
	desc, notes := parseEditorSections(body)

	if desc != "plain text without markers" {
		t.Fatalf("unexpected description: %q", desc)
	}
	if notes != "" {
		t.Fatalf("expected empty notes, got: %q", notes)
	}
}

func TestMarshalEditorContentWithNotes(t *testing.T) {
	t.Parallel()

	payload := formEditorPayload{
		Title:       "test",
		Status:      "open",
		Priority:    2,
		IssueType:   "task",
		Parent:      "",
		Description: "desc content",
		Notes:       "notes content",
	}

	raw, err := marshalEditorContent(payload)
	if err != nil {
		t.Fatalf("marshalEditorContent returned error: %v", err)
	}

	text := string(raw)
	if !strings.Contains(text, "- DESCRIPTION") {
		t.Fatal("expected DESCRIPTION marker")
	}
	if !strings.Contains(text, "- NOTES") {
		t.Fatal("expected NOTES marker")
	}
	if !strings.Contains(text, "desc content") {
		t.Fatal("expected description content")
	}
	if !strings.Contains(text, "notes content") {
		t.Fatal("expected notes content")
	}
}

func TestMarshalEditorContentEmptyNotes(t *testing.T) {
	t.Parallel()

	payload := formEditorPayload{
		Title:       "test",
		Status:      "open",
		Priority:    2,
		IssueType:   "task",
		Parent:      "",
		Description: "desc only",
		Notes:       "",
	}

	raw, err := marshalEditorContent(payload)
	if err != nil {
		t.Fatalf("marshalEditorContent returned error: %v", err)
	}

	parsed, err := parseEditorContent(raw)
	if err != nil {
		t.Fatalf("parseEditorContent returned error: %v", err)
	}

	if parsed.Description != "desc only" {
		t.Fatalf("unexpected description: %q", parsed.Description)
	}
	if strings.TrimSpace(parsed.Notes) != "" {
		t.Fatalf("expected empty/whitespace notes, got: %q", parsed.Notes)
	}
}

func TestRoundtripEditorContentWithNotes(t *testing.T) {
	t.Parallel()

	payload := formEditorPayload{
		Title:       "test issue",
		Status:      "in_progress",
		Priority:    1,
		IssueType:   "feature",
		Parent:      "",
		Description: "line 1\nline 2\nline 3",
		Notes:       "note 1\nnote 2",
	}

	raw, err := marshalEditorContent(payload)
	if err != nil {
		t.Fatalf("marshalEditorContent returned error: %v", err)
	}

	parsed, err := parseEditorContent(raw)
	if err != nil {
		t.Fatalf("parseEditorContent returned error: %v", err)
	}

	if parsed.Title != payload.Title {
		t.Fatalf("title mismatch: %q vs %q", parsed.Title, payload.Title)
	}
	if parsed.Description != payload.Description {
		t.Fatalf("description mismatch: %q vs %q", parsed.Description, payload.Description)
	}
	if strings.TrimRight(parsed.Notes, "\n") != payload.Notes {
		t.Fatalf("notes mismatch: %q vs %q", parsed.Notes, payload.Notes)
	}
}
