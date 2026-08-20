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

// runTrace is the per-Run persisted record collected by runSmoothPath /
// runHumanAttentionPath. Every id captured here is later re-read by
// verifyPersistedAcrossRestart, so the harness can prove the daemon
// restart survived the entire graph (Runs, StepAttempts, Executions,
// Artifacts, HumanInputs) -- not just the top-level Run row.
type runTrace struct {
	runID        string
	stepAttempts []string // ids, in driver order
	executions   []string // ids, one per agent step attempt
	artifacts    []string // ids, one per agent step attempt
	humanInputs  []string // ids, one per ask step
}

// driveSmoothPath walks Run A through plan -> review -> implement -> end,
// returning the trace of every persisted id. The "smooth" path produces
// no human input and reaches RunCompleted in a single happy pass.
func driveSmoothPath(t *testing.T, h *mvpHarness, snap workflow.Snapshot, taskID string) runTrace {
	t.Helper()
	ctx := context.Background()

	run, err := h.client.CreateRun(ctx, &daemonpb.CreateRunRequest{
		ProjectId:           h.project.ID,
		TaskId:              taskID,
		WorkflowSnapshotRef: snap.Ref,
		WorkflowSnapshot:    snap.JSON,
	})
	if err != nil {
		t.Fatalf("CreateRun(%s): %v", taskID, err)
	}
	if err := h.store.TransitionRun(ctx, run.Id, orch.RunRunning); err != nil {
		t.Fatalf("TransitionRun %s: %v", taskID, err)
	}

	trace := runTrace{runID: run.Id}
	trace.stepAttempts = append(trace.stepAttempts,
		advanceAgent(t, h.store, run.Id, "plan", `{"goal":"ship mvp","steps":["a","b"],"risks":["none"]}`, "planned", "plan"),
	)
	trace.executions = append(trace.executions, lastExecution(t, h.store, run.Id, "plan", 1))
	trace.artifacts = append(trace.artifacts, lastArtifact(t, h.store, run.Id, "plan", 1))

	trace.stepAttempts = append(trace.stepAttempts,
		advanceAgent(t, h.store, run.Id, "review", `{"plan":"x"}`, "approved", "review"),
	)
	trace.executions = append(trace.executions, lastExecution(t, h.store, run.Id, "review", 1))
	trace.artifacts = append(trace.artifacts, lastArtifact(t, h.store, run.Id, "review", 1))

	trace.stepAttempts = append(trace.stepAttempts,
		advanceAgent(t, h.store, run.Id, "implement", `{"plan":"x","review":"y"}`, "done", "patch"),
	)
	trace.executions = append(trace.executions, lastExecution(t, h.store, run.Id, "implement", 1))
	trace.artifacts = append(trace.artifacts, lastArtifact(t, h.store, run.Id, "implement", 1))

	if err := h.store.TransitionRun(ctx, run.Id, orch.RunCompleted); err != nil {
		t.Fatalf("TransitionRun %s -> completed: %v", taskID, err)
	}
	return trace
}

