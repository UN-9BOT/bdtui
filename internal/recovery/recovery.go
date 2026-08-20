// Package recovery reconciles persisted executions against the live runtime
// after a daemon crash or restart and records durable checkpoints for writer
// steps. It is the bridge between orch.Execution durable state and the
// agent.Runtime process lifecycle plus a Git worktree.
//
// The package has three concerns:
//
//   - Reconcile: for every persisted execution, classify it as live / lost
//     against the runtime and apply the policy (writer ⇒ needs_attention,
//     reader ⇒ technical retry / technical failure).
//   - Checkpoint: after a writer step succeeds with a non-empty diff, create a
//     Git commit in the controller's worktree and persist the resulting HEAD
//     as result_commit. An empty diff is recorded as HEAD without an empty
//     commit so history stays clean.
//   - Reset: on the rare path that needs it, take a recovered writer back to
//     queued via orch.RequestRunRetry (the controller owns that call site).
//
// The package intentionally has no dependency on the Herdr client so it can
// be developed and tested ahead of the Herdr integration.
package recovery

import (
	"context"
	"errors"

	"bdtui/internal/agent"
)

// Sentinel errors. They are exported so the controller and tests can branch
// on them with errors.Is.
var (
	// ErrCheckpointNotGitRepo is returned when the working dir is not a Git
	// repository (or HEAD cannot be resolved). The caller should surface this
	// as a hard failure for writer steps; for reader steps it can be ignored.
	ErrCheckpointNotGitRepo = errors.New("recovery: working dir is not a Git repository")

	// ErrCheckpointDirty is returned when the working tree has local changes
	// that were not produced by the writer step. The controller should refuse
	// the checkpoint rather than silently mixing unrelated work.
	ErrCheckpointDirty = errors.New("recovery: working tree has unexpected local changes")

	// ErrCheckpointNoOp is returned when the diff against HEAD is empty. The
	// caller treats this as success but skips the empty commit and records
	// the current HEAD as result_commit.
	ErrCheckpointNoOp = errors.New("recovery: checkpoint diff is empty")
)

// Checkpoint captures the inputs and outputs of a successful writer
// checkpoint. Callers may inspect DiffEmpty / CommitSHA when the controller
// needs to record additional metadata (e.g. skip the empty-commit case).
type Checkpoint struct {
	Worktree  string // absolute path to the writer's Git worktree
	RunID     string // run id, used as part of the commit footer for traceability
	StepID    string // step id, used in the commit footer
	Summary   string // human-readable subject line for the checkpoint commit
	Body      string // optional commit body; appended after the subject
	BeforeSHA string // HEAD before the commit (empty on first commit)
	CommitSHA string // resulting commit SHA (== BeforeSHA on no-op)
	DiffEmpty bool   // true when the diff against BeforeSHA is empty
}

// Worktree abstracts the Git operations recovery needs. The concrete
// implementation in internal/recovery/gitcheckout is exercised by tests;
// callers may substitute a fake to exercise reconcile paths without a real
// repository.
type Worktree interface {
	// ResolveHead returns the current HEAD commit SHA, or
	// ErrCheckpointNotGitRepo if the working dir is not a repository.
	ResolveHead(ctx context.Context, workdir string) (string, error)

	// IsDirty reports whether the working tree has local changes vs HEAD
	// (untracked files are excluded so checkpoint metadata files left by the
	// agent do not block the commit). A dirty tree returns
	// ErrCheckpointDirty.
	IsDirty(ctx context.Context, workdir string) error

	// DiffEmpty reports whether the diff against HEAD (staged + unstaged
	// changes) is empty. It does not error on an empty repo.
	DiffEmpty(ctx context.Context, workdir string) (bool, error)

	// Commit writes a checkpoint commit with the given subject/body and
	// returns the resulting commit SHA. The commit is empty when there are
	// no changes (callers should consult DiffEmpty first to decide whether
	// to record an empty commit).
	Commit(ctx context.Context, workdir, subject, body string) (string, error)
}

