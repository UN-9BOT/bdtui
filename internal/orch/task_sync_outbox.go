package orch

import (
	"context"
	"database/sql"
	"time"
)

// TaskSyncOutbox is the durable record of a failed TaskStore sync
// against a particular Run. The Run status is the source of truth for
// the lifecycle; the outbox records "this Run still owes a sync to
// Beads" so the future controller reconciler can act on it.
//
// The Outcome field is the canonical Run classification (the same
// enum the TaskStore consumes), so the reconciler can replay the
// sync by calling TaskStore.SyncTerminal without re-deriving the
// mapping from orch.RunStatus.
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
// schema change.
const (
	TaskSyncPending = "pending"
	TaskSyncDone    = "done"
)

// AppendTaskSyncOutbox records a pending sync. The same Run may have
// multiple rows over its lifetime (e.g. a retry that failed again),
// so we do not enforce a unique constraint on RunID. The reconciler
// queries by status='pending' so only the latest pending row matters.
func (s *Store) AppendTaskSyncOutbox(ctx context.Context, e *TaskSyncOutbox) error {
	if e.Status == "" {
		e.Status = TaskSyncPending
	}
	if e.CreatedAt.IsZero() {
		e.CreatedAt = nowUTC()
	}
	e.UpdatedAt = nowUTC()

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO task_sync_outbox(run_id, task_id, outcome, status, retry_count, last_error, created_at, updated_at)
		 VALUES(?, ?, ?, ?, ?, ?, ?, ?)`,
		e.RunID, e.TaskID, e.Outcome, e.Status, e.RetryCount, e.LastError,
		timeString(e.CreatedAt), timeString(e.UpdatedAt),
	)
	return err
}

// ListPendingTaskSyncOutbox returns pending sync rows, oldest first.
// The reconciler consumes this list and either marks rows done or
// retries them.
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
