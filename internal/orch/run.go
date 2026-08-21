package orch

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"
)

// activeRunStatusesSQL is the SQL list of non-terminal run statuses.
const activeRunStatusesSQL = "('queued','running','waiting_human','needs_attention')"

func (s *Store) CreateRun(ctx context.Context, r *Run) error {
	if r.ID == "" {
		r.ID = uuid.NewString()
	}
	if r.Status == "" {
		r.Status = RunQueued
	}
	if !r.Status.Valid() {
		return errInvalidStatus(r.Status)
	}
	now := nowUTC()
	if r.CreatedAt.IsZero() {
		r.CreatedAt = now
	}
	r.UpdatedAt = now

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if r.TaskID != "" {
		var exists bool
		if err := tx.QueryRowContext(ctx,
			`SELECT EXISTS(SELECT 1 FROM runs WHERE project_id = ? AND task_id = ? AND status IN `+activeRunStatusesSQL+`)`,
			r.ProjectID, r.TaskID,
		).Scan(&exists); err != nil {
			return err
		}
		if exists {
			return ErrActiveRunExists
		}
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO runs(id, project_id, task_id, status, workflow_snapshot_ref, workflow_snapshot,
		                  task_snapshot, current_step_id, needs_attention_reason, error, created_at, updated_at, started_at, completed_at)
		 VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.ProjectID, r.TaskID, string(r.Status), r.WorkflowSnapshotRef, r.WorkflowSnapshot,
		r.TaskSnapshot,
		nullString(r.CurrentStepID), nullString(r.NeedsAttentionReason), nullString(r.Error),
		timeString(r.CreatedAt), timeString(r.UpdatedAt), timeStringPtr(r.StartedAt), timeStringPtr(r.CompletedAt),
	); err != nil {
		return err
	}

	if err := appendEventMapTx(ctx, tx, &r.ID, EventRunCreated, map[string]any{
		"run_id": r.ID, "project_id": r.ProjectID, "task_id": r.TaskID, "status": r.Status,
	}); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) GetRun(ctx context.Context, id string) (*Run, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, project_id, task_id, status, workflow_snapshot_ref, workflow_snapshot,
		        task_snapshot, current_step_id, needs_attention_reason, error, created_at, updated_at, started_at, completed_at
		 FROM runs WHERE id = ?`, id)

	r := &Run{}
	var status string
	var created, updated string
	var currentStep, reason, errStr sql.NullString
	var started, completed sql.NullString

	if err := row.Scan(&r.ID, &r.ProjectID, &r.TaskID, &status, &r.WorkflowSnapshotRef, &r.WorkflowSnapshot,
		&r.TaskSnapshot, &currentStep, &reason, &errStr, &created, &updated, &started, &completed); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	r.Status = RunStatus(status)
	r.CurrentStepID = strPtr(currentStep)
	r.NeedsAttentionReason = strPtr(reason)
	r.Error = strPtr(errStr)

	var err error
	if r.CreatedAt, err = parseTime(created); err != nil {
		return nil, err
	}
	if r.UpdatedAt, err = parseTime(updated); err != nil {
		return nil, err
	}
	if r.StartedAt, err = timePtr(started); err != nil {
		return nil, err
	}
	if r.CompletedAt, err = timePtr(completed); err != nil {
		return nil, err
	}
	return r, nil
}

func (s *Store) ListRunsByProject(ctx context.Context, projectID string) ([]Run, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id FROM runs WHERE project_id = ? ORDER BY created_at`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	runs := make([]Run, 0, len(ids))
	for _, id := range ids {
		r, err := s.GetRun(ctx, id)
		if err != nil {
			return nil, err
		}
		runs = append(runs, *r)
	}
	return runs, nil
}

// ListRuns returns every run ordered by creation time. The daemon uses this
// for the unfiltered list; callers that already have a project should prefer
// ListRunsByProject.
func (s *Store) ListRuns(ctx context.Context) ([]Run, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM runs ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	runs := make([]Run, 0, len(ids))
	for _, id := range ids {
		r, err := s.GetRun(ctx, id)
		if err != nil {
			return nil, err
		}
		runs = append(runs, *r)
	}
	return runs, nil
}

// TransitionRun atomically moves a run to `to` if that transition is legal per
// the Run state machine. It updates only lifecycle timestamps and audit state;
// metadata fields (current_step_id, needs_attention_reason, error) are managed
// by dedicated setters so a transition never silently clears them.
func (s *Store) TransitionRun(ctx context.Context, id string, to RunStatus) error {
	if !to.Valid() {
		return errInvalidStatus(to)
	}
	now := nowUTC()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var cur RunStatus
	var started sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT status, started_at FROM runs WHERE id = ?`, id).Scan(&cur, &started); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}

	if !CanTransitionRun(cur, to) {
		return ErrInvalidTransition
	}

	if to == RunRunning && !started.Valid {
		started = sql.NullString{String: timeString(now), Valid: true}
	}
	var completed any
	if to.Terminal() {
		completed = timeString(now)
	}

	res, err := tx.ExecContext(ctx,
		`UPDATE runs SET status = ?, updated_at = ?, started_at = ?, completed_at = ?
		 WHERE id = ? AND status = ?`,
		string(to), timeString(now), started, completed, id, cur,
	)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrInvalidTransition
	}

	if err := appendEventMapTx(ctx, tx, &id, EventRunTransition, map[string]any{
		"run_id": id, "from": cur, "to": to,
	}); err != nil {
		return err
	}
	return tx.Commit()
}

// RequestRunRetry re-queues a run that is in needs_attention. It is a command
// ("please retry this run"), not a bare transition: the run returns to queued
// so the controller/scheduler picks it up again. Both needs_attention_reason
// and error are cleared because they describe the now-resolved problem and
// only make sense while the run is in needs_attention.
func (s *Store) RequestRunRetry(ctx context.Context, id string) error {
	now := nowUTC()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var cur RunStatus
	if err := tx.QueryRowContext(ctx, `SELECT status FROM runs WHERE id = ?`, id).Scan(&cur); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if cur != RunNeedsAttention {
		return ErrInvalidTransition
	}

	res, err := tx.ExecContext(ctx,
		`UPDATE runs SET status = ?, needs_attention_reason = NULL, error = NULL, updated_at = ?
		 WHERE id = ? AND status = ?`,
		string(RunQueued), timeString(now), id, cur,
	)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrInvalidTransition
	}

	if err := appendEventMapTx(ctx, tx, &id, EventRunRetryRequest, map[string]any{
		"run_id": id, "from": cur, "to": RunQueued,
	}); err != nil {
		return err
	}
	return tx.Commit()
}

// SetRunCurrentStep sets (or clears, when stepID is nil) the current step of a
// run without changing its status.
func (s *Store) SetRunCurrentStep(ctx context.Context, id string, stepID *string) error {
	return s.setRunField(ctx, id, "current_step_id", nullString(stepID))
}

// SetRunNeedsAttentionReason sets (or clears) the needs_attention reason.
func (s *Store) SetRunNeedsAttentionReason(ctx context.Context, id string, reason *string) error {
	return s.setRunField(ctx, id, "needs_attention_reason", nullString(reason))
}

// SetRunError sets (or clears) the run error.
func (s *Store) SetRunError(ctx context.Context, id string, errMsg *string) error {
	return s.setRunField(ctx, id, "error", nullString(errMsg))
}

func (s *Store) setRunField(ctx context.Context, id, column string, value any) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE runs SET `+column+` = ?, updated_at = ? WHERE id = ?`,
		value, timeString(nowUTC()), id,
	)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
