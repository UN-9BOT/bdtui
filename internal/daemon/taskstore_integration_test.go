package daemon

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"bdtui/internal/daemon/daemonpb"
	"bdtui/internal/orch"
	"bdtui/internal/taskstore"
	"bdtui/internal/taskstore/taskstoretest"

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
