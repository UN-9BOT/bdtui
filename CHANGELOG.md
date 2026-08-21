# Changelog

All notable changes to this project will be documented in this file.

This project uses date-based release tags in format `YYYY.MM.DD-pr<PR_NUMBER>-<MERGE_SHA7>`.

## Unreleased

- TaskStore boundary: new `internal/taskstore` package with a Beads
  adapter that owns the high-level task lifecycle (todo / in_progress /
  done / blocked). The daemon now claims tasks on Run creation and
  syncs terminal status back to Beads; if Beads is unavailable, Run
  launch refuses to start.
- `runs.task_snapshot` column (migration 7) freezes the TaskStore
  snapshot at Run creation so external edits do not mutate the view
  the controller / agent prompts see.
- `bdtuid` accepts `--beads-dir` to enable the lifecycle integration;
  the legacy behaviour is preserved when the flag is empty.

## 2026.02.23

- Dashboard UX improvements and mouse interaction fixes.
- Initial release preparation structure.
