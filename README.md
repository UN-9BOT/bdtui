# bdtui

A Go/Bubble Tea TUI inspired by `bdui`, focused on:
- Kanban-only workflow
- keyboard-first issue management (no `:` command bar)
- full task management via `bd` CLI (CRUD + dependencies)

## Run

```bash
make build
./bin/bdtui
```

## Install from GitHub Releases (macOS/Linux)

1. Open the Releases page and choose the desired generated tag (`YYYY.MM.DD-pr<PR_NUMBER>-<MERGE_SHA7>`).
2. Download one binary matching your platform:
   - `bdtui-darwin-amd64`
   - `bdtui-darwin-arm64`
   - `bdtui-linux-amd64`
   - `bdtui-linux-arm64`
3. Download `checksums.txt` from the same release.
4. Verify checksum before running:

```bash
sha256sum -c checksums.txt --ignore-missing
chmod +x bdtui-<os>-<arch>
./bdtui-<os>-<arch>
```

Example for Linux amd64:

```bash
sha256sum bdtui-linux-amd64
grep "bdtui-linux-amd64" checksums.txt
chmod +x bdtui-linux-amd64
./bdtui-linux-amd64
```

## Build from source

Prerequisites:
- Go toolchain version from `go.mod`
- `bd` CLI in `PATH`

Commands:

```bash
git clone <repo-url>
cd bdtui
make test
make build
./bin/bdtui
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
- `Left click` - select issue in board; click ghost parent row to focus parent (board mode only)
- `Enter` / `Space` - focus details panel for selected issue
- details mode: `j/k` or `↑/↓` scroll, `d` open description, `n` open notes, `Ctrl+X` external edit, `Esc` close

### Issue Actions
- `n` - create issue
- `N` - create issue with `parent` prefilled from selected issue
- `b` - create blocked issue from selected issue (selected issue becomes blocker)
- `e` - edit selected issue
- `Ctrl+X` (board) - open selected issue in `$EDITOR`, then return to `Edit Issue`
- `Ctrl+X` (form) - open form fields in `$EDITOR` as Markdown with YAML frontmatter (`--- ... ---`)
- `d` - delete (preview cascade + confirm)
- `x` - close/reopen
- `p/P` - cycle priority forward/back
- `s/S` - cycle status forward/back
- `z` - toggle hide/show children
- `y` - copy selected issue id to clipboard

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
- `/` or `f` - focus search
- `Ctrl+F` - expand filters (in search mode)
- `c` - clear search and filters
- `Ctrl+C` - clear search and filters

### Dependencies (`g` leader)
- `g B` - blocker picker (`Space` toggle, `Enter/Esc` apply)
- `g p` - interactive parent picker (`↑/↓`, `Enter`)
- `g u` - jump to parent
- `g P` - clear parent
- `g d` - dependencies list
- `g D` - toggle dim override (`auto → bright → dim → auto`)
- `g o` - toggle sort mode (`status_date_only` / `priority_then_status_date`)

### Tmux (`t` leader)
- `t s` - send selected issue to attached tmux target
- `t S` - pick tmux target, then send selected issue
- `t a` - attach/change tmux target without sending
- `t d` - detach current tmux target

On first send, bdtui opens a tmux target picker and then pastes one of:
- `skill $beads start implement task <issue-id>`
- `skill $beads start implement task <issue-id> (epic <parent-id>)` when parent is an epic

In tmux picker, current cursor target is live-marked in tmux (`M`), and mark auto-clears 5 seconds after picker exit.

### Misc
- `r` - refresh
- `?` - help
- `q` - quit

## Notes

- Data is read and mutated only via `bd` binary.
- `blocked` column is derived automatically for `open` issues with unresolved blockers.
- if an issue has parent(s) in other status columns, those parents are shown above it as dimmed ghost tree rows.
- Release/change policy references:
  - [LICENSE](./LICENSE) (GNU GPL v3)
  - [CHANGELOG.md](./CHANGELOG.md) (date-based release tags with PR/SHA suffix)
  - [SECURITY.md](./SECURITY.md)
  - [CONTRIBUTING.md](./CONTRIBUTING.md)
  - [docs/RELEASE_RUNBOOK.md](./docs/RELEASE_RUNBOOK.md)
  - [docs/POST_RELEASE_CHECKLIST.md](./docs/POST_RELEASE_CHECKLIST.md)
- Watcher is polling-based (`bd list --json` + hash compare).
- Repository layout overview: [docs/STRUCTURE.md](./docs/STRUCTURE.md)
- Dashboard sort mode is persisted in beads kv (`bdtui.sort_mode`):
  - `status_date_only`: `updated_at` desc, then id
  - `priority_then_status_date`: priority asc, then `updated_at` desc, then id
- Editor mode (`Ctrl+X`) uses YAML frontmatter:
  - `--- ... ---` for fields (`title/status/priority/type/parent`)
  - text after closing `---` is interpreted as multiline `description`