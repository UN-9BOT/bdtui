package agent

import (
	"context"
	"errors"
)

// Invocation is a provider-agnostic description of what the runtime should
// run. The adapter builds it from a Request; the runtime executes it.
type Invocation struct {
	Bin   string
	Args  []string
	Dir   string
	Stdin []byte
}

// Execution is the durable, runtime-side identity of a single attempt. The
// runtime owns the mapping from ID to the live process and uses it for
// wait/stop/inspect/reattach. This is distinct from the agent-level session
// id captured by the adapter (Result.SessionID): ExecutionID lives in the
// runtime (Herdr pane/process), SessionID lives in the agent (Maki session).
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

// Runtime owns the lifecycle of an external process invocation. It is the
// seam where the future HerdrRuntime plugs in: HerdrRuntime spawns the agent
// inside a Herdr pane, captures pane/process identity as Execution.ID, and
// supports reattach on daemon restart. ExecRuntime is the MVP default and
// demonstrates the same async lifecycle semantics using os/exec directly.
//
// Lifecycle:
//
//	Spawn(ctx, inv) -> Execution   // durable ID allocated; process started
//	Wait(ctx, exec) -> RuntimeResult // blocks until completion
//	Stop(ctx, exec) -> error         // terminates the running process
//
// Spawn/Wait/Stop may be called from different goroutines and across daemon
// restarts (reattach by Execution.ID).
type Runtime interface {
	Spawn(ctx context.Context, inv Invocation) (Execution, error)
	Wait(ctx context.Context, exec Execution) (RuntimeResult, error)
	Stop(ctx context.Context, exec Execution) error
}

// ErrAlreadyDone is returned by Spawn/Wait when the execution has already
// terminated.
var ErrAlreadyDone = errors.New("agent: runtime: execution already done")

// ErrNotFound is returned by Wait/Stop when the execution ID is unknown to
// the runtime.
var ErrNotFound = errors.New("agent: runtime: execution not found")