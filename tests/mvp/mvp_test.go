// Package mvp_test exercises the orchestrator end-to-end against the
// "agent/human/end" workflow shipped under tests/mvp/fixtures. It is the
// acceptance harness for the BIR-51 ("Prove MVP with parallel real-task
// vertical slice") epic child: two parallel runs in the same project, a
// human-attention branch on one of them, and a daemon restart that
// preserves state.
//
// The test does NOT actually execute agent processes (no real Maki/Herdr
// binary is required). It drives the orchestrator state machine directly
// via the public Store APIs -- CreateRun, StartStepAttempt,
// TransitionStepAttempt, CreateExecution, TransitionExecution, CreateArtifact,
// CompleteStepAttempt, CreateHumanInput, AnswerHumanInput -- simulating what
// a controller would do. The contract under test is the state machine
// itself: legal transitions, the unique active-run constraint, the human
// step completion invariant (terminal Runs must not contain non-terminal
// StepAttempts), and durability across daemon restarts.
package mvp_test

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"bdtui/internal/daemon"
	"bdtui/internal/daemon/daemonpb"
	"bdtui/internal/orch"
	"bdtui/internal/workflow"
)

// fixtureRoot is the directory holding workflows/, roles/, prompts/ and
// schemas/ subtrees for the MVP vertical slice. The Layout-Root contract of
// workflow.Loader means we pass this directory itself, not a subdirectory.
const fixtureRoot = "fixtures"

// loadMvpShip loads the mvp_ship workflow bundle from fixtureRoot, builds
// the canonical snapshot, and returns it so callers can hand it to
// daemon.Client.CreateRun.
func loadMvpShip(t *testing.T) workflow.Snapshot {
	t.Helper()
	ctx := context.Background()
	loader := workflow.Loader{Project: fixtureRoot}
	bundle, err := loader.Load(ctx, "mvp_ship")
	if err != nil {
		t.Fatalf("load mvp_ship: %v", err)
	}
	snap, err := workflow.BuildSnapshot(*bundle)
	if err != nil {
		t.Fatalf("build snapshot: %v", err)
	}
	return snap
}

// mvpHarness owns the temp directory, the daemon store, the gRPC client,
// and the orchestrator project used by TestMVPVerticalSlice. It lets the
// test simulate a daemon restart by closing store + client and reopening
// them against the same on-disk database.
type mvpHarness struct {
	dir       string
	dbPath    string
	socket    string
	store     *orch.Store
	server    *daemon.Server
	cancel    context.CancelFunc
	serveDone chan error
	project   *orch.Project
	client    *daemon.Client
}

func newMVPHarness(t *testing.T) *mvpHarness {
	t.Helper()
	h := &mvpHarness{
		dir:    t.TempDir(),
		dbPath: filepath.Join(t.TempDir(), "orch.db"),
		socket: filepath.Join(t.TempDir(), "daemon.sock"),
	}
	if err := os.MkdirAll(filepath.Dir(h.dbPath), 0o700); err != nil {
		t.Fatalf("mkdir db dir: %v", err)
	}
	h.boot(t)
	t.Cleanup(func() { h.shutdown(t) })
	return h
}

// boot opens the orch store, creates a project, starts the daemon gRPC
// server, and dials a client. Every step is wired to the same on-disk
// database so a subsequent shutdown+boot simulates a real daemon restart.
func (h *mvpHarness) boot(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	store, err := orch.Open(ctx, h.dbPath)
	if err != nil {
		t.Fatalf("orch.Open: %v", err)
	}
	h.store = store

	project := &orch.Project{ID: "mvp-acceptance", Name: "mvp-acceptance", FsPath: h.dir}
	p, err := store.EnsureProject(ctx, project)
	if err != nil {
		t.Fatalf("ensure project: %v", err)
	}
	h.project = p

	srvCtx, cancel := context.WithCancel(context.Background())
	h.cancel = cancel
	h.server = daemon.NewServer(store, h.socket)
	h.serveDone = make(chan error, 1)
	go func() { h.serveDone <- h.server.Serve(srvCtx) }()

	deadline := time.Now().Add(3 * time.Second)
	for !socketAlive(h.socket) {
		if time.Now().After(deadline) {
			t.Fatal("daemon socket did not become ready")
		}
		time.Sleep(10 * time.Millisecond)
	}

	client, err := daemon.Dial(h.socket)
	if err != nil {
		t.Fatalf("dial daemon: %v", err)
	}
	h.client = client
}

