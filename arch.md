# bdtui architecture

## Scope
- single view: Kanban (`open` / `in_progress` / `blocked` / `closed`)
- keyboard-first task management
- backend mutations via `bd` CLI
- plugin runtime with enable/disable toggles (current built-in plugin: `herdr`)

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
- `plugin_herdr.go` - herdr plugin (target picker, send-text, tab-focus fallback)

## Data flow
1. find `.beads`
2. load issues via `bd list --json`
3. normalize dependencies and derived display status
4. render columns
5. perform mutations via `bd` write commands
6. reload issues after each mutation
7. execute plugin actions separately (no issue reload), e.g. `t s` -> herdr send
8. in `mux_picker`, selection is local to bdtui; no live external preview/marking

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
- `mux_picker`
