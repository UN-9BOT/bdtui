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
