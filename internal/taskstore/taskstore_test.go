package taskstore_test

import (
	"context"
	"errors"
	"testing"

	"bdtui/internal/taskstore"
	"bdtui/internal/taskstore/taskstoretest"
)

func TestMapRunOutcomeToTaskStatus(t *testing.T) {
	cases := []struct {
		outcome taskstore.RunOutcome
		want    taskstore.TaskStatus
		wantErr bool
	}{
		{taskstore.RunCompleted, taskstore.TaskDone, false},
		{taskstore.RunFailed, taskstore.TaskBlocked, false},
		{taskstore.RunNeedsAttention, taskstore.TaskBlocked, false},
		{taskstore.RunCancelled, taskstore.TaskTodo, false},
		{taskstore.RunOutcome("nonsense"), "", true},
	}
	for _, c := range cases {
		t.Run(string(c.outcome), func(t *testing.T) {
			got, err := taskstore.MapRunOutcomeToTaskStatus(c.outcome)
			if (err != nil) != c.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, c.wantErr)
			}
			if got != c.want {
				t.Fatalf("status = %q, want %q", got, c.want)
			}
		})
	}
}

func TestTaskStatusValid(t *testing.T) {
	for _, s := range []taskstore.TaskStatus{
		taskstore.TaskTodo, taskstore.TaskInProgress,
		taskstore.TaskDone, taskstore.TaskBlocked,
	} {
		if !s.Valid() {
			t.Errorf("status %q should be valid", s)
		}
	}
	if taskstore.TaskStatus("open").Valid() {
		t.Errorf("Beads raw status %q should map out before reaching Valid()", "open")
	}
}

func TestRunOutcomeValid(t *testing.T) {
	for _, o := range []taskstore.RunOutcome{
		taskstore.RunCompleted, taskstore.RunFailed,
		taskstore.RunNeedsAttention, taskstore.RunCancelled,
	} {
		if !o.Valid() {
			t.Errorf("outcome %q should be valid", o)
		}
	}
	if taskstore.RunOutcome("stuck").Valid() {
		t.Errorf("outcome %q should NOT be valid", "stuck")
	}
}

func TestFakeGetMissing(t *testing.T) {
	f := taskstoretest.New()
	_, err := f.Get(context.Background(), "abc")
	if !errors.Is(err, taskstore.ErrTaskNotFound) {
		t.Fatalf("err = %v, want ErrTaskNotFound", err)
	}
}

func TestFakeClaimTransition(t *testing.T) {
	f := taskstoretest.New().Seed("t1", "first", taskstore.TaskTodo)
	ctx := context.Background()

	snap, err := f.Claim(ctx, "t1")
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if snap.Status != taskstore.TaskInProgress {
		t.Errorf("snapshot status = %q, want in_progress", snap.Status)
	}
	if snap.Title != "first" {
		t.Errorf("snapshot title = %q, want first", snap.Title)
	}

	// Second claim must report already claimed.
	_, err = f.Claim(ctx, "t1")
	if !errors.Is(err, taskstore.ErrTaskAlreadyClaimed) {
		t.Fatalf("second Claim: err = %v, want ErrTaskAlreadyClaimed", err)
	}
}

func TestFakeClaimMissing(t *testing.T) {
	f := taskstoretest.New()
	_, err := f.Claim(context.Background(), "missing")
	if !errors.Is(err, taskstore.ErrTaskNotFound) {
		t.Fatalf("err = %v, want ErrTaskNotFound", err)
	}
}

func TestFakeSyncTerminal(t *testing.T) {
	f := taskstoretest.New().Seed("t1", "first", taskstore.TaskInProgress)
	ctx := context.Background()

	if err := f.SyncTerminal(ctx, "t1", taskstore.RunCompleted, 1); err != nil {
		t.Fatalf("SyncTerminal(completed): %v", err)
	}
	if err := f.SyncTerminal(ctx, "t1", taskstore.RunFailed, 2); err != nil {
		t.Fatalf("SyncTerminal(failed): %v", err)
	}
	if err := f.SyncTerminal(ctx, "t1", taskstore.RunNeedsAttention, 3); err != nil {
		t.Fatalf("SyncTerminal(needs_attention): %v", err)
	}
	if err := f.SyncTerminal(ctx, "t1", taskstore.RunCancelled, 4); err != nil {
		t.Fatalf("SyncTerminal(cancelled): %v", err)
	}

	updates := f.Updates()
	if len(updates) != 4 {
		t.Fatalf("len(updates) = %d, want 4", len(updates))
	}
	// Last update after the full cycle should be Todo.
	snap, err := f.Get(ctx, "t1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if snap.Status != taskstore.TaskTodo {
		t.Fatalf("final status = %q, want todo", snap.Status)
	}
	// Update sequence should match the four outcomes in order.
	want := []taskstore.RunOutcome{
		taskstore.RunCompleted, taskstore.RunFailed,
		taskstore.RunNeedsAttention, taskstore.RunCancelled,
	}
	for i, w := range want {
		if updates[i].Outcome != w {
			t.Errorf("updates[%d].Outcome = %q, want %q", i, updates[i].Outcome, w)
		}
	}
}

func TestFakeSyncTerminalInvalid(t *testing.T) {
	f := taskstoretest.New().Seed("t1", "first", taskstore.TaskInProgress)
	err := f.SyncTerminal(context.Background(), "t1", taskstore.RunOutcome("stuck"), 1)
	if !errors.Is(err, taskstore.ErrInvalidOutcome) {
		t.Fatalf("err = %v, want ErrInvalidOutcome", err)
	}
}

func TestFakeSyncTerminalMissing(t *testing.T) {
	f := taskstoretest.New()
	err := f.SyncTerminal(context.Background(), "missing", taskstore.RunCompleted, 1)
	if !errors.Is(err, taskstore.ErrTaskNotFound) {
		t.Fatalf("err = %v, want ErrTaskNotFound", err)
	}
}

func TestTaskClone(t *testing.T) {
	orig := &taskstore.Task{
		ID:    "x",
		Title: "y",
	}
	cp := orig.Clone()
	if cp == orig {
		t.Fatalf("Clone returned same pointer")
	}
	cp.Title = "z"
	if orig.Title != "y" {
		t.Errorf("Clone did not deep copy: orig.Title mutated to %q", orig.Title)
	}
	if (*taskstore.Task)(nil).Clone() != nil {
		t.Errorf("Clone of nil should be nil")
	}
}
