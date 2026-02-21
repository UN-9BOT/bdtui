# bdtui architecture

## Scope
- single view: Kanban (open / in_progress / blocked / closed)
- keyboard-only task management
- backend: `bd CLI` only
- plugin runtime: включаемые расширения (первый plugin: `tmux`)

## Core files
- `main.go` - entrypoint
- `config.go` - flags
- `finder.go` - `.beads` discovery
- `bd_client.go` - wrapper над `bd` commands
- `model.go` - state + layout + data grouping
- `update.go` - message dispatcher
- `update_keyboard.go` - key routing and actions
- `view.go` - render board and modals
- `forms.go` - create/edit/filter form logic
- `styles.go` - lipgloss styles
- `keymap.go` - centralized hotkeys
- `plugins.go` - plugin registry + enable/disable toggles
- `plugin_tmux.go` - tmux plugin (target picker + buffer write)

## Data flow
1. find `.beads`
2. load issues via `bd list --json`
3. normalize dependencies and derived status
4. render columns
5. perform mutations via `bd` write commands
6. reload issues after each mutation
7. plugin-actions выполняются отдельно (без reload), например `Y` -> вставка ID в tmux pane

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
