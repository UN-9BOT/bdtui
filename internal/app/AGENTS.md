# internal/app — Core TUI Module

**Role:** Bubble Tea TUI implementation (Elm Architecture: Model-Update-View)

## Entry Flow

```
cmd/bdtui/main.go → app.Run(args) → parseConfig → newModel → tea.NewProgram → p.Run()
```

## Key Files

| File                 | Lines | Purpose                                                |
| -------------------- | ----- | ------------------------------------------------------ |
| `model.go`           | 862   | `Model` struct (40+ fields), state management          |
| `view.go`            | 2388  | `View()` renderer, all UI composition                  |
| `update.go`          | 306   | `Update()` message dispatcher                          |
| `update_keyboard.go` | 1724  | Keyboard routing by mode                               |
| `update_mouse.go`    | —     | Mouse click/scroll handling                            |
| `types.go`           | 314   | `Status`, `Issue`, `Mode`, `Filter`, internal messages |
| `bd_client.go`       | 456   | `BdClient` wraps `bd` CLI                              |
| `forms.go`           | 555   | `IssueForm`, `FilterForm`, validation                  |
| `config.go`          | —     | CLI flags: `--beads-dir`, `--no-watch`, `--plugins`    |
| `plugin_tmux.go`     | 404   | Tmux integration                                       |
| `external_editor.go` | —     | `$EDITOR` with YAML frontmatter                        |
| `test_api.go`        | 368   | Exports internals for `tests/`                         |

## Model Structure

```go
type Model struct {
    // Config
    Cfg       Config
    BeadsDir  string
    Client    *BdClient

    // Data
    Issues []Issue
    ByID   map[string]*Issue
    Columns map[Status][]Issue

    // Selection
    SelectedCol  int
    SelectedIdx  map[Status]int
    ScrollOffset map[Status]int

    // Modes (16 total)
    Mode    Mode  // board/details/search/create/edit/pickers/modals
    Leader  bool

    // Forms & Pickers
    Form          *IssueForm
    ParentPicker  *ParentPickerState
    BlockerPicker *BlockerPickerState

    // UI State
    ShowDetails bool
    Toast       string
    Loading     bool
    SortMode    SortMode
}
```

## Modes

`board` | `details` | `search` | `create` | `edit` | `parent_picker` |
`blocker_picker` | `tmux_picker` | `dep_list` | `confirm_delete` |
`description_preview` | `notes_preview` | `help` | `filter` | `prompt` |
`confirm_closed_parent_create`

## Message Types

- `loadedMsg` — Issues loaded from `bd list`
- `opMsg` — Operation completed (create/update/delete)
- `pluginMsg` — Plugin operation result
- `tickMsg` — 2-second timer for toast expiry
- `tea.KeyMsg` / `tea.MouseMsg` — Input events

## Patterns

**Adding a new keybinding:**

1. Add to `update_keyboard.go` in appropriate mode handler
2. Update `internal/ui/keymap.go` for help text

**Adding a new mode:**

1. Add to `Mode` enum in `types.go`
2. Add handler in `update.go` dispatcher
3. Add render case in `view.go`
4. Add keyboard routing in `update_keyboard.go`

**Adding a new bd CLI operation:**

1. Add method to `BdClient` in `bd_client.go`
2. Create internal message type in `types.go`
3. Handle in `update.go` dispatcher

## Conventions

- All state changes through `Update()` — no direct mutation
- Use `tea.Cmd` for async operations (bd CLI calls, timers)
- Toast messages via `setToast(kind, msg)`
- Form validation before `submitForm()`
