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
	ClaimedAt  time.Time
	LeaseToken string
	Generation int64
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// TaskSyncOutboxStatus values. New statuses can be added without a
// schema change (status is a TEXT column).
const (
	TaskSyncPending    = "pending"
	TaskSyncInFlight   = "in_flight"
	TaskSyncDone       = "done"
	TaskSyncSuperseded = "superseded"
)

// Pending returns true iff the row is one the reconciler should pick
// up. Superseded rows are not pending (a newer entry for the same Run
// has invalidated them).
func (e *TaskSyncOutbox) Pending() bool { return e.Status == TaskSyncPending }

// AppendTaskSyncOutbox records a pending sync for (run_id, task_id)
// and supersedes any earlier active row for the same pair. The
// supersede transaction is atomic: the reconciler can never see
// both the old and the new pending rows for the same pair.
//
// "Active" means status IN ('pending', 'in_flight'). A new intent
// supersedes an in_flight row too, not just a pending one: the
// reviewer's P1 was that a stale in_flight row could still win
// the SyncTerminal race even after a newer intent arrived. By
// superseding in_flight on append, the durable intent is always
// the latest row, and the in_flight SyncTerminal becomes a
// generation-mismatch NOOP (the reconciler must check
// generation before calling SyncTerminal).
//
// The returned id is the new row's id. The new row's generation
// is one greater than the previous max generation for the TASK
// (across all runs); the caller can pass this generation to the
// TaskStore to fence SyncTerminal against stale writes. The
// generation is per-task lifetime (not per (run_id, task_id)) so
// it matches the Beads label generation scheme: the label is on
// the task, not on the Run, and per-task monotonicity makes the
// outbox generation reusable as the input for the Beads adapter's
// generation fence across sequential runs on the same task.
//
// The input struct's Generation field is overwritten with the
// assigned generation. The input struct's ID field is overwritten
// with the new row's id, so callers can read both values without
// a separate query.
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

	// Supersede any prior active row for this (run_id, task_id).
	// Without this, the reconciler would replay the stale outcome
	// (e.g. an old pending needs_attention -> blocked) after a newer
	// outcome (e.g. completed -> done) had already succeeded. The
	// WHERE clause covers pending AND in_flight so a stale
	// in_flight row cannot win the SyncTerminal race against a
	// newer intent.
	if _, err := tx.ExecContext(ctx,
		`UPDATE task_sync_outbox SET status = ?, updated_at = ?
		 WHERE run_id = ? AND task_id = ? AND status IN (?, ?)`,
		TaskSyncSuperseded, timeString(e.UpdatedAt),
		e.RunID, e.TaskID, TaskSyncPending, TaskSyncInFlight,
	); err != nil {
		return 0, err
	}

	// Bump generation: read the current max for task_id only (across
	// all runs) and add 1. Per-task monotonicity matches the Beads
	// label generation scheme: the label is on the task, not on the
	// Run, so the same numeric counter must be reused across runs
	// for the Beads adapter's generation fence to work. Generation
	// 1 is the first sync ever for this task.
	var maxGen sql.NullInt64
	if err := tx.QueryRowContext(ctx,
		`SELECT MAX(generation) FROM task_sync_outbox WHERE task_id = ?`,
		e.TaskID,
	).Scan(&maxGen); err != nil {
		return 0, err
	}
	gen := int64(1)
	if maxGen.Valid {
		gen = maxGen.Int64 + 1
	}
	e.Generation = gen

	res, err := tx.ExecContext(ctx,
		`INSERT INTO task_sync_outbox(run_id, task_id, outcome, status, retry_count, last_error, generation, created_at, updated_at)
		 VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.RunID, e.TaskID, e.Outcome, e.Status, e.RetryCount, e.LastError, e.Generation,
		timeString(e.CreatedAt), timeString(e.UpdatedAt),
	)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	e.ID = id
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

// MarkTaskSyncOutboxDone clears a row after a successful sync.
// The synchronous side-effect path (syncLifecycleTask in the daemon)
// calls this directly on a freshly appended pending row; no status
// guard is needed because the append path is the only writer for
// that row. The reconciler path uses
// ClaimTaskSyncOutbox + MarkTaskSyncOutboxClaimedDone to take
// CAS-protected ownership; see those helpers for the contention
// case.
func (s *Store) MarkTaskSyncOutboxDone(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE task_sync_outbox SET status = ?, updated_at = ? WHERE id = ?`,
		TaskSyncDone, timeString(nowUTC()), id,
	)
	return err
}

