# Test Database for bdtui

This directory contains a comprehensive test database for bdtui development and
testing.

## Purpose

The test database includes issues covering all possible variations:

### Statuses (5)

- `open` - 12 issues
- `in_progress` - 8 issues
- `blocked` - 4 issues
- `closed` - 5 issues
- `tombstone` - 2 issues

### Priorities (5)

- `0` (Critical) - 3 issues
- `1` (High) - 10 issues
- `2` (Medium) - 17 issues
- `3` (Low) - 4 issues
- `4` (Backlog) - 2 issues

### Issue Types (5)

- `epic` - 5 issues (with children)
- `feature` - 8 issues
- `task` - 17 issues
- `bug` - 5 issues
- `chore` - 3 issues

### Special Test Cases

| ID                   | Description                                       |
| -------------------- | ------------------------------------------------- |
| test-001 to test-004 | Epics in different statuses                       |
| test-005 to test-008 | Features with parent (test-001) - ghost row tests |
| test-009 to test-011 | Tasks with parent (test-002) + blockers           |
| test-012 to test-016 | Bugs across all priorities and statuses           |
| test-017 to test-019 | Chores in different statuses                      |
| test-021 to test-025 | Standalone issues (no parent) in each status      |
| test-026 to test-027 | Deep nesting (L1, L2) for ghost row tests         |
| test-028             | Rich content (code blocks, lists, tables, links)  |
| test-029             | Multiple labels (8 labels)                        |
| test-030             | Very long title for truncation testing            |
| test-031             | Minimal metadata (no description, notes, labels)  |
| test-032             | Unicode test (Russian, Chinese, Japanese, emoji)  |
| test-033             | Child of closed epic (ghost from closed column)   |
| test-034 to test-038 | Parent with 4 children in different statuses      |
| test-039 to test-040 | Blocker chain                                     |
| test-041             | Empty epic (no children)                          |
| test-042             | All fields populated                              |

## Usage

```bash
# Run bdtui with test database
make test-db

# Or directly:
./bin/bdtui --beads-dir $(pwd)/tests/fixtures/testdb/.beads
```

## First-Time Setup

The test database uses bd 0.59.0 with Dolt. On first use, the Dolt database
needs to be initialized from the JSONL source:

```bash
cd tests/fixtures/testdb
bd init --from-jsonl --prefix test
```

After initialization, `make test-db` will work. The config has
`dolt.auto-start: true` so bd will auto-start the Dolt server when needed.

## Regenerating

If you need to modify the test data:

1. Edit `tests/fixtures/testdb/.beads/issues.jsonl`
2. Re-initialize the Dolt database:
   ```bash
   cd tests/fixtures/testdb
   rm -rf .beads/dolt .beads/metadata.json
   bd init --from-jsonl --prefix test
   ```
