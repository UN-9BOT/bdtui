package orch

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// Sentinel errors.
var (
	ErrNotFound          = errors.New("orch: not found")
	ErrInvalidTransition = errors.New("orch: invalid status transition")
	ErrInvalidStatus     = errors.New("orch: invalid status")
	ErrActiveRunExists   = errors.New("orch: an active run already exists for this task")
)

// Store is a SQLite-backed store for orchestrator durable state.
//
// Relational state is authoritative. Events are append-only and are written
// inside the same transaction as the state change that produced them. All
// write transactions run with BEGIN IMMEDIATE (via the _txlock DSN option), so
// concurrent writers serialize cleanly and read-modify-write transitions are
// safe.
type Store struct {
	db *sql.DB
}

// Open opens (or creates) the SQLite database at path and applies migrations.
// The path must be a filesystem path; it is converted to an absolute path and
// wrapped in a file: URI so DSN options are applied to every pooled connection.
func Open(ctx context.Context, path string) (*Store, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}

	u := &url.URL{Scheme: "file", Path: abs}
	q := u.Query()
	q.Set("_txlock", "immediate")
	q.Set("_foreign_keys", "on")
	q.Set("_busy_timeout", "5000")
	q.Set("_journal_mode", "WAL")
	q.Set("_synchronous", "NORMAL")
	u.RawQuery = q.Encode()

	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, err
	}

	s := &Store{db: db}
	if err := s.Migrate(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// Close closes the underlying database.
func (s *Store) Close() error {
	return s.db.Close()
}

// Migrate applies any pending migrations in order and verifies the checksum of
// already-applied migrations.
func (s *Store) Migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version    INTEGER PRIMARY KEY,
		name       TEXT NOT NULL,
		checksum   TEXT NOT NULL,
		applied_at TEXT NOT NULL
	)`); err != nil {
		return err
	}

	for _, m := range migrations {
		sum := checksum(m.sql)

		var existing string
		err := s.db.QueryRowContext(ctx,
			`SELECT checksum FROM schema_migrations WHERE version = ?`, m.version,
		).Scan(&existing)

		switch {
		case err == nil:
			if existing != sum {
				return fmt.Errorf("migration %d (%s): checksum mismatch (db=%s want=%s)", m.version, m.name, existing, sum)
			}
			continue
		case errors.Is(err, sql.ErrNoRows):
			// apply below
		default:
			return err
		}

		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, m.sql); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %d (%s): %w", m.version, m.name, err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO schema_migrations(version, name, checksum, applied_at) VALUES(?, ?, ?, ?)`,
			m.version, m.name, sum, timeString(time.Now()),
		); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

// --- shared helpers ---

func errInvalidStatus(v any) error {
	return fmt.Errorf("%w: %v", ErrInvalidStatus, v)
}

func checksum(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func jsonString(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func nullString(s *string) any {
	if s == nil {
		return nil
	}
	return *s
}

func strPtr(ns sql.NullString) *string {
	if !ns.Valid {
		return nil
	}
	s := ns.String
	return &s
}

func nowUTC() time.Time {
	return time.Now().UTC()
}

func timeString(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

func timeStringPtr(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func parseTime(s string) (time.Time, error) {
	return time.Parse(time.RFC3339Nano, s)
}

func timePtr(ns sql.NullString) (*time.Time, error) {
	if !ns.Valid {
		return nil, nil
	}
	t, err := parseTime(ns.String)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// nextEventSeq atomically allocates the next per-stream sequence number.
func nextEventSeq(ctx context.Context, tx *sql.Tx, streamID string) (int64, error) {
	var seq int64
	err := tx.QueryRowContext(ctx, `
		INSERT INTO event_counters(stream_id, next_seq) VALUES(?, 2)
		ON CONFLICT(stream_id) DO UPDATE SET next_seq = next_seq + 1
		RETURNING next_seq - 1
	`, streamID).Scan(&seq)
	return seq, err
}

// nextAttempt atomically allocates the next per-(run,step) attempt number.
func nextAttempt(ctx context.Context, tx *sql.Tx, runID, stepID string) (int, error) {
	var attempt int
	err := tx.QueryRowContext(ctx, `
		INSERT INTO step_attempt_counters(run_id, step_id, next_attempt) VALUES(?, ?, 2)
		ON CONFLICT(run_id, step_id) DO UPDATE SET next_attempt = next_attempt + 1
		RETURNING next_attempt - 1
	`, runID, stepID).Scan(&attempt)
	return attempt, err
}
