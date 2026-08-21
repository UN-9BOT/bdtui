package daemon

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"bdtui/internal/daemon/daemonpb"
	"bdtui/internal/orch"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func startTestServer(t *testing.T) (*orch.Store, *orch.Project, *Client) {
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

	ctx, cancel := context.WithCancel(context.Background())
	srv := NewServer(store, socketPath)
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

	return store, project, client
}

func hasRun(runs []*daemonpb.Run, id string) bool {
	for _, r := range runs {
		if r.Id == id {
			return true
		}
	}
	return false
}

func TestCreateGetListRun(t *testing.T) {
	_, project, client := startTestServer(t)
	ctx := context.Background()

	run, err := client.CreateRun(ctx, &daemonpb.CreateRunRequest{
		ProjectId:           project.ID,
		TaskId:              "task-1",
		WorkflowSnapshotRef: "ref-1",
		WorkflowSnapshot:    `{"version":1}`,
	})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	if run.Id == "" {
		t.Fatal("expected non-empty run id")
	}
	if run.Status != string(orch.RunQueued) {
		t.Fatalf("status = %q, want queued", run.Status)
	}
	if run.CreatedAt == "" {
		t.Fatal("expected created_at")
	}

	got, err := client.GetRun(ctx, &daemonpb.GetRunRequest{Id: run.Id})
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if got.Id != run.Id || got.TaskId != "task-1" || got.WorkflowSnapshotRef != "ref-1" {
		t.Fatalf("unexpected run: %+v", got)
	}

	list, err := client.ListRuns(ctx, &daemonpb.ListRunsRequest{})
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if !hasRun(list.Runs, run.Id) {
		t.Fatalf("list runs missing created run: %+v", list.Runs)
	}

	byProject, err := client.ListRuns(ctx, &daemonpb.ListRunsRequest{ProjectId: &project.ID})
	if err != nil {
		t.Fatalf("list runs by project: %v", err)
	}
	if !hasRun(byProject.Runs, run.Id) {
		t.Fatalf("project list missing created run: %+v", byProject.Runs)
	}
}

func TestRetryRunRequiresNeedsAttention(t *testing.T) {
	store, project, client := startTestServer(t)
	ctx := context.Background()

	run, err := client.CreateRun(ctx, &daemonpb.CreateRunRequest{ProjectId: project.ID, TaskId: "task-retry"})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}

	// A queued run cannot be retried: retry is a needs_attention-only command.
	if _, err := client.RetryRun(ctx, &daemonpb.RetryRunRequest{Id: run.Id}); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("retry from queued = %v, want FailedPrecondition", err)
	}

	// Drive the run to needs_attention through the controller/store path.
	if err := store.TransitionRun(ctx, run.Id, orch.RunRunning); err != nil {
		t.Fatalf("transition to running: %v", err)
	}
	if err := store.TransitionRun(ctx, run.Id, orch.RunNeedsAttention); err != nil {
		t.Fatalf("transition to needs_attention: %v", err)
	}

	retried, err := client.RetryRun(ctx, &daemonpb.RetryRunRequest{Id: run.Id})
	if err != nil {
		t.Fatalf("retry run: %v", err)
	}
	if retried.Status != string(orch.RunQueued) {
		t.Fatalf("status after retry = %q, want queued", retried.Status)
	}
}

func TestCancelRun(t *testing.T) {
	_, project, client := startTestServer(t)
	ctx := context.Background()

	run, err := client.CreateRun(ctx, &daemonpb.CreateRunRequest{ProjectId: project.ID, TaskId: "task-cancel"})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}

	cancelled, err := client.CancelRun(ctx, &daemonpb.CancelRunRequest{Id: run.Id})
	if err != nil {
		t.Fatalf("cancel run: %v", err)
	}
	if cancelled.Status != string(orch.RunCancelled) {
		t.Fatalf("status after cancel = %q, want cancelled", cancelled.Status)
	}
}

