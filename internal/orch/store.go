package orch

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

// Sentinel errors.
var (
	ErrNotFound          = errors.New("orch: not found")
	ErrInvalidTransition = errors.New("orch: invalid status transition")
	ErrInvalidStatus     = errors.New("orch: invalid status")
)

// Store is a SQLite-backed store for orchestrator durable state.
//
// Relational state is authoritative. Events are append-only and are written
// inside the same transaction as the state change that produced them.
type Store struct {
	db *sql.DB
}

// Open opens (or creates) the SQLite database at path and applies migrations.
func Open(ctx context.Context, path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	// Single connection keeps per-connection PRAGMAs (foreign_keys) effective
	// and serializes writes, avoiding "database is locked" in the MVP.
	db.SetMaxOpenConns(1)

	for _, pragma := range []string{
		"PRAGMA foreign_keys = ON",
		"PRAGMA busy_timeout = 5000",
		"PRAGMA journal_mode = WAL",
	} {
		if _, err := db.ExecContext(ctx, pragma); err != nil {
			db.Close()
			return nil, err
		}
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

// Migrate applies any pending migrations in order.
func (s *Store) Migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version    INTEGER PRIMARY KEY,
		name       TEXT NOT NULL,
		applied_at TEXT NOT NULL
	)`); err != nil {
		return err
	}

	for _, m := range migrations {
		var applied bool
		if err := s.db.QueryRowContext(ctx,
			`SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = ?)`, m.version,
		).Scan(&applied); err != nil {
			return err
		}
		if applied {
			continue
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
			`INSERT INTO schema_migrations(version, name, applied_at) VALUES(?, ?, ?)`,
			m.version, m.name, timeString(time.Now()),
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

// --- Projects ---

func (s *Store) CreateProject(ctx context.Context, p *Project) error {
	if p.ID == "" {
		p.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	if p.CreatedAt.IsZero() {
		p.CreatedAt = now
	}
	p.UpdatedAt = now

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO projects(id, name, fs_path, git_remote, created_at, updated_at)
		 VALUES(?, ?, ?, ?, ?, ?)`,
		p.ID, p.Name, p.FsPath, p.GitRemote, timeString(p.CreatedAt), timeString(p.UpdatedAt),
	)
	if err != nil {
		return err
	}
	return s.AppendEvent(ctx, nil, EventProjectUpserted, mustJSON(map[string]string{"project_id": p.ID}))
}

