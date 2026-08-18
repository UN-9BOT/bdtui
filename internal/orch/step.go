package orch

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"
)

func (s *Store) CreateStepAttempt(ctx context.Context, sa *StepAttempt) error {
	if sa.ID == "" {
		sa.ID = uuid.NewString()
	}
	if sa.Status == "" {
		sa.Status = StepQueued
	}
	if !sa.Status.Valid() {
		return errInvalidStatus(sa.Status)
	}
	now := nowUTC()
	if sa.CreatedAt.IsZero() {
		sa.CreatedAt = now
	}
	sa.UpdatedAt = now

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO step_attempts(id, run_id, step_id, attempt, status, inputs, result, error,
		                           created_at, updated_at, started_at, completed_at)
		 VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sa.ID, sa.RunID, sa.StepID, sa.Attempt, string(sa.Status), sa.Inputs,
		nullString(sa.Result), nullString(sa.Error), timeString(sa.CreatedAt), timeString(sa.UpdatedAt),
		timeStringPtr(sa.StartedAt), timeStringPtr(sa.CompletedAt),
	); err != nil {
		return err
	}

	if err := appendEventMapTx(ctx, tx, &sa.RunID, EventStepCreated, map[string]any{
		"run_id": sa.RunID, "step_id": sa.StepID, "attempt": sa.Attempt, "status": sa.Status,
	}); err != nil {
		return err
	}
	return tx.Commit()
}

// StartStepAttempt atomically allocates the next attempt number for a step,
// inserts a queued StepAttempt and appends its event in one transaction.
func (s *Store) StartStepAttempt(ctx context.Context, runID, stepID, inputs string) (*StepAttempt, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	attempt, err := nextAttempt(ctx, tx, runID, stepID)
	if err != nil {
		return nil, err
	}

	sa := &StepAttempt{
		ID:      uuid.NewString(),
		RunID:   runID,
		StepID:  stepID,
		Attempt: attempt,
		Status:  StepQueued,
		Inputs:  inputs,
	}
	now := nowUTC()
	sa.CreatedAt = now
	sa.UpdatedAt = now

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO step_attempts(id, run_id, step_id, attempt, status, inputs, result, error,
		                           created_at, updated_at, started_at, completed_at)
		 VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sa.ID, sa.RunID, sa.StepID, sa.Attempt, string(sa.Status), sa.Inputs,
		nil, nil, timeString(sa.CreatedAt), timeString(sa.UpdatedAt), nil, nil,
	); err != nil {
		return nil, err
	}

	if err := appendEventMapTx(ctx, tx, &runID, EventStepCreated, map[string]any{
		"run_id": runID, "step_id": stepID, "attempt": attempt, "status": sa.Status,
	}); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return sa, nil
}

func (s *Store) GetStepAttempt(ctx context.Context, id string) (*StepAttempt, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, run_id, step_id, attempt, status, inputs, result, error,
		        created_at, updated_at, started_at, completed_at
		 FROM step_attempts WHERE id = ?`, id)

	sa := &StepAttempt{}
	var status string
	var created, updated string
	var result, errStr, started, completed sql.NullString

	if err := row.Scan(&sa.ID, &sa.RunID, &sa.StepID, &sa.Attempt, &status, &sa.Inputs,
		&result, &errStr, &created, &updated, &started, &completed); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	sa.Status = StepAttemptStatus(status)
	sa.Result = strPtr(result)
	sa.Error = strPtr(errStr)

	var err error
	if sa.CreatedAt, err = parseTime(created); err != nil {
		return nil, err
	}
	if sa.UpdatedAt, err = parseTime(updated); err != nil {
		return nil, err
	}
	if sa.StartedAt, err = timePtr(started); err != nil {
		return nil, err
	}
	if sa.CompletedAt, err = timePtr(completed); err != nil {
		return nil, err
	}
	return sa, nil
}

// TransitionStepAttempt atomically moves a step attempt to `to` if the
// transition is legal per the StepAttempt state machine.
func (s *Store) TransitionStepAttempt(ctx context.Context, id string, to StepAttemptStatus) error {
	if !to.Valid() {
		return errInvalidStatus(to)
	}
	now := nowUTC()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var cur StepAttemptStatus
	var runID string
	var started sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT status, run_id, started_at FROM step_attempts WHERE id = ?`, id).
		Scan(&cur, &runID, &started); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if !CanTransitionStepAttempt(cur, to) {
		return ErrInvalidTransition
	}

	if to == StepRunning && !started.Valid {
		started = sql.NullString{String: timeString(now), Valid: true}
	}
	var completed any
	if to.Terminal() {
		completed = timeString(now)
	}

	res, err := tx.ExecContext(ctx,
		`UPDATE step_attempts SET status = ?, updated_at = ?, started_at = ?, completed_at = ? WHERE id = ? AND status = ?`,
		string(to), timeString(now), started, completed, id, cur,
	)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrInvalidTransition
	}

	if err := appendEventMapTx(ctx, tx, &runID, EventStepTransition, map[string]any{
		"run_id": runID, "step_attempt_id": id, "from": cur, "to": to,
	}); err != nil {
		return err
	}
	return tx.Commit()
}