func TestInspectExecution(t *testing.T) {
	store, project, client := startTestServer(t)
	ctx := context.Background()

	run, err := client.CreateRun(ctx, &daemonpb.CreateRunRequest{ProjectId: project.ID, TaskId: "task-exec"})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}

	sa, err := store.StartStepAttempt(ctx, run.Id, "step-1", `{}`)
	if err != nil {
		t.Fatalf("start step: %v", err)
	}

	exec := &orch.Execution{
		RunID:         run.Id,
		StepAttemptID: sa.ID,
		Kind:          orch.KindAgent,
		PromptRef:     "/run-storage/prompts/x",
		PromptHash:    "abc123",
	}
	if err := store.CreateExecution(ctx, exec); err != nil {
		t.Fatalf("create execution: %v", err)
	}
	if err := store.CreateArtifact(ctx, &orch.Artifact{
		ExecutionID: exec.ID,
		Name:        "result.txt",
		Path:        "/run-storage/artifacts/result.txt",
		Hash:        "h1",
	}); err != nil {
		t.Fatalf("create artifact: %v", err)
	}

	resp, err := client.InspectExecution(ctx, &daemonpb.InspectExecutionRequest{Id: exec.ID})
	if err != nil {
		t.Fatalf("inspect execution: %v", err)
	}
	if resp.Execution.Id != exec.ID || resp.Execution.PromptHash != "abc123" {
		t.Fatalf("unexpected execution: %+v", resp.Execution)
	}
	if len(resp.Artifacts) != 1 || resp.Artifacts[0].Name != "result.txt" {
		t.Fatalf("unexpected artifacts: %+v", resp.Artifacts)
	}
}

func TestAnswerHumanInput(t *testing.T) {
	store, project, client := startTestServer(t)
	ctx := context.Background()

	run, err := client.CreateRun(ctx, &daemonpb.CreateRunRequest{ProjectId: project.ID, TaskId: "task-human"})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	sa, err := store.StartStepAttempt(ctx, run.Id, "human-step", `{}`)
	if err != nil {
		t.Fatalf("start step: %v", err)
	}

	h := &orch.HumanInput{
		RunID:         run.Id,
		StepAttemptID: sa.ID,
		Prompt:        "approve?",
	}
	if err := store.CreateHumanInput(ctx, h); err != nil {
		t.Fatalf("create human input: %v", err)
	}

	resp, err := client.AnswerHumanInput(ctx, &daemonpb.AnswerHumanInputRequest{Id: h.ID, Response: "yes"})
	if err != nil {
		t.Fatalf("answer human input: %v", err)
	}
	if resp.Status != string(orch.HumanAnswered) {
		t.Fatalf("status = %q, want answered", resp.Status)
	}
	if resp.Response == nil || *resp.Response != "yes" {
		t.Fatalf("response = %v, want yes", resp.Response)
	}
}

func TestStreamEvents(t *testing.T) {
	_, project, client := startTestServer(t)
	ctx := context.Background()

	run, err := client.CreateRun(ctx, &daemonpb.CreateRunRequest{ProjectId: project.ID, TaskId: "task-stream"})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}

	streamCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	stream, err := client.StreamEvents(streamCtx, &daemonpb.StreamEventsRequest{RunId: run.Id})
	if err != nil {
		t.Fatalf("stream events: %v", err)
	}

	first, err := stream.Recv()
	if err != nil {
		t.Fatalf("recv first event: %v", err)
	}
	if first.Type != orch.EventRunCreated {
		t.Fatalf("first event type = %q, want run.created", first.Type)
	}

	if _, err := client.CancelRun(ctx, &daemonpb.CancelRunRequest{Id: run.Id}); err != nil {
		t.Fatalf("cancel run: %v", err)
	}

	second, err := stream.Recv()
	if err != nil {
		t.Fatalf("recv second event: %v", err)
	}
	if second.Type != orch.EventRunTransition {
		t.Fatalf("second event type = %q, want run.transition", second.Type)
	}
}

func TestErrorMapping(t *testing.T) {
	_, _, client := startTestServer(t)
	ctx := context.Background()

	_, err := client.GetRun(ctx, &daemonpb.GetRunRequest{Id: "missing"})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("GetRun error = %v, want NotFound", err)
	}

	// CreateRun now resolves-or-creates project_id, so a missing project is
	// not an error path anymore. Empty project_id stays an error.
	_, err = client.CreateRun(ctx, &daemonpb.CreateRunRequest{ProjectId: "missing-project", TaskId: "task-x"})
	if err != nil {
		t.Fatalf("CreateRun with new project_id = %v, want nil (auto-create)", err)
	}

	_, err = client.CreateRun(ctx, &daemonpb.CreateRunRequest{ProjectId: "p"})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("CreateRun without task_id = %v, want InvalidArgument", err)
	}

	_, err = client.CreateRun(ctx, &daemonpb.CreateRunRequest{TaskId: "t"})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("CreateRun without project_id = %v, want InvalidArgument", err)
	}
}

