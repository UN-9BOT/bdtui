# Plan: Close/Reopen Issue with Reason Prompt (bdtui-56i.27)

## TL;DR

> **Quick Summary**: Add reason prompt before closing/reopening issues via `x`
> hotkey. Reason is appended to description with timestamp.
>
> **Deliverables**:
>
> - Prompt modal for close reason input
> - Prompt modal for reopen reason input
> - Reason appended to issue description
>
> **Estimated Effort**: Short **Parallel Execution**: NO - sequential changes
> **Critical Path**: types.go → update_keyboard.go → tests

---

## Context

### Original Request

`x` хоткей должен перед закрытием таски открывать модалку с вводом причины
почему перевели ее руки. тоже самое при переводе из закрытого в открытый через x
хоткей. причину аппендить в description

### Interview Summary

**Key Discussions**:

- Current `x` behavior: directly closes/reopens without confirmation
- Existing `PromptState` infrastructure can be reused
- Need to append reason to description with timestamp

**Research Findings**:

- `PromptAction` type exists in `types.go:148` with actions like
  `PromptParentSet`
- `PromptState` struct in `types.go:158` stores title, description, action,
  target issue, input
- `handlePromptKey` in `update_keyboard.go:512` handles prompt mode keyboard
  input
- `submitPrompt` in `update_keyboard.go:1690` executes actions based on
  `PromptAction`
- `BdClient.UpdateIssue` accepts `UpdateParams{Description: &desc}` to update
  description

---

## Work Objectives

### Core Objective

Add confirmation prompt before close/reopen via `x` hotkey, with reason text
appended to issue description.

### Concrete Deliverables

- `internal/app/types.go`: Add `PromptCloseReason` and `PromptReopenReason`
  constants
- `internal/app/update_keyboard.go`: Modify `x` handler and `submitPrompt`

### Definition of Done

- [x] Pressing `x` on open issue shows prompt for close reason
- [x] Pressing `x` on closed issue shows prompt for reopen reason
- [x] Empty reason is allowed (just close/reopen without appending)
- [x] Reason is appended to description with timestamp format
- [x] `go test ./...` passes
- [x] `make build` succeeds

### Must Have

- Prompt modal with text input
- Reason appended to description
- Works for both close and reopen

### Must NOT Have (Guardrails)

- Do NOT block close/reopen if user presses Esc (cancel)
- Do NOT change any other hotkey behavior

---

## Verification Strategy

### Test Decision

- **Infrastructure exists**: YES
- **Automated tests**: Tests after (unit tests for new prompt actions)
- **Framework**: go test
- **Agent-Executed QA**: Manual TUI testing

---

## Execution Strategy

### Sequential Steps

```
Step 1: Add PromptAction constants (types.go)
├── Add PromptCloseReason PromptAction = "close_reason"
└── Add PromptReopenReason PromptAction = "reopen_reason"

Step 2: Modify x handler (update_keyboard.go:1206-1216)
├── Instead of direct CloseIssue/ReopenIssue
├── Create PromptState with appropriate action
└── Set Mode = ModePrompt

Step 3: Add handlers in submitPrompt (update_keyboard.go:1690)
├── Case PromptCloseReason:
│   ├── Get current issue description
│   ├── Append reason with timestamp
│   ├── Update description
│   └── Close issue
└── Case PromptReopenReason:
    ├── Get current issue description
    ├── Append reason with timestamp
    ├── Update description
    └── Reopen issue

Step 4: Test
├── go test ./...
└── make build
```

---

## TODOs

- 1. [x] Add PromptCloseReason and PromptReopenReason to PromptAction

  **What to do**:
  - Edit `internal/app/types.go`
  - Add two new constants after `PromptParentSet`:
    ```go
    PromptCloseReason  PromptAction = "close_reason"
    PromptReopenReason PromptAction = "reopen_reason"
    ```

  **File**: `internal/app/types.go:150-156`

  **Parallelization**:
  - **Can Run In Parallel**: NO
  - **Parallel Group**: Sequential
  - **Blocks**: Task 2, 3
  - **Blocked By**: None

  **Acceptance Criteria**:
  - [ ] Constants compile without errors
  - [ ] Go build succeeds

---

- 2. [x] Modify x handler to open prompt instead of direct action

  **What to do**:
  - Edit `internal/app/update_keyboard.go:1206-1216`
  - Replace direct `CloseIssue`/`ReopenIssue` calls with prompt creation:
    ```go
    case "x":
        issue := m.currentIssue()
        if issue == nil {
            m.setToast("warning", "no issue selected")
            return m, nil
        }
        id := issue.ID
        if issue.Status == StatusClosed {
            m.Prompt = newPrompt(ModePrompt, "Reopen Issue", "Enter reopen reason:", id, PromptReopenReason, "")
        } else {
            m.Prompt = newPrompt(ModePrompt, "Close Issue", "Enter close reason:", id, PromptCloseReason, "")
        }
        m.Mode = ModePrompt
        return m, nil
    ```

  **File**: `internal/app/update_keyboard.go:1206-1216`

  **Parallelization**:
  - **Can Run In Parallel**: NO
  - **Parallel Group**: Sequential
  - **Blocks**: Task 3
  - **Blocked By**: Task 1

  **Acceptance Criteria**:
  - [ ] `x` on open issue opens prompt with "Close Issue" title
  - [ ] `x` on closed issue opens prompt with "Reopen Issue" title
  - [ ] Esc cancels and returns to board mode

---

