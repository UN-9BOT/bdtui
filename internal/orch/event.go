package orch

import (
	"context"
	"database/sql"
)

// AppendEvent inserts an append-only event, allocating the next per-run (or
// per-project when runID is nil) sequence number atomically.
func (s *Store) AppendEvent(ctx context.Context, runID *string, typ, payload string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := appendEventTx(ctx, tx, runID, typ, payload); err != nil {
		return err
	}
	return tx.Commit()
}

// ListEventsByRun returns events for a run ordered by sequence.
func (s *Store) ListEventsByRun(ctx context.Context, runID string) ([]Event, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, run_id, seq, type, payload, created_at FROM events WHERE run_id = ? ORDER BY seq`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []Event
	for rows.Next() {
		var e Event
		var rid sql.NullString
		var created string
		if err := rows.Scan(&e.ID, &rid, &e.Seq, &e.Type, &e.Payload, &created); err != nil {
			return nil, err
		}
		e.RunID = strPtr(rid)
		if e.CreatedAt, err = parseTime(created); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

// appendEventTx writes one event inside an existing transaction. Project-scoped
// events use an empty stream id (runID == nil).
func appendEventTx(ctx context.Context, tx *sql.Tx, runID *string, typ, payload string) error {
	streamID := ""
	if runID != nil {
		streamID = *runID
	}
	seq, err := nextEventSeq(ctx, tx, streamID)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx,
		`INSERT INTO events(run_id, seq, type, payload, created_at) VALUES(?, ?, ?, ?, ?)`,
		nullString(runID), seq, typ, payload, timeString(nowUTC()),
	)
	return err
}

// appendEventMapTx marshals fields to JSON and appends an event, propagating
// serialization errors so a transaction cannot silently lose event data.
func appendEventMapTx(ctx context.Context, tx *sql.Tx, runID *string, typ string, fields map[string]any) error {
	payload, err := jsonString(fields)
	if err != nil {
		return err
	}
	return appendEventTx(ctx, tx, runID, typ, payload)
}
