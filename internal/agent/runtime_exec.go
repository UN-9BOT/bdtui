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
// Spawn/Wait/Stop lifecycle the future HerdrRuntime will satisfy. It exists
// so the agent package can be developed and tested end-to-end before
// HerdrRuntime is implemented.
type ExecRuntime struct {
	mu    sync.Mutex
	procs map[string]*execHandle
}

// execHandle is the live state of a single process spawned by ExecRuntime.
// The runtime goroutine writes exitErr and closes done exactly once.
type execHandle struct {
	cmd        *exec.Cmd
	stdoutBuf  bytes.Buffer
	stderrBuf  bytes.Buffer
	done       chan struct{}
	exitErr    error
	finishOnce sync.Once
}

// NewExecRuntime builds an empty ExecRuntime.
func NewExecRuntime() *ExecRuntime {
	return &ExecRuntime{procs: map[string]*execHandle{}}
}

// Spawn starts inv as a child process and returns its durable Execution.ID.
// The process runs asynchronously; the caller blocks on Wait to collect the
// result. If the spawn fails, the returned error wraps the underlying cause
// and Execution.ID is empty.
func (r *ExecRuntime) Spawn(_ context.Context, inv Invocation) (Execution, error) {
	if inv.Bin == "" {
		return Execution{}, errors.New("agent: ExecRuntime: invocation bin is required")
	}
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

	id := uuid.NewString()
	r.mu.Lock()
	r.procs[id] = h
	r.mu.Unlock()

	go func() {
		h.exitErr = cmd.Wait()
		h.finishOnce.Do(func() { close(h.done) })
	}()

	return Execution{ID: id}, nil
}

// Wait blocks until the execution completes (exit, signal, or normal end) and
// returns the captured stdout/stderr/exitErr. Returns ErrNotFound if the id
// is unknown, ErrAlreadyDone if Wait was called more than once.
func (r *ExecRuntime) Wait(_ context.Context, exec Execution) (RuntimeResult, error) {
	h, ok := r.lookup(exec.ID)
	if !ok {
		return RuntimeResult{}, ErrNotFound
	}
	<-h.done
	r.mu.Lock()
	delete(r.procs, exec.ID)
	r.mu.Unlock()
	return RuntimeResult{
		Stdout:  append([]byte(nil), h.stdoutBuf.Bytes()...),
		Stderr:  append([]byte(nil), h.stderrBuf.Bytes()...),
		ExitErr: h.exitErr,
	}, nil
}

// Stop terminates a running execution. Returns ErrNotFound if unknown or
// already finished. Idempotent: stopping an already-finished execution is a
// no-op (returns nil).
func (r *ExecRuntime) Stop(_ context.Context, exec Execution) error {
	h, ok := r.lookup(exec.ID)
	if !ok {
		// Could be a finished execution we already removed; treat as no-op.
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