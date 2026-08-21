package beads_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"bdtui/internal/taskstore"
	"bdtui/internal/taskstore/beads"
)

// fixture initialises a fresh Beads repository under a temporary directory
// and returns the project root. The root is the directory that contains
// .beads/ and is what the adapter uses as cmd.Dir.
func fixture(t *testing.T) string {
	t.Helper()
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

	// Second claim rejects with ErrTaskAlreadyClaimed.
	_, err = store.Claim(context.Background(), id)
	if !errors.Is(err, taskstore.ErrTaskAlreadyClaimed) {
		t.Fatalf("second Claim: err = %v, want ErrTaskAlreadyClaimed", err)
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
		if err := store.SyncTerminal(context.Background(), id, c.outcome); err != nil {
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
	err := store.SyncTerminal(context.Background(), id, taskstore.RunOutcome("stuck"))
	if !errors.Is(err, taskstore.ErrInvalidOutcome) {
		t.Fatalf("err = %v, want ErrInvalidOutcome", err)
	}
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

// TestParallelClaims demonstrates that two concurrent Claim calls on the
// same task can race; the unique active-run rule is enforced at the
// orchestrator layer, not by the TaskStore. The test just records the
// behaviour so future readers understand the boundary.
func TestParallelClaims(t *testing.T) {
	root := fixture(t)
	id := createTask(t, root, "Race")
	store := beads.NewStore(beads.New(root))

	var wg sync.WaitGroup
	results := make([]error, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := store.Claim(context.Background(), id)
			results[i] = err
		}(i)
	}
	wg.Wait()

	successes := 0
	for _, e := range results {
		if e == nil {
			successes++
		}
	}
	if successes == 0 {
		t.Errorf("at least one Claim should succeed")
	}
	if successes >= 4 {
		t.Errorf("more than one Claim succeeded (%d) — adapter should at least *try* to enforce single-claim", successes)
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
	show   []byte
	showErr error
	updates []string
}

func (f *fakeClient) Show(ctx context.Context, id string) ([]byte, error) {
	if f.showErr != nil {
		return nil, f.showErr
	}
	return f.show, nil
}
func (f *fakeClient) Update(ctx context.Context, id string, status string) ([]byte, error) {
	f.updates = append(f.updates, id+":"+status)
	return []byte(`{}`), nil
}