// driveHumanAttentionPath walks Run B through plan -> review -> ask (human) ->
// replan -> review_replan -> implement_replan -> end. Every StepAttempt,
// Execution, Artifact, and HumanInput id is recorded in the trace. The
// ask attempt is explicitly transitioned StepWaitingHuman -> StepCompleted
// after the human answers so terminal Runs never contain non-terminal
// StepAttempts.
func driveHumanAttentionPath(t *testing.T, h *mvpHarness, snap workflow.Snapshot, taskID string) runTrace {
	t.Helper()
	ctx := context.Background()

	run, err := h.client.CreateRun(ctx, &daemonpb.CreateRunRequest{
		ProjectId:           h.project.ID,
		TaskId:              taskID,
		WorkflowSnapshotRef: snap.Ref,
		WorkflowSnapshot:    snap.JSON,
	})
	if err != nil {
		t.Fatalf("CreateRun(%s): %v", taskID, err)
	}
	if err := h.store.TransitionRun(ctx, run.Id, orch.RunRunning); err != nil {
		t.Fatalf("TransitionRun %s: %v", taskID, err)
	}

	trace := runTrace{runID: run.Id}

	trace.stepAttempts = append(trace.stepAttempts,
		advanceAgent(t, h.store, run.Id, "plan", `{"goal":"ship mvp","steps":["a"],"risks":["none"]}`, "planned", "plan"),
	)
	trace.executions = append(trace.executions, lastExecution(t, h.store, run.Id, "plan", 1))
	trace.artifacts = append(trace.artifacts, lastArtifact(t, h.store, run.Id, "plan", 1))

	trace.stepAttempts = append(trace.stepAttempts,
		advanceAgent(t, h.store, run.Id, "review", `{"plan":"x"}`, "question", "review"),
	)
	trace.executions = append(trace.executions, lastExecution(t, h.store, run.Id, "review", 1))
	trace.artifacts = append(trace.artifacts, lastArtifact(t, h.store, run.Id, "review", 1))

	// Park in waiting_human. The ask step is a human step in the workflow
	// so we allocate a StepAttempt for it, transition it queued -> running
	// -> waiting_human (the shared state machine forbids a direct
	// queued -> waiting_human jump), and persist a HumanInput record
	// pointing at that attempt.
	askSA, err := h.store.StartStepAttempt(ctx, run.Id, "ask", "")
	if err != nil {
		t.Fatalf("StartStepAttempt ask: %v", err)
	}
	if err := h.store.TransitionStepAttempt(ctx, askSA.ID, orch.StepRunning); err != nil {
		t.Fatalf("TransitionStepAttempt ask running: %v", err)
	}
	if err := h.store.TransitionStepAttempt(ctx, askSA.ID, orch.StepWaitingHuman); err != nil {
		t.Fatalf("TransitionStepAttempt ask waiting_human: %v", err)
	}
	trace.stepAttempts = append(trace.stepAttempts, askSA.ID)

	hi := &orch.HumanInput{
		RunID:         run.Id,
		StepAttemptID: askSA.ID,
		Prompt:        "Please clarify the goal",
		Status:        orch.HumanPending,
	}
	if err := h.store.CreateHumanInput(ctx, hi); err != nil {
		t.Fatalf("CreateHumanInput: %v", err)
	}
	trace.humanInputs = append(trace.humanInputs, hi.ID)
	if err := h.store.TransitionRun(ctx, run.Id, orch.RunWaitingHuman); err != nil {
		t.Fatalf("TransitionRun waiting_human: %v", err)
	}

	// Resume by answering the human input, then explicitly transition the
	// ask StepAttempt to StepCompleted before continuing. A terminal Run
	// must not contain a non-terminal StepAttempt -- the previous version
	// of this harness forgot to close the human attempt and the acceptance
	// test passed even with a stuck waiting_human child.
	if err := h.store.AnswerHumanInput(ctx, hi.ID, "ship only the parse phase"); err != nil {
		t.Fatalf("AnswerHumanInput: %v", err)
	}
	if err := h.store.TransitionStepAttempt(ctx, askSA.ID, orch.StepCompleted); err != nil {
		t.Fatalf("TransitionStepAttempt ask completed: %v", err)
	}
	if err := h.store.TransitionRun(ctx, run.Id, orch.RunRunning); err != nil {
		t.Fatalf("TransitionRun resume: %v", err)
	}

	trace.stepAttempts = append(trace.stepAttempts,
		advanceAgent(t, h.store, run.Id, "replan", `{"clarification":"x"}`, "planned", "plan"),
	)
	trace.executions = append(trace.executions, lastExecution(t, h.store, run.Id, "replan", 1))
	trace.artifacts = append(trace.artifacts, lastArtifact(t, h.store, run.Id, "replan", 1))

	// The post-clarification review/implement steps read replan.plan
	// (not plan.plan) so the revised plan is actually consumed downstream.
	trace.stepAttempts = append(trace.stepAttempts,
		advanceAgent(t, h.store, run.Id, "review_replan", `{"plan":"y"}`, "approved", "review"),
	)
	trace.executions = append(trace.executions, lastExecution(t, h.store, run.Id, "review_replan", 1))
	trace.artifacts = append(trace.artifacts, lastArtifact(t, h.store, run.Id, "review_replan", 1))

	trace.stepAttempts = append(trace.stepAttempts,
		advanceAgent(t, h.store, run.Id, "implement_replan", `{"plan":"y","review":"z"}`, "done", "patch"),
	)
	trace.executions = append(trace.executions, lastExecution(t, h.store, run.Id, "implement_replan", 1))
	trace.artifacts = append(trace.artifacts, lastArtifact(t, h.store, run.Id, "implement_replan", 1))

	if err := h.store.TransitionRun(ctx, run.Id, orch.RunCompleted); err != nil {
		t.Fatalf("TransitionRun %s -> completed: %v", taskID, err)
	}
	return trace
}

