package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"bdtui/internal/daemon/daemonpb"
	"bdtui/internal/orch"
	"bdtui/internal/taskstore"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const eventPollInterval = 100 * time.Millisecond

// Service implements the daemon gRPC API over an orch.Store. It is the only
// writer to the store while the daemon is running, keeping concurrent clients
// serialized through SQLite's write transactions.
//
// When a TaskStore is configured (via NewServiceWithTasks), the service
// uses it to enforce the high-level task lifecycle bound to Beads: every
// CreateRun claims the task, and every transition to a terminal Run
// status synchronises the task back via the TaskStore. If the TaskStore
// is unavailable, CreateRun refuses to start (the spec'd "if Beads is
// unavailable, Run launch must not start" guarantee).
type Service struct {
	daemonpb.UnimplementedOrchestratorServer
	store *orch.Store
	tasks taskstore.TaskStore // optional; nil disables lifecycle integration
}

// NewService builds a Service with no TaskStore integration. This is the
// backward-compatible constructor for existing tests and tooling that have
// no Beads backend configured.
func NewService(store *orch.Store) *Service {
	return &Service{store: store}
}

// NewServiceWithTasks builds a Service with the given TaskStore wired
// in. The same store is used for both the SQLite orchestrator state and
// the high-level task lifecycle. The TaskStore may be nil; in that case
// the service behaves identically to NewService.
func NewServiceWithTasks(store *orch.Store, tasks taskstore.TaskStore) *Service {
	return &Service{store: store, tasks: tasks}
}

// HasTaskStore reports whether the service has a TaskStore wired in.
// Callers (and tests) use this to gate assertions that depend on the
// lifecycle integration.
func (s *Service) HasTaskStore() bool { return s.tasks != nil }

func (s *Service) CreateRun(ctx context.Context, req *daemonpb.CreateRunRequest) (*daemonpb.Run, error) {
	if req.TaskId == "" {
		return nil, status.Error(codes.InvalidArgument, "task_id is required")
	}
	if req.ProjectId == "" {
		return nil, status.Error(codes.InvalidArgument, "project_id is required")
	}
	if err := s.resolveOrCreateProject(ctx, req.ProjectId); err != nil {
		return nil, toStatus(err)
	}

	var taskSnap string
	if s.tasks != nil {
		// Claim the task in the TaskStore. If the backend is unavailable
		// or the task is already claimed, refuse to start the Run: the
		// spec requires "if Beads is unavailable, Run launch must not
		// start" and a double-claim would violate the single-active-Run
		// invariant. The atomic `bd update --claim` primitive is what
		// makes this safe under concurrent CreateRun calls.
		snap, err := s.tasks.Claim(ctx, req.TaskId)
		if err != nil {
			return nil, taskStoreToStatus(err)
		}
		taskSnap = encodeTaskSnapshot(snap)
	}

	r := &orch.Run{
		ProjectID:           req.ProjectId,
		TaskID:              req.TaskId,
		Status:              orch.RunQueued,
		WorkflowSnapshotRef: req.WorkflowSnapshotRef,
		WorkflowSnapshot:    req.WorkflowSnapshot,
		TaskSnapshot:        taskSnap,
	}
	if err := s.store.CreateRun(ctx, r); err != nil {
		return nil, toStatus(err)
	}
	if s.tasks != nil {
		// Audit the successful claim; the event stream is the only
		// operator-visible record of the high-level lifecycle binding.
		s.recordTaskClaimed(ctx, r)
	}
	return runToProto(r), nil
}

// recordTaskClaimed appends a task.claimed event so the controller and
// the operator can observe the high-level claim even if the controller
// itself never asked for it. Payload mirrors the orchestrator CreateRun
// output so event consumers can correlate.
func (s *Service) recordTaskClaimed(ctx context.Context, r *orch.Run) {
	runID := r.ID
	_ = s.store.AppendEvent(ctx, &runID, orch.EventTaskClaimed, encodeTaskClaimedEvent(r))
}

