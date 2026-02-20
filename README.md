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

## Горячие клавиши

### Навигация
- `j/k` или `↑/↓` - выбор задачи в колонке
- `h/l` или `←/→` - смена колонки
- `0` / `G` - первая / последняя задача
- `Enter` / `Space` - показать детали

### Задачи
- `n` - create issue
- `e` - edit selected issue
- `d` - delete (preview + confirm)
- `x` - close/reopen
- `p` - cycle priority
- `s` - cycle status
- `a` - quick assignee
- `Shift+L` - quick labels

### Поиск/фильтры
- `/` - поиск
- `f` - фильтры
- `c` - сброс поиска и фильтров

### Dependencies (`g` leader)
- `g b` - add blocker
- `g B` - remove blocker
- `g p` - set parent
- `g P` - clear parent
- `g d` - dependencies list

### Прочее
- `r` - refresh
- `?` - help
- `q` / `Ctrl+C` - quit

## Примечания

- Данные читаются и изменяются только через `bd` бинарь.
- Колонка `blocked` рассчитывается автоматически для `open` задач с незакрытыми блокерами.
- Watcher реализован polling-циклом (`bd list --json` + hash compare).
