package orch

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"
)

func (s *Store) CreateHumanInput(ctx context.Context, h *HumanInput) error {
	if h.ID == "" {
		h.ID = uuid.NewString()
	}
	if h.Status == "" {
		h.Status = HumanPending
	}
	if !h.Status.Valid() {
		return errInvalidStatus(h.Status)
	}
	now := nowUTC()
	if h.CreatedAt.IsZero() {
		h.CreatedAt = now
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO human_inputs(id, run_id, step_attempt_id, execution_id, prompt, response, status, created_at, answered_at)
		 VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		h.ID, h.RunID, h.StepAttemptID, nullString(h.ExecutionID), h.Prompt,
		nullString(h.Response), string(h.Status), timeString(h.CreatedAt), timeStringPtr(h.AnsweredAt),
	); err != nil {
		return err
	}

	if err := appendEventMapTx(ctx, tx, &h.RunID, EventHumanRequested, map[string]any{
		"run_id": h.RunID, "human_input_id": h.ID,
	}); err != nil {
		return err
	}
	return tx.Commit()
}

// GetHumanInput returns a single human input by ID.
func (s *Store) GetHumanInput(ctx context.Context, id string) (*HumanInput, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, run_id, step_attempt_id, execution_id, prompt, response, status, created_at, answered_at
		 FROM human_inputs WHERE id = ?`, id)

	h := &HumanInput{}
	var execID, response sql.NullString
	var created string
	var answered sql.NullString
	if err := row.Scan(&h.ID, &h.RunID, &h.StepAttemptID, &execID, &h.Prompt, &response, &h.Status, &created, &answered); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	h.ExecutionID = strPtr(execID)
	h.Response = strPtr(response)

	var err error
	if h.CreatedAt, err = parseTime(created); err != nil {
		return nil, err
	}
	if h.AnsweredAt, err = timePtr(answered); err != nil {
		return nil, err
	}
	return h, nil
}

// AnswerHumanInput atomically records the response and marks the input answered.
func (s *Store) AnswerHumanInput(ctx context.Context, id, response string) error {
	now := nowUTC()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var cur HumanInputStatus
	var runID string
	if err := tx.QueryRowContext(ctx, `SELECT status, run_id FROM human_inputs WHERE id = ?`, id).Scan(&cur, &runID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if cur != HumanPending {
		return ErrInvalidTransition
	}

	res, err := tx.ExecContext(ctx,
		`UPDATE human_inputs SET status = ?, response = ?, answered_at = ? WHERE id = ? AND status = ?`,
		string(HumanAnswered), response, timeString(now), id, cur,
	)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrInvalidTransition
	}

	if err := appendEventMapTx(ctx, tx, &runID, EventHumanAnswered, map[string]any{
		"run_id": runID, "human_input_id": id,
	}); err != nil {
		return err
	}
	return tx.Commit()
}

// ListHumanInputsByRun returns every human input attached to a run,
// oldest-first. Used by the Runs tab so the operator can answer a
// waiting_human run from the coarse list (the Run row itself only
// carries the status flag, not the human_input ids).
func (s *Store) ListHumanInputsByRun(ctx context.Context, runID string) ([]HumanInput, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, run_id, step_attempt_id, execution_id, prompt, response, status, created_at, answered_at
		 FROM human_inputs WHERE run_id = ? ORDER BY created_at, id`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []HumanInput
	for rows.Next() {
		var h HumanInput
		var execID, response, answered sql.NullString
		var created string
		if err := rows.Scan(&h.ID, &h.RunID, &h.StepAttemptID, &execID, &h.Prompt,
			&response, &h.Status, &created, &answered); err != nil {
			return nil, err
		}
		h.ExecutionID = strPtr(execID)
		h.Response = strPtr(response)
		if h.CreatedAt, err = parseTime(created); err != nil {
			return nil, err
		}
		if h.AnsweredAt, err = timePtr(answered); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