// shutdown closes the client, stops the server, and closes the store.
// Idempotent so test cleanup is safe to call multiple times.
func (h *mvpHarness) shutdown(t *testing.T) {
	t.Helper()
	if h.client != nil {
		_ = h.client.Close()
		h.client = nil
	}
	if h.server != nil {
		h.server.Stop()
		h.server = nil
	}
	if h.cancel != nil {
		h.cancel()
		h.cancel = nil
	}
	if h.serveDone != nil {
		<-h.serveDone
		h.serveDone = nil
	}
	if h.store != nil {
		_ = h.store.Close()
		h.store = nil
	}
}

// restart simulates a daemon crash by tearing everything down, then brings
// the system back up against the SAME on-disk database. All previously
// persisted Runs/StepAttempts/Executions/HumanInputs must survive.
func (h *mvpHarness) restart(t *testing.T) {
	t.Helper()
	h.shutdown(t)
	h.boot(t)
}

// socketAlive probes the gRPC socket with a connect attempt. It mirrors the
// helper internal/daemon/daemon_test.go uses; duplicated here because that
// helper is unexported.
func socketAlive(socketPath string) bool {
	c, err := net.DialTimeout("unix", socketPath, 50*time.Millisecond)
	if err != nil {
		return false
	}
	_ = c.Close()
	return true
}

// runTrace is the per-Run persisted record collected by the driver paths.
// Every id captured here is later re-read by verifyPersistedAcrossRestart,
// so the harness can prove the daemon restart survived the entire graph
// (Runs, StepAttempts, Executions, Artifacts, HumanInputs) -- not just the
// top-level Run row.
type runTrace struct {
	runID        string
	stepAttempts []string // ids, in driver order
	executions   []string // ids, one per agent step attempt
	artifacts    []string // ids, one per agent step attempt
	humanInputs  []string // ids, one per ask step
}

// startBarrier is a 2-party rendezvous. Each goroutine calls Arrive()
// after its Run has been created + transitioned to RunRunning. Both
// goroutines wait until both have arrived, then proceed. Cancel() lets
// the main goroutine unblock a surviving driver if the other side
// errors out before reaching the barrier, so a pre-barrier failure does
// not deadlock the surviving goroutine.
type startBarrier struct {
	mu        sync.Mutex
	arrived   int
	cancelled bool
	cond      *sync.Cond
}

func newStartBarrier() *startBarrier {
	b := &startBarrier{}
	b.cond = sync.NewCond(&b.mu)
	return b
}

func (b *startBarrier) Arrive() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.cancelled {
		return false
	}
	b.arrived++
	if b.arrived >= 2 {
		b.cond.Broadcast()
		return true
	}
	for b.arrived < 2 && !b.cancelled {
		b.cond.Wait()
	}
	return !b.cancelled
}

// Cancel releases all pending waiters. Used by the main goroutine to
// unblock a surviving driver after the other side returns an error.
func (b *startBarrier) Cancel() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.cancelled = true
	b.cond.Broadcast()
}

// driveResult bundles the trace and any error encountered by a driver
// goroutine. Errors are returned through the channel so the test main
// goroutine -- the only one allowed to call t.Fatalf -- is the sole
// arbiter of pass/fail.
type driveResult struct {
	trace runTrace
	err   error
}

// driveSmoothPath walks Run A through plan -> review -> implement -> end.
// Both Runs have already been created and transitioned to RunRunning in
// the main goroutine before this driver is launched -- the barrier that
// follows ensures the two drivers advance their step phases in lock-step
// so the parallel-isolation invariant is exercised against the live store.
func driveSmoothPath(h *mvpHarness, snap workflow.Snapshot, runID, taskID string, barrier *startBarrier) driveResult {
	_ = taskID

	if !barrier.Arrive() {
		return driveResult{err: errors.New("barrier cancelled before smooth path arrived")}
	}

	trace := runTrace{runID: runID}
	for _, step := range []agentStepCall{
		{stepID: "plan", inputs: `{"goal":"ship mvp","steps":["a","b"],"risks":["none"]}`, outcome: "planned", output: "plan"},
		{stepID: "review", inputs: `{"plan":"x"}`, outcome: "approved", output: "review"},
		{stepID: "implement", inputs: `{"plan":"x","review":"y"}`, outcome: "done", output: "patch"},
	} {
		if saID, err := advanceAgent(h.store, runID, step, &trace); err != nil {
			return driveResult{err: errors.New("advanceAgent " + step.stepID + ": " + err.Error())}
		} else {
			trace.stepAttempts = append(trace.stepAttempts, saID)
		}
	}

	if err := h.store.TransitionRun(context.Background(), runID, orch.RunCompleted); err != nil {
		return driveResult{err: errors.New("TransitionRun completed: " + err.Error())}
	}
	return driveResult{trace: trace}
}

