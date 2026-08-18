package orch

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	s, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func newProject(t *testing.T, s *Store, name string) *Project {
	t.Helper()
	p := &Project{Name: name, FsPath: "/" + name}
	if err := s.CreateProject(context.Background(), p); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	return p
}

func newRun(t *testing.T, s *Store, projectID, taskID string) *Run {
	t.Helper()
	r := &Run{ProjectID: projectID, TaskID: taskID, Status: RunQueued}
	if err := s.CreateRun(context.Background(), r); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	return r
}

func TestMigrateIdempotent(t *testing.T) {
	s := newTestStore(t)
	if err := s.Migrate(context.Background()); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
}

func TestProjectCreateGetUpdate(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	p := &Project{Name: "acme", FsPath: "/tmp/acme", GitRemote: "git@example.com:acme.git"}
	if err := s.CreateProject(ctx, p); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if p.ID == "" {
		t.Fatal("expected generated project ID")
	}

	got, err := s.GetProject(ctx, p.ID)
	if err != nil {
		t.Fatalf("GetProject: %v", err)
	}
	if got.FsPath != "/tmp/acme" || got.GitRemote != "git@example.com:acme.git" {
		t.Fatalf("unexpected project: %+v", got)
	}

	got.FsPath = "/new/path"
	got.GitRemote = "git@example.com:acme2.git"
	if err := s.UpdateProject(ctx, got); err != nil {
		t.Fatalf("UpdateProject: %v", err)
	}
	after, _ := s.GetProject(ctx, p.ID)
	if after.FsPath != "/new/path" || after.GitRemote != "git@example.com:acme2.git" {
		t.Fatalf("mutable attributes not updated: %+v", after)
	}
	if after.ID != p.ID {
		t.Fatal("project ID must be stable")
	}

	// Both create and update emit project.upserted (audit stream visibility).
	var upserted int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM events WHERE run_id IS NULL AND type = ?`, EventProjectUpserted,
	).Scan(&upserted); err != nil {
		t.Fatal(err)
	}
	if upserted != 2 {
		t.Fatalf("expected 2 project.upserted events, got %d", upserted)
	}
}

func TestRunLifecycleAtomicTransitions(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	p := newProject(t, s, "p")
	r := newRun(t, s, p.ID, "task-1")

	// queued -> running sets started_at.
	if err := s.TransitionRun(ctx, r.ID, RunRunning); err != nil {
		t.Fatalf("queued->running: %v", err)
	}
	got, _ := s.GetRun(ctx, r.ID)
	if got.Status != RunRunning || got.StartedAt == nil {
		t.Fatalf("expected running with started_at, got %+v", got)
	}

	// running -> waiting_human.
	if err := s.TransitionRun(ctx, r.ID, RunWaitingHuman); err != nil {
		t.Fatalf("running->waiting_human: %v", err)
	}
	// waiting_human -> needs_attention, then set the reason separately.
	if err := s.TransitionRun(ctx, r.ID, RunNeedsAttention); err != nil {
		t.Fatalf("waiting_human->needs_attention: %v", err)
	}
	if err := s.SetRunNeedsAttentionReason(ctx, r.ID, strPtrTo("writer ambiguous")); err != nil {
		t.Fatalf("SetRunNeedsAttentionReason: %v", err)
	}
	got, _ = s.GetRun(ctx, r.ID)
	if got.Status != RunNeedsAttention || got.NeedsAttentionReason == nil || *got.NeedsAttentionReason != "writer ambiguous" {
		t.Fatalf("unexpected needs_attention state: %+v", got)
	}

	// needs_attention -> completed sets completed_at.
	if err := s.TransitionRun(ctx, r.ID, RunCompleted); err != nil {
		t.Fatalf("needs_attention->completed: %v", err)
	}
	got, _ = s.GetRun(ctx, r.ID)
	if got.Status != RunCompleted || got.CompletedAt == nil {
		t.Fatalf("expected completed with completed_at, got %+v", got)
	}
}

func TestTransitionRunPreservesMetadata(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	p := newProject(t, s, "p")
	r := newRun(t, s, p.ID, "task-1")

	step := "plan_review"
	if err := s.SetRunCurrentStep(ctx, r.ID, &step); err != nil {
		t.Fatal(err)
	}
	if err := s.SetRunError(ctx, r.ID, strPtrTo("boom")); err != nil {
		t.Fatal(err)
	}

	// A pure status transition must not clear metadata.
	if err := s.TransitionRun(ctx, r.ID, RunRunning); err != nil {
		t.Fatal(err)
	}
	if err := s.TransitionRun(ctx, r.ID, RunWaitingHuman); err != nil {
		t.Fatal(err)
	}

	got, _ := s.GetRun(ctx, r.ID)
	if got.CurrentStepID == nil || *got.CurrentStepID != "plan_review" {
		t.Fatalf("current_step_id was wiped: %+v", got)
	}
	if got.Error == nil || *got.Error != "boom" {
		t.Fatalf("error was wiped: %+v", got)
	}
}

func TestTransitionRunStateMachine(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	p := newProject(t, s, "p")

	// queued -> completed is illegal (must go through running).
	r := newRun(t, s, p.ID, "task-1")
	if err := s.TransitionRun(ctx, r.ID, RunCompleted); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("queued->completed: expected ErrInvalidTransition, got %v", err)
	}

	// terminal -> running is illegal.
	r2 := newRun(t, s, p.ID, "task-2")
	_ = s.TransitionRun(ctx, r2.ID, RunRunning)
	_ = s.TransitionRun(ctx, r2.ID, RunCompleted)
	if err := s.TransitionRun(ctx, r2.ID, RunRunning); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("completed->running: expected ErrInvalidTransition, got %v", err)
	}
}

func TestCreateRunEnforcesSingleActivePerTask(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	p := newProject(t, s, "p")

	_ = newRun(t, s, p.ID, "task-shared")

	// Same project + same task: rejected.
	r2 := &Run{ProjectID: p.ID, TaskID: "task-shared", Status: RunQueued}
	if err := s.CreateRun(ctx, r2); !errors.Is(err, ErrActiveRunExists) {
		t.Fatalf("expected ErrActiveRunExists, got %v", err)
	}
}

func TestActiveTaskUniquenessIsProjectScoped(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	pA := newProject(t, s, "a")
	pB := newProject(t, s, "b")

	_ = newRun(t, s, pA.ID, "task-123")

	// Same task id in a different project is allowed.
	rB := &Run{ProjectID: pB.ID, TaskID: "task-123", Status: RunQueued}
	if err := s.CreateRun(ctx, rB); err != nil {
		t.Fatalf("cross-project task should be allowed, got %v", err)
	}

	// But a second active run for the same (project, task) is rejected.
	rA2 := &Run{ProjectID: pA.ID, TaskID: "task-123", Status: RunQueued}
	if err := s.CreateRun(ctx, rA2); !errors.Is(err, ErrActiveRunExists) {
		t.Fatalf("expected ErrActiveRunExists within same project, got %v", err)
	}
}

func TestEventsAppendOnlyPerRunSeq(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	p := newProject(t, s, "p")
	r := newRun(t, s, p.ID, "task-1")

	_ = s.AppendEvent(ctx, &r.ID, "custom.one", `{"a":1}`)
	_ = s.AppendEvent(ctx, &r.ID, "custom.two", `{"a":2}`)

	events, err := s.ListEventsByRun(ctx, r.ID)
	if err != nil {
		t.Fatal(err)
	}
	// run.created (from CreateRun) + two custom events.
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}
	for i, e := range events {
		if e.Seq != int64(i+1) {
			t.Fatalf("event %d: expected seq %d, got %d", i, i+1, e.Seq)
		}
	}
	if events[0].Type != EventRunCreated {
		t.Fatalf("expected first event %s, got %s", EventRunCreated, events[0].Type)
	}
}

func TestConcurrentAppendEvent(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	p := newProject(t, s, "p")
	r := newRun(t, s, p.ID, "task-1")

	const n = 50
	var wg sync.WaitGroup
	errCh := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if err := s.AppendEvent(ctx, &r.ID, "concurrent", fmt.Sprintf(`{"i":%d}`, i)); err != nil {
				errCh <- err
			}
		}(i)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatalf("AppendEvent: %v", err)
	}

	events, err := s.ListEventsByRun(ctx, r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != n+1 {
		t.Fatalf("expected %d events, got %d", n+1, len(events))
	}
	seen := make(map[int64]bool, len(events))
	for _, e := range events {
		if seen[e.Seq] {
			t.Fatalf("duplicate seq %d", e.Seq)
		}
		seen[e.Seq] = true
	}
	for i := int64(1); i <= n+1; i++ {
		if !seen[i] {
			t.Fatalf("missing seq %d", i)
		}
	}
}

func TestStartStepAttemptAtomicNumbering(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	p := newProject(t, s, "p")
	r := newRun(t, s, p.ID, "task-1")

	a1, err := s.StartStepAttempt(ctx, r.ID, "step-x", `{"in":1}`)
	if err != nil {
		t.Fatal(err)
	}
	a2, err := s.StartStepAttempt(ctx, r.ID, "step-x", `{"in":2}`)
	if err != nil {
		t.Fatal(err)
	}
	a3, err := s.StartStepAttempt(ctx, r.ID, "other-step", `{"in":3}`)
	if err != nil {
		t.Fatal(err)
	}

	if a1.Attempt != 1 || a2.Attempt != 2 {
		t.Fatalf("attempt numbers not monotonic per step: %d, %d", a1.Attempt, a2.Attempt)
	}
	if a3.Attempt != 1 {
		t.Fatalf("attempt numbering must be per (run, step): got %d", a3.Attempt)
	}
	if a1.Status != StepQueued {
		t.Fatalf("expected queued, got %s", a1.Status)
	}
}

func TestConcurrentStartStepAttempt(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	p := newProject(t, s, "p")
	r := newRun(t, s, p.ID, "task-1")

	const n = 25
	var wg sync.WaitGroup
	errCh := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := s.StartStepAttempt(ctx, r.ID, "step-x", "{}"); err != nil {
				errCh <- err
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatalf("StartStepAttempt: %v", err)
	}

	// Attempts must be unique and cover 1..n.
	seen := make(map[int]bool)
	rows, err := s.db.QueryContext(ctx, `SELECT attempt FROM step_attempts WHERE run_id = ? AND step_id = ?`, r.ID, "step-x")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var attempt int
		if err := rows.Scan(&attempt); err != nil {
			t.Fatal(err)
		}
		if seen[attempt] {
			t.Fatalf("duplicate attempt %d", attempt)
		}
		seen[attempt] = true
	}
	for i := 1; i <= n; i++ {
		if !seen[i] {
			t.Fatalf("missing attempt %d", i)
		}
	}
}

func TestExecutionLifecycle(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	p := newProject(t, s, "p")
	r := newRun(t, s, p.ID, "task-1")
	sa, _ := s.StartStepAttempt(ctx, r.ID, "step-x", "{}")

	e := &Execution{
		RunID:         r.ID,
		StepAttemptID: sa.ID,
		Kind:          KindAgent,
		Status:        ExecQueued,
		PromptRef:     "runs/" + r.ID + "/exec/prompt.md",
		PromptHash:    "sha256:abc",
	}
	if err := s.CreateExecution(ctx, e); err != nil {
		t.Fatal(err)
	}
	if e.ID == "" {
		t.Fatal("expected execution ID")
	}

	if err := s.TransitionExecution(ctx, e.ID, ExecRunning); err != nil {
		t.Fatal(err)
	}
	if err := s.TransitionExecution(ctx, e.ID, ExecCompleted); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetExecution(ctx, e.ID)
	if got.Status != ExecCompleted || got.CompletedAt == nil {
		t.Fatalf("unexpected execution: %+v", got)
	}
	if got.PromptRef != e.PromptRef || got.PromptHash != e.PromptHash {
		t.Fatalf("prompt ref/hash not round-tripped: %+v", got)
	}

	if err := s.TransitionExecution(ctx, e.ID, ExecRunning); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("expected ErrInvalidTransition, got %v", err)
	}
}

func TestArtifactLifecycle(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	p := newProject(t, s, "p")
	r := newRun(t, s, p.ID, "task-1")
	sa, _ := s.StartStepAttempt(ctx, r.ID, "step-x", "{}")
	e := &Execution{RunID: r.ID, StepAttemptID: sa.ID, Kind: KindAgent, Status: ExecCompleted}
	_ = s.CreateExecution(ctx, e)

	a := &Artifact{ExecutionID: e.ID, Name: "report.md", Path: "artifacts/report.md", Hash: "sha256:def"}
	if err := s.CreateArtifact(ctx, a); err != nil {
		t.Fatal(err)
	}
	arts, err := s.ListArtifactsByExecution(ctx, e.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(arts) != 1 || arts[0].Name != "report.md" || arts[0].Hash != "sha256:def" {
		t.Fatalf("unexpected artifacts: %+v", arts)
	}
}

func TestLaunchIntentResolve(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	p := newProject(t, s, "p")

	li := &LaunchIntent{ProjectID: p.ID, TaskID: "task-1", WorkflowRef: "wf.yaml", Inputs: "{}"}
	if err := s.CreateLaunchIntent(ctx, li); err != nil {
		t.Fatal(err)
	}

	runID := "run-1"
	if err := s.ResolveLaunchIntent(ctx, li.ID, LaunchAccepted, &runID); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetLaunchIntent(ctx, li.ID)
	if got.Status != LaunchAccepted || got.RunID == nil || *got.RunID != "run-1" || got.ResolvedAt == nil {
		t.Fatalf("unexpected resolved intent: %+v", got)
	}
	if got.TaskID != "task-1" {
		t.Fatalf("task_id not round-tripped: %+v", got)
	}

	if err := s.ResolveLaunchIntent(ctx, li.ID, LaunchRejected, nil); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("expected ErrInvalidTransition, got %v", err)
	}
}

func TestHumanInputAnswer(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	p := newProject(t, s, "p")
	r := newRun(t, s, p.ID, "task-1")
	sa, _ := s.StartStepAttempt(ctx, r.ID, "human-step", "{}")

	h := &HumanInput{RunID: r.ID, StepAttemptID: sa.ID, Prompt: "approve?"}
	if err := s.CreateHumanInput(ctx, h); err != nil {
		t.Fatal(err)
	}

	if err := s.AnswerHumanInput(ctx, h.ID, "yes"); err != nil {
		t.Fatal(err)
	}
	if err := s.AnswerHumanInput(ctx, h.ID, "no"); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("expected ErrInvalidTransition, got %v", err)
	}
}

func strPtrTo(s string) *string { return &s }
