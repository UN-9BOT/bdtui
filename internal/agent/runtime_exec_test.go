package agent

import (
	"context"
	"testing"
	"time"
)

// TestExecRuntimeSpawnWait drives ExecRuntime through a real os/exec child
// (`/bin/sh -c "echo hello"`) using a controller-allocated ExecutionID,
// asserting Spawn uses that ID, Wait captures stdout, and Inspect reports
// running/completed correctly.
func TestExecRuntimeSpawnWait(t *testing.T) {
	r := NewExecRuntime()
	execID := AllocateExecutionID()
	exec, err := r.Spawn(context.Background(), Invocation{
		ExecutionID: execID,
		Bin:         "/bin/sh",
		Args:        []string{"-c", "echo hello"},
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	if exec.ID != execID {
		t.Fatalf("Execution.ID=%q want %q (controller-allocated)", exec.ID, execID)
	}

	ins, err := r.Inspect(context.Background(), exec)
	if err != nil || !ins.Found || !ins.Running {
		t.Fatalf("Inspect during run: %+v %v", ins, err)
	}

	res, err := r.Wait(context.Background(), exec)
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if string(res.Stdout) != "hello\n" {
		t.Fatalf("stdout=%q", res.Stdout)
	}
	if res.ExitErr != nil {
		t.Fatalf("ExitErr=%v", res.ExitErr)
	}

	// After completion Inspect reports Found && !Running.
	ins, _ = r.Inspect(context.Background(), exec)
	if !ins.Found || ins.Running {
		t.Fatalf("Inspect after completion: %+v", ins)
	}

	// Second Wait returns the buffered result (handle retained).
	res2, err := r.Wait(context.Background(), exec)
	if err != nil || string(res2.Stdout) != "hello\n" {
		t.Fatalf("second Wait: %v stdout=%q", err, res2.Stdout)
	}
}

// TestExecRuntimeReattachSameProcess verifies Reattach works for an
// execution that is still in the runtime's map (same-process recovery,
// or completion-buffer retrieval).
func TestExecRuntimeReattachSameProcess(t *testing.T) {
	r := NewExecRuntime()
	execID := AllocateExecutionID()
	exec, err := r.Spawn(context.Background(), Invocation{
		ExecutionID: execID,
		Bin:         "/bin/sh",
		Args:        []string{"-c", "echo reattach"},
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	res, err := r.Reattach(context.Background(), exec)
	if err != nil {
		t.Fatalf("Reattach: %v", err)
	}
	if string(res.Stdout) != "reattach\n" {
		t.Fatalf("stdout=%q", res.Stdout)
	}
}

// TestExecRuntimeReattachLostExecution verifies Reattach returns
// ErrLostExecution for an ID not in the map (the cross-restart crash
// recovery signal).
func TestExecRuntimeReattachLostExecution(t *testing.T) {
	r := NewExecRuntime()
	_, err := r.Reattach(context.Background(), Execution{ID: "unknown-id"})
	if err != ErrLostExecution {
		t.Fatalf("expected ErrLostExecution, got %v", err)
	}
	if _, err := r.Wait(context.Background(), Execution{ID: "unknown-id"}); err != ErrLostExecution {
		t.Fatalf("expected ErrLostExecution from Wait, got %v", err)
	}
}

// TestExecRuntimeSpawnRequiresExecutionID verifies Spawn rejects an empty
// ExecutionID — the durable-id contract.
func TestExecRuntimeSpawnRequiresExecutionID(t *testing.T) {
	r := NewExecRuntime()
	if _, err := r.Spawn(context.Background(), Invocation{Bin: "/bin/sh", Args: []string{"-c", "true"}}); err == nil {
		t.Fatal("expected error for empty ExecutionID")
	}
}

// TestExecRuntimeSpawnRejectsDuplicate verifies Spawn rejects a duplicate
// ExecutionID so the persisted identity cannot be silently shadowed.
func TestExecRuntimeSpawnRejectsDuplicate(t *testing.T) {
	r := NewExecRuntime()
	id := AllocateExecutionID()
	if _, err := r.Spawn(context.Background(), Invocation{
		ExecutionID: id, Bin: "/bin/sh", Args: []string{"-c", "sleep 10"},
	}); err != nil {
		t.Fatalf("first Spawn: %v", err)
	}
	if _, err := r.Spawn(context.Background(), Invocation{
		ExecutionID: id, Bin: "/bin/sh", Args: []string{"-c", "true"},
	}); err != ErrDuplicateExecution {
		t.Fatalf("expected ErrDuplicateExecution, got %v", err)
	}
}

// TestExecRuntimeStop verifies Stop terminates a long-running child and
// causes Wait to return with a non-nil ExitErr.
func TestExecRuntimeStop(t *testing.T) {
	r := NewExecRuntime()
	execID := AllocateExecutionID()
	exec, err := r.Spawn(context.Background(), Invocation{
		ExecutionID: execID,
		Bin:         "/bin/sh",
		Args:        []string{"-c", "sleep 10"},
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	if err := r.Stop(context.Background(), exec); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	done := make(chan RuntimeResult, 1)
	go func() {
		rr, _ := r.Wait(context.Background(), exec)
		done <- rr
	}()
	select {
	case rr := <-done:
		if rr.ExitErr == nil {
			t.Fatal("expected non-nil ExitErr after Stop")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Wait did not return after Stop")
	}
}