func (s *Store) GetProject(ctx context.Context, id string) (*Project, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, name, fs_path, git_remote, created_at, updated_at FROM projects WHERE id = ?`, id)
	p := &Project{}
	var created, updated string
	if err := row.Scan(&p.ID, &p.Name, &p.FsPath, &p.GitRemote, &created, &updated); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	var err error
	if p.CreatedAt, err = parseTime(created); err != nil {
		return nil, err
	}
	if p.UpdatedAt, err = parseTime(updated); err != nil {
		return nil, err
	}
	return p, nil
}

// UpdateProject mutates the project's movable attributes (name, fs_path,
// git_remote). The ID is immutable.
func (s *Store) UpdateProject(ctx context.Context, p *Project) error {
	p.UpdatedAt = time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		`UPDATE projects SET name = ?, fs_path = ?, git_remote = ?, updated_at = ? WHERE id = ?`,
		p.Name, p.FsPath, p.GitRemote, timeString(p.UpdatedAt), p.ID,
	)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// --- Runs ---

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
		return fmt.Errorf("%w: %q", ErrInvalidStatus, r.Status)
	}
	now := time.Now().UTC()
	if r.CreatedAt.IsZero() {
		r.CreatedAt = now
	}
	r.UpdatedAt = now

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO runs(id, project_id, status, workflow_snapshot_ref, workflow_snapshot,
		                  current_step_id, needs_attention_reason, error, created_at, updated_at, started_at, completed_at)
		 VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.ProjectID, string(r.Status), r.WorkflowSnapshotRef, r.WorkflowSnapshot,
		nullString(r.CurrentStepID), nullString(r.NeedsAttentionReason), nullString(r.Error),
		timeString(r.CreatedAt), timeString(r.UpdatedAt), timeStringPtr(r.StartedAt), timeStringPtr(r.CompletedAt),
	)
	if err != nil {
		return err
	}
	return s.AppendEvent(ctx, &r.ID, EventRunCreated, mustJSON(map[string]string{"run_id": r.ID, "status": string(r.Status)}))
}

func (s *Store) GetRun(ctx context.Context, id string) (*Run, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, project_id, status, workflow_snapshot_ref, workflow_snapshot,
		        current_step_id, needs_attention_reason, error, created_at, updated_at, started_at, completed_at
		 FROM runs WHERE id = ?`, id)

	r := &Run{}
	var status string
	var created, updated string
	var currentStep, reason, errStr sql.NullString
	var started, completed sql.NullString

	if err := row.Scan(&r.ID, &r.ProjectID, &status, &r.WorkflowSnapshotRef, &r.WorkflowSnapshot,
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

// TransitionRun atomically moves a run from any of the allowed `from` statuses
// to `to`, updating timestamps and audit state in a single transaction. It
// returns ErrInvalidTransition if the current status is not in `from`.
func (s *Store) TransitionRun(ctx context.Context, id string, from []RunStatus, to RunStatus, opts RunTransitionOpts) error {
	if !to.Valid() {
		return fmt.Errorf("%w: %q", ErrInvalidStatus, to)
	}
	now := time.Now().UTC()

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

	if !allowed(cur, from) {
		return ErrInvalidTransition
	}

	if to == RunRunning && !started.Valid {
		started = sql.NullString{String: timeString(now), Valid: true}
	}
	var completed any
	if to.Terminal() {
		completed = timeString(now)
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE runs SET status = ?, updated_at = ?, needs_attention_reason = ?, current_step_id = ?,
		                error = ?, started_at = ?, completed_at = ?
		 WHERE id = ?`,
		string(to), timeString(now), opts.NeedsAttentionReason, opts.CurrentStepID,
		opts.Error, started, completed, id,
	); err != nil {
		return err
	}

	if err := appendEventTx(ctx, tx, &id, EventRunTransition,
		mustJSON(map[string]any{"run_id": id, "from": cur, "to": to})); err != nil {
		return err
	}
	return tx.Commit()
}

// --- Step attempts ---

func (s *Store) CreateStepAttempt(ctx context.Context, sa *StepAttempt) error {
	if sa.ID == "" {
		sa.ID = uuid.NewString()
	}
	if sa.Status == "" {
		sa.Status = StepQueued
	}
	if !sa.Status.Valid() {
		return fmt.Errorf("%w: %q", ErrInvalidStatus, sa.Status)
	}
	now := time.Now().UTC()
	if sa.CreatedAt.IsZero() {
		sa.CreatedAt = now
	}
	sa.UpdatedAt = now

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO step_attempts(id, run_id, step_id, attempt, status, inputs, result, error,
		                           created_at, updated_at, started_at, completed_at)
		 VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sa.ID, sa.RunID, sa.StepID, sa.Attempt, string(sa.Status), sa.Inputs,
		nullString(sa.Result), nullString(sa.Error), timeString(sa.CreatedAt), timeString(sa.UpdatedAt),
		timeStringPtr(sa.StartedAt), timeStringPtr(sa.CompletedAt),
	)
	if err != nil {
		return err
	}
	return s.AppendEvent(ctx, &sa.RunID, EventStepCreated,
		mustJSON(map[string]any{"run_id": sa.RunID, "step_id": sa.StepID, "attempt": sa.Attempt, "status": sa.Status}))
}

// StartStepAttempt atomically allocates the next attempt number for a step,
// inserts a queued StepAttempt and appends its event in one transaction.
func (s *Store) StartStepAttempt(ctx context.Context, runID, stepID, inputs string) (*StepAttempt, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var attempt int
	if err := tx.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(attempt), 0) + 1 FROM step_attempts WHERE run_id = ? AND step_id = ?`,
		runID, stepID,
	).Scan(&attempt); err != nil {
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
	now := time.Now().UTC()
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

	if err := appendEventTx(ctx, tx, &runID, EventStepCreated,
		mustJSON(map[string]any{"run_id": runID, "step_id": stepID, "attempt": attempt, "status": sa.Status})); err != nil {
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

// TransitionStepAttempt atomically moves a step attempt between statuses.
func (s *Store) TransitionStepAttempt(ctx context.Context, id string, from []StepAttemptStatus, to StepAttemptStatus) error {
	if !to.Valid() {
		return fmt.Errorf("%w: %q", ErrInvalidStatus, to)
	}
	now := time.Now().UTC()

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
	if !allowed(cur, from) {
		return ErrInvalidTransition
	}

	if to == StepRunning && !started.Valid {
		started = sql.NullString{String: timeString(now), Valid: true}
	}
	var completed any
	if to.Terminal() {
		completed = timeString(now)
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE step_attempts SET status = ?, updated_at = ?, started_at = ?, completed_at = ? WHERE id = ?`,
		string(to), timeString(now), started, completed, id,
	); err != nil {
		return err
	}

	if err := appendEventTx(ctx, tx, &runID, EventStepTransition,
		mustJSON(map[string]any{"run_id": runID, "step_attempt_id": id, "from": cur, "to": to})); err != nil {
		return err
	}
	return tx.Commit()
}

// --- Executions ---

func (s *Store) CreateExecution(ctx context.Context, e *Execution) error {
	if e.ID == "" {
		e.ID = uuid.NewString()
	}
	if !e.Kind.Valid() {
		return fmt.Errorf("%w: kind %q", ErrInvalidStatus, e.Kind)
	}
	if e.Status == "" {
		e.Status = ExecQueued
	}
	if !e.Status.Valid() {
		return fmt.Errorf("%w: %q", ErrInvalidStatus, e.Status)
	}
	now := time.Now().UTC()
	if e.CreatedAt.IsZero() {
		e.CreatedAt = now
	}
	e.UpdatedAt = now

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO executions(id, run_id, step_attempt_id, kind, status, pane_id, process_id,
		                        prompt, result_json, result_commit, artifacts, error,
		                        created_at, updated_at, started_at, completed_at)
		 VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.ID, e.RunID, e.StepAttemptID, string(e.Kind), string(e.Status), nullString(e.PaneID), nullString(e.ProcessID),
		e.Prompt, nullString(e.ResultJSON), nullString(e.ResultCommit), e.Artifacts, nullString(e.Error),
		timeString(e.CreatedAt), timeString(e.UpdatedAt), timeStringPtr(e.StartedAt), timeStringPtr(e.CompletedAt),
	)
	if err != nil {
		return err
	}
	return s.AppendEvent(ctx, &e.RunID, EventExecCreated,
		mustJSON(map[string]any{"run_id": e.RunID, "execution_id": e.ID, "kind": e.Kind, "status": e.Status}))
}

func (s *Store) GetExecution(ctx context.Context, id string) (*Execution, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, run_id, step_attempt_id, kind, status, pane_id, process_id,
		        prompt, result_json, result_commit, artifacts, error,
		        created_at, updated_at, started_at, completed_at
		 FROM executions WHERE id = ?`, id)

	e := &Execution{}
	var kind, status, created, updated string
	var pane, proc, resultJSON, resultCommit, errStr, started, completed sql.NullString

	if err := row.Scan(&e.ID, &e.RunID, &e.StepAttemptID, &kind, &status, &pane, &proc,
		&e.Prompt, &resultJSON, &resultCommit, &e.Artifacts, &errStr, &created, &updated, &started, &completed); err != nil {
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

// TransitionExecution atomically moves an execution between statuses.
func (s *Store) TransitionExecution(ctx context.Context, id string, from []ExecutionStatus, to ExecutionStatus) error {
	if !to.Valid() {
		return fmt.Errorf("%w: %q", ErrInvalidStatus, to)
	}
	now := time.Now().UTC()

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
	if !allowed(cur, from) {
		return ErrInvalidTransition
	}

	if to == ExecRunning && !started.Valid {
		started = sql.NullString{String: timeString(now), Valid: true}
	}
	var completed any
	if to.Terminal() {
		completed = timeString(now)
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE executions SET status = ?, updated_at = ?, started_at = ?, completed_at = ? WHERE id = ?`,
		string(to), timeString(now), started, completed, id,
	); err != nil {
		return err
	}

	if err := appendEventTx(ctx, tx, &runID, EventExecTransition,
		mustJSON(map[string]any{"run_id": runID, "execution_id": id, "from": cur, "to": to})); err != nil {
		return err
	}
	return tx.Commit()
}

// --- Launch intents ---

func (s *Store) CreateLaunchIntent(ctx context.Context, li *LaunchIntent) error {
	if li.ID == "" {
		li.ID = uuid.NewString()
	}
	if li.Status == "" {
		li.Status = LaunchPending
	}
	if !li.Status.Valid() {
		return fmt.Errorf("%w: %q", ErrInvalidStatus, li.Status)
	}
	now := time.Now().UTC()
	if li.CreatedAt.IsZero() {
		li.CreatedAt = now
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO launch_intents(id, project_id, workflow_ref, inputs, status, run_id, created_at, resolved_at)
		 VALUES(?, ?, ?, ?, ?, ?, ?, ?)`,
		li.ID, li.ProjectID, li.WorkflowRef, li.Inputs, string(li.Status), nullString(li.RunID),
		timeString(li.CreatedAt), timeStringPtr(li.ResolvedAt),
	)
	if err != nil {
		return err
	}
	return s.AppendEvent(ctx, nil, EventIntentCreated,
		mustJSON(map[string]any{"intent_id": li.ID, "project_id": li.ProjectID, "status": li.Status}))
}

func (s *Store) GetLaunchIntent(ctx context.Context, id string) (*LaunchIntent, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, project_id, workflow_ref, inputs, status, run_id, created_at, resolved_at
		 FROM launch_intents WHERE id = ?`, id)

	li := &LaunchIntent{}
	var status, created string
	var runID, resolved sql.NullString

	if err := row.Scan(&li.ID, &li.ProjectID, &li.WorkflowRef, &li.Inputs, &status,
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
		return fmt.Errorf("%w: %q", ErrInvalidStatus, to)
	}
	now := time.Now().UTC()

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

	if _, err := tx.ExecContext(ctx,
		`UPDATE launch_intents SET status = ?, run_id = ?, resolved_at = ? WHERE id = ?`,
		string(to), runID, timeString(now), id,
	); err != nil {
		return err
	}

	if err := appendEventTx(ctx, tx, nil, EventIntentResolved,
		mustJSON(map[string]any{"intent_id": id, "to": to, "run_id": runID})); err != nil {
		return err
	}
	return tx.Commit()
}

// --- Human inputs ---

func (s *Store) CreateHumanInput(ctx context.Context, h *HumanInput) error {
	if h.ID == "" {
		h.ID = uuid.NewString()
	}
	if h.Status == "" {
		h.Status = HumanPending
	}
	if !h.Status.Valid() {
		return fmt.Errorf("%w: %q", ErrInvalidStatus, h.Status)
	}
	now := time.Now().UTC()
	if h.CreatedAt.IsZero() {
		h.CreatedAt = now
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO human_inputs(id, run_id, step_attempt_id, execution_id, prompt, response, status, created_at, answered_at)
		 VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		h.ID, h.RunID, h.StepAttemptID, nullString(h.ExecutionID), h.Prompt,
		nullString(h.Response), string(h.Status), timeString(h.CreatedAt), timeStringPtr(h.AnsweredAt),
	)
	if err != nil {
		return err
	}
	return s.AppendEvent(ctx, &h.RunID, EventHumanRequested,
		mustJSON(map[string]any{"run_id": h.RunID, "human_input_id": h.ID}))
}

// AnswerHumanInput atomically records the response and marks the input answered.
func (s *Store) AnswerHumanInput(ctx context.Context, id, response string) error {
	now := time.Now().UTC()

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

	if _, err := tx.ExecContext(ctx,
		`UPDATE human_inputs SET status = ?, response = ?, answered_at = ? WHERE id = ?`,
		string(HumanAnswered), response, timeString(now), id,
	); err != nil {
		return err
	}

	if err := appendEventTx(ctx, tx, &runID, EventHumanAnswered,
		mustJSON(map[string]any{"run_id": runID, "human_input_id": id})); err != nil {
		return err
	}
	return tx.Commit()
}

// --- Events ---

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

// --- helpers ---

func appendEventTx(ctx context.Context, tx *sql.Tx, runID *string, typ, payload string) error {
	var seq int64
	if runID == nil {
		if err := tx.QueryRowContext(ctx,
			`SELECT COALESCE(MAX(seq), 0) + 1 FROM events WHERE run_id IS NULL`).Scan(&seq); err != nil {
			return err
		}
	} else {
		if err := tx.QueryRowContext(ctx,
			`SELECT COALESCE(MAX(seq), 0) + 1 FROM events WHERE run_id = ?`, *runID).Scan(&seq); err != nil {
			return err
		}
	}

	_, err := tx.ExecContext(ctx,
		`INSERT INTO events(run_id, seq, type, payload, created_at) VALUES(?, ?, ?, ?, ?)`,
		runID, seq, typ, payload, timeString(time.Now().UTC()),
	)
	return err
}

func allowed[T ~string](cur T, from []T) bool {
	for _, f := range from {
		if cur == f {
			return true
		}
	}
	return false
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
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
