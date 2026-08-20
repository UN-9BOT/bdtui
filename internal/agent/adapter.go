package agent

import (
	"context"
	"errors"
	"fmt"
)

// Adapter is the provider-agnostic boundary that translates a controller-
// built Request into a concrete agent invocation. It does NOT own the
// process lifecycle; that belongs to the Runtime.
type Adapter interface {
	BuildInvocation(ctx context.Context, req Request) (Invocation, error)
	ParseResult(ctx context.Context, req Request, raw RuntimeResult) (Result, error)
}

// RunAgent is the composition facade. It honours the durable execution_id
// invariant:
//
//   - The controller allocates ExecutionID (UUID), persists it in the orch
//     store BEFORE calling RunAgent, and passes it on every call.
//   - Reattach=false (the default): fresh spawn. BuildInvocation produces an
//     Invocation whose ExecutionID equals req.ExecutionID; the runtime
//     Spawns under that ID.
//   - Reattach=true: the controller is recovering a previously-persisted
//     attempt. RunAgent Inspects the runtime for that ID and Waits. If the
//     runtime has no record (daemon died, ExecRuntime is fresh), it returns
//     ErrLostExecution so the controller can resolve it into needs_attention
//     or technical retry per the runtime/recovery contract.
//
// RunAgent NEVER Spawns when req.Reattach is true, so a duplicate writer
// execution cannot be triggered by accident during recovery.
func RunAgent(ctx context.Context, adapter Adapter, runtime Runtime, req Request) (Result, error) {
	if adapter == nil {
		return Result{IsError: true, ExecutionID: req.ExecutionID}, errors.New("agent: RunAgent: adapter is nil")
	}
	if runtime == nil {
		return Result{IsError: true, ExecutionID: req.ExecutionID}, errors.New("agent: RunAgent: runtime is nil")
	}
	if req.ExecutionID == "" {
		return Result{IsError: true}, errors.New("agent: RunAgent: ExecutionID is required (controller must allocate + persist before invoking)")
	}

	exec := Execution{ID: req.ExecutionID}

	if req.Reattach {
		raw, err := runtime.Reattach(ctx, exec)
		res, parseErr := adapter.ParseResult(ctx, req, raw)
		res.ExecutionID = exec.ID
		if err != nil {
			res.IsError = true
			return res, err
		}
		if parseErr != nil && !res.IsError {
			res.IsError = true
			return res, parseErr
		}
		if res.IsError {
			return res, firstErr(parseErr, nil)
		}
		res.ResultJSON = readFile(req.OutputPaths.Result)
		res.Artifacts = readArtifacts(req.OutputPaths.Artifacts)
		if parseErr != nil {
			res.IsError = true
			return res, parseErr
		}
		return res, nil
	}

	inv, err := adapter.BuildInvocation(ctx, req)
	if err != nil {
		return Result{IsError: true, ExecutionID: exec.ID}, err
	}
	if inv.ExecutionID != exec.ID {
		return Result{IsError: true, ExecutionID: exec.ID}, fmt.Errorf("agent: RunAgent: adapter produced Invocation.ExecutionID=%q, want %q", inv.ExecutionID, exec.ID)
	}

	spawned, err := runtime.Spawn(ctx, inv)
	if err != nil {
		return Result{IsError: true, ExecutionID: exec.ID}, err
	}
	if spawned.ID != exec.ID {
		// Runtime ignored inv.ExecutionID; this violates the durable-id
		// contract. Abort so the controller does not persist a mismatched id.
		_ = runtime.Stop(ctx, spawned)
		return Result{IsError: true, ExecutionID: spawned.ID}, fmt.Errorf("agent: RunAgent: runtime returned Execution.ID=%q, want %q", spawned.ID, exec.ID)
	}

	raw, waitErr := runtime.Wait(ctx, spawned)
	res, parseErr := adapter.ParseResult(ctx, req, raw)
	res.ExecutionID = exec.ID

	if waitErr != nil && !res.IsError {
		res.IsError = true
		if parseErr == nil {
			return res, waitErr
		}
	}

	if res.IsError {
		return res, firstErr(parseErr, waitErr)
	}

	res.ResultJSON = readFile(req.OutputPaths.Result)
	res.Artifacts = readArtifacts(req.OutputPaths.Artifacts)
	if parseErr != nil {
		res.IsError = true
		return res, parseErr
	}
	return res, nil
}

func firstErr(a, b error) error {
	if a != nil {
		return a
	}
	return b
}