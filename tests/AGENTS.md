# tests — Black-Box Integration Tests

**Package:** `bdtui_test` (external test package) **Count:** 38 test files

## Structure

```
tests/
├── compat_test.go              # Type aliases for internal types
├── update_keyboard_*_test.go   # 11 files — keyboard handling
├── view_*_test.go              # 8 files — rendering
├── forms_*_test.go             # 3 files — form behavior
├── confirm_*_test.go           # 2 files — modals
├── plugin_*_test.go            # 1 file — tmux plugin
└── model_*_test.go             # misc unit tests
```

## Test API

`internal/app/test_api.go` exports internal functions:

- `NewModel()`, `ParseConfig()`, `NewIssueFormCreate()`
- Model methods: `HandleKey()`, `BuildColumnRows()`, `RenderBoard()`
- Utilities: `CycleStatus()`, `DetailLines()`, `ParseEditorContent()`

## Type Exposure

`compat_test.go` provides type aliases:

```go
import b "bdtui/internal/app"
type model = b.Model
type Status = b.Status
type Issue = b.Issue
// ... all exported types
```

## Conventions

| Convention      | Pattern                                    |
| --------------- | ------------------------------------------ |
| Package         | `package bdtui_test`                       |
| File naming     | `<feature>_<aspect>_test.go`               |
| Function naming | `Test<Function>_<Scenario>`                |
| Parallelism     | `t.Parallel()` in most tests               |
| Fixtures        | Direct struct construction (no builders)   |
| Fakes           | `fakeTmuxRunner` pattern for external deps |

## Example Test

```go
func TestHandleFormKey_EnterSubmitsForm(t *testing.T) {
    t.Parallel()
    m := newTestModel(t)
    m.Form = &b.IssueForm{Title: "Test", Create: true}

    next, _ := m.HandleKey(keyMsg(key.Enter))

    if next.Form != nil {
        t.Fatal("form should be nil after submit")
    }
}
```

## Running

```bash
make test   # go test ./...
```

## Adding New Tests

1. Create `tests/<feature>_<aspect>_test.go`
2. Use `package bdtui_test`
3. Import types via `compat_test.go` aliases
4. Use test API from `internal/app/test_api.go`
5. Call `t.Parallel()` for parallel-safe tests