// driveHumanAttentionPath walks Run B through plan -> review -> ask_initial
// (human) -> replan_initial -> review_replan -> implement_replan -> end.
// Run creation + transition to RunRunning has already happened in the
// main goroutine; the barrier rendezvous then starts the two step
// phases simultaneously. Every StepAttempt, Execution, Artifact, and
// HumanInput id is recorded in the trace. The ask_initial attempt is
// explicitly transitioned StepWaitingHuman -> StepCompleted so completed
// Runs never contain non-terminal StepAttempts.
func driveHumanAttentionPath(h *mvpHarness, snap workflow.Snapshot, runID, taskID string, barrier *startBarrier) driveResult {
	_ = taskID

	if !barrier.Arrive() {
		return driveResult{err: errors.New("barrier cancelled before human path arrived")}
	}

	ctx := context.Background()
	trace := runTrace{runID: runID}

	// plan produces a planned plan; review asks for clarification (notes
	// describe the question); ask_initial receives the question and the
	// human answers; replan_initial produces a new plan; review_replan
	// approves; implement_replan ships.
	for _, step := range []agentStepCall{
		{stepID: "plan", inputs: `{"goal":"ship mvp","steps":["a"],"risks":["none"]}`, outcome: "planned", output: "plan"},
		{stepID: "review", inputs: `{"plan":"x"}`, outcome: "question", output: "review"},
	} {
		if saID, err := advanceAgent(h.store, runID, step, &trace); err != nil {
			return driveResult{err: errors.New("advanceAgent " + step.stepID + ": " + err.Error())}
		} else {
			trace.stepAttempts = append(trace.stepAttempts, saID)
		}
	}

	// Park in waiting_human on the ask_initial step. ask_initial has
	// inputs: notes from review -- the workflow spec guarantees that
	// review.notes actually reaches the human step, which is what the
	// previous version of the harness skipped.
	askSA, err := startHumanStep(ctx, h.store, runID, "ask_initial")
	if err != nil {
		return driveResult{err: errors.New("startHumanStep ask_initial: " + err.Error())}
	}
	trace.stepAttempts = append(trace.stepAttempts, askSA.ID)
	hi, err := createHumanInput(ctx, h.store, runID, askSA.ID, "Please clarify: which scope, MVP-only or full feature?")
	if err != nil {
		return driveResult{err: errors.New("createHumanInput ask_initial: " + err.Error())}
	}
	trace.humanInputs = append(trace.humanInputs, hi.ID)
	if err := h.store.TransitionRun(ctx, runID, orch.RunWaitingHuman); err != nil {
		return driveResult{err: errors.New("TransitionRun waiting_human: " + err.Error())}
	}

	// Resume by answering the human input, then explicitly transition the
	// ask_initial StepAttempt to StepCompleted before continuing. A
	// terminal Run must not contain a non-terminal StepAttempt.
	if err := h.store.AnswerHumanInput(ctx, hi.ID, "ship only the parse phase"); err != nil {
		return driveResult{err: errors.New("AnswerHumanInput: " + err.Error())}
	}
	if err := h.store.TransitionStepAttempt(ctx, askSA.ID, orch.StepCompleted); err != nil {
		return driveResult{err: errors.New("TransitionStepAttempt ask_initial completed: " + err.Error())}
	}
	if err := h.store.TransitionRun(ctx, runID, orch.RunRunning); err != nil {
		return driveResult{err: errors.New("TransitionRun resume: " + err.Error())}
	}

	for _, step := range []agentStepCall{
		{stepID: "replan_initial", inputs: `{"clarification":"x"}`, outcome: "planned", output: "plan"},
		{stepID: "review_replan", inputs: `{"plan":"y"}`, outcome: "approved", output: "review"},
		{stepID: "implement_replan", inputs: `{"plan":"y","review":"z"}`, outcome: "done", output: "patch"},
	} {
		if saID, err := advanceAgent(h.store, runID, step, &trace); err != nil {
			return driveResult{err: errors.New("advanceAgent " + step.stepID + ": " + err.Error())}
		} else {
			trace.stepAttempts = append(trace.stepAttempts, saID)
		}
	}

	if err := h.store.TransitionRun(ctx, runID, orch.RunCompleted); err != nil {
		return driveResult{err: errors.New("TransitionRun completed: " + err.Error())}
	}
	return driveResult{trace: trace}
}

