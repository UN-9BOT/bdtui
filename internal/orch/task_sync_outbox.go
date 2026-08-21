package orch

import (
	"context"
	"database/sql"
	"time"
)

// TaskSyncOutbox is the durable record of a pending TaskStore sync
// against a particular Run. The Run status is the source of truth for
// the lifecycle; the outbox records "this Run still owes a sync to
// Beads" so the future controller reconciler can act on it.
//
// The Outcome field is the canonical Run classification (the same
// enum the TaskStore consumes), so the reconciler can replay the
// sync by calling TaskStore.SyncTerminal without re-deriving the
// mapping from orch.RunStatus.
//
// The outbox models the "current desired state" for a Run's Beads
// sync: only one pending row per (run_id, task_id) is durable, and
// newer supersede rows automatically invalidate older ones. The
// reconciler queries by status='pending' and skips 'superseded' rows.
type TaskSyncOutbox struct {
	ID         int64
	RunID      string
	TaskID     string
	Outcome    string
	Status     string
	RetryCount int
	LastError  string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// TaskSyncOutboxStatus values. New statuses can be added without a
// schema change (status is a TEXT column).
const (
	TaskSyncPending    = "pending"
	TaskSyncDone       = "done"
	TaskSyncSuperseded = "superseded"
)

// Pending returns true iff the row is one the reconciler should pick
// up. Superseded rows are not pending (a newer entry for the same Run
// has invalidated them).
func (e *TaskSyncOutbox) Pending() bool { return e.Status == TaskSyncPending }

// AppendTaskSyncOutbox records a pending sync for (run_id, task_id)
// and supersedes any earlier pending row for the same pair. The
// supersede transaction is atomic: the reconciler can never see
// both the old and the new pending rows for the same pair.
//
// The returned id is the new row's id; the caller passes it to
// MarkTaskSyncOutboxDone on a successful sync. The earlier pending
// row's id is irrelevant because it is now superseded.
func (s *Store) AppendTaskSyncOutbox(ctx context.Context, e *TaskSyncOutbox) (int64, error) {
	if e.Status == "" {
		e.Status = TaskSyncPending
	}
	if e.CreatedAt.IsZero() {
		e.CreatedAt = nowUTC()
	}
	e.UpdatedAt = nowUTC()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	// Supersede any prior pending row for this (run_id, task_id).
	// Without this, the reconciler would replay the stale outcome
	// (e.g. an old pending needs_attention -> blocked) after a newer
	// outcome (e.g. completed -> done) had already succeeded.
	if _, err := tx.ExecContext(ctx,
		`UPDATE task_sync_outbox SET status = ?, updated_at = ?
		 WHERE run_id = ? AND task_id = ? AND status = ?`,
		TaskSyncSuperseded, timeString(e.UpdatedAt), e.RunID, e.TaskID, TaskSyncPending,
	); err != nil {
		return 0, err
	}

	res, err := tx.ExecContext(ctx,
		`INSERT INTO task_sync_outbox(run_id, task_id, outcome, status, retry_count, last_error, created_at, updated_at)
		 VALUES(?, ?, ?, ?, ?, ?, ?, ?)`,
		e.RunID, e.TaskID, e.Outcome, e.Status, e.RetryCount, e.LastError,
		timeString(e.CreatedAt), timeString(e.UpdatedAt),
	)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return id, nil
}

// ListPendingTaskSyncOutbox returns pending sync rows, oldest first.
// The reconciler consumes this list and either marks rows done or
// retries them. Superseded rows are NOT included — the supersede
// happens at insert time, so the reconciler never sees stale entries.
func (s *Store) ListPendingTaskSyncOutbox(ctx context.Context) ([]TaskSyncOutbox, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, run_id, task_id, outcome, status, retry_count, last_error, created_at, updated_at
		 FROM task_sync_outbox WHERE status = ? ORDER BY id ASC`,
		TaskSyncPending)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []TaskSyncOutbox
	for rows.Next() {
		var e TaskSyncOutbox
		var created, updated string
		if err := rows.Scan(&e.ID, &e.RunID, &e.TaskID, &e.Outcome, &e.Status, &e.RetryCount, &e.LastError, &created, &updated); err != nil {
			return nil, err
		}
		if t, err := parseTime(created); err == nil {
			e.CreatedAt = t
		}
		if t, err := parseTime(updated); err == nil {
			e.UpdatedAt = t
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// MarkTaskSyncOutboxDone clears a pending row after a successful retry.
// Once the row is 'done', the Run no longer owes a sync.
func (s *Store) MarkTaskSyncOutboxDone(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE task_sync_outbox SET status = ?, updated_at = ? WHERE id = ?`,
		TaskSyncDone, timeString(nowUTC()), id,
	)
	return err
}

