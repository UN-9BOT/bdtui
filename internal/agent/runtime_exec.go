package agent

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
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
//
// Durable-ID atomicity: Spawn reserves inv.ExecutionID in the map (with a
// "reserved" placeholder) BEFORE calling cmd.Start(). Two concurrent
// Spawn calls with the same ID race on the mutex; the loser sees the
// reservation and returns ErrDuplicateExecution. Only one cmd.Start() ever
// runs per ID, so a writer execution cannot be duplicated.
type ExecRuntime struct {
	mu    sync.Mutex
	procs map[string]*execHandle
}

type handleState int

const (
	handleReserved handleState = iota // ID claimed, cmd.Start() not yet called
	handleStarted                     // cmd.Start() succeeded, process is running
)

type execHandle struct {
	state      handleState
	cmd        *exec.Cmd
	stdoutBuf  bytes.Buffer
	stderrBuf  bytes.Buffer
	startDone  chan struct{}
	done       chan struct{}
	exitErr    error
	finishOnce sync.Once
}

func NewExecRuntime() *ExecRuntime {
	return &ExecRuntime{procs: map[string]*execHandle{}}
}

// Spawn atomically reserves inv.ExecutionID and then starts the child
// process. The reservation is committed before cmd.Start() so concurrent
// Spawn calls with the same ID cannot both reach Start. If Start fails the
// reservation is rolled back; if it succeeds the handle is moved to
// handleStarted and a goroutine waits for completion.
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
	h := &execHandle{
		state:     handleReserved,
		startDone: make(chan struct{}),
		done:      make(chan struct{}),
	}
	r.procs[inv.ExecutionID] = h
	r.mu.Unlock()

	cmd := exec.Command(inv.Bin, inv.Args...)
	configureProcess(cmd)
	if inv.Dir != "" {
		cmd.Dir = inv.Dir
	}
	if len(inv.Stdin) > 0 {
		cmd.Stdin = bytes.NewReader(inv.Stdin)
	} else {
		cmd.Stdin = io.NopCloser(bytes.NewReader(nil))
	}
	cmd.Stdout = &h.stdoutBuf
	cmd.Stderr = &h.stderrBuf

	if err := cmd.Start(); err != nil {
		// Roll back the reservation and unblock any waiters with the
		// start error so they do not deadlock on h.done.
		r.mu.Lock()
		delete(r.procs, inv.ExecutionID)
		close(h.startDone)
		r.mu.Unlock()
		h.exitErr = err
		h.finishOnce.Do(func() { close(h.done) })
		return Execution{}, err
	}

	r.mu.Lock()
	h.cmd = cmd
	h.state = handleStarted
	close(h.startDone)
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
// return the same buffered result.
func (r *ExecRuntime) Wait(ctx context.Context, exec Execution) (RuntimeResult, error) {
	h, ok := r.lookup(exec.ID)
	if !ok {
		return RuntimeResult{}, ErrLostExecution
	}
	return r.waitHandle(ctx, exec.ID, h)
}

func (r *ExecRuntime) waitHandle(_ context.Context, _ string, h *execHandle) (RuntimeResult, error) {
	<-h.done
	return RuntimeResult{
		Stdout:  append([]byte(nil), h.stdoutBuf.Bytes()...),
		Stderr:  append([]byte(nil), h.stderrBuf.Bytes()...),
		ExitErr: h.exitErr,
	}, nil
}

// Stop terminates a running execution. No-op if the ID is unknown or the
// process already exited.
func (r *ExecRuntime) Stop(ctx context.Context, exec Execution) error {
	h, ok := r.lookup(exec.ID)
	if !ok {
		return nil
	}

	r.mu.Lock()
	state := h.state
	startDone := h.startDone
	r.mu.Unlock()
	if state == handleReserved {
		select {
		case <-startDone:
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	r.mu.Lock()
	state = h.state
	var proc *os.Process
	if h.cmd != nil {
		proc = h.cmd.Process
	}
	r.mu.Unlock()
	if state != handleStarted || proc == nil {
		return nil
	}
	select {
	case <-h.done:
		return nil
	default:
	}
	return stopProcess(proc)
}

func (r *ExecRuntime) lookup(id string) (*execHandle, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	h, ok := r.procs[id]
	return h, ok
}

// AllocateExecutionID returns a new UUID suitable as a controller-side
// durable execution identity.
func AllocateExecutionID() string {
	return uuid.NewString()
}