// encodeTaskClaimedEvent serialises the claim record. The payload is
// JSON with the task id and the snapshot-at timestamp so the event log
// is self-describing.
func encodeTaskClaimedEvent(r *orch.Run) string {
	b, err := json.Marshal(struct {
		RunID      string `json:"run_id"`
		ProjectID  string `json:"project_id"`
		TaskID     string `json:"task_id"`
		HasSnapshot bool  `json:"has_task_snapshot"`
	}{
		RunID:       r.ID,
		ProjectID:   r.ProjectID,
		TaskID:      r.TaskID,
		HasSnapshot: r.TaskSnapshot != "",
	})
	if err != nil {
		return ""
	}
	return string(b)
}

// encodeTaskSnapshot serialises a TaskStore snapshot into the JSON form
// persisted on the runs row. The taskstore package does not depend on
// encoding/json so the daemon owns the wire format.
func encodeTaskSnapshot(t *taskstore.Task) string {
	if t == nil {
		return ""
	}
	b, err := json.Marshal(struct {
		ID          string           `json:"id"`
		Title       string           `json:"title"`
		Description string           `json:"description"`
		Status      taskstore.TaskStatus `json:"status"`
		Priority    int              `json:"priority"`
		IssueType   string           `json:"issue_type"`
		SnapshotAt  string           `json:"snapshot_at"`
	}{
		ID:          t.ID,
		Title:       t.Title,
		Description: t.Description,
		Status:      t.Status,
		Priority:    t.Priority,
		IssueType:   t.IssueType,
		SnapshotAt:  t.SnapshotAt.UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		return ""
	}
	return string(b)
}

// syncLifecycleTask is the unified sync hook for run transitions. It
// covers all four cases in the spec's terminal / blocked mapping:
//
//	completed        -> done
//	failed           -> blocked
//	needs_attention  -> blocked (non-terminal! the Run is still active
//	                   but the task high-level state is blocked)
//	cancelled        -> todo
//
// `needs_attention` is intentionally non-terminal in the orchestrator
// state machine (the Run is recoverable), but the spec mandates the
// task high-level state move to blocked. We therefore apply the sync
// for needs_attention as well as the three terminal states; the
// `outcomeForRunStatus` helper is the only place that knows which
// outcomes apply.
//
// The sync is durable in three layers:
//
//   - The outbox row is persisted BEFORE the external TaskStore side
//     effect. If the outbox persist itself fails, we MUST NOT
//     attempt the external side effect: a side effect without a
//     durable intent is unrecoverable. The persist failure is
//     surfaced as a task.sync_failed event so the operator has an
//     audit trail to manually reconcile by.
//   - The actual TaskStore.SyncTerminal call runs in a detached
//     context with its own deadline, so the caller's HTTP timeout or
//     cancellation does not abort the sync attempt mid-flight.
//   - On success, the outbox row is marked done in the same detached
//     context. On failure, the row stays pending and the reconciler
//     retries it. The reconciler only ever sees the LATEST pending
//     row for a (run_id, task_id) pair because AppendTaskSyncOutbox
//     supersedes earlier pending rows in the same transaction.
func (s *Service) syncLifecycleTask(ctx context.Context, run *orch.Run) {
	if s.tasks == nil || run.TaskID == "" {
		return
	}
	outcome, err := outcomeForRunStatus(run.Status)
	if err != nil {
		// Caller invoked with a status that has no TaskStore mapping
		// (queued / running / waiting_human). Nothing to do.
		return
	}

	// Persist the durable intent FIRST, in the caller's context so
	// the write is bounded by the same deadline as the Run transition.
	// If this fails, we DO NOT attempt the external side effect:
	// the side effect without a durable intent is unrecoverable.
	outbox := &orch.TaskSyncOutbox{
		RunID:   run.ID,
		TaskID:  run.TaskID,
		Outcome: string(outcome),
		Status:  orch.TaskSyncPending,
	}
	outboxID, outboxErr := s.store.AppendTaskSyncOutbox(ctx, outbox)
	if outboxErr != nil {
		// The reconciler cannot find this Run if the outbox row was
		// not persisted. Surface the failure as an audit event in
		// the caller's context so the operator has a trail; the Run
		// row is already terminal in the orchestrator store, so the
		// operator can also read the orchestrator events.
		s.recordTaskSyncFailed(ctx, run, outcome, outboxErr)
		return
	}

	// Detached context for the external side effect. The caller's
	// deadline / cancellation MUST NOT abort the sync attempt
	// because the Run is already terminal and we still owe the Beads
	// backend a sync. A fresh 30s deadline is generous for a single
	// `bd update` call.
	syncCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := s.tasks.SyncTerminal(syncCtx, run.TaskID, outcome, outbox.Generation); err != nil {
		s.recordTaskSyncFailed(syncCtx, run, outcome, err)
		return
	}
	// Success: clear the outbox row. The audit event is the
	// human-facing trail; the outbox row's only purpose is to feed
	// the reconciler.
	_ = s.store.MarkTaskSyncOutboxDone(syncCtx, outboxID)
}

// recordTaskSyncFailed appends a task.sync_failed event so the sync
// failure leaves a durable, observable trail. The payload captures the
// Run id, the TaskStore id, the attempted outcome and the verbatim
// error message so the operator (and the controller) can decide
// whether to retry, surface it, or treat the Beads back-end as offline.
//
// In addition to the audit event, the function also appends a row to
// the durable task_sync_outbox so the future controller reconciler has
// a queryable list of Runs that still owe a Beads sync. The outbox
// row is the source of truth for "this Run still owes a sync"; the
// event is the audit/tui trail. Without the outbox, an already-closed
// Run has no path to a retry because the events are append-only and
// not re-driven.
func (s *Service) recordTaskSyncFailed(ctx context.Context, run *orch.Run, outcome taskstore.RunOutcome, syncErr error) {
	runID := run.ID
	b, err := json.Marshal(struct {
		RunID    string                 `json:"run_id"`
		ProjectID string                `json:"project_id"`
		TaskID   string                 `json:"task_id"`
		Status   string                 `json:"run_status"`
		Outcome  taskstore.RunOutcome   `json:"outcome"`
		Err      string                 `json:"error"`
	}{
		RunID:    run.ID,
		ProjectID: run.ProjectID,
		TaskID:   run.TaskID,
		Status:   string(run.Status),
		Outcome:  outcome,
		Err:      syncErr.Error(),
	})
	if err != nil {
		// Marshalling a fixed struct cannot fail in practice; fall back
		// to a minimal payload so the failure is still observably
		// recorded.
		b = []byte(fmt.Sprintf(`{"run_id":%q,"task_id":%q,"error":%q}`, run.ID, run.TaskID, syncErr.Error()))
	}
	_ = s.store.AppendEvent(ctx, &runID, orch.EventTaskSyncFailed, string(b))
}

// syncTerminalTask is a thin wrapper that preserves the historic name
// for callers that have not switched to the unified name. It is
// identical to syncLifecycleTask.
func (s *Service) syncTerminalTask(ctx context.Context, run *orch.Run) {
	s.syncLifecycleTask(ctx, run)
}

// outcomeForRunStatus returns the TaskStore outcome the given Run
// status maps to, or ErrInvalidOutcome if the status has no mapping
// (queued / running / waiting_human). The mapping is the spec's
// lifecycle contract; see internal/taskstore.MapRunOutcomeToTaskStatus.
func outcomeForRunStatus(s orch.RunStatus) (taskstore.RunOutcome, error) {
	switch s {
	case orch.RunCompleted:
		return taskstore.RunCompleted, nil
	case orch.RunFailed:
		return taskstore.RunFailed, nil
	case orch.RunNeedsAttention:
		return taskstore.RunNeedsAttention, nil
	case orch.RunCancelled:
		return taskstore.RunCancelled, nil
	default:
		return "", taskstore.ErrInvalidOutcome
	}
}

// taskStoreToStatus maps taskstore sentinel errors onto gRPC status
// codes. The mapping is conservative: anything that looks like
// "unavailable" maps to codes.Unavailable so the client can retry, and
// "already claimed" maps to codes.AlreadyExists so the client surfaces
// the existing active run.
func taskStoreToStatus(err error) error {
	switch {
	case errors.Is(err, taskstore.ErrTaskNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, taskstore.ErrTaskAlreadyClaimed):
		return status.Error(codes.AlreadyExists, err.Error())
	case errors.Is(err, taskstore.ErrTaskStoreUnavailable):
		return status.Error(codes.Unavailable, err.Error())
	case errors.Is(err, taskstore.ErrInvalidOutcome):
		return status.Error(codes.InvalidArgument, err.Error())
	default:
		return status.Error(codes.Internal, err.Error())
	}
}

// resolveOrCreateProject treats project_id as the canonical project handle.
// Idempotent: on a fresh id the row is created; on a repeat call the existing
// row is returned without modification.
func (s *Service) resolveOrCreateProject(ctx context.Context, id string) error {
	_, err := s.store.EnsureProject(ctx, &orch.Project{
		ID:   id,
		Name: id,
	})
	return err
}

func (s *Service) ListRuns(ctx context.Context, req *daemonpb.ListRunsRequest) (*daemonpb.ListRunsResponse, error) {
	var (
		runs []orch.Run
		err  error
	)
	if req.ProjectId != nil && *req.ProjectId != "" {
		runs, err = s.store.ListRunsByProject(ctx, *req.ProjectId)
	} else {
		runs, err = s.store.ListRuns(ctx)
	}
	if err != nil {
		return nil, toStatus(err)
	}

	resp := &daemonpb.ListRunsResponse{Runs: make([]*daemonpb.Run, 0, len(runs))}
	for i := range runs {
		resp.Runs = append(resp.Runs, runToProto(&runs[i]))
	}
	return resp, nil
}

func (s *Service) GetRun(ctx context.Context, req *daemonpb.GetRunRequest) (*daemonpb.Run, error) {
	r, err := s.store.GetRun(ctx, req.Id)
	if err != nil {
		return nil, toStatus(err)
	}
	return runToProto(r), nil
}

func (s *Service) AnswerHumanInput(ctx context.Context, req *daemonpb.AnswerHumanInputRequest) (*daemonpb.HumanInput, error) {
	if req.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}
	if err := s.store.AnswerHumanInput(ctx, req.Id, req.Response); err != nil {
		return nil, toStatus(err)
	}
	h, err := s.store.GetHumanInput(ctx, req.Id)
	if err != nil {
		return nil, toStatus(err)
	}
	return humanInputToProto(h), nil
}

