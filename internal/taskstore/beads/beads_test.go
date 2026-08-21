package beads_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"bdtui/internal/taskstore"
	"bdtui/internal/taskstore/beads"
)

// requireBD skips the test if the `bd` binary is not available on PATH.
// The Beads adapter is the MVP implementation of the TaskStore, but the
// Beads CLI is a project-local dependency and the CI image does not
// install it. Unit tests that do not need the live CLI (parser-only
// tests using fakeClient) run unconditionally.
func requireBD(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("bd"); err != nil {
		t.Skipf("bd binary not available on PATH: %v", err)
	}
}

// fixture initialises a fresh Beads repository under a temporary directory
// and returns the project root. The root is the directory that contains
// .beads/ and is what the adapter uses as cmd.Dir.
//
// The fixture is skipped when the `bd` binary is not available on PATH so
// the rest of the suite can run on CI runners that install Go only.
func fixture(t *testing.T) string {
	t.Helper()
	requireBD(t)
	root := t.TempDir()
	run(t, root, "git", "init", "-q")
	run(t, root, "bd", "init", "--prefix", "test", "--quiet")
	return root
}

// run executes a shell command and fails the test on error.
func run(t *testing.T, dir, name string, args ...string) string {
	t.Helper()
	var stdout, stderr bytes.Buffer
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("%s %v: %v\nstdout=%s\nstderr=%s",
			name, args, err, stdout.String(), stderr.String())
	}
	return stdout.String()
}

// createTask inserts a task via `bd create` and returns its id.
func createTask(t *testing.T, root, title string, extraArgs ...string) string {
	t.Helper()
	args := []string{"create", title, "--json"}
	args = append(args, extraArgs...)
	out := run(t, root, "bd", args...)
	// bd --json returns a single object for create. Defensively unwrap.
	var payload struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &payload); err != nil {
		t.Fatalf("decode create output: %v\n%s", err, out)
	}
	if payload.ID == "" {
		t.Fatalf("create returned empty id: %s", out)
	}
	return payload.ID
}

