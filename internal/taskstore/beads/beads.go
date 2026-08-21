// Package beads is the Beads (`bd`) adapter for the taskstore.TaskStore
// contract. It is the MVP implementation the orchestrator uses to resolve
// and synchronize high-level task lifecycle state.
//
// Mapping between TaskStore statuses and Beads statuses is fixed:
//
//	taskstore.Todo       ->  Beads "open"
//	taskstore.InProgress ->  Beads "in_progress"
//	taskstore.Done       ->  Beads "closed"
//	taskstore.Blocked    ->  Beads "blocked"
//
// The rest of the field set (title, description, priority, issue_type) is
// passed through unchanged. The Beads CLI is the only side-effect: any
// reachability failure surfaces as taskstore.ErrTaskStoreUnavailable so the
// orchestrator can refuse to launch a Run.
package beads

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"bdtui/internal/taskstore"
)

// Client is the subset of the `bd` CLI that the TaskStore needs. It is
// exposed so tests can inject a fake without shelling out.
type Client interface {
	// Show returns the raw JSON output of `bd show <id> --json`. A missing
	// task is reported by returning a non-nil error that wraps
	// taskstore.ErrTaskNotFound.
	Show(ctx context.Context, id string) ([]byte, error)
	// Update returns the raw JSON output of `bd update <id> --status <s> --json`.
	Update(ctx context.Context, id string, status string) ([]byte, error)
	// Claim returns the raw JSON output of `bd update <id> --claim --json`.
	// The claim is atomic at the Beads level: it transitions todo ->
	// in_progress and assigns the issue to the current user in a single
	// write, so two concurrent Claim calls cannot both observe a "free"
	// task. The CLI rejects the claim with a non-zero exit when the task
	// is already in_progress and claimed by someone else; the adapter
	// surfaces that as ErrTaskAlreadyClaimed.
	Claim(ctx context.Context, id string) ([]byte, error)
}

// CLI is the default Client that shells out to the `bd` binary. Dir
// controls which Beads repo is queried; pass the project's .beads root.
type CLI struct {
	Bin string // executable path; defaults to "bd"
	Dir string // working directory; non-empty value is passed as cmd.Dir
}

// Compile-time check that CLI satisfies Client.
var _ Client = (*CLI)(nil)

// New returns a CLI that invokes `bd` from the given directory. The
// directory must be the project root (where `.beads/` lives); the adapter
// only needs cd access to that root.
func New(dir string) *CLI {
	return &CLI{Bin: "bd", Dir: dir}
}

// Show shells out to `bd show <id> --json`.
func (c *CLI) Show(ctx context.Context, id string) ([]byte, error) {
	return c.run(ctx, "show", id, "--json")
}

// Update shells out to `bd update <id> --status <status> --json`.
func (c *CLI) Update(ctx context.Context, id string, status string) ([]byte, error) {
	return c.run(ctx, "update", id, "--status", status, "--json")
}

// Claim shells out to `bd update <id> --claim --json`. The flag is the
// only supported atomic claim primitive in Beads; the predecessor
// Show + Update --status in_progress pair was racy by construction.
//
// The CLI distinguishes three failure modes:
//
//   - task not found        -> wraps taskstore.ErrTaskNotFound
//   - already claimed / not claimable -> wraps taskstore.ErrTaskAlreadyClaimed
//   - backend unreachable / other -> wraps taskstore.ErrTaskStoreUnavailable
//
// The distinction is required because the caller must reject an
// "already claimed" CreateRun with codes.AlreadyExists (a duplicate
// claim retry) but treat a missing task as a true failure.
func (c *CLI) Claim(ctx context.Context, id string) ([]byte, error) {
	bin := c.Bin
	if bin == "" {
		bin = "bd"
	}
	cmd := exec.CommandContext(ctx, bin, "update", id, "--claim", "--json")
	if c.Dir != "" {
		cmd.Dir = c.Dir
	}
	var out, errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("%w: %w", taskstore.ErrTaskStoreUnavailable, ctx.Err())
		}
		if looksLikeNotFound(errOut.String(), out.Bytes()) {
			return nil, fmt.Errorf("%w: %s", taskstore.ErrTaskNotFound, strings.TrimSpace(errOut.String()))
		}
		if looksLikeAlreadyClaimed(errOut.String(), out.Bytes()) {
			return nil, fmt.Errorf("%w: %s", taskstore.ErrTaskAlreadyClaimed, strings.TrimSpace(errOut.String()))
		}
		return nil, fmt.Errorf("%w: bd update --claim: %v: %s",
			taskstore.ErrTaskStoreUnavailable, err, strings.TrimSpace(errOut.String()))
	}
	return out.Bytes(), nil
}

