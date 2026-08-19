// Package agent defines the generic, provider-agnostic boundary between the
// orchestrator controller and a concrete agent runner (the MVP being Maki),
// plus the deterministic prompt envelope and the completion checks that gate a
// step attempt on machine-verifiable evidence.
//
// The boundary has three layers so a future runtime (HerdrRuntime) can own the
// process lifecycle without changing the controller or the adapter:
//
//	Controller -> RunAgent(adapter, runtime, req)
//	             |                       |
//	          MakiAdapter             Runtime (HerdrRuntime / ExecRuntime)
//	             |                       |
//	      build invocation        spawn / wait / stop / reattach
//	      parse wire format       (durable ExecutionID)
//
// Workflow authoring is human-driven: this package only reads and validates
// definitions; it never creates or edits them. Agents do not create or edit
// workflows either; they execute a fully rendered prompt and produce a
// controller-assigned result.json plus any declared artifacts.
package agent

import "bdtui/internal/workflow"

// Request is a provider-agnostic request to run a single agent step. The
// controller builds the deterministic prompt envelope and assigns the output
// paths; the runtime owns the process lifecycle; the adapter only translates
// the request into the underlying CLI/protocol.
type Request struct {
	// ExecutionID is the durable runtime-side identity for this attempt. The
	// runtime uses it to spawn/inspect/reattach/stop the underlying process.
	// Empty means "allocate one". Pass an existing ID on reattach after a
	// daemon restart.
	ExecutionID string

	// SessionKey identifies a reusable agent session. MVP keeps one session
	// per (run, role) so revise/review loops for the same role may reuse the
	// agent's conversation context. The adapter maps this to the underlying
	// agent protocol's resume primitive.
	SessionKey string

	// Prompt is the deterministic, fully-rendered prompt envelope.
	Prompt string

	// WorkingDir is the Git worktree the agent operates in.
	WorkingDir string

	// OutputPaths are controller-assigned absolute paths inside Run storage
	// where the agent must write its structured result and declared artifacts.
	OutputPaths OutputPaths

	// Contract describes the shape of the result `data` and the control-plane
	// invariants the completion check enforces. DeclaredOutputs is the
	// resolved set of role-declared artifact names; BuildEnvelope rejects if
	// any of them is missing a controller-assigned path.
	Contract ResultContract
}

// OutputPaths are the controller-assigned absolute paths inside Run storage
// where the agent must write its structured result and declared artifacts.
type OutputPaths struct {
	// Result is the absolute path of result.json.
	Result string

	// Artifacts maps a role-declared output name to its absolute path in Run
	// storage. Declared artifacts live outside the Git worktree and are
	// immutable per step attempt.
	Artifacts map[string]string
}

// ResultContract is the single, resolved contract for a step attempt. It
// combines the role-declared schema, the allowed control-plane outcomes, and
// the declared artifact names so the mandatory-artifact invariant cannot be
// bypassed by the caller omitting an argument.
//
// The JSON Schema validates the result `data` object only; the `outcome`
// field is a control-plane concern validated separately by the controller.
type ResultContract struct {
	// Schema is the JSON Schema for the result.json `data` object.
	Schema string

	// AllowedOutcomes is the set of semantic outcomes the role may report.
	AllowedOutcomes []string

	// DeclaredOutputs is the resolved set of role-declared output names. The
	// completion check requires every name to have a non-empty artifact.
	DeclaredOutputs []string
}

// TaskSnapshot is the immutable snapshot of the source Kanban task for a run.
type TaskSnapshot struct {
	ID          string
	Title       string
	Description string
}

// ProjectInstruction is a snapshotted project instruction file
// (AGENTS.md/CLAUDE.md/known skills where applicable).
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
// runtime/process-level facts the controller needs (session id, stop reason,
// error flag) and the bytes the completion check validates (result.json,
// artifacts). Result does not carry the semantic outcome; that is extracted
// and validated by the completion check.
type Result struct {
	// ExecutionID echoes the durable runtime-side identity for this attempt.
	ExecutionID string

	// SessionID is the underlying agent session id; the adapter persists it
	// per SessionKey so the next invocation for the same role may resume.
	SessionID string

	// StopReason is the underlying agent stop reason.
	StopReason string

	// IsError is true if the underlying process signalled a failure (agent
	// reported is_error=true, non-zero exit, or unparseable output).
	IsError bool

	// ResultJSON is the raw bytes of result.json as read from OutputPaths.Result
	// after process completion. Empty if the file is missing.
	ResultJSON []byte

	// Artifacts maps a role-declared output name to its raw bytes as read from
	// the assigned path. Missing artifacts are simply absent from the map.
	Artifacts map[string][]byte

	// Raw is the raw agent output text for diagnostics. The completion check
	// does not inspect it.
	Raw string
}

// Completion is the validated completion of a step attempt. Outcome is
// guaranteed to be in Contract.AllowedOutcomes.
type Completion struct {
	Outcome string
}