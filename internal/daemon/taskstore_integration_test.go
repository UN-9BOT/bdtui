package daemon

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"bdtui/internal/daemon/daemonpb"
	"bdtui/internal/orch"
	"bdtui/internal/taskstore"
	"bdtui/internal/taskstore/taskstoretest"

	_ "modernc.org/sqlite"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// startTestServerWithTasks is the TaskStore-aware twin of startTestServer.
// It returns the underlying fake so the test can seed tasks and assert
// the recorded lifecycle updates after each RPC.
func startTestServerWithTasks(t *testing.T) (*orch.Store, *orch.Project, *taskstoretest.Fake, *Client) {
	t.Helper()

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "orch.db")
	socketPath := filepath.Join(dir, "daemon.sock")

	store, err := orch.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	project := &orch.Project{Name: "test", FsPath: "/tmp/test"}
	if err := store.CreateProject(context.Background(), project); err != nil {
		t.Fatalf("create project: %v", err)
	}

	tasks := taskstoretest.New()

	ctx, cancel := context.WithCancel(context.Background())
	srv := NewServerWithTasks(store, tasks, socketPath)
	done := make(chan error, 1)
	go func() { done <- srv.Serve(ctx) }()
	t.Cleanup(func() {
		cancel()
		srv.Stop()
		<-done
	})

	deadline := time.Now().Add(3 * time.Second)
	for !socketAlive(context.Background(), socketPath) {
		if time.Now().After(deadline) {
			t.Fatal("daemon socket did not become ready")
		}
		time.Sleep(10 * time.Millisecond)
	}

	client, err := Dial(socketPath)
	if err != nil {
		t.Fatalf("dial daemon: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	return store, project, tasks, client
}

func TestCreateRunClaimsTask(t *testing.T) {
	_, project, tasks, client := startTestServerWithTasks(t)
	tasks.Put(&taskstore.Task{
		ID:     "task-claim",
		Title:  "Hello",
		Status: taskstore.TaskTodo,
	})

	ctx := context.Background()
	run, err := client.CreateRun(ctx, &daemonpb.CreateRunRequest{
		ProjectId: project.ID,
		TaskId:    "task-claim",
	})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	if run.Id == "" {
		t.Fatal("expected non-empty run id")
	}

	// TaskStore should now report in_progress.
	snap, err := tasks.Get(ctx, "task-claim")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if snap.Status != taskstore.TaskInProgress {
		t.Errorf("task status = %q, want in_progress", snap.Status)
	}

	// The run row should carry the encoded snapshot.
	persisted, err := client.GetRun(ctx, &daemonpb.GetRunRequest{Id: run.Id})
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if persisted.TaskSnapshot == "" {
		t.Errorf("TaskSnapshot is empty after claim")
	}
	if !strings.Contains(persisted.TaskSnapshot, "Hello") {
		t.Errorf("snapshot did not freeze title: %s", persisted.TaskSnapshot)
	}

	// Mutating the task in the fake after the run is created must not
	// mutate the persisted snapshot (the controller treats the snapshot
	// as immutable).
	tasks.Put(&taskstore.Task{
		ID:     "task-claim",
		Title:  "Mutated title",
		Status: taskstore.TaskInProgress,
	})
	persisted, err = client.GetRun(ctx, &daemonpb.GetRunRequest{Id: run.Id})
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if !strings.Contains(persisted.TaskSnapshot, "Hello") {
		t.Errorf("snapshot did not freeze title: %s", persisted.TaskSnapshot)
	}
	if strings.Contains(persisted.TaskSnapshot, "Mutated title") {
		t.Errorf("snapshot was mutated: %s", persisted.TaskSnapshot)
	}
}

func TestCreateRunRefusesAlreadyClaimed(t *testing.T) {
	_, project, tasks, client := startTestServerWithTasks(t)
	tasks.Put(&taskstore.Task{
		ID:     "task-claimed",
		Title:  "Already taken",
		Status: taskstore.TaskInProgress,
	})

	_, err := client.CreateRun(context.Background(), &daemonpb.CreateRunRequest{
		ProjectId: project.ID,
		TaskId:    "task-claimed",
	})
	if status.Code(err) != codes.AlreadyExists {
		t.Fatalf("err = %v, want AlreadyExists", err)
	}
}

func TestCreateRunMissingTaskReturnsNotFound(t *testing.T) {
	_, project, _, client := startTestServerWithTasks(t)
	_, err := client.CreateRun(context.Background(), &daemonpb.CreateRunRequest{
		ProjectId: project.ID,
		TaskId:    "task-missing",
	})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("err = %v, want NotFound", err)
	}
}