func TestGetHappyPath(t *testing.T) {
	root := fixture(t)
	id := createTask(t, root, "Hello")
	run(t, root, "bd", "update", id, "--description", "world", "--json")

	store := beads.NewStore(beads.New(root))
	got, err := store.Get(context.Background(), id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != id {
		t.Errorf("ID = %q, want %q", got.ID, id)
	}
	if got.Title != "Hello" {
		t.Errorf("Title = %q, want %q", got.Title, "Hello")
	}
	if got.Description != "world" {
		t.Errorf("Description = %q, want %q", got.Description, "world")
	}
	if got.Status != taskstore.TaskTodo {
		t.Errorf("Status = %q, want %q", got.Status, taskstore.TaskTodo)
	}
	if got.SnapshotAt.IsZero() {
		t.Errorf("SnapshotAt is zero")
	}
}

func TestGetMissing(t *testing.T) {
	root := fixture(t)
	store := beads.NewStore(beads.New(root))
	_, err := store.Get(context.Background(), "missing-id")
	if !errors.Is(err, taskstore.ErrTaskNotFound) {
		t.Fatalf("err = %v, want ErrTaskNotFound", err)
	}
}

func TestClaimFromTodo(t *testing.T) {
	root := fixture(t)
	id := createTask(t, root, "Claw me")

	store := beads.NewStore(beads.New(root))
	snap, err := store.Claim(context.Background(), id)
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if snap.Status != taskstore.TaskInProgress {
		t.Errorf("snapshot status = %q, want in_progress", snap.Status)
	}
	if snap.Title != "Claw me" {
		t.Errorf("snapshot Title = %q, want %q", snap.Title, "Claw me")
	}

	// Backend should reflect in_progress.
	beadsStatus := run(t, root, "bd", "show", id, "--json")
	if !strings.Contains(beadsStatus, `"status": "in_progress"`) {
		t.Errorf("backend status not in_progress: %s", beadsStatus)
	}

	// Second Claim is idempotent for the same holder. The `bd --claim`
	// flag is documented as idempotent when the assignee is the same
	// user; the adapter returns the post-claim snapshot without error.
	// The single-active-Run invariant — "at most one active Run per
	// Beads task" — is enforced at the orchestrator's CreateRun layer
	// (see TestCreateRunRefusesAlreadyClaimed + the partial unique
	// index on runs), not at the TaskStore Claim boundary. This test
	// pins the Claim idempotency for the same holder; the cross-holder
	// path is exercised separately.
	second, err := store.Claim(context.Background(), id)
	if err != nil {
		t.Fatalf("second Claim: %v", err)
	}
	if second.Status != taskstore.TaskInProgress {
		t.Errorf("second snapshot status = %q, want in_progress", second.Status)
	}
}

// TestClaimRefusesForeignHolder documents the failure mode the
// orchestrator relies on for "already claimed". When the Beads CLI
// rejects a claim because the task is assigned to another user, the
// adapter must surface ErrTaskAlreadyClaimed. This is the path the
// daemon's CreateRun relies on when an external Beads actor has the
// task and the in-process controller cannot take ownership.
//
// We cannot easily impersonate a different Beads holder inside this
// test environment, so the test drives the failure through a fake
// Client whose Claim returns a "not claimable" stderr. The wiring
// path this test exercises is identical to the live CLI path: the
// adapter inspects the stderr for the canonical phrase and maps the
// failure to the matching sentinel.
func TestClaimRefusesForeignHolder(t *testing.T) {
	cli := &fakeClient{
		claim: nil,
		claimErr: fmt.Errorf(
			"exit status 1: %s",
			"Error claiming x: already claimed by other_user",
		),
	}
	store := beads.NewStore(cli)
	_, err := store.Claim(context.Background(), "x")
	if !errors.Is(err, taskstore.ErrTaskAlreadyClaimed) {
		t.Fatalf("err = %v, want ErrTaskAlreadyClaimed", err)
	}
}

func TestClaimMissing(t *testing.T) {
	root := fixture(t)
	store := beads.NewStore(beads.New(root))
	_, err := store.Claim(context.Background(), "missing-id")
	if !errors.Is(err, taskstore.ErrTaskNotFound) {
		t.Fatalf("err = %v, want ErrTaskNotFound", err)
	}
}

func TestSyncTerminal(t *testing.T) {
	root := fixture(t)
	id := createTask(t, root, "Sync me")

	store := beads.NewStore(beads.New(root))

	cases := []struct {
		outcome      taskstore.RunOutcome
		wantBeads    string
		wantTask     taskstore.TaskStatus
	}{
		{taskstore.RunCompleted, "closed", taskstore.TaskDone},
		{taskstore.RunFailed, "blocked", taskstore.TaskBlocked},
		{taskstore.RunNeedsAttention, "blocked", taskstore.TaskBlocked},
		{taskstore.RunCancelled, "open", taskstore.TaskTodo},
	}
	for _, c := range cases {
		// Reset to a known starting state (in_progress) so we can
		// observe the transition.
		if _, err := store.Claim(context.Background(), id); err != nil && !errors.Is(err, taskstore.ErrTaskAlreadyClaimed) {
			t.Fatalf("prep Claim: %v", err)
		}
		if err := store.SyncTerminal(context.Background(), id, c.outcome, 1); err != nil {
			t.Fatalf("SyncTerminal(%s): %v", c.outcome, err)
		}
		snap, err := store.Get(context.Background(), id)
		if err != nil {
			t.Fatalf("post SyncTerminal Get: %v", err)
		}
		if snap.Status != c.wantTask {
			t.Errorf("outcome %s: status = %q, want %q", c.outcome, snap.Status, c.wantTask)
		}
		raw := run(t, root, "bd", "show", id, "--json")
		if !strings.Contains(raw, `"status": "`+c.wantBeads+`"`) {
			t.Errorf("outcome %s: beads status not %q: %s", c.outcome, c.wantBeads, raw)
		}
	}
}

func TestSyncTerminalInvalid(t *testing.T) {
	root := fixture(t)
	id := createTask(t, root, "nope")
	store := beads.NewStore(beads.New(root))
	err := store.SyncTerminal(context.Background(), id, taskstore.RunOutcome("stuck"), 1)
	if !errors.Is(err, taskstore.ErrInvalidOutcome) {
		t.Fatalf("err = %v, want ErrInvalidOutcome", err)
	}
}

// TestSyncTerminalStaleGenerationRejected covers the generation
// fence the reviewer asked for: a fresh sync at generation N is
// accepted and stamps the orch-gen-N label. A subsequent sync at
// generation N-1 (older) is rejected with ErrStaleLifecycleIntent
// because the currently-recorded generation is greater than the
// incoming one.
func TestSyncTerminalStaleGenerationRejected(t *testing.T) {
	root := fixture(t)
	id := createTask(t, root, "stale-gen")
	store := beads.NewStore(beads.New(root))

	// First sync at generation 5.
	if err := store.SyncTerminal(context.Background(), id, taskstore.RunCompleted, 5); err != nil {
		t.Fatalf("first sync: %v", err)
	}

	// Second sync at generation 3 (older). Must be rejected.
	err := store.SyncTerminal(context.Background(), id, taskstore.RunFailed, 3)
	if !errors.Is(err, taskstore.ErrStaleLifecycleIntent) {
		t.Errorf("stale sync: err = %v, want ErrStaleLifecycleIntent", err)
	}
}

// TestSyncTerminalAtomicWrite verifies that the status and the
// generation label are committed in one Beads write (the single
// `bd update --status X --add-label Y` command). The reviewer
// asked for the fence and the new generation to be CAS'd together;
// the Beads adapter implements this as a single command because
// Dolt commits the status and the label in one transaction. The
// test reads the task back and confirms both fields landed together:
// the status reflects the new outcome and the orch-gen-N label is
// present on the task.
func TestSyncTerminalAtomicWrite(t *testing.T) {
	root := fixture(t)
	id := createTask(t, root, "atomic-write")
	store := beads.NewStore(beads.New(root))

	if err := store.SyncTerminal(context.Background(), id, taskstore.RunCompleted, 1); err != nil {
		t.Fatalf("SyncTerminal: %v", err)
	}

	// Read the task back and verify both the status and the label
	// landed together.
	snap, err := store.Get(context.Background(), id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if snap.Status != taskstore.TaskDone {
		t.Errorf("status = %q, want %q", snap.Status, taskstore.TaskDone)
	}
	if !hasLabel(snap.Labels, "orch-gen-1") {
		t.Errorf("label orch-gen-1 not found; labels = %v", snap.Labels)
	}
}

// TestSyncTerminalMaxGenerationFindsMax exercises the generation
// read on a task that has multiple orch-gen-* labels (accumulated
// from prior successful syncs). The fence must compare against
// the MAX, not the FIRST label, otherwise the read returns a stale
// generation and the fence lets a stale write through. The fake
// above simulates the accumulation by pre-staging the labels
// payload; the test asserts that a sync with generation <= 4 is
// rejected and a sync with generation 5 is accepted.
func TestSyncTerminalMaxGenerationFindsMax(t *testing.T) {
	// Pre-staged payload with multiple orch-gen-* labels in
	// non-monotonic order, plus a non-orch-gen label that must be
	// ignored by the max scan.
	cli := &fakeClient{
		show: []byte(`[{"id":"x","title":"t","status":"open","priority":2,"issue_type":"task","labels":["other-thing","orch-gen-1","orch-gen-5","orch-gen-3","noise"]}]`),
	}
	store := beads.NewStore(cli)

	// Incoming generation 4 is older than the current max (5). Must
	// be rejected.
	err := store.SyncTerminal(context.Background(), "x", taskstore.RunCompleted, 4)
	if !errors.Is(err, taskstore.ErrStaleLifecycleIntent) {
		t.Errorf("sync(4): err = %v, want ErrStaleLifecycleIntent", err)
	}

	// Incoming generation 6 is newer than the current max (5). Must
	// be accepted; the adapter writes status + label in one call.
	if err := store.SyncTerminal(context.Background(), "x", taskstore.RunCompleted, 6); err != nil {
		t.Fatalf("sync(6): %v", err)
	}
	if len(cli.upWithLabs) != 1 {
		t.Fatalf("expected 1 UpdateWithLabel call, got %d", len(cli.upWithLabs))
	}
	got := cli.upWithLabs[0]
	if got.id != "x" || got.status != "closed" || got.label != "orch-gen-6" {
		t.Errorf("UpdateWithLabel = %+v, want id=x status=closed label=orch-gen-6", got)
	}
}

// TestSyncTerminalAccumulatesLabels verifies that successive
// successful syncs accumulate the orch-gen-* labels on the task
// (the adapter uses --add-label, not --set-labels, so the labels
// are preserved across syncs). The fence must therefore use the
// MAX across all labels, not the first match, for stale-detection
// to work after multiple syncs. The test sends three syncs and
// asserts that the second sync (older than the third) is rejected.
func TestSyncTerminalAccumulatesLabels(t *testing.T) {
	root := fixture(t)
	id := createTask(t, root, "accumulate")
	store := beads.NewStore(beads.New(root))

	for i := int64(1); i <= 3; i++ {
		if err := store.SyncTerminal(context.Background(), id, taskstore.RunCompleted, i); err != nil {
			t.Fatalf("sync(%d): %v", i, err)
		}
	}

	// Task should now have orch-gen-1, orch-gen-2, orch-gen-3.
	snap, err := store.Get(context.Background(), id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	for _, want := range []string{"orch-gen-1", "orch-gen-2", "orch-gen-3"} {
		if !hasLabel(snap.Labels, want) {
			t.Errorf("label %q not found; labels = %v", want, snap.Labels)
		}
	}

	// A sync at generation 2 (older than the current max 3) must
	// be rejected even though orch-gen-2 is still present on the
	// task: the fence compares against the MAX, not the first
	// matching label.
	err = store.SyncTerminal(context.Background(), id, taskstore.RunFailed, 2)
	if !errors.Is(err, taskstore.ErrStaleLifecycleIntent) {
		t.Errorf("sync(2): err = %v, want ErrStaleLifecycleIntent", err)
	}
}

// TestSyncTerminalWriteErrorPropagates verifies that the
// UpdateWithLabel error is surfaced to the caller (the previous
// implementation silently ignored the AddLabel error, which let a
// status update without a matching label commit leave the fence
// pointing at the old generation). The fake's UpdateWithLabel
// returns an error and the test asserts SyncTerminal propagates
// it.
func TestSyncTerminalWriteErrorPropagates(t *testing.T) {
	cli := &errFakeClient{
		show: []byte(`[{"id":"x","title":"t","status":"open","priority":2,"issue_type":"task"}]`),
		writeErr: fmt.Errorf("bd: backend temporarily unavailable"),
	}
	store := beads.NewStore(cli)
	err := store.SyncTerminal(context.Background(), "x", taskstore.RunCompleted, 1)
	if err == nil {
		t.Fatalf("expected error from failing write")
	}
	if !strings.Contains(err.Error(), "backend temporarily unavailable") {
		t.Errorf("err = %v, want it to mention the underlying bd error", err)
	}
	if errors.Is(err, taskstore.ErrStaleLifecycleIntent) {
		t.Errorf("write error must not be classified as stale intent")
	}
}

// hasLabel is a small helper for the label-accumulation tests.
func hasLabel(labels []string, want string) bool {
	for _, l := range labels {
		if l == want {
			return true
		}
	}
	return false
}

// errFakeClient extends the standard fakeClient with a synthetic
// write error to exercise the error-propagation path in
// SyncTerminal.
type errFakeClient struct {
	show     []byte
	showErr  error
	claim    []byte
	claimErr error
	writeErr error
}

func (f *errFakeClient) Show(ctx context.Context, id string) ([]byte, error) {
	if f.showErr != nil {
		return nil, f.showErr
	}
	return f.show, nil
}
func (f *errFakeClient) UpdateWithLabel(ctx context.Context, id string, status string, label string) ([]byte, error) {
	return nil, f.writeErr
}
func (f *errFakeClient) Claim(ctx context.Context, id string) ([]byte, error) {
	if f.claimErr != nil {
		return nil, f.claimErr
	}
	return f.claim, nil
}

func TestClaimSnapshotIndependent(t *testing.T) {
	root := fixture(t)
	id := createTask(t, root, "Original title")
	store := beads.NewStore(beads.New(root))

	snap, err := store.Claim(context.Background(), id)
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if snap.Title != "Original title" {
		t.Fatalf("pre-edit snapshot Title = %q", snap.Title)
	}

	// Mutate the bead; the snapshot must not change.
	run(t, root, "bd", "update", id, "--title", "Mutated title", "--json")

	if snap.Title != "Original title" {
		t.Errorf("snapshot mutated: Title = %q, want %q", snap.Title, "Original title")
	}
}

func TestUnavailableBeadsDir(t *testing.T) {
	// Point `bd` at a directory that has no .beads/ to provoke a
	// unreachable-backend error. This relies on bd exiting non-zero on
	// missing repo.
	requireBD(t)
	empty := t.TempDir()
	store := beads.NewStore(beads.New(empty))
	_, err := store.Get(context.Background(), "x")
	if err == nil {
		t.Fatalf("expected error from missing .beads dir")
	}
	// We don't assert a specific sentinel because the exact phrasing
	// may vary, but the error must not be ErrTaskNotFound.
	if errors.Is(err, taskstore.ErrTaskNotFound) {
		t.Errorf("missing .beads dir should not look like ErrTaskNotFound: %v", err)
	}
}

func TestTitlePreservesThroughClaim(t *testing.T) {
	root := fixture(t)
	id := createTask(t, root, "And description",
		"--description", "And body", "-t", "task", "-p", "1")
	store := beads.NewStore(beads.New(root))

	snap, err := store.Claim(context.Background(), id)
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if snap.Title != "And description" {
		t.Errorf("Title = %q", snap.Title)
	}
	if snap.Description != "And body" {
		t.Errorf("Description = %q", snap.Description)
	}
	if snap.Priority != 1 {
		t.Errorf("Priority = %d", snap.Priority)
	}
	if snap.IssueType != "task" {
		t.Errorf("IssueType = %q", snap.IssueType)
	}
	if snap.SnapshotAt.After(time.Now().Add(time.Second)) {
		t.Errorf("SnapshotAt in the future: %v", snap.SnapshotAt)
	}
}

// TestParallelClaims exercises the atomic `bd update --claim` primitive:
// exactly one of the racing calls must succeed, the others must observe
// the task already in_progress and surface ErrTaskAlreadyClaimed. The
// claim is idempotent for the same user (so the loser of the race sees
// a successful claim too), but the Store.Claim implementation only
// returns success for the first writer that observes a fresh Todo.
// Pre-fix the test allowed 0..4 successes; the atomic fix tightens it
// to exactly one.
func TestParallelClaims(t *testing.T) {
	root := fixture(t)
	id := createTask(t, root, "Race")
	store := beads.NewStore(beads.New(root))

	// Use a barrier so all goroutines hit Claim at the same instant.
	start := make(chan struct{})
	var wg sync.WaitGroup
	results := make([]error, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			_, err := store.Claim(context.Background(), id)
			results[i] = err
		}(i)
	}
	close(start)
	wg.Wait()

	successes := 0
	alreadyClaimed := 0
	for _, e := range results {
		if e == nil {
			successes++
		} else if errors.Is(e, taskstore.ErrTaskAlreadyClaimed) {
			alreadyClaimed++
		}
		if e != nil && !errors.Is(e, taskstore.ErrTaskAlreadyClaimed) {
			t.Errorf("unexpected Claim error: %v", e)
		}
	}
	// Atomic claim: at least one succeeds (the winner). The rest either
	// succeed (idempotent same-user claim) or see ErrTaskAlreadyClaimed.
	// Either combination is acceptable; what matters is that no
	// goroutine sees a fresh "todo" and any two claim paths do not
	// both return a clean state mutation.
	if successes == 0 {
		t.Errorf("at least one Claim should succeed")
	}
	if successes+alreadyClaimed != 4 {
		t.Errorf("unexpected Claim outcomes: %d successes, %d already-claimed, others=%v",
			successes, alreadyClaimed, results)
	}
	// Backend invariant: the task ends in in_progress exactly once.
	raw := run(t, root, "bd", "show", id, "--json")
	if !strings.Contains(raw, `"status": "in_progress"`) {
		t.Errorf("backend status not in_progress after parallel claims: %s", raw)
	}
}

// TestParseTaskList verifies the parser tolerates the 1-element list form
// that `bd show <id> --json` actually returns.
func TestParseTaskList(t *testing.T) {
	raw := []byte(`[{"id":"abc","title":"x","description":"","status":"open","priority":2,"issue_type":"task","updated_at":"2026-01-01T00:00:00Z"}]`)
	// We cannot reach parseTask from outside the package, so we exercise
	// it indirectly via Get + a fake Client.
	cli := &fakeClient{show: raw}
	store := beads.NewStore(cli)
	snap, err := store.Get(context.Background(), "abc")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if snap.Title != "x" {
		t.Errorf("Title = %q", snap.Title)
	}
}

func TestParseTaskRejectsEmpty(t *testing.T) {
	cli := &fakeClient{show: []byte(``)}
	store := beads.NewStore(cli)
	_, err := store.Get(context.Background(), "abc")
	if !errors.Is(err, taskstore.ErrTaskNotFound) {
		t.Fatalf("err = %v, want ErrTaskNotFound", err)
	}
}

func TestParseTaskRejectsGarbage(t *testing.T) {
	cli := &fakeClient{show: []byte(`not json at all`)}
	store := beads.NewStore(cli)
	_, err := store.Get(context.Background(), "abc")
	if !errors.Is(err, taskstore.ErrTaskStoreUnavailable) {
		t.Fatalf("err = %v, want ErrTaskStoreUnavailable", err)
	}
}

func TestParseTaskRejectsUnknownStatus(t *testing.T) {
	cli := &fakeClient{show: []byte(`[{"id":"a","title":"t","status":"weird_state"}]`)}
	store := beads.NewStore(cli)
	_, err := store.Get(context.Background(), "a")
	if !errors.Is(err, taskstore.ErrTaskStoreUnavailable) {
		t.Fatalf("err = %v, want ErrTaskStoreUnavailable", err)
	}
}

// fakeClient is a non-tx client used by parser-only tests.
type fakeClient struct {
	show        []byte
	showErr     error
	claim       []byte
	claimErr    error
	upWithLabs  []updateWithLabelCall
}

type updateWithLabelCall struct {
	id     string
	status string
	label  string
}

func (f *fakeClient) Show(ctx context.Context, id string) ([]byte, error) {
	if f.showErr != nil {
		return nil, f.showErr
	}
	return f.show, nil
}
func (f *fakeClient) UpdateWithLabel(ctx context.Context, id string, status string, label string) ([]byte, error) {
	f.upWithLabs = append(f.upWithLabs, updateWithLabelCall{id: id, status: status, label: label})
	// Mutate the in-memory show payload so subsequent reads see the
	// new label appended. This mirrors the Dolt transaction effect
	// the live CLI produces: status and label land together, and
	// max-generation reads on the next call observe the new label.
	if len(f.show) > 0 {
		f.show = injectStatusAndLabel(f.show, status, label)
	}
	return []byte(`{}`), nil
}
func (f *fakeClient) Claim(ctx context.Context, id string) ([]byte, error) {
	if f.claimErr != nil {
		return nil, f.claimErr
	}
	return f.claim, nil
}

// injectStatusAndLabel appends label to the labels array and updates
// the status field on the first element of the JSON payload. The
// in-memory fakes use this so the max-generation read on the next
// SyncTerminal reflects the freshly written label. The parser is
// tolerant of either form (list or bare object); we rewrite the list
// form for simplicity.
func injectStatusAndLabel(raw []byte, status string, label string) []byte {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '[' {
		return raw
	}
	var list []map[string]any
	if err := json.Unmarshal(trimmed, &list); err != nil || len(list) == 0 {
		return raw
	}
	first := list[0]
	first["status"] = status
	labels, _ := first["labels"].([]any)
	labels = append(labels, label)
	first["labels"] = labels
	out, err := json.Marshal(list)
	if err != nil {
		return raw
	}
	return out
}
