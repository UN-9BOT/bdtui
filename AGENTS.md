# PROJECT KNOWLEDGE BASE

**Generated:** 2026-03-12 **Stack:** Go 1.25 + Bubble Tea TUI

## OVERVIEW

bdtui is a keyboard-first Kanban TUI for issue management via `bd` CLI.
Reads/mutates data only through `bd` binary. Features ghost parent rows, blocked
column derivation, tmux integration.

## STRUCTURE

```
bdtui/
├── cmd/bdtui/           # CLI entrypoint (main.go → app.Run)
├── internal/app/        # Core TUI (Model, Update, View, forms, plugins) [SEE AGENTS.md]
├── internal/ui/         # Styles, keymaps, string utilities
├── internal/adapters/   # External: beads discovery, clipboard
├── internal/logger/     # File logging
├── tests/               # Black-box integration tests [SEE AGENTS.md]
├── docs/                # Release runbooks, structure docs
└── bin/                 # Built binary output
```

## WHERE TO LOOK

| Task                | Location                                     | Notes                                    |
| ------------------- | -------------------------------------------- | ---------------------------------------- |
| Entry point flow    | `cmd/bdtui/main.go` → `internal/app/main.go` | `app.Run()` starts Bubble Tea            |
| TUI state/model     | `internal/app/model.go`                      | `Model` struct, all state                |
| Event handling      | `internal/app/update.go`                     | `Update()` dispatcher                    |
| Rendering           | `internal/app/view.go`                       | `View()` renderer (2388 lines)           |
| Keyboard handling   | `internal/app/update_keyboard.go`            | KeyMsg routing                           |
| Mouse handling      | `internal/app/update_mouse.go`               | Click, scroll                            |
| Forms (create/edit) | `internal/app/forms.go`                      | IssueForm, validation                    |
| bd CLI wrapper      | `internal/app/bd_client.go`                  | `BdClient` wraps `bd` binary             |
| Config/flags        | `internal/app/config.go`                     | `--beads-dir`, `--no-watch`, `--plugins` |
| Tmux plugin         | `internal/app/plugin_tmux.go`                | Send issue to tmux                       |
| External editor     | `internal/app/external_editor.go`            | `$EDITOR` integration                    |
| Styles              | `internal/ui/styles.go`                      | lipgloss color palette                   |
| Keymaps (display)   | `internal/ui/keymap.go`                      | Help text, not bindings                  |
| Integration tests   | `tests/`                                     | 38 black-box tests                       |
| Test API            | `internal/app/test_api.go`                   | Exposes internals for tests              |

## CODE MAP

| Symbol     | Type   | Location                       | Role                                 |
| ---------- | ------ | ------------------------------ | ------------------------------------ |
| `Model`    | struct | `internal/app/model.go:15`     | TUI state container                  |
| `Issue`    | struct | `internal/app/types.go:39`     | Issue data (ID, Title, Status, etc.) |
| `Status`   | type   | `internal/app/types.go:10`     | open/in_progress/blocked/closed      |
| `Mode`     | type   | `internal/app/types.go:127`    | board/details/search/create/etc.     |
| `BdClient` | struct | `internal/app/bd_client.go:19` | `bd` CLI wrapper                     |
| `Run`      | func   | `internal/app/main.go:11`      | Application entry                    |
| `Update`   | method | `internal/app/update.go:11`    | Bubble Tea update                    |
| `View`     | method | `internal/app/view.go`         | Bubble Tea render                    |
| `Keymap`   | struct | `internal/ui/keymap.go`        | Help display keys                    |
| `Styles`   | struct | `internal/ui/styles.go`        | lipgloss styles                      |

## CONVENTIONS

- **Go version**: 1.25.6 (from go.mod)
- **TUI framework**: Bubble Tea (charmbracelet/bubbletea)
- **Styling**: lipgloss (ANSI 256 colors)
- **Tests**: Black-box in `tests/`, unit tests colocated with source
- **Test package**: External tests use `package bdtui_test`
- **No .golangci-lint.yml**: Default Go tooling only

## ANTI-PATTERNS

- ❌ Place `.go` files in repo root — use `cmd/` or `internal/`
- ❌ Use markdown TODO lists — use `bd` (beads) only
- ❌ Use external issue trackers — `bd` is single source of truth
- ❌ Ignore errors (`_` assignment) — handle all explicitly
- ❌ Use `panic` for normal errors — panics for unrecoverable only
- ❌ Create goroutines without lifecycle — causes leaks

## COMMANDS

```bash
make test              # go test ./...
make build             # builds to bin/bdtui
./bin/bdtui            # run TUI
./bin/bdtui --help     # CLI options
bd ready               # find available issues
bd show <id>           # view issue
```

## NOTES

- Data lives in `.beads/` (dogfooding the app's own backend)
- `blocked` column is auto-derived for open issues with unresolved blockers
- Ghost rows show parents in other status columns
- Release tags: `YYYY.MM.DD-pr<N>-<SHA7>` on PR merge to master
- Editor mode (`Ctrl+X`) uses YAML frontmatter for fields

---

<!-- BEGIN BEADS INTEGRATION -->

## Issue Tracking with bd (beads)

**IMPORTANT**: This project uses **bd (beads)** for ALL issue tracking.

### Quick Reference

```bash
bd ready --json                          # Find available work
bd show <id>                             # View issue details
bd update <id> --claim --json            # Claim work
bd close <id> --reason "Done" --json     # Complete work
bd create "Title" -d "desc" -t task -p 2 --json  # Create issue
```

### Issue Types & Priorities

| Type      | Use For                     |
| --------- | --------------------------- |
| `bug`     | Something broken            |
| `feature` | New functionality           |
| `task`    | Tests, docs, refactoring    |
| `epic`    | Large feature with subtasks |
| `chore`   | Maintenance                 |

| Priority | Meaning                        |
| -------- | ------------------------------ |
| 0        | Critical (security, data loss) |
| 1        | High (major features)          |
| 2        | Medium (default)               |
| 3        | Low (polish)                   |
| 4        | Backlog                        |

### Agent Workflow

1. `bd ready` → find unblocked issues
2. `bd update <id> --claim` → claim atomically
3. Implement, test, document
4. `bd close <id> --reason "Done"` → complete

### Rules

- ✅ Use `bd` for ALL task tracking
- ✅ Always use `--json` for programmatic use
- ✅ Link discovered work with `--deps discovered-from:<parent-id>`
- ❌ Do NOT create markdown TODO lists
- ❌ Do NOT use external issue trackers

<!-- END BEADS INTEGRATION -->