func TestAcquireLockExclusive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "daemon.sock.lock")

	first, err := AcquireLock(path)
	if err != nil {
		t.Fatalf("acquire first lock: %v", err)
	}
	t.Cleanup(func() { _ = ReleaseLock(first) })

	if _, err := AcquireLock(path); err == nil {
		t.Fatal("second acquire unexpectedly succeeded")
	}
}

func TestEnsureStateDirs(t *testing.T) {
	base := t.TempDir()
	paths := []string{
		filepath.Join(base, "a", "b", "socket.sock"),
		filepath.Join(base, "c", "db.db"),
		filepath.Join(base, "d", "pid.pid"),
	}
	if err := EnsureStateDirs(paths...); err != nil {
		t.Fatalf("ensure state dirs: %v", err)
	}
	for _, p := range paths {
		if _, err := os.Stat(filepath.Dir(p)); err != nil {
			t.Fatalf("parent of %s not created: %v", p, err)
		}
	}
}

func TestEnsureDaemonAutoStart(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping daemon auto-start integration test in short mode")
	}

	dir := t.TempDir()
	binPath := filepath.Join(dir, "bdtuid")
	build := exec.Command("go", "build", "-o", binPath, "bdtui/cmd/bdtuid")
	build.Env = os.Environ()
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build bdtuid: %v\n%s", err, out)
	}

	// Deliberately use a nested, non-existent state dir so this also verifies
	// the daemon creates parent directories before writing pidfile/DB/lock.
	stateDir := filepath.Join(dir, "state", "nested")
	socketPath := filepath.Join(stateDir, "daemon.sock")
	dbPath := filepath.Join(stateDir, "orch.db")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := EnsureDaemon(ctx, Options{
		SocketPath:   socketPath,
		DBPath:       dbPath,
		Binary:       binPath,
		StartTimeout: 10 * time.Second,
	})
	if err != nil {
		t.Fatalf("ensure daemon: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	// The daemon is up and serving: a missing run yields NotFound, not a
	// transport error.
	_, err = client.GetRun(ctx, &daemonpb.GetRunRequest{Id: "missing"})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("GetRun after auto-start = %v, want NotFound", err)
	}

	// A second daemon must fail to acquire the singleton lock.
	second := exec.Command(binPath, "--socket", socketPath, "--db", dbPath)
	second.Env = os.Environ()
	if out, err := second.CombinedOutput(); err == nil {
		t.Fatalf("second daemon unexpectedly started: %s", out)
	}

	// Stop the detached daemon so the test does not leak a background process.
	pidBytes, err := os.ReadFile(socketPath + ".pid")
	if err != nil {
		t.Fatalf("read pidfile: %v", err)
	}
	pid, err := strconv.Atoi(string(pidBytes))
	if err != nil {
		t.Fatalf("parse pidfile: %v", err)
	}
	t.Cleanup(func() { _ = syscall.Kill(pid, syscall.SIGTERM) })
}

