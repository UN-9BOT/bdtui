package orch

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"
)

// activeRunStatusesSQL is the SQL list of non-terminal run statuses.
const activeRunStatusesSQL = "('queued','running','waiting_human','needs_attention')"

// RunTransitionOpts carries optional fields updated during a run transition.
type RunTransitionOpts struct {
	NeedsAttentionReason *string
	CurrentStepID        *string
	Error                *string
}

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
			`SELECT EXISTS(SELECT 1 FROM runs WHERE task_id = ? AND status IN `+activeRunStatusesSQL+`)`,
			r.TaskID,
		).Scan(&exists); err != nil {
			return err
		}
		if exists {
			return ErrActiveRunExists
		}
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO runs(id, project_id, task_id, status, workflow_snapshot_ref, workflow_snapshot,
		                  current_step_id, needs_attention_reason, error, created_at, updated_at, started_at, completed_at)
		 VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.ProjectID, r.TaskID, string(r.Status), r.WorkflowSnapshotRef, r.WorkflowSnapshot,
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
		        current_step_id, needs_attention_reason, error, created_at, updated_at, started_at, completed_at
		 FROM runs WHERE id = ?`, id)

	r := &Run{}
	var status string
	var created, updated string
	var currentStep, reason, errStr sql.NullString
	var started, completed sql.NullString

	if err := row.Scan(&r.ID, &r.ProjectID, &r.TaskID, &status, &r.WorkflowSnapshotRef, &r.WorkflowSnapshot,
		&currentStep, &reason, &errStr, &created, &updated, &started, &completed); err != nil {
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

// TransitionRun atomically moves a run to `to` if that transition is legal per
// the Run state machine. It updates timestamps and audit state in a single
// transaction and returns ErrInvalidTransition for illegal transitions.
func (s *Store) TransitionRun(ctx context.Context, id string, to RunStatus, opts RunTransitionOpts) error {
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
		`UPDATE runs SET status = ?, updated_at = ?, needs_attention_reason = ?, current_step_id = ?,
		                error = ?, started_at = ?, completed_at = ?
		 WHERE id = ? AND status = ?`,
		string(to), timeString(now), opts.NeedsAttentionReason, opts.CurrentStepID,
		opts.Error, started, completed, id, cur,
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
