package main

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
		mode:                     ModeEdit,
		showDetails:              true,
		detailsIssueID:           issue.ID,
		resumeDetailsAfterEditor: true,
		form:                     newIssueFormEdit(&clone, []Issue{issue}),
	}

	next, cmd := m.Update(formEditorMsg{
		payload: formEditorPayload{
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

	if got.mode != ModeDetails {
		t.Fatalf("expected mode=%s, got %s", ModeDetails, got.mode)
	}
	if !got.showDetails {
		t.Fatalf("expected details panel to stay active")
	}
	if got.form != nil {
		t.Fatalf("expected form to be cleared after auto-submit")
	}
	if got.resumeDetailsAfterEditor {
		t.Fatalf("expected resumeDetailsAfterEditor to be reset")
	}
	if got.detailsIssueID != issue.ID {
		t.Fatalf("expected detailsIssueID=%q, got %q", issue.ID, got.detailsIssueID)
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
		mode:                     ModeEdit,
		showDetails:              true,
		detailsIssueID:           issue.ID,
		resumeDetailsAfterEditor: true,
		form:                     newIssueFormEdit(&clone, []Issue{issue}),
	}

	next, cmd := m.Update(formEditorMsg{err: errors.New("editor failed")})
	got := next.(model)

	if got.mode != ModeDetails {
		t.Fatalf("expected mode=%s, got %s", ModeDetails, got.mode)
	}
	if !got.showDetails {
		t.Fatalf("expected details panel to stay active")
	}
	if got.form != nil {
		t.Fatalf("expected form to be cleared on editor error")
	}
	if got.resumeDetailsAfterEditor {
		t.Fatalf("expected resumeDetailsAfterEditor to be reset")
	}
	if got.toast != "editor failed" {
		t.Fatalf("expected error toast, got %q", got.toast)
	}
	if cmd != nil {
		t.Fatalf("expected nil cmd on editor error")
	}
}
