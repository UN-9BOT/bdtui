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
//
// UpdateWithLabel is the only write primitive the Beads adapter uses:
// it issues a single `bd update <id> --status X --add-label Y --json`
// command. Dolt's transaction semantics make the status and label
// update atomic together, so the Beads-side fence (current max
// generation on the task) and the write of the new generation are
// committed in one observable step. The orchestrator's per-task sync
// lock (see internal/daemon) closes the read-then-write window so two
// concurrent syncs cannot both observe the same current generation
// and race their writes; the Beads-side fence is then a sanity check
// (catches external edits to the task between the lock release and
// the next sync).
type Client interface {
	// Show returns the raw JSON output of `bd show <id> --json`. A missing
	// task must surface as ErrTaskNotFound so the controller can refuse
	// to launch a Run.
	Show(ctx context.Context, id string) ([]byte, error)
	// UpdateWithLabel returns the raw JSON output of
	// `bd update <id> --status <status> --add-label <label> --json`.
	// The status and label are committed in one Dolt transaction so
	// downstream readers always see the two values together. The
	// TaskStore adapter uses this primitive to atomically push the
	// mapped terminal status and the orch-gen-N label that represents
	// the new generation.
	UpdateWithLabel(ctx context.Context, id string, status string, label string) ([]byte, error)
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

// UpdateWithLabel shells out to
// `bd update <id> --status <status> --add-label <label> --json`. The
// status and label are committed in one Dolt transaction, so the
// downstream view always observes the two fields together. The Beads
// adapter uses this single command as the canonical write primitive
// for SyncTerminal: the generation fence (current max on the task)
// and the new generation label land atomically with the new status.
func (c *CLI) UpdateWithLabel(ctx context.Context, id string, status string, label string) ([]byte, error) {
	return c.run(ctx, "update", id, "--status", status, "--add-label", label, "--json")
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
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Status      string   `json:"status"`
	Priority    int      `json:"priority"`
	IssueType   string   `json:"issue_type"`
	UpdatedAt   string   `json:"updated_at"`
	Labels      []string `json:"labels"`
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
		Labels:      b.Labels,
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

// SyncTerminal implements taskstore.TaskStore. The outcome is mapped
// to a TaskStatus via MapRunOutcomeToTaskStatus and then to a Beads
// status via toBeadsStatus. The generation argument is the
// orchestrator's monotonic counter for the TASK (across all runs);
// the Beads adapter records it as a label on the task so the next
// sync can fence stale writes.
//
// The fence is enforced in two layers:
//
//  1. The orchestrator's per-task sync lock (see internal/daemon)
//     serializes concurrent SyncTerminal calls for the same task. The
//     lock is held only during the Beads side effect, not during the
//     outbox append. Under the lock, the read-modify-write window is
//     closed: no other writer can race its write between the read of
//     the current generation and the write of the new generation.
//
//  2. The Beads-side check (below) compares the orchestrator's
//     incoming generation against the current MAX across all
//     orch-gen-* labels on the task. If the current max is greater
//     than the incoming generation, the write is stale and is
//     rejected with ErrStaleLifecycleIntent. This catches external
//     edits to the task (e.g. a manual `bd update --add-label
//     orch-gen-9`) that would otherwise be silently overwritten.
//
// The status and label are written in a single `bd update` command
// (UpdateWithLabel) so Dolt commits both fields atomically. The
// Beads-side fence is therefore a sanity check, not the primary
// defense; the lock is.
//
// If the sync is rejected as stale, the orchestrator's per-task
// outbox row is marked done (the newer write is the authoritative
// current desired state) and no retry is queued.
func (s *Store) SyncTerminal(ctx context.Context, id string, outcome taskstore.RunOutcome, generation int64) error {
	status, err := taskstore.MapRunOutcomeToTaskStatus(outcome)
	if err != nil {
		return err
	}
	beadsStatus, err := toBeadsStatus(status)
	if err != nil {
		return err
	}
	// Read the current generation label. If the current generation
	// is greater than the one we are about to write, the intent is
	// stale and the write is skipped. We use the MAX across all
	// orch-gen-* labels so accumulation (every successful sync
	// appends a new label) cannot regress the read to a smaller
	// generation when labels are returned in insertion order.
	cur, err := s.maxGeneration(ctx, id)
	if err != nil {
		return err
	}
	if cur > generation {
		return fmt.Errorf("%w: current=%d, incoming=%d", taskstore.ErrStaleLifecycleIntent, cur, generation)
	}
	// Atomic write: status + label in one `bd update --status X
	// --add-label orch-gen-N` invocation. Dolt commits both fields
	// in one transaction, so the label and status land together and
	// downstream readers always see the pair. Errors are NOT
	// ignored: a failed label write means the new generation is not
	// recorded, so the next sync would otherwise see the old max
	// and revert the status. Returning the error surfaces the
	// failure to the caller for retry.
	if _, err := s.Client.UpdateWithLabel(ctx, id, beadsStatus, fmt.Sprintf("orch-gen-%d", generation)); err != nil {
		return err
	}
	return nil
}

// maxGeneration returns the largest orch-gen-* label on the task,
// or 0 if no such label is present. The Beads task may accumulate
// multiple orch-gen-* labels across successful sync attempts (every
// sync appends a new label), so the comparison must use the MAX,
// not the first match, for the fence to reject stale writes
// reliably. Non-orch-gen labels are ignored.
func (s *Store) maxGeneration(ctx context.Context, id string) (int64, error) {
	raw, err := s.Client.Show(ctx, id)
	if err != nil {
		return 0, err
	}
	task, err := parseTask(raw)
	if err != nil {
		return 0, err
	}
	var max int64
	for _, lab := range task.Labels {
		var n int64
		if _, err := fmt.Sscanf(lab, "orch-gen-%d", &n); err == nil {
			if n > max {
				max = n
			}
		}
	}
	return max, nil
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