// Inspect abstracts the runtime inspection/recovery probe. The agent.Runtime
// is the production implementation; tests provide a fake.
type Inspect interface {
	// Live reports whether the execution is still attached to a live
	// runtime process. The runtime should return ErrLostExecution if it has
	// no record of the execution (post-restart crash).
	Live(ctx context.Context, executionID string) (LiveState, error)
}

// LiveState is the result of Inspect.Live.
type LiveState int

const (
	// LiveUnknown indicates the inspect probe could not determine state.
	// Callers should treat this as ambiguous (conservative: needs_attention
	// for writer, retry once for reader).
	LiveUnknown LiveState = iota
	// LiveRunning indicates the runtime still has the execution alive.
	LiveRunning
	// LiveDone indicates the runtime finished the execution and the result
	// is durable on disk.
	LiveDone
)

// Kind classifies an execution for the writer-safety policy. Only KindWriter
// triggers needs_attention on lost executions; other kinds may proceed with
// technical retry.
type Kind int

const (
	KindReader Kind = iota
	KindWriter
)

// Execution is the minimal slice of orch.Execution recovery needs to make a
// policy decision. The controller projects orch.Execution onto this type so
// recovery has no direct database dependency.
type Execution struct {
	ID         string
	RunID      string
	StepID     string
	Status     string // string form of orch.ExecutionStatus; recovery stays provider-agnostic
	Kind       Kind
	PaneID     string // optional, only used for observability / future Herdr integration
	ProcessID  string // optional
	PromptRef  string // controller-assigned path in run storage
	ResultJSON string // controller-assigned path in run storage
}

// Decision is the recovery policy decision for one persisted execution.
type Decision struct {
	ExecutionID string
	Kind        Kind
	Live        LiveState
	// Action is the recommended next step.
	Action Action
	// Reason is a short stable identifier (e.g. "live", "lost_writer",
	// "lost_reader") used in audit logs and tests.
	Reason string
}

// Action is the recommended recovery action.
type Action int

const (
	// ActionNone means the runtime already produced a durable result; the
	// controller should re-attach and complete the step attempt.
	ActionNone Action = iota
	// ActionReattach means the runtime is still running and the controller
	// should re-attach (Inspect + Wait) and resume the step attempt.
	ActionReattach
	// ActionNeedsAttention means a writer execution is lost or ambiguous;
	// the run must enter needs_attention until the user decides.
	ActionNeedsAttention
	// ActionRetry means a reader execution is lost or ambiguous and a
	// technical retry is allowed.
	ActionRetry
)

// Decide reconciles a persisted execution against the live runtime state. The
// rules (per BIR-50):
//
//   - Live running  → ActionReattach (controller picks up the live process).
//   - Live done     → ActionNone      (durable result already on disk).
//   - Lost / unknown + KindWriter → ActionNeedsAttention.
//   - Lost / unknown + KindReader → ActionRetry.
//
// The function is pure and free of side effects; the controller is responsible
// for translating Action into store transitions.
func Decide(exec Execution, live LiveState, inspectErr error) Decision {
	d := Decision{ExecutionID: exec.ID, Kind: exec.Kind, Live: live}
	switch {
	case inspectErr == nil && live == LiveRunning:
		d.Action = ActionReattach
		d.Reason = "live_running"
	case inspectErr == nil && live == LiveDone:
		d.Action = ActionNone
		d.Reason = "live_done"
	case errors.Is(inspectErr, agent.ErrLostExecution):
		d.Action = writerNeedsAttentionOrRetry(exec.Kind)
		d.Reason = "lost"
	default:
		// Conservative: an ambiguous state (e.g. runtime probe errored with
		// something other than ErrLostExecution) is treated like "lost". A
		// writer still requires human attention; a reader may retry once.
		d.Action = writerNeedsAttentionOrRetry(exec.Kind)
		d.Reason = "ambiguous"
	}
	return d
}

func writerNeedsAttentionOrRetry(k Kind) Action {
	if k == KindWriter {
		return ActionNeedsAttention
	}
	return ActionRetry
}
