package orch

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"
)

func (s *Store) CreateExecution(ctx context.Context, e *Execution) error {
	if e.ID == "" {
		e.ID = uuid.NewString()
	}
	if !e.Kind.Valid() {
		return errInvalidStatus(e.Kind)
	}
	if e.Status == "" {
		e.Status = ExecQueued
	}
	if !e.Status.Valid() {
		return errInvalidStatus(e.Status)
	}
	now := nowUTC()
	if e.CreatedAt.IsZero() {
		e.CreatedAt = now
	}
	e.UpdatedAt = now

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO executions(id, run_id, step_attempt_id, kind, status, pane_id, process_id,
		                        prompt_ref, prompt_hash, result_json, result_commit, error,
		                        created_at, updated_at, started_at, completed_at)
		 VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.ID, e.RunID, e.StepAttemptID, string(e.Kind), string(e.Status), nullString(e.PaneID), nullString(e.ProcessID),
		e.PromptRef, e.PromptHash, nullString(e.ResultJSON), nullString(e.ResultCommit), nullString(e.Error),
		timeString(e.CreatedAt), timeString(e.UpdatedAt), timeStringPtr(e.StartedAt), timeStringPtr(e.CompletedAt),
	); err != nil {
		return err
	}

	if err := appendEventMapTx(ctx, tx, &e.RunID, EventExecCreated, map[string]any{
		"run_id": e.RunID, "execution_id": e.ID, "kind": e.Kind, "status": e.Status,
	}); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) GetExecution(ctx context.Context, id string) (*Execution, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, run_id, step_attempt_id, kind, status, pane_id, process_id,
		        prompt_ref, prompt_hash, result_json, result_commit, error,
		        created_at, updated_at, started_at, completed_at
		 FROM executions WHERE id = ?`, id)

	e := &Execution{}
	var kind, status, created, updated string
	var pane, proc, resultJSON, resultCommit, errStr, started, completed sql.NullString

	if err := row.Scan(&e.ID, &e.RunID, &e.StepAttemptID, &kind, &status, &pane, &proc,
		&e.PromptRef, &e.PromptHash, &resultJSON, &resultCommit, &errStr, &created, &updated, &started, &completed); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	e.Kind = ExecutionKind(kind)
	e.Status = ExecutionStatus(status)
	e.PaneID = strPtr(pane)
	e.ProcessID = strPtr(proc)
	e.ResultJSON = strPtr(resultJSON)
	e.ResultCommit = strPtr(resultCommit)
	e.Error = strPtr(errStr)

	var err error
	if e.CreatedAt, err = parseTime(created); err != nil {
		return nil, err
	}
	if e.UpdatedAt, err = parseTime(updated); err != nil {
		return nil, err
	}
	if e.StartedAt, err = timePtr(started); err != nil {
		return nil, err
	}
	if e.CompletedAt, err = timePtr(completed); err != nil {
		return nil, err
	}
	return e, nil
}

// TransitionExecution atomically moves an execution to `to` if the transition
// is legal per the Execution state machine.
func (s *Store) TransitionExecution(ctx context.Context, id string, to ExecutionStatus) error {
	if !to.Valid() {
		return errInvalidStatus(to)
	}
	now := nowUTC()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var cur ExecutionStatus
	var runID string
	var started sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT status, run_id, started_at FROM executions WHERE id = ?`, id).
		Scan(&cur, &runID, &started); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if !CanTransitionExecution(cur, to) {
		return ErrInvalidTransition
	}

	if to == ExecRunning && !started.Valid {
		started = sql.NullString{String: timeString(now), Valid: true}
	}
	var completed any
	if to.Terminal() {
		completed = timeString(now)
	}

	res, err := tx.ExecContext(ctx,
		`UPDATE executions SET status = ?, updated_at = ?, started_at = ?, completed_at = ? WHERE id = ? AND status = ?`,
		string(to), timeString(now), started, completed, id, cur,
	)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrInvalidTransition
	}

	if err := appendEventMapTx(ctx, tx, &runID, EventExecTransition, map[string]any{
		"run_id": runID, "execution_id": id, "from": cur, "to": to,
	}); err != nil {
		return err
	}
	return tx.Commit()
}

// CreateArtifact records an immutable execution artifact.
func (s *Store) CreateArtifact(ctx context.Context, a *Artifact) error {
	if a.ID == "" {
		a.ID = uuid.NewString()
	}
	now := nowUTC()
	if a.CreatedAt.IsZero() {
		a.CreatedAt = now
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO artifacts(id, execution_id, name, path, hash, created_at)
		 VALUES(?, ?, ?, ?, ?, ?)`,
		a.ID, a.ExecutionID, a.Name, a.Path, a.Hash, timeString(a.CreatedAt),
	)
	return err
}

// ListArtifactsByExecution returns artifacts for an execution ordered by name.
func (s *Store) ListArtifactsByExecution(ctx context.Context, executionID string) ([]Artifact, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, execution_id, name, path, hash, created_at FROM artifacts WHERE execution_id = ? ORDER BY name`,
		executionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var artifacts []Artifact
	for rows.Next() {
		var a Artifact
		var created string
		if err := rows.Scan(&a.ID, &a.ExecutionID, &a.Name, &a.Path, &a.Hash, &created); err != nil {
			return nil, err
		}
		if a.CreatedAt, err = parseTime(created); err != nil {
			return nil, err
		}
		artifacts = append(artifacts, a)
	}
	return artifacts, rows.Err()
}