func (c *CLI) run(ctx context.Context, args ...string) ([]byte, error) {
	bin := c.Bin
	if bin == "" {
		bin = "bd"
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	if c.Dir != "" {
		cmd.Dir = c.Dir
	}
	var out, errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut
	err := cmd.Run()
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("%w: %w", taskstore.ErrTaskStoreUnavailable, ctx.Err())
		}
		if looksLikeNotFound(errOut.String(), out.Bytes()) {
			return nil, fmt.Errorf("%w: %s", taskstore.ErrTaskNotFound, strings.TrimSpace(errOut.String()))
		}
		return nil, fmt.Errorf("%w: bd %s: %v: %s",
			taskstore.ErrTaskStoreUnavailable, strings.Join(args, " "), err, strings.TrimSpace(errOut.String()))
	}
	return out.Bytes(), nil
}

// looksLikeNotFound recognises the small handful of phrases `bd` writes
// when asked to operate on a missing id. Both stderr and stdout are
// considered because bd >=1.2 writes the canonical "no issues found"
// message to stderr and a JSON error wrapper to stdout.
func looksLikeNotFound(stderr string, stdout []byte) bool {
	s := strings.ToLower(stderr)
	if strings.Contains(s, "no issue") || strings.Contains(s, "not found") {
		return true
	}
	out := strings.ToLower(string(bytes.TrimSpace(stdout)))
	if strings.Contains(out, `"error"`) && strings.Contains(out, "no issue") {
		return true
	}
	return false
}

// looksLikeAlreadyClaimed recognises the "claim" failure modes:
//   - non-claimable status (closed / pinned / hooked)
//   - claimed by another user
//
// bd >=1.2 wording is "issue not claimable: status X" or "already
// claimed by <user>"; both are surfaces of the same end-state from the
// adapter's perspective: another writer holds the task.
func looksLikeAlreadyClaimed(stderr string, stdout []byte) bool {
	s := strings.ToLower(stderr)
	if strings.Contains(s, "not claimable") || strings.Contains(s, "already claimed") {
		return true
	}
	out := strings.ToLower(string(bytes.TrimSpace(stdout)))
	if strings.Contains(out, `"error"`) && (strings.Contains(out, "not claimable") || strings.Contains(out, "already claimed")) {
		return true
	}
	return false
}

// Store is the Beads-backed implementation of taskstore.TaskStore.
type Store struct {
	Client Client
}

// NewStore builds a Store backed by the given Client.
func NewStore(c Client) *Store {
	return &Store{Client: c}
}

// Compile-time check that Store satisfies taskstore.TaskStore.
var _ taskstore.TaskStore = (*Store)(nil)

// bdTask mirrors the subset of the `bd show <id> --json` payload we care
// about. Beads may add fields; we ignore unknown fields and tolerate
// missing optional ones.
type bdTask struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Status      string `json:"status"`
	Priority    int    `json:"priority"`
	IssueType   string `json:"issue_type"`
	UpdatedAt   string `json:"updated_at"`
}

// toTask converts a parsed bdTask into the taskstore.Task contract. The
// status is mapped through fromBeadsStatus; unknown statuses yield an
// error so callers can tell configured-but-unsupported backends from a
// transient failure.
func toTask(b bdTask) (*taskstore.Task, error) {
	status, err := fromBeadsStatus(b.Status)
	if err != nil {
		return nil, err
	}
	t := &taskstore.Task{
		ID:          b.ID,
		Title:       b.Title,
		Description: b.Description,
		Status:      status,
		Priority:    b.Priority,
		IssueType:   b.IssueType,
		SnapshotAt:  time.Now().UTC(),
	}
	if b.UpdatedAt != "" {
		if ts, perr := time.Parse(time.RFC3339, b.UpdatedAt); perr == nil {
			t.SnapshotAt = ts.UTC()
		}
	}
	return t, nil
}