// ListHumanInputs returns human inputs attached to a single run, or
// every human input in the store when req.RunId is unset. Used by the
// Runs tab to enumerate the human input ids it needs to surface (and
// the operator to answer) for any waiting_human row.
func (s *Service) ListHumanInputs(ctx context.Context, req *daemonpb.ListHumanInputsRequest) (*daemonpb.ListHumanInputsResponse, error) {
	runID := ""
	if req.RunId != nil {
		runID = *req.RunId
	}
	var (
		rows []orch.HumanInput
		err  error
	)
	if runID != "" {
		rows, err = s.store.ListHumanInputsByRun(ctx, runID)
	} else {
		// No run id: list every pending human input. The MVP has no
		// "all inputs" store method, so for now we return an error if
		// the caller didn't narrow the query. This keeps the contract
		// tight and forces clients to scope by run.
		return nil, status.Error(codes.InvalidArgument, "run_id is required")
	}
	if err != nil {
		return nil, toStatus(err)
	}
	resp := &daemonpb.ListHumanInputsResponse{HumanInputs: make([]*daemonpb.HumanInput, 0, len(rows))}
	for i := range rows {
		resp.HumanInputs = append(resp.HumanInputs, humanInputToProto(&rows[i]))
	}
	return resp, nil
}

func (s *Service) RetryRun(ctx context.Context, req *daemonpb.RetryRunRequest) (*daemonpb.Run, error) {
	if err := s.store.RequestRunRetry(ctx, req.Id); err != nil {
		return nil, toStatus(err)
	}
	r, err := s.store.GetRun(ctx, req.Id)
	if err != nil {
		return nil, toStatus(err)
	}
	return runToProto(r), nil
}

