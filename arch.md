# bdtui architecture

## Scope
- single view: Kanban (`open` / `in_progress` / `blocked` / `closed`)
- keyboard-first task management
- backend mutations via `bd` CLI
- plugin runtime with enable/disable toggles (first plugin: `tmux`)

## Core files
- `main.go` - entrypoint
- `config.go` - flags and config parsing
- `finder.go` - `.beads` discovery
- `bd_client.go` - wrapper around `bd` commands
- `model.go` - state, layout, data grouping
- `update.go` - message dispatcher
- `update_keyboard.go` - key routing and actions
- `view.go` - board and modal rendering
- `forms.go` - create/edit/filter form logic
- `styles.go` - lipgloss styles
- `keymap.go` - centralized hotkeys
- `plugins.go` - plugin registry and toggles
- `plugin_tmux.go` - tmux plugin (target picker, mark handling, paste flow)

## Data flow
1. find `.beads`
2. load issues via `bd list --json`
3. normalize dependencies and derived display status
4. render columns
5. perform mutations via `bd` write commands
6. reload issues after each mutation
7. execute plugin actions separately (no issue reload), e.g. `Y` -> tmux paste
8. in `tmux_picker`, current target is marked in tmux and auto-cleaned after 5s

## Layout constraints
- board columns are weight-based (equal widths)
- bordered panels do not use explicit `Height()`
- all user-visible lines are truncated before render
- panel content height accounts for borders (`-2`)

## Modes
- `board`
- `help`
- `search`
- `filter`
- `create`
- `edit`
- `prompt`
- `dep_list`
- `confirm_delete`
- `tmux_picker`
