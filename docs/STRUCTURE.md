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