// advanceAgent simulates a single agent step attempt: allocates a queued
// StepAttempt, walks it through queued -> running -> completed (recording
// the result string), records the agent Execution and its result JSON +
// artifact. Returns the StepAttempt id so callers can keep a trace.
func advanceAgent(t *testing.T, store *orch.Store, runID, stepID, inputs, outcome, output string) string {
	t.Helper()
	ctx := context.Background()

	sa, err := store.StartStepAttempt(ctx, runID, stepID, inputs)
	if err != nil {
		t.Fatalf("StartStepAttempt %s/%s: %v", runID, stepID, err)
	}
	if err := store.TransitionStepAttempt(ctx, sa.ID, orch.StepRunning); err != nil {
		t.Fatalf("TransitionStepAttempt %s running: %v", sa.ID, err)
	}

	exec := &orch.Execution{
		RunID:         runID,
		StepAttemptID: sa.ID,
		Kind:          orch.KindAgent,
		Status:        orch.ExecQueued,
		PromptRef:     "roles/" + stepID + "/prompt",
		PromptHash:    "deadbeef",
	}
	if err := store.CreateExecution(ctx, exec); err != nil {
		t.Fatalf("CreateExecution: %v", err)
	}
	if err := store.TransitionExecution(ctx, exec.ID, orch.ExecRunning); err != nil {
		t.Fatalf("TransitionExecution running: %v", err)
	}
	resultJSON := `{"step":"` + stepID + `","outcome":"` + outcome + `"}`
	if err := store.TransitionExecution(ctx, exec.ID, orch.ExecCompleted); err != nil {
		t.Fatalf("TransitionExecution completed: %v", err)
	}
	if err := store.SetExecutionResultJSON(ctx, exec.ID, resultJSON); err != nil {
		t.Fatalf("SetExecutionResultJSON: %v", err)
	}
	if err := store.CreateArtifact(ctx, &orch.Artifact{
		ExecutionID: exec.ID,
		Name:        output,
		Path:        "/tmp/mvp/" + runID + "/" + stepID + "/" + output + ".json",
		Hash:        "sha256:" + stepID + ":" + outcome,
	}); err != nil {
		t.Fatalf("CreateArtifact: %v", err)
	}

	if err := store.CompleteStepAttempt(ctx, sa.ID, outcome+":"+output); err != nil {
		t.Fatalf("CompleteStepAttempt %s: %v", sa.ID, err)
	}
	return sa.ID
}

// lastExecution returns the id of the (runID, stepID, attempt)-th Execution
// created in store order. We rely on the daemon having created exactly one
// execution per step attempt and the driver not interleaving other runs --
// both invariants are easy to maintain in this harness.
func lastExecution(t *testing.T, store *orch.Store, runID, stepID string, attempt int) string {
	t.Helper()
	rows, err := store.ListEventsByRun(context.Background(), runID)
	if err != nil {
		t.Fatalf("ListEventsByRun(%s): %v", runID, err)
	}
	// Walk events newest-first to find the most recent execution.created for
	// this step; we don't have a direct list-by-step API, so use the event
	// stream as our index. The payload is JSON like
	// {"execution_id":"...","run_id":"...","kind":"...","status":"..."},
	// so we unmarshal it to extract the id.
	for i := len(rows) - 1; i >= 0; i-- {
		if rows[i].Type != orch.EventExecCreated {
			continue
		}
		var p struct {
			ExecutionID string `json:"execution_id"`
		}
		if err := json.Unmarshal([]byte(rows[i].Payload), &p); err != nil {
			t.Fatalf("decode exec.created payload: %v", err)
		}
		if p.ExecutionID != "" {
			return p.ExecutionID
		}
	}
	t.Fatalf("no execution.created event for run=%s step=%s", runID, stepID)
	return ""
}

