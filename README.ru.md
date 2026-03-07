# bdtui

TUI на Go/Bubble Tea, вдохновлённый `bdui`, с фокусом на:
- Канбан-воркфлоу
- управление задачами с клавиатуры (без командной строки `:`)
- полный CRUD + зависимости через CLI `bd`

## Запуск

```bash
make build
./bin/bdtui
```

## Установка из GitHub Releases (macOS/Linux)

1. Откройте страницу Releases и выберите нужный тег (`YYYY.MM.DD-pr<PR_NUMBER>-<MERGE_SHA7>`).
2. Скачайте бинарник под вашу платформу:
   - `bdtui-darwin-amd64`
   - `bdtui-darwin-arm64`
   - `bdtui-linux-amd64`
   - `bdtui-linux-arm64`
3. Скачайте `checksums.txt` из того же релиза.
4. Проверьте контрольную сумму перед запуском:

```bash
sha256sum -c checksums.txt --ignore-missing
chmod +x bdtui-<os>-<arch>
./bdtui-<os>-<arch>
```

Пример для Linux amd64:

```bash
sha256sum bdtui-linux-amd64
grep "bdtui-linux-amd64" checksums.txt
chmod +x bdtui-linux-amd64
./bdtui-linux-amd64
```

## Сборка из исходников

Требования:
- Go toolchain версии из `go.mod`
- CLI `bd` в `PATH`

Команды:

```bash
git clone <repo-url>
cd bdtui
make test
make build
./bin/bdtui
```

Опции:
- `--beads-dir /abs/path/to/.beads`
- `--no-watch`
- `--plugins tmux,-foo` (включить/выключить плагины, `tmux` включён по умолчанию)

## Горячие клавиши

### Навигация
- `j/k` или `↑/↓` — выбор задачи в текущей колонке
- `h/l` или `←/→` — переключение колонок
- `0` / `G` — первая / последняя задача в колонке
- `Левый клик` — выбор задачи на доске; клик по ghost-родителю фокусирует родителя (только в режиме доски)
- `Enter` / `Space` — фокус панели деталей выбранной задачи
- режим деталей: `j/k` или `↑/↓` скролл, `d` открыть описание, `n` открыть заметки, `Ctrl+X` внешний редактор, `Esc` закрыть

### Действия с задачами
- `n` — создать задачу
- `N` — создать задачу с `parent` из выбранной задачи
- `b` — создать заблокированную задачу от выбранной (выбранная становится блокером)
- `e` — редактировать выбранную задачу
- `Ctrl+X` (доска) — открыть выбранную задачу в `$EDITOR`, затем вернуться в `Edit Issue`
- `Ctrl+X` (форма) — открыть поля формы в `$EDITOR` как Markdown с YAML frontmatter (`--- ... ---`)
- `d` — удалить (превью каскада + подтверждение)
- `x` — закрыть/открыть задачу
- `p/P` — цикл приоритета вперёд/назад
- `s/S` — цикл статуса вперёд/назад
- `z` — скрыть/показать дочерние задачи
- `y` — скопировать id выбранной задачи в буфер обмена

`parent` в Create/Edit выбирается интерактивно:
- закрытые задачи исключены из кандидатов
- сортировка кандидатов: тип задачи (`epic`, `feature`, `task`, ...), затем приоритет
- боковая панель выбора parent показывает `title` и цветные метаданные кандидатов

Поведение `Create Issue`:
- `↑/↓` переключение полей
- `Tab/Shift+Tab` цикл enum-полей (`status`, `priority`, `type`, `parent`)
- `Enter` сохраняет
- `Esc` закрывает форму если `title` пустой; иначе сохраняет

### Поиск / Фильтры
- `/` или `f` — фокус поиска
- `Ctrl+F` — развернуть фильтры (в режиме поиска)
- `c` — очистить поиск и фильтры
- `Ctrl+C` — очистить поиск и фильтры

### Зависимости (leader `g`)
- `g B` — выбор блокеров (`Space` переключить, `Enter/Esc` применить)
- `g p` — интерактивный выбор parent (`↑/↓`, `Enter`)
- `g u` — перейти к parent
- `g P` — очистить parent
- `g d` — список зависимостей
- `g D` — переключить dim override (`auto → bright → dim → auto`)
- `g o` — переключить режим сортировки (`status_date_only` / `priority_then_status_date`)

### Tmux (leader `t`)
- `t s` — отправить выбранную задачу в прикреплённый tmux target
- `t S` — выбрать tmux target, затем отправить выбранную задачу
- `t a` — прикрепить/сменить tmux target без отправки
- `t d` — открепить текущий tmux target

При первой отправке bdtui открывает picker tmux target и вставляет одну из команд:
- `skill $beads start implement task <issue-id>`
- `skill $beads start implement task <issue-id> (epic <parent-id>)` если parent — epic

В tmux picker текущий target курсора помечается в tmux (`M`), метка авто-очищается через 5 секунд после выхода из picker.

### Разное
- `r` — обновить
- `?` — справка
- `q` — выход

## Заметки

- Данные читаются и изменяются только через бинарник `bd`.
- Колонка `blocked` вычисляется автоматически для `open` задач с неразрешёнными блокерами.
- Если задача имеет parent(s) в других статус-колонках, эти parent'ы показываются над ней как приглушённые ghost-строки.
- Ссылки по релизам/политике изменений:
  - [LICENSE](./LICENSE) (GNU GPL v3)
  - [CHANGELOG.md](./CHANGELOG.md) (date-based release tags with PR/SHA suffix)
  - [SECURITY.md](./SECURITY.md)
  - [CONTRIBUTING.md](./CONTRIBUTING.md)
  - [docs/RELEASE_RUNBOOK.md](./docs/RELEASE_RUNBOOK.md)
  - [docs/POST_RELEASE_CHECKLIST.md](./docs/POST_RELEASE_CHECKLIST.md)
- Watcher работает на опросе (`bd list --json` + сравнение хешей).
- Обзор структуры репозитория: [docs/STRUCTURE.md](./docs/STRUCTURE.md)
- Режим сортировки доски сохраняется в beads kv (`bdtui.sort_mode`):
  - `status_date_only`: `updated_at` desc, затем id
  - `priority_then_status_date`: priority asc, затем `updated_at` desc, затем id
- Режим редактора (`Ctrl+X`) использует YAML frontmatter:
  - `--- ... ---` для полей (`title/status/priority/type/parent`)
  - текст после закрывающего `---` интерпретируется как многострочное `description`