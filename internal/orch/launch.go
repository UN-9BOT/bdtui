package orch

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"
)

func (s *Store) CreateLaunchIntent(ctx context.Context, li *LaunchIntent) error {
	if li.ID == "" {
		li.ID = uuid.NewString()
	}
	if li.Status == "" {
		li.Status = LaunchPending
	}
	if !li.Status.Valid() {
		return errInvalidStatus(li.Status)
	}
	now := nowUTC()
	if li.CreatedAt.IsZero() {
		li.CreatedAt = now
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO launch_intents(id, project_id, task_id, workflow_ref, inputs, status, run_id, created_at, resolved_at)
		 VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		li.ID, li.ProjectID, li.TaskID, li.WorkflowRef, li.Inputs, string(li.Status), nullString(li.RunID),
		timeString(li.CreatedAt), timeStringPtr(li.ResolvedAt),
	); err != nil {
		return err
	}

	if err := appendEventMapTx(ctx, tx, nil, EventIntentCreated, map[string]any{
		"intent_id": li.ID, "project_id": li.ProjectID, "task_id": li.TaskID, "status": li.Status,
	}); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) GetLaunchIntent(ctx context.Context, id string) (*LaunchIntent, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, project_id, task_id, workflow_ref, inputs, status, run_id, created_at, resolved_at
		 FROM launch_intents WHERE id = ?`, id)

	li := &LaunchIntent{}
	var status, created string
	var runID, resolved sql.NullString

	if err := row.Scan(&li.ID, &li.ProjectID, &li.TaskID, &li.WorkflowRef, &li.Inputs, &status,
		&runID, &created, &resolved); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	li.Status = LaunchIntentStatus(status)
	li.RunID = strPtr(runID)

	var err error
	if li.CreatedAt, err = parseTime(created); err != nil {
		return nil, err
	}
	if li.ResolvedAt, err = timePtr(resolved); err != nil {
		return nil, err
	}
	return li, nil
}

// ResolveLaunchIntent atomically transitions a pending intent to a terminal
// status and (for acceptance) records the created run id.
func (s *Store) ResolveLaunchIntent(ctx context.Context, id string, to LaunchIntentStatus, runID *string) error {
	if !to.Valid() || to == LaunchPending {
		return errInvalidStatus(to)
	}
	now := nowUTC()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var cur LaunchIntentStatus
	if err := tx.QueryRowContext(ctx, `SELECT status FROM launch_intents WHERE id = ?`, id).Scan(&cur); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if cur != LaunchPending {
		return ErrInvalidTransition
	}

	res, err := tx.ExecContext(ctx,
		`UPDATE launch_intents SET status = ?, run_id = ?, resolved_at = ? WHERE id = ? AND status = ?`,
		string(to), nullString(runID), timeString(now), id, cur,
	)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrInvalidTransition
	}

	if err := appendEventMapTx(ctx, tx, nil, EventIntentResolved, map[string]any{
		"intent_id": id, "to": to, "run_id": runID,
	}); err != nil {
		return err
	}
	return tx.Commit()
}
