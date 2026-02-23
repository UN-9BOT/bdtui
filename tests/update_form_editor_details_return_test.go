package bdtui_test

import (
	"errors"
	"testing"
)

func TestUpdateFormEditorMsg_FromDetailsReturnsToDetailsAndSubmits(t *testing.T) {
	t.Parallel()

	issue := Issue{
		ID:          "bdtui-y3v.3",
		Title:       "details ctrl+x",
		Description: "old description",
		Status:      StatusOpen,
		Display:     StatusOpen,
		Priority:    2,
		IssueType:   "task",
		Assignee:    "unbot",
	}
	clone := issue

	m := model{
		Mode:                     ModeEdit,
		ShowDetails:              true,
		DetailsIssueID:           issue.ID,
		ResumeDetailsAfterEditor: true,
		Form:                     newIssueFormEdit(&clone, []Issue{issue}),
	}

	next, cmd := m.Update(formEditorMsg{
		Payload: formEditorPayload{
			Title:       "details ctrl+x updated",
			Description: issue.Description,
			Status:      string(issue.Status),
			Priority:    issue.Priority,
			IssueType:   issue.IssueType,
			Assignee:    issue.Assignee,
			Labels:      "",
			Parent:      "",
		},
	})
	got := next.(model)

	if got.Mode != ModeDetails {
		t.Fatalf("expected mode=%s, got %s", ModeDetails, got.Mode)
	}
	if !got.ShowDetails {
		t.Fatalf("expected details panel to stay active")
	}
	if got.Form != nil {
		t.Fatalf("expected form to be cleared after auto-submit")
	}
	if got.ResumeDetailsAfterEditor {
		t.Fatalf("expected resumeDetailsAfterEditor to be reset")
	}
	if got.DetailsIssueID != issue.ID {
		t.Fatalf("expected detailsIssueID=%q, got %q", issue.ID, got.DetailsIssueID)
	}
	if cmd == nil {
		t.Fatalf("expected update command for edited payload")
	}
}

func TestUpdateFormEditorMsg_FromDetailsErrorReturnsToDetails(t *testing.T) {
	t.Parallel()

	issue := Issue{
		ID:        "bdtui-y3v.3",
		Title:     "details ctrl+x",
		Status:    StatusOpen,
		Display:   StatusOpen,
		Priority:  2,
		IssueType: "task",
		Assignee:  "unbot",
	}
	clone := issue

	m := model{
		Mode:                     ModeEdit,
		ShowDetails:              true,
		DetailsIssueID:           issue.ID,
		ResumeDetailsAfterEditor: true,
		Form:                     newIssueFormEdit(&clone, []Issue{issue}),
	}

	next, cmd := m.Update(formEditorMsg{Err: errors.New("editor failed")})
	got := next.(model)

	if got.Mode != ModeDetails {
		t.Fatalf("expected mode=%s, got %s", ModeDetails, got.Mode)
	}
	if !got.ShowDetails {
		t.Fatalf("expected details panel to stay active")
	}
	if got.Form != nil {
		t.Fatalf("expected form to be cleared on editor error")
	}
	if got.ResumeDetailsAfterEditor {
		t.Fatalf("expected resumeDetailsAfterEditor to be reset")
	}
	if got.Toast != "editor failed" {
		t.Fatalf("expected error toast, got %q", got.Toast)
	}
	if cmd != nil {
		t.Fatalf("expected nil cmd on editor error")
	}
}
