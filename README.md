# bdtui

Go/Bubble Tea версия `bdui` с фокусом на:
- только Kanban
- управление задачами только hotkeys (без `:`/cmd bar)
- полный task management через `bd CLI` (CRUD + deps)

## Запуск

```bash
cd ../bdtui
go build ./...
./bdtui
```

Опции:
- `--beads-dir /abs/path/to/.beads`
- `--no-watch`
- `--plugins tmux,-foo` (вкл/выкл plugins, по умолчанию `tmux` включен)

## Горячие клавиши

### Навигация
- `j/k` или `↑/↓` - выбор задачи в колонке
- `h/l` или `←/→` - смена колонки
- `0` / `G` - первая / последняя задача
- `Enter` / `Space` - расширить нижний инфо-блок по выбранной задаче

### Задачи
- `n` - create issue
- `e` - edit selected issue
- `Ctrl+X` (на board) - открыть выбранную задачу в `$EDITOR`, затем вернуться в `Edit Issue`
- `Ctrl+X` (в форме) - открыть поля в `$EDITOR` как Markdown с YAML frontmatter (`--- ... ---`)
- `d` - delete (preview + confirm)
- `x` - close/reopen
- `p` - cycle priority
- `s` - cycle status
- `a` - quick assignee
- `y` - copy selected issue id to clipboard
- `Y` - вставить selected issue id в выбранный `tmux` pane (tmux plugin)
- `Shift+L` - quick labels

`parent` в форме Create/Edit выбирается интерактивно стрелками:
- из кандидатов исключаются задачи со статусом `closed`
- сортировка кандидатов: сначала по типу (`epic`, `feature`, `task`, ...), затем по приоритету
- в режиме выбора `parent` справа показывается отдельный список кандидатов с `title` и цветными метками

Поведение в `Create Issue`:
- `↑/↓` переключают поля формы
- `Tab/Shift+Tab` делают cycle enum-полей (`status`, `priority`, `type`, `parent`)
- `Enter` и `Esc` одинаково сохраняют форму (защита от случайного `Esc`)

### Поиск/фильтры
- `/` - поиск
- `f` - фильтры
- `c` - сброс поиска и фильтров

### Dependencies (`g` leader)
- `g b` - add blocker
- `g B` - remove blocker
- `g p` - интерактивный parent picker (↑/↓, Enter)
- `g P` - clear parent
- `g d` - dependencies list

### Прочее
- `r` - refresh
- `?` - help
- `q` - quit

## Примечания

- Данные читаются и изменяются только через `bd` бинарь.
- Колонка `blocked` рассчитывается автоматически для `open` задач с незакрытыми блокерами.
- Watcher реализован polling-циклом (`bd list --json` + hash compare).
- `Y` при первом вызове открывает picker tmux target (сессия/pane), затем вставляет ID в pane через `tmux set-buffer` + `tmux paste-buffer`.
- В editor-режиме (`Ctrl+X`) используется YAML frontmatter:
  - блок `--- ... ---` для полей (`title/status/priority/type/assignee/labels/parent`)
  - текст после закрывающего `---` трактуется как `description` (можно многострочно)
