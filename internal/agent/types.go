// Package agent defines the generic, provider-agnostic boundary between the
// orchestrator controller and a concrete agent runner (the MVP being Maki),
// plus the deterministic prompt envelope and the completion checks that gate a
// step attempt on machine-verifiable evidence.
//
// Workflow authoring is human-driven: this package only reads and validates
// definitions; it never creates or edits them. Agents do not create or edit
// workflows either; they execute a fully rendered prompt and produce a
// controller-assigned result.json plus any declared artifacts.
package agent

import "bdtui/internal/workflow"

// Request is a provider-agnostic request to run a single agent step. The
// controller builds the deterministic prompt envelope and assigns the output
// paths; the adapter only translates this into the underlying CLI/protocol.
type Request struct {
	// SessionKey identifies a reusable agent session. MVP keeps one session per
	// (run, role) so revise/review loops for the same role may reuse context.
	SessionKey string

	// Prompt is the deterministic, fully-rendered prompt envelope.
	Prompt string

	// WorkingDir is the Git worktree the agent operates in.
	WorkingDir string

	// OutputPaths are controller-assigned absolute paths inside Run storage
	// where the agent must write its structured result and declared artifacts.
	OutputPaths OutputPaths

	// Contract describes the shape of result.json and the semantic outcomes
	// the role may report.
	Contract ResultContract
}

// OutputPaths are the controller-assigned absolute paths inside Run storage
// where the agent must write its structured result and declared artifacts.
// Path values are opaque to the adapter; only Result and Artifacts are read.
type OutputPaths struct {
	// Result is the absolute path of result.json.
	Result string

	// Artifacts maps a role-declared output name to its absolute path in Run
	// storage. Declared artifacts live outside the Git worktree and are
	// immutable per step attempt.
	Artifacts map[string]string
}

// ResultContract describes the result.json shape (a JSON Schema) and the
// semantic outcomes the role may report. The completion check enforces both.
type ResultContract struct {
	// Schema is the raw JSON Schema text. The agent is expected to write a
	// result.json that satisfies it; the completion check re-validates.
	Schema string

	// AllowedOutcomes is the set of semantic outcomes the role may report.
	// The result.json must carry exactly one of these as its outcome.
	AllowedOutcomes []string
}

// TaskSnapshot is the immutable snapshot of the source Kanban task for a run.
// The controller captures it at Run start; the envelope embeds it verbatim so
// the agent sees the same task description the human author reviewed.
type TaskSnapshot struct {
	ID          string
	Title       string
	Description string
}

// ProjectInstruction is a snapshotted project instruction file
// (AGENTS.md/CLAUDE.md/known skills where applicable). Snapshotted content is
// embedded in the envelope so the agent cannot be confused by later edits.
type ProjectInstruction struct {
	Name    string
	Content string
}

// EnvelopeInput is the fully-resolved input the controller passes to
// BuildEnvelope. Ordering of Instructions is significant (the controller
// supplies them in discovery order); Inputs are rendered in sorted-key order
// so the envelope is byte-deterministic across runs with equal inputs.
type EnvelopeInput struct {
	Role         workflow.RoleContract
	RolePrompt   string
	Task         TaskSnapshot
	Instructions []ProjectInstruction
	Inputs       map[string]any
	OutputPaths  OutputPaths
	Contract     ResultContract
}

// Result is the raw outcome of a single agent invocation. It carries the
// process-level facts the controller needs (session id, stop reason, error
// flag) and the bytes the completion check validates (result.json, artifacts).
// Result does not carry the semantic outcome; that is extracted and validated
// by the completion check.
type Result struct {
	// SessionID is the underlying agent session id; the controller persists it
	// per SessionKey so the next invocation for the same role may resume.
	SessionID string

	// StopReason is the underlying agent stop reason (e.g. end_turn, tool_use).
	StopReason string

	// IsError is true if the underlying process signalled a failure (Maki
	// `is_error=true`, non-zero exit, or unparseable output).
	IsError bool

	// ResultJSON is the raw bytes of result.json as read from OutputPaths.Result
	// after process completion. It is empty if the file is missing.
	ResultJSON []byte

	// Artifacts maps a role-declared output name to its raw bytes as read from
	// the assigned path. Missing artifacts are simply absent from the map.
	Artifacts map[string][]byte

	// Raw is the raw text/stdout from the agent for diagnostics. The completion
	// check does not inspect it.
	Raw string
}

// Completion is the validated completion of a step attempt. It is produced by
// the completion check after the adapter reports process completion.
type Completion struct {
	// Outcome is the validated semantic outcome selected by the agent. It is
	// guaranteed to be in Contract.AllowedOutcomes.
	Outcome string
}