func (s *Service) CancelRun(ctx context.Context, req *daemonpb.CancelRunRequest) (*daemonpb.Run, error) {
	if err := s.store.TransitionRun(ctx, req.Id, orch.RunCancelled); err != nil {
		return nil, toStatus(err)
	}
	r, err := s.store.GetRun(ctx, req.Id)
	if err != nil {
		return nil, toStatus(err)
	}
	s.syncTerminalTask(ctx, r)
	return runToProto(r), nil
}

func (s *Service) InspectExecution(ctx context.Context, req *daemonpb.InspectExecutionRequest) (*daemonpb.InspectExecutionResponse, error) {
	e, err := s.store.GetExecution(ctx, req.Id)
	if err != nil {
		return nil, toStatus(err)
	}
	artifacts, err := s.store.ListArtifactsByExecution(ctx, req.Id)
	if err != nil {
		return nil, toStatus(err)
	}

	resp := &daemonpb.InspectExecutionResponse{
		Execution: executionToProto(e),
		Artifacts: make([]*daemonpb.Artifact, 0, len(artifacts)),
	}
	for i := range artifacts {
		resp.Artifacts = append(resp.Artifacts, artifactToProto(&artifacts[i]))
	}
	return resp, nil
}

// ListExecutions returns the executions attached to a run. The
// BIR-54 contract scopes by run so the operator only sees the
// executions of their selected run; callers must supply run_id.
func (s *Service) ListExecutions(ctx context.Context, req *daemonpb.ListExecutionsRequest) (*daemonpb.ListExecutionsResponse, error) {
	runID := ""
	if req.RunId != nil {
		runID = *req.RunId
	}
	if runID == "" {
		return nil, status.Error(codes.InvalidArgument, "run_id is required")
	}
	rows, err := s.store.ListExecutionsByRun(ctx, runID)
	if err != nil {
		return nil, toStatus(err)
	}
	resp := &daemonpb.ListExecutionsResponse{Executions: make([]*daemonpb.Execution, 0, len(rows))}
	for i := range rows {
		resp.Executions = append(resp.Executions, executionToProto(&rows[i]))
	}
	return resp, nil
}

