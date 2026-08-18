package daemon

import (
	"time"

	"bdtui/internal/daemon/daemonpb"
	"bdtui/internal/orch"
)

func timeToProto(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

func timePtrToProto(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.UTC().Format(time.RFC3339Nano)
	return &s
}

func runToProto(r *orch.Run) *daemonpb.Run {
	return &daemonpb.Run{
		Id:                   r.ID,
		ProjectId:            r.ProjectID,
		TaskId:               r.TaskID,
		Status:               string(r.Status),
		WorkflowSnapshotRef:  r.WorkflowSnapshotRef,
		WorkflowSnapshot:     r.WorkflowSnapshot,
		CurrentStepId:        r.CurrentStepID,
		NeedsAttentionReason: r.NeedsAttentionReason,
		Error:                r.Error,
		CreatedAt:            timeToProto(r.CreatedAt),
		UpdatedAt:            timeToProto(r.UpdatedAt),
		StartedAt:            timePtrToProto(r.StartedAt),
		CompletedAt:          timePtrToProto(r.CompletedAt),
	}
}

func humanInputToProto(h *orch.HumanInput) *daemonpb.HumanInput {
	return &daemonpb.HumanInput{
		Id:            h.ID,
		RunId:         h.RunID,
		StepAttemptId: h.StepAttemptID,
		ExecutionId:   h.ExecutionID,
		Prompt:        h.Prompt,
		Response:      h.Response,
		Status:        string(h.Status),
		CreatedAt:     timeToProto(h.CreatedAt),
		AnsweredAt:    timePtrToProto(h.AnsweredAt),
	}
}

func executionToProto(e *orch.Execution) *daemonpb.Execution {
	return &daemonpb.Execution{
		Id:            e.ID,
		RunId:         e.RunID,
		StepAttemptId: e.StepAttemptID,
		Kind:          string(e.Kind),
		Status:        string(e.Status),
		PaneId:        e.PaneID,
		ProcessId:     e.ProcessID,
		PromptRef:     e.PromptRef,
		PromptHash:    e.PromptHash,
		ResultJson:    e.ResultJSON,
		ResultCommit:  e.ResultCommit,
		Error:         e.Error,
		CreatedAt:     timeToProto(e.CreatedAt),
		UpdatedAt:     timeToProto(e.UpdatedAt),
		StartedAt:     timePtrToProto(e.StartedAt),
		CompletedAt:   timePtrToProto(e.CompletedAt),
	}
}

func artifactToProto(a *orch.Artifact) *daemonpb.Artifact {
	return &daemonpb.Artifact{
		Id:          a.ID,
		ExecutionId: a.ExecutionID,
		Name:        a.Name,
		Path:        a.Path,
		Hash:        a.Hash,
		CreatedAt:   timeToProto(a.CreatedAt),
	}
}

func eventToProto(e *orch.Event) *daemonpb.Event {
	return &daemonpb.Event{
		Id:        e.ID,
		RunId:     e.RunID,
		Seq:       e.Seq,
		Type:      e.Type,
		Payload:   e.Payload,
		CreatedAt: timeToProto(e.CreatedAt),
	}
}