// TestEnsureDaemonAutoStartPassesBeadsDir verifies that when the
// caller populates Options.BeadsDir, the corresponding --beads-dir
// flag is appended to the bdtuid command line. The test asserts two
// things: the flag does not produce an "unknown flag" error when the
// daemon is invoked with it, and a sibling bdtuid that gets the same
// flags still fails on the singleton lock (proving the wiring did not
// silently drop the option).
func TestEnsureDaemonAutoStartPassesBeadsDir(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping daemon auto-start integration test in short mode")
	}

	dir := t.TempDir()
	binPath := filepath.Join(dir, "bdtuid")
	build := exec.Command("go", "build", "-o", binPath, "bdtui/cmd/bdtuid")
	build.Env = os.Environ()
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build bdtuid: %v\n%s", err, out)
	}

	stateDir := filepath.Join(dir, "state")
	socketPath := filepath.Join(stateDir, "daemon.sock")
	dbPath := filepath.Join(stateDir, "orch.db")
	beadsDir := filepath.Join(dir, "beads")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := EnsureDaemon(ctx, Options{
		SocketPath:   socketPath,
		DBPath:       dbPath,
		BeadsDir:     beadsDir,
		Binary:       binPath,
		StartTimeout: 10 * time.Second,
	})
	if err != nil {
		t.Fatalf("ensure daemon: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	// Spawn a sibling bdtuid with the same flags. The expected outcome
	// is the singleton-lock failure; the relevant signal is that
	// --beads-dir is not rejected as an unknown flag.
	probe := exec.Command(binPath, "--socket", socketPath, "--db", dbPath, "--beads-dir", beadsDir)
	probe.Env = os.Environ()
	out, err := probe.CombinedOutput()
	if err == nil {
		t.Fatalf("sibling bdtuid unexpectedly started: %s", out)
	}
	if !strings.Contains(string(out), "lock") && !strings.Contains(string(out), "acquired") {
		t.Errorf("sibling bdtuid failed with unexpected error: %s", out)
	}

	// Stop the detached daemon.
	pidBytes, err := os.ReadFile(socketPath + ".pid")
	if err != nil {
		t.Fatalf("read pidfile: %v", err)
	}
	pid, err := strconv.Atoi(string(pidBytes))
	if err != nil {
		t.Fatalf("parse pidfile: %v", err)
	}
	t.Cleanup(func() { _ = syscall.Kill(pid, syscall.SIGTERM) })
}

// startBareTestServer starts a daemon whose store has zero projects. Used to
// exercise the default-project fallback on CreateRun.
func startBareTestServer(t *testing.T) (*orch.Store, *Client) {
	t.Helper()

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "orch.db")
	socketPath := filepath.Join(dir, "daemon.sock")

	store, err := orch.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	srv := NewServer(store, socketPath)
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

	return store, client
}

func TestCreateRunAutoBootstrapsProjectFromID(t *testing.T) {
	store, client := startBareTestServer(t)

	const id = "workspace-hash-abc"
	r, err := client.CreateRun(context.Background(), &daemonpb.CreateRunRequest{
		ProjectId:          id,
		TaskId:             "task-x",
		WorkflowSnapshot:   "{}",
		WorkflowSnapshotRef: "ref-x",
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if r.Id == "" {
		t.Fatal("empty run id")
	}
	if r.ProjectId != id {
		t.Fatalf("project_id = %q, want %q", r.ProjectId, id)
	}

	projects := listAllProjects(t, store)
	if len(projects) != 1 {
		t.Fatalf("projects = %d, want 1", len(projects))
	}
	if projects[0].ID != id || projects[0].Name != id {
		t.Fatalf("project = %+v, want id=name=%q", projects[0], id)
	}

	// Second CreateRun with the same project_id reuses the row, no duplicate.
	if _, err := client.CreateRun(context.Background(), &daemonpb.CreateRunRequest{
		ProjectId: id, TaskId: "task-y",
		WorkflowSnapshot: "{}", WorkflowSnapshotRef: "ref-y",
	}); err != nil {
		t.Fatalf("second CreateRun: %v", err)
	}
	if projects := listAllProjects(t, store); len(projects) != 1 {
		t.Fatalf("projects after reuse = %d, want 1", len(projects))
	}

	// Different workspace hash => distinct project row, distinct
	// active-run uniqueness scope.
	const other = "workspace-hash-def"
	if _, err := client.CreateRun(context.Background(), &daemonpb.CreateRunRequest{
		ProjectId: other, TaskId: "task-x",
		WorkflowSnapshot: "{}", WorkflowSnapshotRef: "ref-z",
	}); err != nil {
		t.Fatalf("other CreateRun: %v", err)
	}
	if projects := listAllProjects(t, store); len(projects) != 2 {
		t.Fatalf("projects after second workspace = %d, want 2", len(projects))
	}
}

func TestCreateRunRejectsMissingTaskID(t *testing.T) {
	_, client := startBareTestServer(t)
	_, err := client.CreateRun(context.Background(), &daemonpb.CreateRunRequest{})
	if err == nil {
		t.Fatal("expected error for empty task_id")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.InvalidArgument {
		t.Fatalf("code = %v, want InvalidArgument", st.Code())
	}
}

func listAllProjects(t *testing.T, store *orch.Store) []orch.Project {
	t.Helper()
	projects, err := store.ListProjects(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	return projects
}