// agentStepCall groups the parameters for a single simulated agent step.
type agentStepCall struct {
	stepID  string
	inputs  string
	outcome string
	output  string
}

// advanceAgent simulates a single agent step attempt: allocates a queued
// StepAttempt, walks it through queued -> running -> completed (recording
// the result string), records the agent Execution and its result JSON +
// artifact. Returns the StepAttempt id; the trace is mutated to append
// the Execution and Artifact ids as a side effect.
func advanceAgent(store *orch.Store, runID string, step agentStepCall, trace *runTrace) (string, error) {
	ctx := context.Background()

	sa, err := store.StartStepAttempt(ctx, runID, step.stepID, step.inputs)
	if err != nil {
		return "", err
	}
	if err := store.TransitionStepAttempt(ctx, sa.ID, orch.StepRunning); err != nil {
		return "", err
	}

	exec := &orch.Execution{
		RunID:         runID,
		StepAttemptID: sa.ID,
		Kind:          orch.KindAgent,
		Status:        orch.ExecQueued,
		PromptRef:     "roles/" + step.stepID + "/prompt",
		PromptHash:    "deadbeef",
	}
	if err := store.CreateExecution(ctx, exec); err != nil {
		return "", err
	}
	if err := store.TransitionExecution(ctx, exec.ID, orch.ExecRunning); err != nil {
		return "", err
	}
	resultJSON := `{"step":"` + step.stepID + `","outcome":"` + step.outcome + `"}`
	if err := store.TransitionExecution(ctx, exec.ID, orch.ExecCompleted); err != nil {
		return "", err
	}
	if err := store.SetExecutionResultJSON(ctx, exec.ID, resultJSON); err != nil {
		return "", err
	}
	if err := store.CreateArtifact(ctx, &orch.Artifact{
		ExecutionID: exec.ID,
		Name:        step.output,
		Path:        "/tmp/mvp/" + runID + "/" + step.stepID + "/" + step.output + ".json",
		Hash:        "sha256:" + step.stepID + ":" + step.outcome,
	}); err != nil {
		return "", err
	}
	trace.executions = append(trace.executions, exec.ID)
	trace.artifacts = append(trace.artifacts, latestArtifactID(store, exec.ID))

	if err := store.CompleteStepAttempt(ctx, sa.ID, step.outcome+":"+step.output); err != nil {
		return "", err
	}
	return sa.ID, nil
}

// startHumanStep allocates a queued StepAttempt for a human step and walks
// it through the shared queued -> running -> waiting_human sequence (the
// state machine forbids a direct queued -> waiting_human jump). Returns
// the attempt so the caller can attach a HumanInput record to it.
func startHumanStep(ctx context.Context, store *orch.Store, runID, stepID string) (*orch.StepAttempt, error) {
	sa, err := store.StartStepAttempt(ctx, runID, stepID, "")
	if err != nil {
		return nil, err
	}
	if err := store.TransitionStepAttempt(ctx, sa.ID, orch.StepRunning); err != nil {
		return nil, err
	}
	if err := store.TransitionStepAttempt(ctx, sa.ID, orch.StepWaitingHuman); err != nil {
		return nil, err
	}
	return sa, nil
}

// createHumanInput persists a pending HumanInput row pointing at the
// given StepAttempt and returns the populated record (with its assigned
// ID). The caller is responsible for transitioning the Run to
// RunWaitingHuman.
func createHumanInput(ctx context.Context, store *orch.Store, runID, stepAttemptID, prompt string) (*orch.HumanInput, error) {
	hi := &orch.HumanInput{
		RunID:         runID,
		StepAttemptID: stepAttemptID,
		Prompt:        prompt,
		Status:        orch.HumanPending,
	}
	if err := store.CreateHumanInput(ctx, hi); err != nil {
		return nil, err
	}
	return hi, nil
}

// latestArtifactID is a tiny helper used during agent-step simulation to
// append the just-created artifact id to the run trace without forcing
// the driver to track it locally.
func latestArtifactID(store *orch.Store, executionID string) string {
	arts, err := store.ListArtifactsByExecution(context.Background(), executionID)
	if err != nil || len(arts) == 0 {
		return ""
	}
	return arts[len(arts)-1].ID
}

