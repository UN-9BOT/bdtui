package orch

import (
	"context"
	"errors"
	"testing"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestMigrateIdempotent(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	if err := s.Migrate(ctx); err != nil {
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
}

func TestRunLifecycleAtomicTransitions(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	p := &Project{Name: "p", FsPath: "/p"}
	if err := s.CreateProject(ctx, p); err != nil {
		t.Fatal(err)
	}

	r := &Run{ProjectID: p.ID, Status: RunQueued, WorkflowSnapshotRef: "abc"}
	if err := s.CreateRun(ctx, r); err != nil {
		t.Fatal(err)
	}
	if r.ID == "" {
		t.Fatal("expected run ID")
	}

	// Invalid transition (not in allowed set) must fail atomically.
	err := s.TransitionRun(ctx, r.ID, []RunStatus{RunCompleted}, RunRunning, RunTransitionOpts{})
	if !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("expected ErrInvalidTransition, got %v", err)
	}

	// queued -> running sets started_at.
	if err := s.TransitionRun(ctx, r.ID, []RunStatus{RunQueued}, RunRunning, RunTransitionOpts{}); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetRun(ctx, r.ID)
	if got.Status != RunRunning || got.StartedAt == nil {
		t.Fatalf("expected running with started_at, got %+v", got)
	}

	// running -> waiting_human (needs_attention_reason carried on later transition).
	if err := s.TransitionRun(ctx, r.ID, []RunStatus{RunRunning}, RunWaitingHuman, RunTransitionOpts{}); err != nil {
		t.Fatal(err)
	}
	if err := s.TransitionRun(ctx, r.ID, []RunStatus{RunWaitingHuman}, RunNeedsAttention, RunTransitionOpts{
		NeedsAttentionReason: strPtrTo("writer ambiguous"),
	}); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GetRun(ctx, r.ID)
	if got.Status != RunNeedsAttention || got.NeedsAttentionReason == nil || *got.NeedsAttentionReason != "writer ambiguous" {
		t.Fatalf("unexpected needs_attention state: %+v", got)
	}

	// needs_attention -> completed sets completed_at.
	if err := s.TransitionRun(ctx, r.ID, []RunStatus{RunNeedsAttention}, RunCompleted, RunTransitionOpts{}); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GetRun(ctx, r.ID)
	if got.Status != RunCompleted || got.CompletedAt == nil {
		t.Fatalf("expected completed with completed_at, got %+v", got)
	}

	// A transition whose source set does not include the current status is
	// rejected even when the run is terminal.
	if err := s.TransitionRun(ctx, r.ID, []RunStatus{RunQueued}, RunRunning, RunTransitionOpts{}); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("expected ErrInvalidTransition for disallowed source, got %v", err)
	}
}

func TestEventsAppendOnlyPerRunSeq(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	p := &Project{Name: "p", FsPath: "/p"}
	_ = s.CreateProject(ctx, p)
	r := &Run{ProjectID: p.ID, Status: RunQueued}
	_ = s.CreateRun(ctx, r)

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

func TestStartStepAttemptAtomicNumbering(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	p := &Project{Name: "p", FsPath: "/p"}
	_ = s.CreateProject(ctx, p)
	r := &Run{ProjectID: p.ID, Status: RunRunning}
	_ = s.CreateRun(ctx, r)

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

func TestExecutionLifecycle(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	p := &Project{Name: "p", FsPath: "/p"}
	_ = s.CreateProject(ctx, p)
	r := &Run{ProjectID: p.ID, Status: RunRunning}
	_ = s.CreateRun(ctx, r)
	sa, _ := s.StartStepAttempt(ctx, r.ID, "step-x", "{}")

	e := &Execution{
		RunID:         r.ID,
		StepAttemptID: sa.ID,
		Kind:          KindAgent,
		Status:        ExecQueued,
		Prompt:        "do the thing",
		Artifacts:     "[]",
	}
	if err := s.CreateExecution(ctx, e); err != nil {
		t.Fatal(err)
	}
	if e.ID == "" {
		t.Fatal("expected execution ID")
	}

	if err := s.TransitionExecution(ctx, e.ID, []ExecutionStatus{ExecQueued}, ExecRunning); err != nil {
		t.Fatal(err)
	}
	if err := s.TransitionExecution(ctx, e.ID, []ExecutionStatus{ExecRunning}, ExecCompleted); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetExecution(ctx, e.ID)
	if got.Status != ExecCompleted || got.CompletedAt == nil {
		t.Fatalf("unexpected execution: %+v", got)
	}

	if err := s.TransitionExecution(ctx, e.ID, []ExecutionStatus{ExecQueued}, ExecRunning); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("expected ErrInvalidTransition, got %v", err)
	}
}

func TestLaunchIntentResolve(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	p := &Project{Name: "p", FsPath: "/p"}
	_ = s.CreateProject(ctx, p)

	li := &LaunchIntent{ProjectID: p.ID, WorkflowRef: "wf.yaml", Inputs: "{}"}
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

	// Already resolved: cannot resolve again.
	if err := s.ResolveLaunchIntent(ctx, li.ID, LaunchRejected, nil); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("expected ErrInvalidTransition, got %v", err)
	}
}

func TestHumanInputAnswer(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	p := &Project{Name: "p", FsPath: "/p"}
	_ = s.CreateProject(ctx, p)
	r := &Run{ProjectID: p.ID, Status: RunWaitingHuman}
	_ = s.CreateRun(ctx, r)
	sa, _ := s.StartStepAttempt(ctx, r.ID, "human-step", "{}")

	h := &HumanInput{RunID: r.ID, StepAttemptID: sa.ID, Prompt: "approve?"}
	if err := s.CreateHumanInput(ctx, h); err != nil {
		t.Fatal(err)
	}

	if err := s.AnswerHumanInput(ctx, h.ID, "yes"); err != nil {
		t.Fatal(err)
	}

	// Answering twice must be rejected.
	if err := s.AnswerHumanInput(ctx, h.ID, "no"); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("expected ErrInvalidTransition, got %v", err)
	}
}

func strPtrTo(s string) *string { return &s }