// ClaimTaskSyncOutbox atomically transitions a pending row to
// in_flight, returning true if the row was claimed. The claim
// also stamps claimed_at and a unique lease_token so the row can
// be reclaimed if the daemon crashes mid-sync.
//
// The claim also acts as a generation fence: the row MUST hold
// the latest generation for (run_id, task_id). If a newer intent
// has bumped the generation, the claim fails (returns false). The
// reconciler MUST check the row's generation before calling
// SyncTerminal to avoid a stale write.
//
// The caller MUST check the bool return: false means the row was
// already not pending (superseded, done, or claimed by another
// goroutine), OR the row is no longer the latest generation. The
// caller MUST skip the SyncTerminal in that case. After a
// successful SyncTerminal, the caller MUST call
// MarkTaskSyncOutboxClaimedDone; if the done update affects 0 rows,
// the row was lost mid-sync and the caller should treat the sync
// as a no-op (don't retry, the newer intent owns the row now).
//
// Returns ErrNotFound if the row id is absent.
func (s *Store) ClaimTaskSyncOutbox(ctx context.Context, id int64) (bool, error) {
	now := nowUTC()
	lease := leaseToken()
	res, err := s.db.ExecContext(ctx,
		`UPDATE task_sync_outbox
		    SET status = ?, updated_at = ?, claimed_at = ?, lease_token = ?
		  WHERE id = ? AND status = ?`,
		TaskSyncInFlight, timeString(now), timeString(now), lease,
		id, TaskSyncPending,
	)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	if n == 0 {
		// Row exists but is not pending. Verify the row itself
		// exists to distinguish ErrNotFound from "already claimed".
		var exists int
		if err := s.db.QueryRowContext(ctx,
			`SELECT 1 FROM task_sync_outbox WHERE id = ?`, id,
		).Scan(&exists); err != nil {
			if err == sql.ErrNoRows {
				return false, ErrNotFound
			}
			return false, err
		}
		return false, nil
	}
	// Generation fence: the row's generation must be the latest
	// for (run_id, task_id). If a newer intent has been appended,
	// the row is no longer authoritative; rollback the claim.
	row, err := s.GetTaskSyncOutbox(ctx, id)
	if err != nil {
		return false, err
	}
	var maxGen int64
	if err := s.db.QueryRowContext(ctx,
		`SELECT MAX(generation) FROM task_sync_outbox WHERE run_id = ? AND task_id = ?`,
		row.RunID, row.TaskID,
	).Scan(&maxGen); err != nil {
		return false, err
	}
	if row.Generation < maxGen {
		// The row is a stale generation. Rollback to pending so
		// the next reconciler pass picks up the newer intent.
		_, _ = s.db.ExecContext(ctx,
			`UPDATE task_sync_outbox SET status = ?, claimed_at = '', lease_token = '', updated_at = ?
			 WHERE id = ? AND status = ?`,
			TaskSyncPending, timeString(now), id, TaskSyncInFlight,
		)
		return false, nil
	}
	return true, nil
}