// IncrementTaskSyncOutboxRetry bumps the retry counter and updates the
// stored error after a failed retry. The reconciler calls this with
// the new error message returned by the TaskStore.
func (s *Store) IncrementTaskSyncOutboxRetry(ctx context.Context, id int64, errMsg string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE task_sync_outbox SET retry_count = retry_count + 1, last_error = ?, updated_at = ? WHERE id = ?`,
		errMsg, timeString(nowUTC()), id,
	)
	return err
}

// RunOutcomeString is the canonical Run classification used by the
// outbox. The literal values match the TaskStore's RunOutcome enum
// so the reconciler can pass them straight back to
// TaskStore.SyncTerminal.
type RunOutcomeString string

const (
	// RunOutcomeString constants. The values are the same as the
	// taskstore RunOutcome enum; they are mirrored here to avoid
	// an orch -> taskstore import cycle.
	RunOutcomeCompleted       RunOutcomeString = "completed"
	RunOutcomeFailed          RunOutcomeString = "failed"
	RunOutcomeNeedsAttention  RunOutcomeString = "needs_attention"
	RunOutcomeCancelled       RunOutcomeString = "cancelled"
)

// outcomeForRunStatus returns the canonical outcome for a Run
// status. Non-terminal statuses (queued / running / waiting_human)
// have no mapping and return ErrNotFound so the caller can mark
// the outbox row as stale.
func outcomeForRunStatus(s RunStatus) (RunOutcomeString, error) {
	switch s {
	case RunCompleted:
		return RunOutcomeCompleted, nil
	case RunFailed:
		return RunOutcomeFailed, nil
	case RunNeedsAttention:
		return RunOutcomeNeedsAttention, nil
	case RunCancelled:
		return RunOutcomeCancelled, nil
	default:
		return "", ErrNotFound
	}
}

// MarkTaskSyncOutboxSupersededIfStale atomically decides whether a
// pending outbox row is still the authoritative current desired
// state for (run_id, task_id) and, if it is not, supersedes it.
// The reconciler MUST call this before retrying the sync: a row
// that has been superseded (by a newer intent), or whose outcome no
// longer matches the current Run status, must NOT be replayed — the
// stale SyncTerminal would revert the Beads task back to the old
// blocked/in_progress state.
//
// A row is "no longer actionable" (returns true) in four cases:
//   - the row was already superseded (by a newer intent) between
//     ListPending and this call — defended by up-front status read
//     of any current status != pending;
//   - the row's outcome does not match the current Run status —
//     it's superseded by THIS call;
//   - Run status has no canonical outcome (non-terminal) — the
//     pending row is stale by construction, superseded by THIS call;
//   - the row is still pending AND matches the Run status, but a
//     NEWER pending row has been appended for the same (run_id,
//     task_id) — the newer row is the authoritative current desired
//     state. This is the second TOCTOU window: between the
//     ListPending snapshot and the actual sync, a newer intent may
//     be appended. Even if THIS row is still pending and matches,
//     it is now legacy and replaying it would race the newer
//     SyncTerminal. Marker defense: before returning "retry",
//     verify THIS id is still the latest pending row for the pair.
//
// "Latest pending" is defined by ROWID order: the append sequence
// assigns monotonically increasing IDs, so the largest id is the
// newest pending row. (status, id) is the natural ordering; we
// filter by status = pending because superseded rows must not
// shadow the latest pending one.
//
// Returns true if the row is no longer actionable (caller should
// skip sync), false if the row is still actionable (caller should
// retry the sync).
func (s *Store) MarkTaskSyncOutboxSupersededIfStale(ctx context.Context, id int64, run *Run) (bool, error) {
	outcome, status, err := s.getTaskSyncOutboxOutcomeStatus(ctx, id)
	if err != nil {
		return false, err
	}
	if status != TaskSyncPending {
		// Already superseded, done, or otherwise not actionable.
		// Caller MUST skip the sync.
		return true, nil
	}
	expected, err := outcomeForRunStatus(run.Status)
	if err != nil {
		// Run status has no mapping (non-terminal): the pending row
		// is stale by construction.
		return markSupersededByID(ctx, s, id)
	}
	if string(expected) != outcome {
		// Outcome does not match the current Run status; the row
		// is stale.
		return markSupersededByID(ctx, s, id)
	}
	// Outcome matches and the row is still pending, but a NEWER
	// pending intent may have been appended since the reconciler
	// listed pending rows. Replay of this row would overwrite the
	// newer SyncTerminal's side effect. Reject in that case.
	latest, err := s.latestTaskSyncOutboxIDForRunTask(ctx, run.ID, run.TaskID)
	if err != nil {
		return false, err
	}
	if latest != id {
		// A newer pending row exists; this row is legacy. Supersede
		// it so the reconciler can advance.
		return markSupersededByID(ctx, s, id)
	}
	// Row is still pending, matches the current Run status, AND
	// is the latest pending row for (run_id, task_id). Retry the
	// sync.
	return false, nil
}

// GetTaskSyncOutboxOutcome returns the recorded outcome for a
// pending outbox row. Returns ErrNotFound for an absent row.
func (s *Store) GetTaskSyncOutboxOutcome(ctx context.Context, id int64) (string, error) {
	var outcome, status string
	err := s.db.QueryRowContext(ctx,
		`SELECT outcome, status FROM task_sync_outbox WHERE id = ?`, id,
	).Scan(&outcome, &status)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", ErrNotFound
		}
		return "", err
	}
	return outcome, nil
}

// getTaskSyncOutboxOutcomeStatus returns (outcome, status) for a
// row. Atomic read; defends the TOCTOU race in
// MarkTaskSyncOutboxSupersededIfStale where the row's status may
// change between ListPending and the stale-check.
func (s *Store) getTaskSyncOutboxOutcomeStatus(ctx context.Context, id int64) (string, string, error) {
	var outcome, status string
	err := s.db.QueryRowContext(ctx,
		`SELECT outcome, status FROM task_sync_outbox WHERE id = ?`, id,
	).Scan(&outcome, &status)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", "", ErrNotFound
		}
		return "", "", err
	}
	return outcome, status, nil
}

// latestTaskSyncOutboxIDForRunTask returns the largest id of a
// pending outbox row for (run_id, task_id), or 0 if none pending.
// Used by MarkTaskSyncOutboxSupersededIfStale to detect the
// "newer intent arrived between ListPending and stale-check"
// window: if THIS row's id is not the latest, a newer pending
// row exists and is the authoritative current desired state.
func (s *Store) latestTaskSyncOutboxIDForRunTask(ctx context.Context, runID, taskID string) (int64, error) {
	var latest sql.NullInt64
	err := s.db.QueryRowContext(ctx,
		`SELECT MAX(id) FROM task_sync_outbox
		 WHERE run_id = ? AND task_id = ? AND status = ?`,
		runID, taskID, TaskSyncPending,
	).Scan(&latest)
	if err != nil {
		return 0, err
	}
	if !latest.Valid {
		return 0, nil
	}
	return latest.Int64, nil
}

// markSupersededByID transitions a pending row to superseded.
// Returns true if the row was transitioned (RowsAffected > 0),
// false otherwise. The WHERE status = pending guard is what makes
// the transition atomic against a concurrent supersede; the caller
// (MarkTaskSyncOutboxSupersededIfStale) inspects the row's status
// up-front to ensure this UPDATE is only called when the row is
// still actionable.
func markSupersededByID(ctx context.Context, s *Store, id int64) (bool, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE task_sync_outbox SET status = ?, updated_at = ?
		 WHERE id = ? AND status = ?`,
		TaskSyncSuperseded, timeString(nowUTC()), id, TaskSyncPending,
	)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// suppress unused warnings if the build flags change; the sql.ErrNoRows
// import is used by callers that classify the error.
var _ = sql.ErrNoRows