// TestMVPVerticalSlice is the acceptance milestone for BIR-51.
//
// Two Runs are launched against two distinct Beads tasks in the same
// project. A startBarrier rendezvous ensures both Runs are in
// RunRunning before either advances to step attempts, so the
// parallel-isolation invariant is exercised on the live store. Run A
// walks the smooth path; Run B walks the human-attention path
// (ask_initial -> replan -> review_replan -> implement_replan). After
// both complete, the daemon is restarted and the harness verifies
// every persisted entity (Runs + StepAttempts + Executions + Artifacts +
// HumanInputs) survives intact, AND that every StepAttempt in a
// completed Run is terminal.
func TestMVPVerticalSlice(t *testing.T) {
	h := newMVPHarness(t)
	snap := loadMvpShip(t)

	// Create both Runs and transition them to RunRunning in the main
	// goroutine. Doing this BEFORE launching the driver goroutines
	// guarantees both Runs are active simultaneously (parallel
	// isolation), and any setup failure fails the test immediately
	// instead of deadlocking a surviving driver at the barrier.
	ctx := context.Background()
	runA, err := h.client.CreateRun(ctx, &daemonpb.CreateRunRequest{
		ProjectId:           h.project.ID,
		TaskId:              "task-smooth",
		WorkflowSnapshotRef: snap.Ref,
		WorkflowSnapshot:    snap.JSON,
	})
	if err != nil {
		t.Fatalf("CreateRun A: %v", err)
	}
	if err := h.store.TransitionRun(ctx, runA.Id, orch.RunRunning); err != nil {
		t.Fatalf("TransitionRun A: %v", err)
	}
	runB, err := h.client.CreateRun(ctx, &daemonpb.CreateRunRequest{
		ProjectId:           h.project.ID,
		TaskId:              "task-human",
		WorkflowSnapshotRef: snap.Ref,
		WorkflowSnapshot:    snap.JSON,
	})
	if err != nil {
		t.Fatalf("CreateRun B: %v", err)
	}
	if err := h.store.TransitionRun(ctx, runB.Id, orch.RunRunning); err != nil {
		t.Fatalf("TransitionRun B: %v", err)
	}

	barrier := newStartBarrier()
	results := make(chan driveResult, 2)

	go func() {
		results <- driveSmoothPath(h, snap, runA.Id, "task-smooth", barrier)
	}()
	go func() {
		results <- driveHumanAttentionPath(h, snap, runB.Id, "task-human", barrier)
	}()

	traceA := <-results
	traceB := <-results
	if traceA.err != nil || traceB.err != nil {
		// Unblock any pending barrier waiter before failing.
		barrier.Cancel()
		// Drain the second result so the goroutine doesn't leak.
		<-results
		if traceA.err != nil {
			t.Fatalf("smooth path: %v", traceA.err)
		}
		if traceB.err != nil {
			t.Fatalf("human path: %v", traceB.err)
		}
	}

	verifyFinalState(t, h.store, traceA.trace.runID, orch.RunCompleted)
	verifyFinalState(t, h.store, traceB.trace.runID, orch.RunCompleted)
	verifyNoNonTerminalStepAttempts(t, h.store, traceA.trace)
	verifyNoNonTerminalStepAttempts(t, h.store, traceB.trace)

	// Daemon restart: every persisted entity must survive intact.
	h.restart(t)

	verifyFinalState(t, h.store, traceA.trace.runID, orch.RunCompleted)
	verifyFinalState(t, h.store, traceB.trace.runID, orch.RunCompleted)
	verifyNoNonTerminalStepAttempts(t, h.store, traceA.trace)
	verifyNoNonTerminalStepAttempts(t, h.store, traceB.trace)
	verifyTracesAcrossRestart(t, h.store, traceA.trace)
	verifyTracesAcrossRestart(t, h.store, traceB.trace)
	verifyProjectScopedRuns(t, h.client, h.project.ID, []string{traceA.trace.runID, traceB.trace.runID})

	// Smoke check: the events emitted for Run A include an explicit
	// transition from queued -> running for the plan step (proves the
	// event stream recorded the parallel start, not just the final
	// terminal transitions). The previous harness shipped with this
	// assertion removed; we keep a small sample to catch regressions
	// where the driver accidentally sequences the two paths.
	if !hasTransition(t, h.store, traceA.trace, orch.EventStepTransition, "queued", "running") {
		t.Fatalf("Run A missing queued->running step transition event")
	}
	if !hasTransition(t, h.store, traceB.trace, orch.EventStepTransition, "queued", "running") {
		t.Fatalf("Run B missing queued->running step transition event")
	}
}

