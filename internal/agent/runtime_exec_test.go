package agent

import (
	"context"
	"testing"
	"time"
)

// TestExecRuntimeSpawnWait drives ExecRuntime through a real os/exec child
// (`/bin/sh -c "echo hello"`), asserting Spawn returns a durable ID, Wait
// captures stdout, and the handle is released after Wait.
func TestExecRuntimeSpawnWait(t *testing.T) {
	r := NewExecRuntime()
	exec, err := r.Spawn(context.Background(), Invocation{
		Bin:  "/bin/sh",
		Args: []string{"-c", "echo hello"},
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	if exec.ID == "" {
		t.Fatal("Execution.ID is empty")
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

	// Second Wait should report ErrNotFound (handle removed).
	if _, err := r.Wait(context.Background(), exec); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound on second Wait, got %v", err)
	}
}

// TestExecRuntimeStop verifies Stop terminates a long-running child and
// causes Wait to return with a non-nil ExitErr.
func TestExecRuntimeStop(t *testing.T) {
	r := NewExecRuntime()
	exec, err := r.Spawn(context.Background(), Invocation{
		Bin:  "/bin/sh",
		Args: []string{"-c", "sleep 10"},
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

// TestExecRuntimeSpawnRejectsEmpty verifies Spawn rejects an empty binary.
func TestExecRuntimeSpawnRejectsEmpty(t *testing.T) {
	r := NewExecRuntime()
	if _, err := r.Spawn(context.Background(), Invocation{}); err == nil {
		t.Fatal("expected error for empty Bin")
	}
}