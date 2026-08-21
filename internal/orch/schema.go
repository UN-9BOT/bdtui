package orch

// migration is an ordered, one-way schema migration.
type migration struct {
	version int
	name    string
	sql     string
}

// migrations is the full ordered list. Append new entries with increasing
// version numbers; never edit or reorder existing entries once released. The
// checksum recorded in schema_migrations protects against post-release edits.
var migrations = []migration{
	{
		version: 1,
		name:    "core_domain",
		sql: `
CREATE TABLE projects (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL,
    fs_path    TEXT NOT NULL,
    git_remote TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE runs (
    id                     TEXT PRIMARY KEY,
    project_id             TEXT NOT NULL REFERENCES projects(id),
    task_id                TEXT NOT NULL DEFAULT '',
    status                 TEXT NOT NULL,
    workflow_snapshot_ref  TEXT NOT NULL DEFAULT '',
    workflow_snapshot      TEXT NOT NULL DEFAULT '',
    current_step_id        TEXT,
    needs_attention_reason TEXT,
    error                  TEXT,
    created_at             TEXT NOT NULL,
    updated_at             TEXT NOT NULL,
    started_at             TEXT,
    completed_at           TEXT
);
CREATE INDEX idx_runs_project ON runs(project_id);
CREATE INDEX idx_runs_status  ON runs(status);
CREATE UNIQUE INDEX idx_runs_active_task ON runs(project_id, task_id)
    WHERE task_id <> '' AND status IN ('queued','running','waiting_human','needs_attention');

CREATE TABLE step_attempts (
    id          TEXT PRIMARY KEY,
    run_id      TEXT NOT NULL REFERENCES runs(id),
    step_id     TEXT NOT NULL,
    attempt     INTEGER NOT NULL,
    status      TEXT NOT NULL,
    inputs      TEXT NOT NULL DEFAULT '{}',
    result      TEXT,
    error       TEXT,
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL,
    started_at  TEXT,
    completed_at TEXT,
    UNIQUE(run_id, step_id, attempt)
);
CREATE INDEX idx_step_attempts_run ON step_attempts(run_id);

CREATE TABLE executions (
    id              TEXT PRIMARY KEY,
    run_id          TEXT NOT NULL REFERENCES runs(id),
    step_attempt_id TEXT NOT NULL REFERENCES step_attempts(id),
    kind            TEXT NOT NULL,
    status          TEXT NOT NULL,
    pane_id         TEXT,
    process_id      TEXT,
    prompt_ref      TEXT NOT NULL DEFAULT '',
    prompt_hash     TEXT NOT NULL DEFAULT '',
    result_json     TEXT,
    result_commit   TEXT,
    error           TEXT,
    created_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL,
    started_at      TEXT,
    completed_at    TEXT
);
CREATE INDEX idx_executions_run          ON executions(run_id);
CREATE INDEX idx_executions_step_attempt ON executions(step_attempt_id);

CREATE TABLE artifacts (
    id           TEXT PRIMARY KEY,
    execution_id TEXT NOT NULL REFERENCES executions(id),
    name         TEXT NOT NULL,
    path         TEXT NOT NULL,
    hash         TEXT NOT NULL,
    created_at   TEXT NOT NULL
);
CREATE INDEX idx_artifacts_execution ON artifacts(execution_id);

CREATE TABLE launch_intents (
    id           TEXT PRIMARY KEY,
    project_id   TEXT NOT NULL REFERENCES projects(id),
    task_id      TEXT NOT NULL DEFAULT '',
    workflow_ref TEXT NOT NULL,
    inputs       TEXT NOT NULL DEFAULT '{}',
    status       TEXT NOT NULL,
    run_id       TEXT REFERENCES runs(id),
    created_at   TEXT NOT NULL,
    resolved_at  TEXT
);
CREATE INDEX idx_launch_intents_project ON launch_intents(project_id);

CREATE TABLE human_inputs (
    id              TEXT PRIMARY KEY,
    run_id          TEXT NOT NULL REFERENCES runs(id),
    step_attempt_id TEXT NOT NULL REFERENCES step_attempts(id),
    execution_id    TEXT,
    prompt          TEXT NOT NULL,
    response        TEXT,
    status          TEXT NOT NULL,
    created_at      TEXT NOT NULL,
    answered_at     TEXT
);
CREATE INDEX idx_human_inputs_run ON human_inputs(run_id);

CREATE TABLE events (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id     TEXT,
    seq        INTEGER NOT NULL,
    type       TEXT NOT NULL,
    payload    TEXT NOT NULL DEFAULT '{}',
    created_at TEXT NOT NULL,
    UNIQUE(run_id, seq)
);
CREATE INDEX idx_events_run_seq ON events(run_id, seq);

CREATE TABLE event_counters (
    stream_id TEXT PRIMARY KEY,
    next_seq  INTEGER NOT NULL DEFAULT 2
);

CREATE TABLE step_attempt_counters (
    run_id       TEXT NOT NULL,
    step_id      TEXT NOT NULL,
    next_attempt INTEGER NOT NULL DEFAULT 2,
    PRIMARY KEY (run_id, step_id)
);
`,
	},
	{
		version: 7,
		name:    "task_snapshot",
		sql: `
-- task_snapshot stores the immutable TaskStore snapshot captured at
-- Run creation. The controller freezes the bead's title, description
-- and metadata here so external edits during the Run do not mutate the
-- view the controller / agent step prompts see. JSON because the set
-- of fields is small and the orchestrator does not need to query it.
ALTER TABLE runs ADD COLUMN task_snapshot TEXT NOT NULL DEFAULT '';
`,
	},
}
