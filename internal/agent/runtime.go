package agent

import (
	"context"
	"errors"
)

// Invocation is a provider-agnostic description of what the runtime should
// run. ExecutionID is the durable, controller-allocated identity that the
// runtime MUST use as the spawned Execution.ID; the adapter copies it from
// Request.ExecutionID so the runtime contract is enforced end-to-end.
type Invocation struct {
	ExecutionID string
	Bin         string
	Args        []string
	Dir         string
	Stdin       []byte
}

// Execution is the durable, runtime-side identity of a single attempt.
type Execution struct {
	ID string
}

// RuntimeResult is the captured outcome of a runtime invocation. It is the
// raw bytes + exit status; the adapter then parses the protocol-specific
// portion (e.g. Maki wire format) out of stdout.
type RuntimeResult struct {
	Stdout  []byte
	Stderr  []byte
	ExitErr error
}

// InspectResult describes what the runtime knows about an Execution ID.
type InspectResult struct {
	// Found is true if the runtime has a record of this ID (running or
	// already completed with a buffered result).
	Found bool
	// Running is true if the underlying process is still alive.
	Running bool
}

// Runtime owns the lifecycle of an external process invocation. It is the
// seam where the future HerdrRuntime plugs in.
//
// Durable-id contract:
//
//   - Spawn(ctx, Invocation{ExecutionID, ...}) MUST use inv.ExecutionID as
//     the spawned Execution.ID. The controller has already persisted that
//     ID, so a mismatch is a fatal contract violation.
//   - Reattach(ctx, Execution{ID}) is the recovery path: it Inspects the ID
//     and, if found, blocks until completion and returns the buffered
//     result. If the runtime has no record (e.g. ExecRuntime after a
//     daemon restart), it returns ErrLostExecution.
//   - Inspect(ctx, Execution{ID}) exposes the Found/Running state for
//     callers that want to decide between Wait and Stop without Wait
//     blocking.
//   - Wait blocks until the execution completes.
//   - Stop terminates a running execution.
type Runtime interface {
	Spawn(ctx context.Context, inv Invocation) (Execution, error)
	Reattach(ctx context.Context, exec Execution) (RuntimeResult, error)
	Inspect(ctx context.Context, exec Execution) (InspectResult, error)
	Wait(ctx context.Context, exec Execution) (RuntimeResult, error)
	Stop(ctx context.Context, exec Execution) error
}

// ErrDuplicateExecution is returned by Spawn when inv.ExecutionID already
// has a live record in the runtime.
var ErrDuplicateExecution = errors.New("agent: runtime: duplicate execution id")