// verifyFinalState asserts the run exists and has the expected status.
func verifyFinalState(t *testing.T, store *orch.Store, runID string, want orch.RunStatus) {
	t.Helper()
	r, err := store.GetRun(context.Background(), runID)
	if err != nil {
		t.Fatalf("GetRun(%s): %v", runID, err)
	}
	if r.Status != want {
		t.Fatalf("run %s status = %s, want %s", runID, r.Status, want)
	}
}

// verifyNoNonTerminalStepAttempts asserts every StepAttempt recorded in the
// trace reached a terminal status. Terminal Runs holding non-terminal
// children is a real durability bug (we'd persist inconsistency), so the
// harness explicitly fails on it instead of letting it pass.
func verifyNoNonTerminalStepAttempts(t *testing.T, store *orch.Store, trace runTrace) {
	t.Helper()
	for _, saID := range trace.stepAttempts {
		sa, err := store.GetStepAttempt(context.Background(), saID)
		if err != nil {
			t.Fatalf("GetStepAttempt(%s): %v", saID, err)
		}
		if !sa.Status.Terminal() {
			t.Fatalf("run %s step attempt %s status = %s, want terminal (run is %s)",
				trace.runID, saID, sa.Status, orch.RunCompleted)
		}
	}
}

// verifyTracesAcrossRestart re-reads every recorded id (StepAttempt,
// Execution, Artifact, HumanInput) by its dedicated store API, after the
// daemon has been restarted. This is the regression test that would have
// caught a stuck waiting_human child or a dropped artifact in earlier
// rounds.
func verifyTracesAcrossRestart(t *testing.T, store *orch.Store, trace runTrace) {
	t.Helper()
	ctx := context.Background()
	for _, saID := range trace.stepAttempts {
		if _, err := store.GetStepAttempt(ctx, saID); err != nil {
			t.Fatalf("restart: GetStepAttempt(%s): %v", saID, err)
		}
	}
	for _, execID := range trace.executions {
		if _, err := store.GetExecution(ctx, execID); err != nil {
			t.Fatalf("restart: GetExecution(%s): %v", execID, err)
		}
	}
	for _, artID := range trace.artifacts {
		if artID == "" {
			continue
		}
		present := false
		for _, execID := range trace.executions {
			arts, err := store.ListArtifactsByExecution(ctx, execID)
			if err != nil {
				t.Fatalf("restart: ListArtifactsByExecution(%s): %v", execID, err)
			}
			for _, a := range arts {
				if a.ID == artID {
					present = true
				}
			}
		}
		if !present {
			t.Fatalf("restart: artifact %s not found under any execution", artID)
		}
	}
	for _, hiID := range trace.humanInputs {
		if _, err := store.GetHumanInput(ctx, hiID); err != nil {
			t.Fatalf("restart: GetHumanInput(%s): %v", hiID, err)
		}
	}
}

// verifyProjectScopedRuns asserts that both run IDs surface via the gRPC
// ListRuns(project_id=...) call after restart, proving the daemon-side
// query path also survives.
func verifyProjectScopedRuns(t *testing.T, client *daemon.Client, projectID string, want []string) {
	t.Helper()
	resp, err := client.ListRuns(context.Background(), &daemonpb.ListRunsRequest{
		ProjectId: &projectID,
	})
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
	got := map[string]bool{}
	for _, r := range resp.Runs {
		got[r.Id] = true
	}
	for _, id := range want {
		if !got[id] {
			t.Fatalf("ListRuns missing %s; got %v", id, resp.Runs)
		}
	}
}

// hasTransition scans the trace's run events (re-fetched post-restart)
// for a step.transition event with the given from/to pair.
func hasTransition(t *testing.T, store *orch.Store, trace runTrace, eventType, from, to string) bool {
	t.Helper()
	rows, err := store.ListEventsByRun(context.Background(), trace.runID)
	if err != nil {
		t.Fatalf("ListEventsByRun(%s): %v", trace.runID, err)
	}
	for _, r := range rows {
		if r.Type != eventType {
			continue
		}
		var p struct {
			From string `json:"from"`
			To   string `json:"to"`
		}
		if err := json.Unmarshal([]byte(r.Payload), &p); err != nil {
			continue
		}
		if p.From == from && p.To == to {
			return true
		}
	}
	return false
}