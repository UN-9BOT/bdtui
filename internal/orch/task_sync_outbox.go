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

// suppress unused warnings if the build flags change; the sql.ErrNoRows
// import is used by callers that classify the error.
var _ = sql.ErrNoRows
