# Repository Structure

This repository uses a standard Go CLI layout centered on `cmd` + `internal`.

## Top-level layout

- `cmd/bdtui/`
  - CLI entrypoint (`main.go`).
  - Imports `internal/app` and runs `app.Run`.
- `internal/app/`
  - Main application package (model, update, view, forms, plugins, config, runtime).
  - Contains the operational logic that previously lived in repository root.
- `internal/ui/`
  - Reusable UI helpers and primitives (`keymap`, `styles`, common UI utils).
- `internal/adapters/`
  - External/system integrations:
  - `beads/` for `.beads` discovery and watch target helpers.
  - `clipboard/` for clipboard operations.
- `internal/daemon/`
  - gRPC daemon (bdtuid) that owns orchestrator durable state and the
    task lifecycle. Exposes the Orchestrator proto surface.
- `internal/orch/`
  - SQLite-backed orchestrator store: projects, runs, step attempts,
    executions, human inputs, launch intents, events.
- `internal/taskstore/`
  - TaskStore boundary (interface + Beads adapter) used by the daemon
    to drive the high-level task lifecycle (todo / in_progress / done
    / blocked). The Beads adapter lives in `internal/taskstore/beads/`.
- `internal/recovery/`
  - Recovery primitives (handle restarts, reattach executions).
- `internal/agent/`
  - Runtime interface and adapters (ExecRuntime today, HerdrRuntime
    upcoming).
- `internal/workflow/`
  - Workflow loader, validator, role model.
- `tests/`
  - Black-box integration-style tests importing `internal/app`.
- `docs/`
  - Release/process documentation plus structure notes.

## Layout rules

- Keep executable entrypoints only in `cmd/*`.
- Keep runtime logic in `internal/app`.
- Keep integration boundaries in `internal/adapters/*`.
- Keep shared rendering/input helpers in `internal/ui`.
- Avoid placing application `.go` files in repository root.