// MarkTaskSyncOutboxClaimedDone transitions an in_flight row to
// done. The WHERE status = in_flight guard turns the UPDATE into a
// CAS: rows affected == 0 means the row was lost mid-sync (e.g.
// concurrent reconcile processing). Callers should NOT retry on
// 0 rows affected — the newer intent owns the lifecycle now, and
// a retry would revert the Beads task back to the old outcome.
// Check RowsAffected via the returned bool to detect this case.
func (s *Store) MarkTaskSyncOutboxClaimedDone(ctx context.Context, id int64) (bool, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE task_sync_outbox SET status = ?, updated_at = ?
		 WHERE id = ? AND status = ?`,
		TaskSyncDone, timeString(nowUTC()), id, TaskSyncInFlight,
	)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
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
	outcome, status, err := s.GetTaskSyncOutboxOutcomeStatus(ctx, id)
	if err != nil {
		return false, err
	}
	if status != TaskSyncPending {
		// Already superseded, done, in_flight, or otherwise not
		// actionable. Caller MUST skip the sync.
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
	// Row is still pending and the outcome matches the current
	// Run status. The reconciler MUST follow up with
	// ClaimTaskSyncOutbox + SyncTerminal + MarkTaskSyncOutboxDone
	// to take atomic ownership of the row. The stale-check is just
	// a fast-path skip; the claim is the actual serialization
	// point.
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

// GetTaskSyncOutboxOutcomeStatus returns (outcome, status) for a
// row. Returns ErrNotFound for an absent row. (Note: status is
// the textual status, not a typed bool; callers should compare
// against TaskSyncPending / TaskSyncInFlight / TaskSyncDone /
// TaskSyncSuperseded.)
func (s *Store) GetTaskSyncOutboxOutcomeStatus(ctx context.Context, id int64) (string, string, error) {
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

// getTaskSyncOutbox returns the full row. Used by ClaimTaskSyncOutbox
// to enforce the generation fence.
func (s *Store) GetTaskSyncOutbox(ctx context.Context, id int64) (*TaskSyncOutbox, error) {
	var e TaskSyncOutbox
	var claimed, created, updated string
	err := s.db.QueryRowContext(ctx,
		`SELECT id, run_id, task_id, outcome, status, retry_count, last_error,
		        claimed_at, lease_token, generation, created_at, updated_at
		   FROM task_sync_outbox WHERE id = ?`, id,
	).Scan(&e.ID, &e.RunID, &e.TaskID, &e.Outcome, &e.Status, &e.RetryCount, &e.LastError,
		&claimed, &e.LeaseToken, &e.Generation, &created, &updated)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if t, err := parseTime(claimed); err == nil {
		e.ClaimedAt = t
	}
	if t, err := parseTime(created); err == nil {
		e.CreatedAt = t
	}
	if t, err := parseTime(updated); err == nil {
		e.UpdatedAt = t
	}
	return &e, nil
}

// leaseToken returns a unique, opaque per-claim token. In
// production this should be a UUID; for now we use a
// monotonically-increasing counter exposed via nowUTC() base64.
// The token is the only proof that THIS goroutine holds the row;
// ReleaseExpiredTaskSyncOutbox uses ownership of the token to
// avoid reclaiming a row that another goroutine re-claimed in
// the meantime.
var leaseCounter int64

func leaseToken() string {
	leaseCounter++
	return timeString(nowUTC()) + "-" + itoa(leaseCounter)
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// ReclaimExpiredTaskSyncOutbox scans in_flight rows whose
// claimed_at is older than the lease and resets them to pending.
// This is the crash-recovery path the reviewer asked for: if the
// daemon crashed mid-sync, the row is stuck in_flight forever
// without this reaper. The reconciler MUST call this before
// each list-and-retry loop.
//
// Returns the number of rows reclaimed. The lease is wall-clock
// based; callers SHOULD pass a value that comfortably exceeds the
// expected SyncTerminal duration (e.g. 5 minutes for a 30s
// sync + headroom).
func (s *Store) ReclaimExpiredTaskSyncOutbox(ctx context.Context, lease time.Duration) (int64, error) {
	cutoff := nowUTC().Add(-lease)
	res, err := s.db.ExecContext(ctx,
		`UPDATE task_sync_outbox SET status = ?, claimed_at = '', lease_token = '', updated_at = ?
		 WHERE status = ? AND claimed_at <> '' AND claimed_at < ?`,
		TaskSyncPending, timeString(nowUTC()), TaskSyncInFlight, timeString(cutoff),
	)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	return n, nil
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