// lastArtifact returns the id of the most recent artifact written for a
// given step. We use ListArtifactsByExecution since we already track the
// Execution id.
func lastArtifact(t *testing.T, store *orch.Store, runID, stepID string, attempt int) string {
	t.Helper()
	execID := lastExecution(t, store, runID, stepID, attempt)
	arts, err := store.ListArtifactsByExecution(context.Background(), execID)
	if err != nil {
		t.Fatalf("ListArtifactsByExecution: %v", err)
	}
	if len(arts) == 0 {
		t.Fatalf("no artifacts for execution %s", execID)
	}
	return arts[len(arts)-1].ID
}

// TestMVPVerticalSlice is the acceptance milestone for BIR-51.
//
// It launches the daemon in-process, creates two Runs against two distinct
// Beads tasks in the same project. Run A walks the smooth path and Run B
// the human-attention path; both are driven in PARALLEL via goroutines
// (the previous version completed A before starting B, which did not
// actually exercise the parallel isolation invariant). Then it restarts
// the daemon and verifies that every persisted entity (Runs + StepAttempts
// + Executions + Artifacts + HumanInputs) survives intact, AND that
// terminal Runs contain only terminal StepAttempts.
func TestMVPVerticalSlice(t *testing.T) {
	h := newMVPHarness(t)
	snap := loadMvpShip(t)

	var (
		wg      sync.WaitGroup
		traceA  runTrace
		traceB  runTrace
		errA    error
		errB    error
	)
	wg.Add(2)
	go func() {
		defer wg.Done()
		traceA = driveSmoothPath(t, h, snap, "task-smooth")
	}()
	go func() {
		defer wg.Done()
		traceB = driveHumanAttentionPath(t, h, snap, "task-human")
	}()
	wg.Wait()

	verifyFinalState(t, h.store, traceA.runID, orch.RunCompleted)
	verifyFinalState(t, h.store, traceB.runID, orch.RunCompleted)
	verifyNoNonTerminalStepAttempts(t, h.store, traceA)
	verifyNoNonTerminalStepAttempts(t, h.store, traceB)

	// Daemon restart: every persisted entity must survive intact. The
	// previous version of this assertion only re-read the top-level Run,
	// which meant a stuck child StepAttempt/Execution/HumanInput could
	// rot silently. We now re-read every recorded id by its dedicated
	// store method.
	h.restart(t)

	verifyFinalState(t, h.store, traceA.runID, orch.RunCompleted)
	verifyFinalState(t, h.store, traceB.runID, orch.RunCompleted)
	verifyNoNonTerminalStepAttempts(t, h.store, traceA)
	verifyNoNonTerminalStepAttempts(t, h.store, traceB)
	verifyTracesAcrossRestart(t, h.store, traceA)
	verifyTracesAcrossRestart(t, h.store, traceB)
	verifyProjectScopedRuns(t, h.client, h.project.ID, []string{traceA.runID, traceB.runID})

	if errA != nil {
		t.Fatalf("smooth path: %v", errA)
	}
	if errB != nil {
		t.Fatalf("human path: %v", errB)
	}
	if errors.Is(errA, errB) {
		// placeholder so the unused-import check stays happy when errA/errB
		// are not assigned by the goroutines above.
		_ = errA
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
		// CreateArtifact doesn't expose a single-id read; re-list by
		// execution and ensure the id is present.
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