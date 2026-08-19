package agent

import (
	"context"
	"errors"
)

// Adapter is the provider-agnostic boundary that translates a controller-
// built Request into a concrete agent invocation. It does NOT own the
// process lifecycle; that belongs to the Runtime. Two responsibilities:
//
//   - BuildInvocation: produce the protocol-specific command (args, stdin,
//     working dir) for the runtime to execute. For Maki this means building
//     the SDK-mode invocation with the right resume flags.
//   - ParseResult: turn the runtime-captured stdout into a normalized
//     Result (session id, stop reason, error flag, raw text). File-system
//     side effects (result.json, declared artifacts) are read by RunAgent,
//     not here.
//
// Session reuse is an adapter concern: the adapter knows how its underlying
// agent protocol resumes sessions and how to look up the prior session id.
type Adapter interface {
	BuildInvocation(ctx context.Context, req Request) (Invocation, error)
	ParseResult(ctx context.Context, req Request, raw RuntimeResult) (Result, error)
}

// RunAgent is the composition facade: it builds the invocation via the
// adapter, drives the runtime lifecycle, parses the protocol output, and
// reads the controller-assigned result.json and declared artifacts from
// disk. Empty ExecutionID is allocated by the runtime.
func RunAgent(ctx context.Context, adapter Adapter, runtime Runtime, req Request) (Result, error) {
	if adapter == nil {
		return Result{IsError: true}, errors.New("agent: RunAgent: adapter is nil")
	}
	if runtime == nil {
		return Result{IsError: true}, errors.New("agent: RunAgent: runtime is nil")
	}

	inv, err := adapter.BuildInvocation(ctx, req)
	if err != nil {
		return Result{IsError: true, ExecutionID: req.ExecutionID}, err
	}

	exec, err := runtime.Spawn(ctx, inv)
	if err != nil {
		return Result{IsError: true, ExecutionID: req.ExecutionID}, err
	}

	raw, waitErr := runtime.Wait(ctx, exec)
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