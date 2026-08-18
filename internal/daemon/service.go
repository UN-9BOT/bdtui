package daemon

import (
	"context"
	"errors"
	"time"

	"bdtui/internal/daemon/daemonpb"
	"bdtui/internal/orch"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const eventPollInterval = 100 * time.Millisecond

// Service implements the daemon gRPC API over an orch.Store. It is the only
// writer to the store while the daemon is running, keeping concurrent clients
// serialized through SQLite's write transactions.
type Service struct {
	daemonpb.UnimplementedOrchestratorServer
	store *orch.Store
}

func NewService(store *orch.Store) *Service {
	return &Service{store: store}
}

func (s *Service) CreateRun(ctx context.Context, req *daemonpb.CreateRunRequest) (*daemonpb.Run, error) {
	if req.ProjectId == "" {
		return nil, status.Error(codes.InvalidArgument, "project_id is required")
	}
	if _, err := s.store.GetProject(ctx, req.ProjectId); err != nil {
		return nil, toStatus(err)
	}

	runStatus := orch.RunQueued
	if req.Status != "" {
		runStatus = orch.RunStatus(req.Status)
		if !runStatus.Valid() {
			return nil, status.Errorf(codes.InvalidArgument, "invalid run status %q", req.Status)
		}
	}

	r := &orch.Run{
		ProjectID:           req.ProjectId,
		TaskID:              req.TaskId,
		Status:              runStatus,
		WorkflowSnapshotRef: req.WorkflowSnapshotRef,
		WorkflowSnapshot:    req.WorkflowSnapshot,
	}
	if err := s.store.CreateRun(ctx, r); err != nil {
		return nil, toStatus(err)
	}
	return runToProto(r), nil
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

func (s *Service) RetryRun(ctx context.Context, req *daemonpb.RetryRunRequest) (*daemonpb.Run, error) {
	if err := s.store.TransitionRun(ctx, req.Id, orch.RunRunning); err != nil {
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
	events, err := store.ListEventsByRun(ctx, runID)
	if err != nil {
		return toStatus(err)
	}
	for i := range events {
		if events[i].Seq > *after {
			if err := stream.Send(eventToProto(&events[i])); err != nil {
				return err
			}
			*after = events[i].Seq
		}
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
