package agent

import "context"

// Adapter is the provider-agnostic boundary between the controller and a
// concrete agent runner. Implementations hide the underlying CLI/protocol
// (MakiAdapter hides `maki --print`, a future ALKAdapter would hide ALK's
// SDK, etc.) and translate a controller-built Request into a single agent
// invocation that returns a process-level Result.
//
// The controller is responsible for assembling the deterministic prompt
// envelope, assigning output paths, and validating completion. The adapter is
// responsible for: launching the underlying process, capturing the process
// exit code and any session/stop metadata, and reading the controller-assigned
// result.json and declared artifacts from disk after process completion.
type Adapter interface {
	// Run executes the agent invocation described by req. It returns when the
	// underlying process completes (cleanly or otherwise). Result.Artifacts is
	// populated for every role-declared output whose assigned path exists and
	// is readable after process completion; missing artifacts are simply absent.
	// Result.ResultJSON is populated from OutputPaths.Result when present.
	Run(ctx context.Context, req Request) (Result, error)
}

// RunOpts are the per-invocation options for a CommandRunner.
type RunOpts struct {
	// Dir is the working directory for the spawned process. Empty means
	// inherit the runner's current directory.
	Dir string
	// Stdin is the data piped to the process on its standard input. Empty
	// means no stdin.
	Stdin []byte
}

// CommandRunner is the low-level exec abstraction used by concrete adapters.
// It exists so MakiAdapter (and any future adapter) can be unit-tested with a
// fake runner without spawning a real binary.
type CommandRunner interface {
	// Run executes the given binary with args under opts. It returns the
	// captured stdout, stderr, and a non-nil error if the process exited
	// non-zero or could not be started. Callers should not interpret the
	// error; they should consult the captured output and decide based on its
	// structured contents.
	Run(ctx context.Context, bin string, args []string, opts RunOpts) (stdout, stderr []byte, err error)
}