func (s *Service) StreamEvents(req *daemonpb.StreamEventsRequest, stream daemonpb.Orchestrator_StreamEventsServer) error {
	ctx := stream.Context()
	if req.RunId == "" {
		return status.Error(codes.InvalidArgument, "run_id is required")
	}
	if _, err := s.store.GetRun(ctx, req.RunId); err != nil {
		return toStatus(err)
	}

	after := req.AfterSeq
	ticker := time.NewTicker(eventPollInterval)
	defer ticker.Stop()

	for {
		if err := sendEventsAfter(ctx, s.store, stream, req.RunId, &after); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func sendEventsAfter(ctx context.Context, store *orch.Store, stream daemonpb.Orchestrator_StreamEventsServer, runID string, after *int64) error {
	events, err := store.ListEventsByRunAfter(ctx, runID, *after)
	if err != nil {
		return toStatus(err)
	}
	for i := range events {
		if err := stream.Send(eventToProto(&events[i])); err != nil {
			return err
		}
		*after = events[i].Seq
	}
	return nil
}

func toStatus(err error) error {
	switch {
	case errors.Is(err, orch.ErrNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, orch.ErrInvalidTransition):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, orch.ErrInvalidStatus):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, orch.ErrActiveRunExists):
		return status.Error(codes.AlreadyExists, err.Error())
	default:
		return status.Error(codes.Internal, err.Error())
	}
}