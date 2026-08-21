// Package taskstoretest provides a deterministic in-memory TaskStore
// implementation intended for tests in any package that depends on the
// taskstore boundary.
package taskstoretest

import (
	"context"
	"fmt"
	"sync"
	"time"

	"bdtui/internal/taskstore"
)

// Fake is an in-memory TaskStore. It is safe for concurrent use; the
// typical test lifecycle is single-threaded, but the controller tests
// exercise the TaskStore from multiple goroutines and the fake must not
// race.
type Fake struct {
	mu      sync.Mutex
	now     func() time.Time
	tasks   map[string]*taskstore.Task
	updates []RecordedUpdate
}

// RecordedUpdate captures one Successive write through Claim or SyncTerminal
// for later assertions.
type RecordedUpdate struct {
	TaskID  string
	Outcome taskstore.RunOutcome // empty for Claim
	Title   string               // copied from the snapshot at update time
}

// New returns a fresh Fake with no tasks.
func New() *Fake {
	return &Fake{
		now:   func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) },
		tasks: make(map[string]*taskstore.Task),
	}
}

// Put inserts (or replaces) a task. Useful for seeding test fixtures.
func (f *Fake) Put(t *taskstore.Task) *Fake {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := t.Clone()
	if cp.SnapshotAt.IsZero() {
		cp.SnapshotAt = f.now()
	}
	f.tasks[cp.ID] = cp
	return f
}

// Seed is shorthand for Put with a task that only has the title and
// status set; everything else falls back to safe defaults.
func (f *Fake) Seed(id, title string, status taskstore.TaskStatus) *Fake {
	return f.Put(&taskstore.Task{
		ID:     id,
		Title:  title,
		Status: status,
	})
}

// Updates returns a copy of the recorded Claim / SyncTerminal calls in
// invocation order.
func (f *Fake) Updates() []RecordedUpdate {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]RecordedUpdate, len(f.updates))
	copy(out, f.updates)
	return out
}

// Get implements TaskStore.
func (f *Fake) Get(ctx context.Context, id string) (*taskstore.Task, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, ok := f.tasks[id]
	if !ok {
		return nil, fmt.Errorf("%w: %s", taskstore.ErrTaskNotFound, id)
	}
	return t.Clone(), nil
}

// Claim implements TaskStore. The fake is "atomic" because the goroutine
// holds the mutex for the entire operation.
func (f *Fake) Claim(ctx context.Context, id string) (*taskstore.Task, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, ok := f.tasks[id]
	if !ok {
		return nil, fmt.Errorf("%w: %s", taskstore.ErrTaskNotFound, id)
	}
	if t.Status == taskstore.TaskInProgress {
		return nil, fmt.Errorf("%w: %s", taskstore.ErrTaskAlreadyClaimed, id)
	}
	t.Status = taskstore.TaskInProgress
	t.SnapshotAt = f.now()
	f.tasks[id] = t.Clone()
	f.updates = append(f.updates, RecordedUpdate{
		TaskID: id,
		Title:  t.Title,
	})
	return t.Clone(), nil
}

// SyncTerminal implements TaskStore.
func (f *Fake) SyncTerminal(ctx context.Context, id string, outcome taskstore.RunOutcome) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, ok := f.tasks[id]
	if !ok {
		return fmt.Errorf("%w: %s", taskstore.ErrTaskNotFound, id)
	}
	status, err := taskstore.MapRunOutcomeToTaskStatus(outcome)
	if err != nil {
		return err
	}
	t.Status = status
	t.SnapshotAt = f.now()
	f.tasks[id] = t.Clone()
	f.updates = append(f.updates, RecordedUpdate{
		TaskID:  id,
		Outcome: outcome,
		Title:   t.Title,
	})
	return nil
}

// Ensure the fake satisfies the interface at compile time.
var _ taskstore.TaskStore = (*Fake)(nil)