// parseTask accepts both the list-form (`bd show <id> --json` returns a
// 1-element list for a single id) and the bare-object form (some bd
// versions/configs return a single object). It refuses empty payloads and
// reports malformed JSON as ErrTaskStoreUnavailable since that is
// indicative of a protocol mismatch rather than a missing task.
func parseTask(raw []byte) (*taskstore.Task, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return nil, fmt.Errorf("%w: empty response", taskstore.ErrTaskNotFound)
	}
	switch raw[0] {
	case '[':
		var list []bdTask
		if err := json.Unmarshal(raw, &list); err != nil {
			return nil, fmt.Errorf("%w: malformed task list: %v", taskstore.ErrTaskStoreUnavailable, err)
		}
		if len(list) == 0 {
			return nil, fmt.Errorf("%w: empty task list", taskstore.ErrTaskNotFound)
		}
		return toTask(list[0])
	case '{':
		var one bdTask
		if err := json.Unmarshal(raw, &one); err != nil {
			return nil, fmt.Errorf("%w: malformed task: %v", taskstore.ErrTaskStoreUnavailable, err)
		}
		if one.ID == "" {
			return nil, errors.New("beads: task missing id")
		}
		return toTask(one)
	default:
		return nil, fmt.Errorf("%w: unexpected payload: %s", taskstore.ErrTaskStoreUnavailable, string(raw))
	}
}

// Get implements taskstore.TaskStore.
func (s *Store) Get(ctx context.Context, id string) (*taskstore.Task, error) {
	raw, err := s.Client.Show(ctx, id)
	if err != nil {
		return nil, err
	}
	return parseTask(raw)
}

// Claim implements taskstore.TaskStore via the atomic `bd update --claim`
// primitive. The CLI is the single mutation: it rejects the claim when
// the task is already in_progress and claimed by someone else, and is
// idempotent for the same user. The returned Task is the post-claim
// snapshot taken from the claim response itself, so the snapshot is
// congruent with what the backend now reports.
func (s *Store) Claim(ctx context.Context, id string) (*taskstore.Task, error) {
	raw, err := s.Client.Claim(ctx, id)
	if err != nil {
		// Map the three failure modes to the contract's sentinel errors.
		// The Claim method is the only one that must distinguish
		// "already claimed" from "task missing" from "backend down".
		lower := strings.ToLower(err.Error())
		if strings.Contains(lower, "no issue") || strings.Contains(lower, "not found") {
			return nil, fmt.Errorf("%w: %s", taskstore.ErrTaskNotFound, id)
		}
		if strings.Contains(lower, "not claimable") || strings.Contains(lower, "already claimed") {
			return nil, fmt.Errorf("%w: %s", taskstore.ErrTaskAlreadyClaimed, id)
		}
		return nil, err
	}
	snap, err := parseTask(raw)
	if err != nil {
		return nil, err
	}
	// The Beads response carries the post-claim state; surface it as
	// in_progress even if the CLI labels it differently (e.g. "open"
	// when the local Beads config does not include "in_progress"). The
	// TaskStore contract is the post-claim canonical state.
	snap.Status = taskstore.TaskInProgress
	snap.SnapshotAt = time.Now().UTC()
	return snap, nil
}

// SyncTerminal implements taskstore.TaskStore. The outcome is mapped to a
// TaskStatus via MapRunOutcomeToTaskStatus and then to a Beads status via
// toBeadsStatus. The actual write is a single `bd update` call.
func (s *Store) SyncTerminal(ctx context.Context, id string, outcome taskstore.RunOutcome) error {
	status, err := taskstore.MapRunOutcomeToTaskStatus(outcome)
	if err != nil {
		return err
	}
	beadsStatus, err := toBeadsStatus(status)
	if err != nil {
		return err
	}
	_, err = s.Client.Update(ctx, id, beadsStatus)
	return err
}

// toBeadsStatus maps the taskstore vocabulary to the Beads CLI vocabulary.
func toBeadsStatus(s taskstore.TaskStatus) (string, error) {
	switch s {
	case taskstore.TaskTodo:
		return "open", nil
	case taskstore.TaskInProgress:
		return "in_progress", nil
	case taskstore.TaskDone:
		return "closed", nil
	case taskstore.TaskBlocked:
		return "blocked", nil
	default:
		return "", fmt.Errorf("%w: %s", taskstore.ErrInvalidOutcome, s)
	}
}

// fromBeadsStatus maps the Beads CLI vocabulary back to the taskstore
// vocabulary. Unknown statuses are surfaced as ErrTaskStoreUnavailable so
// the controller treats a misconfigured backend as a hard failure rather
// than silently mis-categorising the task.
func fromBeadsStatus(s string) (taskstore.TaskStatus, error) {
	switch s {
	case "open":
		return taskstore.TaskTodo, nil
	case "in_progress":
		return taskstore.TaskInProgress, nil
	case "closed":
		return taskstore.TaskDone, nil
	case "blocked":
		return taskstore.TaskBlocked, nil
	default:
		return "", fmt.Errorf("%w: unsupported beads status %q", taskstore.ErrTaskStoreUnavailable, s)
	}
}
