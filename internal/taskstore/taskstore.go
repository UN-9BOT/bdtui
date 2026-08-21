// Package taskstore defines the orchestrator's narrow boundary to the
// project's task / Kanban backend.
//
// The TaskStore is the canonical source of the high-level task lifecycle
// (todo / in_progress / done / blocked). It is also responsible for the
// atomic claim that starts a Run: calling Claim snapshots the task and
// transitions it to in_progress in a single observable step, and the
// returned Task is treated as a frozen snapshot for the lifetime of the Run
// regardless of subsequent edits in the backend.
//
// The TaskStore is intentionally narrow. It does NOT encode workflow stage,
// current step, queue position, or any other per-Run state. Those live in
// the orchestrator's SQLite store (see internal/orch). Anything that varies
// during a Run is the controller's responsibility, not the TaskStore's.
package taskstore

import (
	"context"
	"errors"
	"time"
)

// TaskStatus is the high-level lifecycle surfaced by the TaskStore. Only
// these four values are allowed; Beads-specific raw values (open, closed)
// are mapped to one of these by the adapter.
type TaskStatus string

const (
	TaskTodo       TaskStatus = "todo"
	TaskInProgress TaskStatus = "in_progress"
	TaskDone       TaskStatus = "done"
	TaskBlocked    TaskStatus = "blocked"
)

// Valid reports whether s is one of the defined TaskStatus values.
func (s TaskStatus) Valid() bool {
	switch s {
	case TaskTodo, TaskInProgress, TaskDone, TaskBlocked:
		return true
	default:
		return false
	}
}

// RunOutcome is the controller's terminal classification of a Run. The
// TaskStore maps it back to a TaskStatus per MapRunOutcomeToTaskStatus.
type RunOutcome string

const (
	RunCompleted      RunOutcome = "completed"
	RunFailed         RunOutcome = "failed"
	RunNeedsAttention RunOutcome = "needs_attention"
	RunCancelled      RunOutcome = "cancelled"
)

// Valid reports whether o is one of the defined RunOutcome values.
func (o RunOutcome) Valid() bool {
	switch o {
	case RunCompleted, RunFailed, RunNeedsAttention, RunCancelled:
		return true
	default:
		return false
	}
}

// MapRunOutcomeToTaskStatus applies the canonical lifecycle mapping:
//
//	completed        -> done
//	failed           -> blocked
//	needs_attention  -> blocked
//	cancelled        -> todo
//
// The mapping is fixed by the TaskStore contract so the controller does not
// have to know about TaskStatus conversion.
func MapRunOutcomeToTaskStatus(o RunOutcome) (TaskStatus, error) {
	switch o {
	case RunCompleted:
		return TaskDone, nil
	case RunFailed, RunNeedsAttention:
		return TaskBlocked, nil
	case RunCancelled:
		return TaskTodo, nil
	default:
		return "", ErrInvalidOutcome
	}
}

// Task is the immutable snapshot of a TaskStore task at the moment it was
// resolved. The controller treats the fields below as frozen for the
// duration of the Run they back; external edits in the backend MUST NOT
// mutate this struct.
type Task struct {
	ID          string
	Title       string
	Description string
	Status      TaskStatus
	Priority    int
	IssueType   string
	// SnapshotAt is the wall-clock time the TaskStore froze the fields above.
	SnapshotAt time.Time
}

// Clone returns a deep copy of t. Callers that retain a Task past the
// controller's lifetime for caching or audit purposes should clone rather
// than share the pointer.
func (t *Task) Clone() *Task {
	if t == nil {
		return nil
	}
	cp := *t
	return &cp
}

// TaskStore is the orchestrator's bound to the task / Kanban backend.
//
// Contract:
//   - Get returns ErrTaskNotFound when the task is absent.
//   - Claim atomically snapshots the task and transitions it to
//     in_progress. The returned Task is the post-claim snapshot and MUST
//     be treated as immutable for the lifetime of the Run it backs.
//     "Atomic" here means race-free with respect to the backend: two
//     concurrent Claim calls on the same task cannot both observe a
//     free task. The CLI implementation is idempotent for the same
//     holder (the ORCH daemon's identity), so Claim returns the
//     in_progress snapshot without error in that case. The "already
//     claimed by another holder" failure mode is mapped to
//     ErrTaskAlreadyClaimed. The single-active-Run invariant ("at most
//     one Run per task") is enforced at the orchestrator's CreateRun
//     layer via the partial unique index, so the TaskStore cannot
//     itself produce that inequality — it can only refuse to proceed
//     when the BACKEND reports a foreign holder.
//   - SyncTerminal pushes the controller's terminal Run classification
//     back to the task using the mapping in MapRunOutcomeToTaskStatus.
//   - All methods MUST return ErrTaskStoreUnavailable when the backend
//     is unreachable so the controller can refuse to launch a Run.
//
// The "atomic claim" semantic is what makes the orchestrator's spec
// promise "if Beads is unavailable, Run launch must not start" possible:
// when the backend is reachable, the post-claim snapshot is congruent
// with the backend state regardless of how many concurrent calls
// attempt to claim the same task.
type TaskStore interface {
	Get(ctx context.Context, id string) (*Task, error)
	Claim(ctx context.Context, id string) (*Task, error)
	SyncTerminal(ctx context.Context, id string, outcome RunOutcome) error
}

// Sentinel errors. Adapters MUST wrap these with %w so callers can use
// errors.Is for routing.
var (
	// ErrTaskNotFound is returned when the requested task does not exist
	// in the backend.
	ErrTaskNotFound = errors.New("taskstore: task not found")
	// ErrTaskAlreadyClaimed is returned by Claim when the task is already
	// in_progress and the controller cannot take ownership.
	ErrTaskAlreadyClaimed = errors.New("taskstore: task already claimed")
	// ErrTaskStoreUnavailable is returned when the backend is unreachable
	// or returned an error not classifiable as ErrTaskNotFound.
	ErrTaskStoreUnavailable = errors.New("taskstore: task store unavailable")
	// ErrInvalidOutcome is returned when SyncTerminal receives an outcome
	// that has no defined TaskStatus mapping.
	ErrInvalidOutcome = errors.New("taskstore: invalid run outcome")
)