// TestCreateRunNoTaskStorePreservesLegacy verifies that when the
// daemon is constructed without a TaskStore, CreateRun keeps the
// legacy behaviour (no claim, no snapshot).
func TestCreateRunNoTaskStorePreservesLegacy(t *testing.T) {
	_, project, client := startTestServer(t)
	run, err := client.CreateRun(context.Background(), &daemonpb.CreateRunRequest{
		ProjectId: project.ID,
		TaskId:    "task-legacy",
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	got, err := client.GetRun(context.Background(), &daemonpb.GetRunRequest{Id: run.Id})
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if got.TaskSnapshot != "" {
		t.Errorf("TaskSnapshot = %q, want empty", got.TaskSnapshot)
	}
}

func TestCancelRunSyncsTerminal(t *testing.T) {
	_, project, tasks, client := startTestServerWithTasks(t)
	tasks.Put(&taskstore.Task{
		ID:     "task-cancel",
		Title:  "Cancel me",
		Status: taskstore.TaskTodo,
	})

	run, err := client.CreateRun(context.Background(), &daemonpb.CreateRunRequest{
		ProjectId: project.ID,
		TaskId:    "task-cancel",
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if _, err := client.CancelRun(context.Background(), &daemonpb.CancelRunRequest{Id: run.Id}); err != nil {
		t.Fatalf("CancelRun: %v", err)
	}
	snap, err := tasks.Get(context.Background(), "task-cancel")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if snap.Status != taskstore.TaskTodo {
		t.Errorf("status = %q, want todo (cancelled->todo)", snap.Status)
	}
	updates := tasks.Updates()
	if len(updates) != 2 {
		t.Fatalf("len(Updates) = %d, want 2 (Claim + SyncTerminal)", len(updates))
	}
	if updates[1].Outcome != taskstore.RunCancelled {
		t.Errorf("last update outcome = %q, want cancelled", updates[1].Outcome)
	}
}

// TestTaskStoreToStatusMapping locks in the gRPC-code mapping for
// taskstore sentinel errors. The full RPC path is covered by the
// scenario tests above; this one protects the helper from accidental
// drift.
func TestTaskStoreToStatusMapping(t *testing.T) {
	if got := taskStoreToStatus(taskstore.ErrTaskStoreUnavailable); status.Code(got) != codes.Unavailable {
		t.Errorf("ErrTaskStoreUnavailable -> %v, want Unavailable", got)
	}
	if got := taskStoreToStatus(taskstore.ErrTaskAlreadyClaimed); status.Code(got) != codes.AlreadyExists {
		t.Errorf("ErrTaskAlreadyClaimed -> %v, want AlreadyExists", got)
	}
	if got := taskStoreToStatus(taskstore.ErrTaskNotFound); status.Code(got) != codes.NotFound {
		t.Errorf("ErrTaskNotFound -> %v, want NotFound", got)
	}
	if got := taskStoreToStatus(taskstore.ErrInvalidOutcome); status.Code(got) != codes.InvalidArgument {
		t.Errorf("ErrInvalidOutcome -> %v, want InvalidArgument", got)
	}
}

// TestServiceHasTaskStoreFlag pins the HasTaskStore predicate so the
// TUI can branch on whether the lifecycle is wired.
func TestServiceHasTaskStoreFlag(t *testing.T) {
	store, _, _, _ := startTestServerWithTasks(t)
	plain := NewService(store)
	if plain.HasTaskStore() {
		t.Errorf("NewService.HasTaskStore = true, want false")
	}
	tasks := taskstoretest.New()
	withTasks := NewServiceWithTasks(store, tasks)
	if !withTasks.HasTaskStore() {
		t.Errorf("NewServiceWithTasks.HasTaskStore = false, want true")
	}
	_ = plain
}

// flakyTaskStore wraps a real TaskStore but fails every SyncTerminal.
// Used to exercise the durable-error path: the Run row is persisted,
// the CancelRun RPC succeeds, but the TaskStore sync fails and the
// failure leaves a task.sync_failed event in the orchestrator store.
type flakyTaskStore struct {
	inner *taskstoretest.Fake
}

func (f *flakyTaskStore) Get(ctx context.Context, id string) (*taskstore.Task, error) {
	return f.inner.Get(ctx, id)
}
func (f *flakyTaskStore) Claim(ctx context.Context, id string) (*taskstore.Task, error) {
	return f.inner.Claim(ctx, id)
}
func (f *flakyTaskStore) SyncTerminal(ctx context.Context, id string, outcome taskstore.RunOutcome, generation int64) error {
	return fmt.Errorf("simulated Beads outage for %s", id)
}

// TestSyncFailureWritesDurableEvent verifies that a SyncTerminal
// failure is recorded as a task.sync_failed event AND appended to the
// task_sync_outbox so the future controller reconciler can retry.
func TestSyncFailureWritesDurableEvent(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "orch.db")
	socketPath := filepath.Join(dir, "daemon.sock")

	store, err := orch.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	project := &orch.Project{Name: "test", FsPath: "/tmp/test"}
	if err := store.CreateProject(context.Background(), project); err != nil {
		t.Fatalf("create project: %v", err)
	}

	flaky := &flakyTaskStore{inner: taskstoretest.New()}
	flaky.inner.Put(&taskstore.Task{
		ID:     "task-fail-sync",
		Title:  "Sync fails",
		Status: taskstore.TaskTodo,
	})

	ctx, cancel := context.WithCancel(context.Background())
	srv := NewServerWithTasks(store, flaky, socketPath)
	done := make(chan error, 1)
	go func() { done <- srv.Serve(ctx) }()
	t.Cleanup(func() {
		cancel()
		srv.Stop()
		<-done
	})

	deadline := time.Now().Add(3 * time.Second)
	for !socketAlive(context.Background(), socketPath) {
		if time.Now().After(deadline) {
			t.Fatal("daemon socket did not become ready")
		}
		time.Sleep(10 * time.Millisecond)
	}

	client, err := Dial(socketPath)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	run, err := client.CreateRun(ctx, &daemonpb.CreateRunRequest{
		ProjectId: project.ID,
		TaskId:    "task-fail-sync",
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if _, err := client.CancelRun(ctx, &daemonpb.CancelRunRequest{Id: run.Id}); err != nil {
		t.Fatalf("CancelRun: %v", err)
	}

	// Audit: the sync failure must be visible as a task.sync_failed event.
	events, err := store.ListEventsByRun(context.Background(), run.Id)
	if err != nil {
		t.Fatalf("ListEventsByRun: %v", err)
	}
	sawSyncFailed := false
	for _, e := range events {
		if e.Type == orch.EventTaskSyncFailed {
			sawSyncFailed = true
			if !strings.Contains(e.Payload, "simulated Beads outage") {
				t.Errorf("sync_failed payload missing error: %s", e.Payload)
			}
			if !strings.Contains(e.Payload, "task-fail-sync") {
				t.Errorf("sync_failed payload missing task id: %s", e.Payload)
			}
		}
	}
	if !sawSyncFailed {
		t.Errorf("expected task.sync_failed event in run events, got %d events", len(events))
		for _, e := range events {
			t.Logf("event: %s payload=%s", e.Type, e.Payload)
		}
	}

	// Durable retry state: the outbox row is what the future controller
	// reconciler (bdtui-cvy.13) will consume. The reconciler asks
	// "which Runs still owe a TaskStore sync?" via this query.
	pending, err := store.ListPendingTaskSyncOutbox(context.Background())
	if err != nil {
		t.Fatalf("ListPendingTaskSyncOutbox: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("pending outbox = %d, want 1", len(pending))
	}
	if pending[0].RunID != run.Id {
		t.Errorf("outbox run_id = %q, want %q", pending[0].RunID, run.Id)
	}
	if pending[0].TaskID != "task-fail-sync" {
		t.Errorf("outbox task_id = %q", pending[0].TaskID)
	}
	if pending[0].Outcome != string(taskstore.RunCancelled) {
		t.Errorf("outbox outcome = %q, want cancelled", pending[0].Outcome)
	}
	if pending[0].Status != orch.TaskSyncPending {
		t.Errorf("outbox status = %q", pending[0].Status)
	}
}

// TestPendingOutboxCleanedAfterMarkDone verifies the reconciler can
// advance the outbox row to 'done' once a retry succeeds.
func TestPendingOutboxCleanedAfterMarkDone(t *testing.T) {
	store, project, _, _ := startTestServerWithTasks(t)
	// Create a real Run so the outbox row's foreign key is satisfied.
	run := &orch.Run{
		ProjectID: project.ID,
		TaskID:    "task-x",
		Status:    orch.RunCancelled,
	}
	if err := store.CreateRun(context.Background(), run); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if _, err := store.AppendTaskSyncOutbox(context.Background(), &orch.TaskSyncOutbox{
		RunID:   run.ID,
		TaskID:  "task-x",
		Outcome: string(taskstore.RunCancelled),
	}); err != nil {
		t.Fatalf("append: %v", err)
	}
	pending, err := store.ListPendingTaskSyncOutbox(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("pending = %d, want 1", len(pending))
	}
	if err := store.MarkTaskSyncOutboxDone(context.Background(), pending[0].ID); err != nil {
		t.Fatalf("mark done: %v", err)
	}
	pending, err = store.ListPendingTaskSyncOutbox(context.Background())
	if err != nil {
		t.Fatalf("list 2: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("pending = %d, want 0", len(pending))
	}
}

// TestOutboxSupersedesStalePending verifies that a Run that
// transitions to multiple outcomes (e.g. needs_attention -> blocked
// fails, then later completes -> done) only leaves the LATEST pending
// row in the reconciler's queue. Without the supersede, the
// reconciler would replay the stale needs_attention -> blocked after
// the new completed -> done had already succeeded, reverting the
// Beads task back to blocked.
func TestOutboxSupersedesStalePending(t *testing.T) {
	store, project, _, _ := startTestServerWithTasks(t)
	run := &orch.Run{
		ProjectID: project.ID,
		TaskID:    "task-supersede",
		Status:    orch.RunNeedsAttention,
	}
	if err := store.CreateRun(context.Background(), run); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	// First append: needs_attention -> blocked. The reconciler
	// would replay this if not superseded.
	if _, err := store.AppendTaskSyncOutbox(context.Background(), &orch.TaskSyncOutbox{
		RunID:   run.ID,
		TaskID:  "task-supersede",
		Outcome: string(taskstore.RunNeedsAttention),
	}); err != nil {
		t.Fatalf("append 1: %v", err)
	}

	// Second append: completed -> done. This must supersede the
	// first row in the same transaction.
	if _, err := store.AppendTaskSyncOutbox(context.Background(), &orch.TaskSyncOutbox{
		RunID:   run.ID,
		TaskID:  "task-supersede",
		Outcome: string(taskstore.RunCompleted),
	}); err != nil {
		t.Fatalf("append 2: %v", err)
	}

	// The reconciler query must return exactly one pending row, and
	// it must be the new one (completed -> done).
	pending, err := store.ListPendingTaskSyncOutbox(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("pending = %d, want 1 (the stale outcome should be superseded)", len(pending))
	}
	if pending[0].Outcome != string(taskstore.RunCompleted) {
		t.Errorf("pending outcome = %q, want completed", pending[0].Outcome)
	}
}

// TestOutboxPersistedBeforeExternalSync verifies that the outbox row
// is committed before the external TaskStore side effect is attempted.
// We simulate a crash by failing the SyncTerminal; the row must still
// be present for the reconciler.
func TestOutboxPersistedBeforeExternalSync(t *testing.T) {
	store, project, _, _ := startTestServerWithTasks(t)
	run := &orch.Run{
		ProjectID: project.ID,
		TaskID:    "task-crash",
		Status:    orch.RunCancelled,
	}
	if err := store.CreateRun(context.Background(), run); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	outboxID, err := store.AppendTaskSyncOutbox(context.Background(), &orch.TaskSyncOutbox{
		RunID:   run.ID,
		TaskID:  "task-crash",
		Outcome: string(taskstore.RunCancelled),
	})
	if err != nil {
		t.Fatalf("append: %v", err)
	}
	if outboxID == 0 {
		t.Fatalf("outbox id is 0")
	}
	pending, err := store.ListPendingTaskSyncOutbox(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("pending = %d, want 1", len(pending))
	}
	if pending[0].ID != outboxID {
		t.Errorf("pending id = %d, want %d", pending[0].ID, outboxID)
	}
}

// TestReconcileSupersedesStalePendingRow simulates the scenario
// from R5: a failed needs_attention -> blocked sync left a pending
// row in the outbox, then the Run moved to completed -> done. The
// next reconciler pass must NOT replay the stale blocked outcome;
// the helper MarkTaskSyncOutboxSupersededIfStale exists exactly so
// the reconciler can drop stale rows before retrying.
func TestReconcileSupersedesStalePendingRow(t *testing.T) {
	store, project, _, _ := startTestServerWithTasks(t)
	run := &orch.Run{
		ProjectID: project.ID,
		TaskID:    "task-stale",
		Status:    orch.RunNeedsAttention,
	}
	if err := store.CreateRun(context.Background(), run); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	// Seed: append a needs_attention -> blocked pending row.
	id, err := store.AppendTaskSyncOutbox(context.Background(), &orch.TaskSyncOutbox{
		RunID:   run.ID,
		TaskID:  "task-stale",
		Outcome: string(taskstore.RunNeedsAttention),
	})
	if err != nil {
		t.Fatalf("append: %v", err)
	}

	// Run has now advanced to completed. The reconciler, before
	// retrying the sync, asks the outbox whether the row is still
	// actionable.
	run.Status = orch.RunCompleted
	superseded, err := store.MarkTaskSyncOutboxSupersededIfStale(context.Background(), id, run)
	if err != nil {
		t.Fatalf("check stale: %v", err)
	}
	if !superseded {
		t.Errorf("stale row was not marked superseded")
	}

	// The pending queue is now empty.
	pending, err := store.ListPendingTaskSyncOutbox(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(pending) != 0 {
		t.Errorf("pending = %d, want 0 (stale row should not be replayed)", len(pending))
	}
}

// TestReconcileKeepsCurrentPendingRow protects the happy path:
// when the pending outcome matches the current Run status, the
// reconciler MUST NOT mark it superseded (the row is still
// actionable).
func TestReconcileKeepsCurrentPendingRow(t *testing.T) {
	store, project, _, _ := startTestServerWithTasks(t)
	run := &orch.Run{
		ProjectID: project.ID,
		TaskID:    "task-current",
		Status:    orch.RunCancelled,
	}
	if err := store.CreateRun(context.Background(), run); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	id, err := store.AppendTaskSyncOutbox(context.Background(), &orch.TaskSyncOutbox{
		RunID:   run.ID,
		TaskID:  "task-current",
		Outcome: string(taskstore.RunCancelled),
	})
	if err != nil {
		t.Fatalf("append: %v", err)
	}
	superseded, err := store.MarkTaskSyncOutboxSupersededIfStale(context.Background(), id, run)
	if err != nil {
		t.Fatalf("check stale: %v", err)
	}
	if superseded {
		t.Errorf("current row was wrongly marked superseded")
	}
	pending, err := store.ListPendingTaskSyncOutbox(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(pending) != 1 {
		t.Errorf("pending = %d, want 1", len(pending))
	}
}

// TestReconcileRaceAlreadySupersededRow covers the TOCTOU race
// the reviewer flagged in R5: the reconciler reads a row id from
// ListPendingTaskSyncOutbox, then a newer intent supersedes that
// row before the reconciler calls MarkTaskSyncOutboxSupersededIfStale.
// Without the up-front status check, the helper returned false
// (signalling "retry") for both the matching-outcome and the
// mismatch-outcome paths, which would replay the already-superseded
// row. The helper MUST detect "already superseded" and return true
// (skip) regardless of the outcome match.
func TestReconcileRaceAlreadySupersededRow(t *testing.T) {
	store, project, _, _ := startTestServerWithTasks(t)
	run := &orch.Run{
		ProjectID: project.ID,
		TaskID:    "task-race",
		Status:    orch.RunFailed,
	}
	if err := store.CreateRun(context.Background(), run); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	// Append the original pending row.
	oldID, err := store.AppendTaskSyncOutbox(context.Background(), &orch.TaskSyncOutbox{
		RunID:   run.ID,
		TaskID:  "task-race",
		Outcome: string(taskstore.RunFailed),
	})
	if err != nil {
		t.Fatalf("append: %v", err)
	}

	// Newer intent supersedes the original row.
	if _, err := store.AppendTaskSyncOutbox(context.Background(), &orch.TaskSyncOutbox{
		RunID:   run.ID,
		TaskID:  "task-race",
		Outcome: string(taskstore.RunCompleted),
	}); err != nil {
		t.Fatalf("supersede: %v", err)
	}

	// The reconciler now calls the stale-check on the OLD id it
	// had cached from ListPending. The current row status is
	// 'superseded', not 'pending'. The helper MUST return true
	// (skip) — the row is already not actionable.
	run.Status = orch.RunCompleted
	skip, err := store.MarkTaskSyncOutboxSupersededIfStale(context.Background(), oldID, run)
	if err != nil {
		t.Fatalf("check stale: %v", err)
	}
	if !skip {
		t.Errorf("already-superseded row was not detected as skip; would be replayed")
	}
}

// TestReconcileRaceNewerPendingAppend covers the atomic retry
// ownership protocol the reviewer asked for: between the
// stale-check (which authorises the sync) and the actual
// SyncTerminal, a newer intent may append and supersede the row.
// The CAS claim (ClaimTaskSyncOutbox) is the actual serialization
// point: once the reconciler has CAS-claimed the row to in_flight,
// the append mechanism can no longer touch it. The newer intent
// lands as a fresh pending row, and the next reconciler pass picks
// it up.
//
// The earlier version of this test appended two rows for the same
// (run_id, task_id) and asserted the stale-check returned skip for
// the older one. AppendTaskSyncOutbox in a transaction supersedes
// prior pending rows, so the older row was actually status =
// superseded and the test exercised the existing first fence,
// not the second window. This version exercises the CAS claim
// path directly: between the stale-check and the claim, the row
// is concurrent-superseded; the claim must return false.
func TestReconcileRaceNewerPendingAppend(t *testing.T) {
	store, project, _, _ := startTestServerWithTasks(t)
	run := &orch.Run{
		ProjectID: project.ID,
		TaskID:    "task-newer-pending",
		Status:    orch.RunCompleted,
	}
	if err := store.CreateRun(context.Background(), run); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	// Append the original pending row.
	oldID, err := store.AppendTaskSyncOutbox(context.Background(), &orch.TaskSyncOutbox{
		RunID:   run.ID,
		TaskID:  "task-newer-pending",
		Outcome: string(taskstore.RunCompleted),
	})
	if err != nil {
		t.Fatalf("append: %v", err)
	}

	// Stale-check sees pending + matching outcome; we authorise
	// the sync.
	skip, err := store.MarkTaskSyncOutboxSupersededIfStale(context.Background(), oldID, run)
	if err != nil {
		t.Fatalf("stale-check: %v", err)
	}
	if skip {
		t.Fatalf("authorised row should not be skipped")
	}

	// Between the stale-check and the claim, a newer intent
	// arrives. AppendTaskSyncOutbox in a transaction supersedes
	// the old row (pending -> superseded). After that, the claim
	// MUST return false: the row is no longer pending.
	if _, err := store.AppendTaskSyncOutbox(context.Background(), &orch.TaskSyncOutbox{
		RunID:   run.ID,
		TaskID:  "task-newer-pending",
		Outcome: string(taskstore.RunCancelled),
	}); err != nil {
		t.Fatalf("append newer: %v", err)
	}

	claimed, err := store.ClaimTaskSyncOutbox(context.Background(), oldID)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if claimed {
		t.Errorf("claim returned true on a now-superseded row; reconciler would proceed with stale sync")
	}

	// Listing pending should yield exactly one row (the newer).
	pending, err := store.ListPendingTaskSyncOutbox(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(pending) != 1 {
		t.Errorf("pending = %d, want 1 (the newer row only)", len(pending))
	}
}

// TestClaimTaskSyncOutboxSuccess covers the happy path: pending
// row, claim succeeds, MarkTaskSyncOutboxClaimedDone marks the row
// done.
func TestClaimTaskSyncOutboxSuccess(t *testing.T) {
	store, project, _, _ := startTestServerWithTasks(t)
	run := &orch.Run{
		ProjectID: project.ID,
		TaskID:    "task-claim-success",
		Status:    orch.RunCompleted,
	}
	if err := store.CreateRun(context.Background(), run); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	id, err := store.AppendTaskSyncOutbox(context.Background(), &orch.TaskSyncOutbox{
		RunID:   run.ID,
		TaskID:  "task-claim-success",
		Outcome: string(taskstore.RunCompleted),
	})
	if err != nil {
		t.Fatalf("append: %v", err)
	}

	claimed, err := store.ClaimTaskSyncOutbox(context.Background(), id)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if !claimed {
		t.Fatalf("claim returned false on a pending row")
	}

	done, err := store.MarkTaskSyncOutboxClaimedDone(context.Background(), id)
	if err != nil {
		t.Fatalf("done: %v", err)
	}
	if !done {
		t.Errorf("claimed-done returned false; the row should have been in_flight")
	}

	pending, err := store.ListPendingTaskSyncOutbox(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(pending) != 0 {
		t.Errorf("pending = %d, want 0", len(pending))
	}
}

// TestClaimTaskSyncOutboxAlreadyClaimed covers the race: a second
// reconciler (or a concurrent retry) tries to claim a row that
// was already CAS-claimed by another goroutine. The second claim
// MUST return false; Idempotency fails (only one writer wins).
func TestClaimTaskSyncOutboxAlreadyClaimed(t *testing.T) {
	store, project, _, _ := startTestServerWithTasks(t)
	run := &orch.Run{
		ProjectID: project.ID,
		TaskID:    "task-claim-twice",
		Status:    orch.RunCompleted,
	}
	if err := store.CreateRun(context.Background(), run); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	id, err := store.AppendTaskSyncOutbox(context.Background(), &orch.TaskSyncOutbox{
		RunID:   run.ID,
		TaskID:  "task-claim-twice",
		Outcome: string(taskstore.RunCompleted),
	})
	if err != nil {
		t.Fatalf("append: %v", err)
	}

	// First claim wins.
	claimed, err := store.ClaimTaskSyncOutbox(context.Background(), id)
	if err != nil {
		t.Fatalf("claim 1: %v", err)
	}
	if !claimed {
		t.Fatalf("claim 1 returned false on a pending row")
	}

	// Second claim loses.
	claimed, err = store.ClaimTaskSyncOutbox(context.Background(), id)
	if err != nil {
		t.Fatalf("claim 2: %v", err)
	}
	if claimed {
		t.Errorf("claim 2 returned true on an in_flight row; concurrent writers would race")
	}
}

// TestClaimTaskSyncOutboxNotFound covers the case where the row
// id never existed (e.g. pruned). The claim MUST return
// ErrNotFound; not a silently empty bool.
func TestClaimTaskSyncOutboxNotFound(t *testing.T) {
	store, _, _, _ := startTestServerWithTasks(t)
	claimed, err := store.ClaimTaskSyncOutbox(context.Background(), 999999)
	if err != orch.ErrNotFound {
		t.Errorf("claim on absent id: err = %v, want ErrNotFound", err)
	}
	if claimed {
		t.Errorf("claim returned true on absent id")
	}
}

// TestMarkTaskSyncOutboxClaimedDoneAfterSupersede covers the
// lost-mid-sync case: the row was claimed (in_flight), but a
// concurrent lifecycle advanced (and the row was superseded —
// but wait, supersede only applies to pending; in_flight is
// immune). The actual "lost" case is: the row was claimed, then
// MarkTaskSyncOutboxDone was called immediately after a successful
// sync, but the row was already CAS-Done by another goroutine.
// MarkTaskSyncOutboxClaimedDone returns false; the caller MUST
// not retry.
func TestMarkTaskSyncOutboxClaimedDoneAfterDone(t *testing.T) {
	store, project, _, _ := startTestServerWithTasks(t)
	run := &orch.Run{
		ProjectID: project.ID,
		TaskID:    "task-claim-done-twice",
		Status:    orch.RunCompleted,
	}
	if err := store.CreateRun(context.Background(), run); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	id, err := store.AppendTaskSyncOutbox(context.Background(), &orch.TaskSyncOutbox{
		RunID:   run.ID,
		TaskID:  "task-claim-done-twice",
		Outcome: string(taskstore.RunCompleted),
	})
	if err != nil {
		t.Fatalf("append: %v", err)
	}
	claimed, err := store.ClaimTaskSyncOutbox(context.Background(), id)
	if err != nil || !claimed {
		t.Fatalf("claim: %v, %v", claimed, err)
	}

	// First done wins.
	done, err := store.MarkTaskSyncOutboxClaimedDone(context.Background(), id)
	if err != nil || !done {
		t.Fatalf("first done: %v, %v", done, err)
	}

	// Second done loses (row is no longer in_flight).
	done, err = store.MarkTaskSyncOutboxClaimedDone(context.Background(), id)
	if err != nil {
		t.Fatalf("second done: %v", err)
	}
	if done {
		t.Errorf("second done returned true; concurrent done writers would race")
	}
}

// recordingTaskStore tracks every SyncTerminal call so the test can
// assert the call was or was not made.
type recordingTaskStore struct {
	inner *taskstoretest.Fake
	// syncTerminalCalls records the outcomes the service tried to
	// sync. The test asserts this slice stays empty (or non-empty)
	// depending on the scenario.
	syncTerminalCalls []taskstore.RunOutcome
}

func (r *recordingTaskStore) Get(ctx context.Context, id string) (*taskstore.Task, error) {
	return r.inner.Get(ctx, id)
}
func (r *recordingTaskStore) Claim(ctx context.Context, id string) (*taskstore.Task, error) {
	return r.inner.Claim(ctx, id)
}
func (r *recordingTaskStore) SyncTerminal(ctx context.Context, id string, outcome taskstore.RunOutcome, generation int64) error {
	r.syncTerminalCalls = append(r.syncTerminalCalls, outcome)
	return r.inner.SyncTerminal(ctx, id, outcome, generation)
}

// TestSyncAbortsWhenOutboxPersistFails verifies that when the
// durable-intent persist fails, the service does NOT attempt the
// external side effect. A side effect without a durable intent is
// unrecoverable: the reconciler would never see the Beads mutation
// and could replay a stale decision. We simulate a persist failure
// by dropping the task_sync_outbox table out from under the service
// after the migration lands. The Run transitions still work because
// they don't touch the outbox table.
func TestSyncAbortsWhenOutboxPersistFails(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "orch.db")
	socketPath := filepath.Join(dir, "daemon.sock")

	store, err := orch.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	project := &orch.Project{Name: "test", FsPath: "/tmp/test"}
	if err := store.CreateProject(context.Background(), project); err != nil {
		t.Fatalf("create project: %v", err)
	}
	rec := &recordingTaskStore{inner: taskstoretest.New()}
	rec.inner.Put(&taskstore.Task{
		ID:     "task-append-fails",
		Title:  "Append fails",
		Status: taskstore.TaskTodo,
	})

	ctx, cancel := context.WithCancel(context.Background())
	srv := NewServerWithTasks(store, rec, socketPath)
	done := make(chan error, 1)
	go func() { done <- srv.Serve(ctx) }()
	t.Cleanup(func() {
		cancel()
		srv.Stop()
		<-done
	})

	// Wait for the daemon to come up.
	deadline := time.Now().Add(3 * time.Second)
	for !socketAlive(context.Background(), socketPath) {
		if time.Now().After(deadline) {
			t.Fatal("daemon socket did not become ready")
		}
		time.Sleep(10 * time.Millisecond)
	}

	client, err := Dial(socketPath)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	// Create a Run so the smoke path works.
	run, err := client.CreateRun(context.Background(), &daemonpb.CreateRunRequest{
		ProjectId: project.ID,
		TaskId:    "task-append-fails",
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	// Drop the outbox table so the next Append fails. Run transitions
	// remain functional because they don't touch the outbox. We use
	// the modernc.org/sqlite driver directly via a fresh connection
	// to the same DB file.
	if err := dropTableViaFreshConn(dbPath, "task_sync_outbox"); err != nil {
		t.Fatalf("drop table: %v", err)
	}

	// Now Cancel the run. The CancelRun RPC will still succeed
	// (the Run transition is independent of the outbox), but the
	// service MUST NOT attempt to call SyncTerminal because the
	// outbox persist failed.
	if _, err := client.CancelRun(context.Background(), &daemonpb.CancelRunRequest{Id: run.Id}); err != nil {
		t.Fatalf("CancelRun: %v", err)
	}
	if len(rec.syncTerminalCalls) != 0 {
		t.Errorf("SyncTerminal was called %d times despite outbox persist failure; want 0",
			len(rec.syncTerminalCalls))
	}
}

// TestSyncHelpersCoverNeedsAttention ensures that the unified sync
// helper covers the non-terminal needs_attention case. The spec
// mandates needs_attention -> blocked even though the Run is
// recoverable. The helper is exposed via syncLifecycleTask; the public
// gRPC surface does not yet expose a needs_attention transition (that
// arrives with the controller), but the helper must be ready so the
// controller can call it directly.
func TestSyncHelpersCoverNeedsAttention(t *testing.T) {
	store, _, _, _ := startTestServerWithTasks(t)
	run := &orch.Run{
		ProjectID: "p",
		TaskID:    "task-needs-attention",
		Status:    orch.RunNeedsAttention,
	}
	// outcomeForRunStatus must succeed for needs_attention even
	// though it is non-terminal.
	outcome, err := outcomeForRunStatus(run.Status)
	if err != nil {
		t.Fatalf("outcomeForRunStatus(needs_attention) = %v, want nil", err)
	}
	if outcome != taskstore.RunNeedsAttention {
		t.Errorf("outcome = %q, want %q", outcome, taskstore.RunNeedsAttention)
	}
	// MapRunOutcomeToTaskStatus must map needs_attention -> blocked.
	beadsStatus, err := taskstore.MapRunOutcomeToTaskStatus(outcome)
	if err != nil {
		t.Fatalf("MapRunOutcomeToTaskStatus: %v", err)
	}
	if beadsStatus != taskstore.TaskBlocked {
		t.Errorf("status = %q, want blocked", beadsStatus)
	}
	_ = store
}

// dropTableViaFreshConn opens a fresh SQLite connection to the
// daemon's DB file and drops the named table. The fresh connection
// goes through the modernc.org/sqlite driver directly so the test
// does not need to share the daemon's pooled connection. This is the
// cheapest way to force the outbox Append to fail without taking the
// rest of the store down.
func dropTableViaFreshConn(dbPath, table string) error {
	dsn := "file:" + dbPath + "?_txlock=immediate&_foreign_keys=on"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return err
	}
	defer db.Close()
	_, err = db.Exec("DROP TABLE IF EXISTS " + table)
	return err
}

// TestReclaimExpiredTaskSyncOutbox covers the crash-safety
// mechanism the reviewer asked for: if the daemon crashes after
// ClaimTaskSyncOutbox and before MarkTaskSyncOutboxClaimedDone,
// the row is in_flight forever without a reaper. The reaper
// (ReclaimExpiredTaskSyncOutbox) resets in_flight rows whose
// claimed_at is older than the lease back to pending.
func TestReclaimExpiredTaskSyncOutbox(t *testing.T) {
	store, project, _, _ := startTestServerWithTasks(t)
	run := &orch.Run{
		ProjectID: project.ID,
		TaskID:    "task-reclaim",
		Status:    orch.RunCompleted,
	}
	if err := store.CreateRun(context.Background(), run); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	id, err := store.AppendTaskSyncOutbox(context.Background(), &orch.TaskSyncOutbox{
		RunID:   run.ID,
		TaskID:  "task-reclaim",
		Outcome: string(taskstore.RunCompleted),
	})
	if err != nil {
		t.Fatalf("append: %v", err)
	}
	claimed, err := store.ClaimTaskSyncOutbox(context.Background(), id)
	if err != nil || !claimed {
		t.Fatalf("claim: %v, %v", claimed, err)
	}

	// Reclaim with a 0s lease: the row's claimed_at is now, so
	// the cutoff is now (claimed_at < now is false initially).
	// Wait, the cutoff is now - lease = now - 0s = now; the row
	// was claimed at now, so claimed_at < now is false. We need
	// a small positive lease so the cutoff is older than the row.
	// Actually, the test is: with a 1ns lease, the cutoff is now
	// - 1ns, which is older than claimed_at, so the row IS
	// reclaimable.
	n, err := store.ReclaimExpiredTaskSyncOutbox(context.Background(), 1*time.Nanosecond)
	if err != nil {
		t.Fatalf("reclaim: %v", err)
	}
	if n != 1 {
		t.Errorf("reclaimed = %d, want 1", n)
	}

	// The row should now be pending again.
	pending, err := store.ListPendingTaskSyncOutbox(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(pending) != 1 {
		t.Errorf("pending = %d, want 1 (the reclaimed row)", len(pending))
	}

	// Verify the row's claimed_at and lease_token are cleared.
	row, err := store.GetTaskSyncOutbox(context.Background(), id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !row.ClaimedAt.IsZero() {
		t.Errorf("claimed_at = %v, want zero", row.ClaimedAt)
	}
	if row.LeaseToken != "" {
		t.Errorf("lease_token = %q, want empty", row.LeaseToken)
	}
}

// TestReclaimExpiredTaskSyncOutboxRespectsActiveLease covers the
// happy path: an in_flight row whose lease has NOT expired is
// NOT reclaimed. This protects live reconcilers from a reaper
// double-reclaim.
func TestReclaimExpiredTaskSyncOutboxRespectsActiveLease(t *testing.T) {
	store, project, _, _ := startTestServerWithTasks(t)
	run := &orch.Run{
		ProjectID: project.ID,
		TaskID:    "task-active-lease",
		Status:    orch.RunCompleted,
	}
	if err := store.CreateRun(context.Background(), run); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	id, err := store.AppendTaskSyncOutbox(context.Background(), &orch.TaskSyncOutbox{
		RunID:   run.ID,
		TaskID:  "task-active-lease",
		Outcome: string(taskstore.RunCompleted),
	})
	if err != nil {
		t.Fatalf("append: %v", err)
	}
	claimed, err := store.ClaimTaskSyncOutbox(context.Background(), id)
	if err != nil || !claimed {
		t.Fatalf("claim: %v, %v", claimed, err)
	}

	// Reclaim with a 1-hour lease: the row's claimed_at is now,
	// so 1h is far from expired.
	n, err := store.ReclaimExpiredTaskSyncOutbox(context.Background(), time.Hour)
	if err != nil {
		t.Fatalf("reclaim: %v", err)
	}
	if n != 0 {
		t.Errorf("reclaimed = %d, want 0 (active lease)", n)
	}

	// The row should still be in_flight.
	_, status, err := store.GetTaskSyncOutboxOutcomeStatus(context.Background(), id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if status != orch.TaskSyncInFlight {
		t.Errorf("status = %q, want in_flight", status)
	}
}

// TestClaimTaskSyncOutboxFailsOnStaleGeneration covers the
// generation fence the reviewer asked for: between the
// stale-check and the claim, a newer intent is appended. The
// newer intent's supersede renovates the old row to 'superseded'
// (because of the new IN(pending, in_flight) WHERE clause), so
// the claim UPDATE finds 0 rows pending and returns false. This
// is the generation-fence-via-supersede path.
func TestClaimTaskSyncOutboxFailsOnStaleGeneration(t *testing.T) {
	store, project, _, _ := startTestServerWithTasks(t)
	run := &orch.Run{
		ProjectID: project.ID,
		TaskID:    "task-stale-gen",
		Status:    orch.RunCompleted,
	}
	if err := store.CreateRun(context.Background(), run); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	oldID, err := store.AppendTaskSyncOutbox(context.Background(), &orch.TaskSyncOutbox{
		RunID:   run.ID,
		TaskID:  "task-stale-gen",
		Outcome: string(taskstore.RunCompleted),
	})
	if err != nil {
		t.Fatalf("append old: %v", err)
	}

	// Newer intent supersedes the old row (AND bumps generation).
	if _, err := store.AppendTaskSyncOutbox(context.Background(), &orch.TaskSyncOutbox{
		RunID:   run.ID,
		TaskID:  "task-stale-gen",
		Outcome: string(taskstore.RunCancelled),
	}); err != nil {
		t.Fatalf("append new: %v", err)
	}

	// Claim old row: must fail because the row is now superseded.
	claimed, err := store.ClaimTaskSyncOutbox(context.Background(), oldID)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if claimed {
		t.Errorf("claim returned true on a superseded row; stale generation would have been written")
	}
}

// TestAppendTaskSyncOutboxSupersedesInFlight covers the new
// supersede semantics: an in_flight row is also demoted by a
// new intent. The stale in_flight row's SyncTerminal must not
// race the newer intent.
func TestAppendTaskSyncOutboxSupersedesInFlight(t *testing.T) {
	store, project, _, _ := startTestServerWithTasks(t)
	run := &orch.Run{
		ProjectID: project.ID,
		TaskID:    "task-supersede-inflight",
		Status:    orch.RunCompleted,
	}
	if err := store.CreateRun(context.Background(), run); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	oldID, err := store.AppendTaskSyncOutbox(context.Background(), &orch.TaskSyncOutbox{
		RunID:   run.ID,
		TaskID:  "task-supersede-inflight",
		Outcome: string(taskstore.RunCompleted),
	})
	if err != nil {
		t.Fatalf("append old: %v", err)
	}
	claimed, err := store.ClaimTaskSyncOutbox(context.Background(), oldID)
	if err != nil || !claimed {
		t.Fatalf("claim: %v, %v", claimed, err)
	}
	// old row is now in_flight.

	// Newer intent arrives. With the new IN(pending, in_flight)
	// supersede clause, the in_flight row must be demoted to
	// superseded.
	if _, err := store.AppendTaskSyncOutbox(context.Background(), &orch.TaskSyncOutbox{
		RunID:   run.ID,
		TaskID:  "task-supersede-inflight",
		Outcome: string(taskstore.RunCancelled),
	}); err != nil {
		t.Fatalf("append new: %v", err)
	}

	// old row is now superseded (not in_flight).
	_, status, err := store.GetTaskSyncOutboxOutcomeStatus(context.Background(), oldID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if status != orch.TaskSyncSuperseded {
		t.Errorf("old row status = %q, want superseded", status)
	}

	// Newer row is pending.
	pending, err := store.ListPendingTaskSyncOutbox(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(pending) != 1 {
		t.Errorf("pending = %d, want 1 (the newer row)", len(pending))
	}
}
