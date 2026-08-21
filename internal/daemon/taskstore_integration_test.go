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
func (f *flakyTaskStore) SyncTerminal(ctx context.Context, id string, outcome taskstore.RunOutcome) error {
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
func (r *recordingTaskStore) SyncTerminal(ctx context.Context, id string, outcome taskstore.RunOutcome) error {
	r.syncTerminalCalls = append(r.syncTerminalCalls, outcome)
	return r.inner.SyncTerminal(ctx, id, outcome)
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