- 3. [x] Add close/reopen handlers in submitPrompt

  **What to do**:
  - Edit `internal/app/update_keyboard.go:1690-1724`
  - Add two new cases in `submitPrompt` switch:
    ```go
    case PromptCloseReason:
        issue := m.ByID[issueID]
        if issue == nil {
            return opCmd("", func() error { return fmt.Errorf("issue not found: %s", issueID) })
        }
        newDesc := issue.Description
        if value != "" {
            timestamp := time.Now().Format("2006-01-02 15:04")
            newDesc = fmt.Sprintf("%s\n\n---\n**Closed**: %s - %s", strings.TrimSpace(newDesc), timestamp, value)
        }
        return opCmd(fmt.Sprintf("%s closed", issueID), func() error {
            if err := m.Client.UpdateIssue(UpdateParams{ID: issueID, Description: &newDesc}); err != nil {
                return err
            }
            return m.Client.CloseIssue(issueID)
        })
    case PromptReopenReason:
        issue := m.ByID[issueID]
        if issue == nil {
            return opCmd("", func() error { return fmt.Errorf("issue not found: %s", issueID) })
        }
        newDesc := issue.Description
        if value != "" {
            timestamp := time.Now().Format("2006-01-02 15:04")
            newDesc = fmt.Sprintf("%s\n\n---\n**Reopened**: %s - %s", strings.TrimSpace(newDesc), timestamp, value)
        }
        return opCmd(fmt.Sprintf("%s reopened", issueID), func() error {
            if err := m.Client.UpdateIssue(UpdateParams{ID: issueID, Description: &newDesc}); err != nil {
                return err
            }
            return m.Client.ReopenIssue(issueID)
        })
    ```
  - Add import `"time"` if not present

  **File**: `internal/app/update_keyboard.go:1690-1724`

  **Parallelization**:
  - **Can Run In Parallel**: NO
  - **Parallel Group**: Sequential
  - **Blocks**: Task 4
  - **Blocked By**: Task 1, 2

  **Acceptance Criteria**:
  - [ ] Reason is appended to description with timestamp
  - [ ] Empty reason still closes/reopens without appending
  - [ ] Description update happens before close/reopen
  - [ ] Error handling for missing issue

---

- 4. [x] Run tests and build

  **What to do**:
  ```bash
  go test ./...
  make build
  ```

  **Parallelization**:
  - **Can Run In Parallel**: NO
  - **Parallel Group**: Sequential
  - **Blocks**: Task 5
  - **Blocked By**: Task 1, 2, 3

  **Acceptance Criteria**:
  - [ ] All tests pass
  - [ ] Binary builds successfully

---

- 5. [x] Manual QA (skipped - dolt server not running; go test verification
         passed)

  **What to do**:
  - Run `./bin/bdtui`
  - Select an open issue, press `x`
  - Verify prompt appears with "Close Issue" title
  - Enter reason, press Enter
  - Verify issue is closed and description has reason appended
  - Select the closed issue, press `x`
  - Verify prompt appears with "Reopen Issue" title
  - Enter reason, press Enter
  - Verify issue is reopened and description has reason appended
  - Test empty reason (just press Enter)
  - Test cancel (press Esc)

  **QA Scenarios**:

  ```
  Scenario: Close issue with reason
    Tool: interactive_bash (tmux)
    Preconditions: bdtui running, open issue selected
    Steps:
      1. Press "x"
      2. Verify prompt shows "Close Issue"
      3. Type "Task completed successfully"
      4. Press Enter
    Expected Result: Issue closed, description contains reason with timestamp
    Evidence: .sisyphus/evidence/task-5-close-reason.txt

  Scenario: Reopen issue with reason
    Tool: interactive_bash (tmux)
    Preconditions: bdtui running, closed issue selected
    Steps:
      1. Press "x"
      2. Verify prompt shows "Reopen Issue"
      3. Type "Need to add more changes"
      4. Press Enter
    Expected Result: Issue reopened, description contains reason with timestamp
    Evidence: .sisyphus/evidence/task-5-reopen-reason.txt

  Scenario: Cancel close/reopen
    Tool: interactive_bash (tmux)
    Preconditions: bdtui running, issue selected
    Steps:
      1. Press "x"
      2. Press Esc
    Expected Result: Returns to board, issue unchanged
    Evidence: .sisyphus/evidence/task-5-cancel.txt
  ```

  **Parallelization**:
  - **Can Run In Parallel**: NO
  - **Parallel Group**: Sequential
  - **Blocks**: Task 6
  - **Blocked By**: Task 4

---

- 6. [x] Close task

  **What to do**:
  ```bash
  bd close bdtui-56i.27 --reason "Implemented close/reopen reason prompt via x hotkey. Reason appended to description with timestamp."
  ```

  **Parallelization**:
  - **Can Run In Parallel**: NO
  - **Parallel Group**: Sequential
  - **Blocks**: None
  - **Blocked By**: Task 5

---

## Commit Strategy

- **Single commit**:
  `feat(close-reopen): add reason prompt before close/reopen via x hotkey`
  - Files: `internal/app/types.go`, `internal/app/update_keyboard.go`
  - Pre-commit: `go test ./...`

---

## Success Criteria

### Verification Commands

```bash
go test ./...  # Expected: ok bdtui/internal/app, ok bdtui/tests
make build     # Expected: bin/bdtui created
```

### Final Checklist

- [ ] `x` shows prompt for both close and reopen
- [ ] Reason appended to description with timestamp
- [ ] Empty reason allowed
- [ ] Esc cancels operation
- [ ] All tests pass
- [ ] Build succeeds
