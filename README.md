# bdtui

A Go/Bubble Tea TUI inspired by `bdui`, focused on:
- Kanban-only workflow
- keyboard-first issue management (no `:` command bar)
- full task management via `bd` CLI (CRUD + dependencies)

## Run

```bash
cd ../bdtui
go build ./...
./bdtui
```

Options:
- `--beads-dir /abs/path/to/.beads`
- `--no-watch`
- `--plugins tmux,-foo` (enable/disable plugins, `tmux` is enabled by default)

## Hotkeys

### Navigation
- `j/k` or `↑/↓` - select issue in current column
- `h/l` or `←/→` - switch column
- `0` / `G` - first / last issue in column
- `Enter` / `Space` - toggle details panel for selected issue

### Issue Actions
- `n` - create issue
- `e` - edit selected issue
- `Ctrl+X` (board) - open selected issue in `$EDITOR`, then return to `Edit Issue`
- `Ctrl+X` (form) - open form fields in `$EDITOR` as Markdown with YAML frontmatter (`--- ... ---`)
- `d` - delete (preview + confirm)
- `x` - close/reopen
- `p` - cycle priority
- `s` - cycle status
- `a` - quick assignee
- `y` - copy selected issue id to clipboard
- `Y` - paste `skill $beads start task ...` command for selected issue into chosen `tmux` pane (tmux plugin)
- `Shift+L` - quick labels

`parent` in Create/Edit is selected interactively:
- closed issues are excluded from candidates
- candidate sort order: issue type (`epic`, `feature`, `task`, ...) then priority
- parent picker sidebar shows candidate `title` and colored metadata

`Create Issue` behavior:
- `↑/↓` switch fields
- `Tab/Shift+Tab` cycle enum fields (`status`, `priority`, `type`, `parent`)
- `Enter` saves
- `Esc` closes form when `title` is empty; otherwise saves

### Search / Filter
- `/` - search
- `f` - filters
- `c` - clear search and filters

### Dependencies (`g` leader)
- `g b` - add blocker
- `g B` - remove blocker
- `g p` - interactive parent picker (`↑/↓`, `Enter`)
- `g P` - clear parent
- `g d` - dependencies list

### Misc
- `r` - refresh
- `?` - help
- `q` - quit

## Notes

- Data is read and mutated only via `bd` binary.
- `blocked` column is derived automatically for `open` issues with unresolved blockers.
- Watcher is polling-based (`bd list --json` + hash compare).
- On first `Y`, bdtui opens a tmux target picker and then pastes one of:
  - `skill $beads start task <issue-id>`
  - `skill $beads start task <issue-id> (epic <parent-id>)` when parent is an epic
- In tmux picker, current cursor target is live-marked in tmux (`M`), and mark auto-clears 5 seconds after picker exit.
- Editor mode (`Ctrl+X`) uses YAML frontmatter:
  - `--- ... ---` for fields (`title/status/priority/type/assignee/labels/parent`)
  - text after closing `---` is interpreted as multiline `description`
