package orch

import "time"

// Project is a durable, addressable workspace. Its ID is stable; FsPath and
// GitRemote are mutable attributes that may change as the project moves.
type Project struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	FsPath    string    `json:"fs_path"`
	GitRemote string    `json:"git_remote"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Run is a single execution of a workflow against a Project. WorkflowSnapshot
// holds the canonical JSON snapshot and WorkflowSnapshotRef its content hash;
// both are populated at Run start from the workflow dependency closure.
type Run struct {
	ID                   string     `json:"id"`
	ProjectID            string     `json:"project_id"`
	Status               RunStatus  `json:"status"`
	WorkflowSnapshotRef  string     `json:"workflow_snapshot_ref"`
	WorkflowSnapshot     string     `json:"workflow_snapshot"`
	CurrentStepID        *string    `json:"current_step_id"`
	NeedsAttentionReason *string    `json:"needs_attention_reason"`
	Error                *string    `json:"error"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
	StartedAt            *time.Time `json:"started_at"`
	CompletedAt          *time.Time `json:"completed_at"`
}

// StepAttempt is one execution attempt of a workflow step (identified by
// StepID) within a Run. Attempt numbers are 1-based and unique per
// (run_id, step_id).
type StepAttempt struct {
	ID          string            `json:"id"`
	RunID       string            `json:"run_id"`
	StepID      string            `json:"step_id"`
	Attempt     int               `json:"attempt"`
	Status      StepAttemptStatus `json:"status"`
	Inputs      string            `json:"inputs"`
	Result      *string           `json:"result"`
	Error       *string           `json:"error"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
	StartedAt   *time.Time        `json:"started_at"`
	CompletedAt *time.Time        `json:"completed_at"`
}

// Execution is a concrete process/agent invocation tied to a StepAttempt.
// PaneID/ProcessID reference an external runtime (e.g. Herdr) for reattachment.
type Execution struct {
	ID            string          `json:"id"`
	RunID         string          `json:"run_id"`
	StepAttemptID string          `json:"step_attempt_id"`
	Kind          ExecutionKind   `json:"kind"`
	Status        ExecutionStatus `json:"status"`
	PaneID        *string         `json:"pane_id"`
	ProcessID     *string         `json:"process_id"`
	Prompt        string          `json:"prompt"`
	ResultJSON    *string         `json:"result_json"`
	ResultCommit  *string         `json:"result_commit"`
	Artifacts     string          `json:"artifacts"`
	Error         *string         `json:"error"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
	StartedAt     *time.Time      `json:"started_at"`
	CompletedAt   *time.Time      `json:"completed_at"`
}

// LaunchIntent is a durable request to launch a Run, persisted before the Run
// itself exists so a launch can be accepted/rejected without losing intent.
type LaunchIntent struct {
	ID          string             `json:"id"`
	ProjectID   string             `json:"project_id"`
	WorkflowRef string             `json:"workflow_ref"`
	Inputs      string             `json:"inputs"`
	Status      LaunchIntentStatus `json:"status"`
	RunID       *string            `json:"run_id"`
	CreatedAt   time.Time          `json:"created_at"`
	ResolvedAt  *time.Time         `json:"resolved_at"`
}

// HumanInput is a durable prompt awaiting a human answer for a human step.
type HumanInput struct {
	ID            string           `json:"id"`
	RunID         string           `json:"run_id"`
	StepAttemptID string           `json:"step_attempt_id"`
	ExecutionID   *string          `json:"execution_id"`
	Prompt        string           `json:"prompt"`
	Response      *string          `json:"response"`
	Status        HumanInputStatus `json:"status"`
	CreatedAt     time.Time        `json:"created_at"`
	AnsweredAt    *time.Time       `json:"answered_at"`
}

// Event is an append-only audit/TUI stream entry. Relational state is
// authoritative; events are never mutated after append. Seq is monotonic per
// run (NULL run_id means a project-scoped event).
type Event struct {
	ID        int64     `json:"id"`
	RunID     *string   `json:"run_id"`
	Seq       int64     `json:"seq"`
	Type      string    `json:"type"`
	Payload   string    `json:"payload"`
	CreatedAt time.Time `json:"created_at"`
}
