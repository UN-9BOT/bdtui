package agent

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os/exec"
	"sync"

	"github.com/google/uuid"
)

// ExecRuntime is the MVP default Runtime. It uses os/exec to spawn the
// agent binary, captures stdout/stderr asynchronously, and exposes the same
// Spawn/Reattach/Inspect/Wait/Stop lifecycle the future HerdrRuntime will
// satisfy. It exists so the agent package can be developed and tested
// end-to-end before HerdrRuntime is implemented.
//
// ExecRuntime keeps completed handles in its map after Wait so a same-process
// Reattach can still retrieve the buffered result. After a daemon restart
// the in-memory map is empty and Reattach returns ErrLostExecution; that
// is the crash-recovery signal the controller resolves per bdtui-6pc.
type ExecRuntime struct {
	mu    sync.Mutex
	procs map[string]*execHandle
}

type execHandle struct {
	cmd        *exec.Cmd
	stdoutBuf  bytes.Buffer
	stderrBuf  bytes.Buffer
	done       chan struct{}
	exitErr    error
	finishOnce sync.Once
}

func NewExecRuntime() *ExecRuntime {
	return &ExecRuntime{procs: map[string]*execHandle{}}
}

// Spawn uses inv.ExecutionID as the durable Execution.ID. The caller must
// pre-allocate that ID; Spawn rejects empty or duplicate IDs so the
// controller can rely on the persisted ID matching the live process.
func (r *ExecRuntime) Spawn(_ context.Context, inv Invocation) (Execution, error) {
	if inv.ExecutionID == "" {
		return Execution{}, errors.New("agent: ExecRuntime: Invocation.ExecutionID is required")
	}
	if inv.Bin == "" {
		return Execution{}, errors.New("agent: ExecRuntime: Invocation.Bin is required")
	}
	r.mu.Lock()
	if _, exists := r.procs[inv.ExecutionID]; exists {
		r.mu.Unlock()
		return Execution{}, ErrDuplicateExecution
	}
	r.mu.Unlock()

	cmd := exec.Command(inv.Bin, inv.Args...)
	if inv.Dir != "" {
		cmd.Dir = inv.Dir
	}
	if len(inv.Stdin) > 0 {
		cmd.Stdin = bytes.NewReader(inv.Stdin)
	} else {
		cmd.Stdin = io.NopCloser(bytes.NewReader(nil))
	}

	h := &execHandle{cmd: cmd, done: make(chan struct{})}
	cmd.Stdout = &h.stdoutBuf
	cmd.Stderr = &h.stderrBuf

	if err := cmd.Start(); err != nil {
		return Execution{}, err
	}

	r.mu.Lock()
	r.procs[inv.ExecutionID] = h
	r.mu.Unlock()

	go func() {
		h.exitErr = cmd.Wait()
		h.finishOnce.Do(func() { close(h.done) })
	}()

	return Execution{ID: inv.ExecutionID}, nil
}

// Reattach is the recovery entry point. If the in-memory map still holds
// the ID, it Waits and returns the (possibly already-completed) result.
// Otherwise it returns ErrLostExecution so the controller can resolve
// needs_attention / technical retry.
func (r *ExecRuntime) Reattach(ctx context.Context, exec Execution) (RuntimeResult, error) {
	h, ok := r.lookup(exec.ID)
	if !ok {
		return RuntimeResult{}, ErrLostExecution
	}
	return r.waitHandle(ctx, exec.ID, h)
}

// Inspect reports whether the ID is known and whether the process is still
// running, without blocking.
func (r *ExecRuntime) Inspect(_ context.Context, exec Execution) (InspectResult, error) {
	h, ok := r.lookup(exec.ID)
	if !ok {
		return InspectResult{}, nil
	}
	select {
	case <-h.done:
		return InspectResult{Found: true, Running: false}, nil
	default:
		return InspectResult{Found: true, Running: true}, nil
	}
}

// Wait blocks until the execution completes. Repeated calls on the same ID
// return the same buffered result (the handle is retained until process
// exit; the map is the source of truth).
func (r *ExecRuntime) Wait(ctx context.Context, exec Execution) (RuntimeResult, error) {
	h, ok := r.lookup(exec.ID)
	if !ok {
		return RuntimeResult{}, ErrLostExecution
	}
	return r.waitHandle(ctx, exec.ID, h)
}

func (r *ExecRuntime) waitHandle(_ context.Context, id string, h *execHandle) (RuntimeResult, error) {
	<-h.done
	return RuntimeResult{
		Stdout:  append([]byte(nil), h.stdoutBuf.Bytes()...),
		Stderr:  append([]byte(nil), h.stderrBuf.Bytes()...),
		ExitErr: h.exitErr,
	}, nil
}

// Stop terminates a running execution. No-op if the ID is unknown or the
// process already exited.
func (r *ExecRuntime) Stop(_ context.Context, exec Execution) error {
	h, ok := r.lookup(exec.ID)
	if !ok {
		return nil
	}
	if h.cmd.Process == nil {
		return nil
	}
	return h.cmd.Process.Kill()
}

func (r *ExecRuntime) lookup(id string) (*execHandle, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	h, ok := r.procs[id]
	return h, ok
}

// AllocateExecutionID returns a new UUID suitable as a controller-side
// durable execution identity. It is a convenience wrapper around the
// uuid package so callers do not import github.com/google/uuid directly.
func AllocateExecutionID() string {
	return uuid.NewString()
}