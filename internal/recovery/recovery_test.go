package recovery

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"testing"

	"bdtui/internal/agent"
)

func runCmd(t *testing.T, dir, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %v: %v\n%s", name, args, err, string(out))
	}
}

func os_WriteFile(dir, name, content string) error {
	return os.WriteFile(dir+"/"+name, []byte(content), 0o644)
}

type fakeWorktree struct {
	resolve func(ctx context.Context, workdir string) (string, error)
	dirty   func(ctx context.Context, workdir string) error
	diffE   func(ctx context.Context, workdir string) (bool, error)
	commit  func(ctx context.Context, workdir, subject, body string) (string, error)
}

func (f *fakeWorktree) ResolveHead(ctx context.Context, w string) (string, error) {
	if f.resolve != nil {
		return f.resolve(ctx, w)
	}
	return "deadbeef", nil
}

func (f *fakeWorktree) IsDirty(ctx context.Context, w string) error {
	if f.dirty != nil {
		return f.dirty(ctx, w)
	}
	return nil
}

func (f *fakeWorktree) DiffEmpty(ctx context.Context, w string) (bool, error) {
	if f.diffE != nil {
		return f.diffE(ctx, w)
	}
	return true, nil
}

func (f *fakeWorktree) Commit(ctx context.Context, w, subject, body string) (string, error) {
	if f.commit != nil {
		return f.commit(ctx, w, subject, body)
	}
	return "", nil
}

type fakeInspect struct {
	live   func(ctx context.Context, executionID string) (LiveState, error)
	result LiveState
	err    error
}

func (f *fakeInspect) Live(ctx context.Context, _ string) (LiveState, error) {
	if f.live != nil {
		return f.live(ctx, "ignored")
	}
	return f.result, f.err
}

func newWriterExec() Execution {
	return Execution{
		ID:     "exec-1",
		RunID:  "run-1",
		StepID: "step-1",
		Status: "running",
		Kind:   KindWriter,
	}
}

func TestDecide_LiveRunning_Reattach(t *testing.T) {
	d := Decide(newWriterExec(), LiveRunning, nil)
	if d.Action != ActionReattach || d.Reason != "live_running" {
		t.Fatalf("unexpected decision: %+v", d)
	}
}

func TestDecide_LiveDone_NoOp(t *testing.T) {
	d := Decide(newWriterExec(), LiveDone, nil)
	if d.Action != ActionNone || d.Reason != "live_done" {
		t.Fatalf("unexpected decision: %+v", d)
	}
}

func TestDecide_LostWriter_NeedsAttention(t *testing.T) {
	d := Decide(newWriterExec(), LiveUnknown, agent.ErrLostExecution)
	if d.Action != ActionNeedsAttention || d.Reason != "lost" {
		t.Fatalf("unexpected decision: %+v", d)
	}
}

func TestDecide_LostReader_Retry(t *testing.T) {
	e := newWriterExec()
	e.Kind = KindReader
	d := Decide(e, LiveUnknown, agent.ErrLostExecution)
	if d.Action != ActionRetry || d.Reason != "lost" {
		t.Fatalf("unexpected decision: %+v", d)
	}
}

func TestDecide_AmbiguousWriter_NeedsAttention(t *testing.T) {
	d := Decide(newWriterExec(), LiveUnknown, errors.New("runtime probe error"))
	if d.Action != ActionNeedsAttention || d.Reason != "ambiguous" {
		t.Fatalf("unexpected decision: %+v", d)
	}
}

func TestDecide_AmbiguousReader_Retry(t *testing.T) {
	e := newWriterExec()
	e.Kind = KindReader
	d := Decide(e, LiveUnknown, errors.New("runtime probe error"))
	if d.Action != ActionRetry || d.Reason != "ambiguous" {
		t.Fatalf("unexpected decision: %+v", d)
	}
}

func TestCheckpoint_RequiresWorktree(t *testing.T) {
	if _, err := WriteCheckpoint(context.Background(), nil, "/tmp", "run", "step", "subject", ""); err == nil {
		t.Fatalf("expected error when worktree is nil")
	}
}

func TestCheckpoint_RejectsEmptySubject(t *testing.T) {
	g := &fakeWorktree{}
	_, err := WriteCheckpoint(context.Background(), g, "/tmp", "run", "step", "", "")
	if err == nil {
		t.Fatalf("expected error when subject is empty")
	}
}

func TestCheckpoint_RejectsEmptyWorkdir(t *testing.T) {
	g := &fakeWorktree{}
	_, err := WriteCheckpoint(context.Background(), g, "", "run", "step", "subject", "")
	if err == nil {
		t.Fatalf("expected error when workdir is empty")
	}
}

func TestCheckpoint_DiffEmpty_ReturnsNoOp(t *testing.T) {
	g := &fakeWorktree{
		resolve: func(ctx context.Context, _ string) (string, error) { return "before-sha", nil },
		diffE:   func(ctx context.Context, _ string) (bool, error) { return true, nil },
	}
	cp, err := WriteCheckpoint(context.Background(), g, "/tmp", "run-1", "step-1", "subject", "")
	if !errors.Is(err, ErrCheckpointNoOp) {
		t.Fatalf("expected ErrCheckpointNoOp, got %v", err)
	}
	if !cp.DiffEmpty || cp.CommitSHA != "before-sha" || cp.BeforeSHA != "before-sha" {
		t.Fatalf("unexpected checkpoint: %+v", cp)
	}
}

// TestCheckpoint_DirtyIsPrecheckedDirectly documents that pre-checkpoint
// dirty-state checks are the controller's responsibility and live on
// Worktree.IsDirty rather than inside WriteCheckpoint itself.
func TestCheckpoint_DirtyIsPrecheckedDirectly(t *testing.T) {
	g := &fakeWorktree{
		dirty: func(ctx context.Context, _ string) error { return ErrCheckpointDirty },
	}
	if err := g.IsDirty(context.Background(), "/tmp"); !errors.Is(err, ErrCheckpointDirty) {
		t.Fatalf("expected ErrCheckpointDirty from IsDirty, got %v", err)
	}
}

func TestCheckpoint_NonEmptyCommit(t *testing.T) {
	g := &fakeWorktree{
		resolve: func(ctx context.Context, _ string) (string, error) { return "before-sha", nil },
		diffE:   func(ctx context.Context, _ string) (bool, error) { return false, nil },
		commit:  func(ctx context.Context, _, subj, body string) (string, error) { return "after-sha", nil },
	}
	cp, err := WriteCheckpoint(context.Background(), g, "/tmp", "run-1", "step-1", "subject", "body")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cp.CommitSHA != "after-sha" || cp.BeforeSHA != "before-sha" || cp.DiffEmpty {
		t.Fatalf("unexpected checkpoint: %+v", cp)
	}
}

func TestCheckpoint_NotRepoIsHardError(t *testing.T) {
	g := &fakeWorktree{
		resolve: func(ctx context.Context, _ string) (string, error) { return "", ErrCheckpointNotGitRepo },
	}
	_, err := WriteCheckpoint(context.Background(), g, "/tmp", "run-1", "step-1", "subject", "")
	if !errors.Is(err, ErrCheckpointNotGitRepo) {
		t.Fatalf("expected ErrCheckpointNotGitRepo, got %v", err)
	}
}

func TestGitWorktree_EmptyRepo_ResolveHead(t *testing.T) {
	dir := t.TempDir()
	runCmd(t, dir, "git", "init", "-q")
	g := NewGitWorktree()
	sha, err := g.ResolveHead(context.Background(), dir)
	if err != nil {
		t.Fatalf("expected empty-repo ResolveHead to succeed with empty SHA, got %v", err)
	}
	if sha != "" {
		t.Fatalf("expected empty SHA, got %q", sha)
	}
}

func TestGitWorktree_NoOpCheckpoint(t *testing.T) {
	dir := t.TempDir()
	runCmd(t, dir, "git", "init", "-q")
	runCmd(t, dir, "git", "config", "user.email", "test@example.com")
	runCmd(t, dir, "git", "config", "user.name", "test")
	runCmd(t, dir, "git", "commit", "--allow-empty", "-q", "-m", "init")
	cp, err := WriteCheckpoint(context.Background(), NewGitWorktree(), dir, "run-1", "step-1", "subject", "")
	if !errors.Is(err, ErrCheckpointNoOp) {
		t.Fatalf("expected ErrCheckpointNoOp on clean tree, got %v", err)
	}
	if !cp.DiffEmpty || cp.CommitSHA == "" {
		t.Fatalf("unexpected checkpoint: %+v", cp)
	}
}

func TestGitWorktree_DirtyTreeBlocked(t *testing.T) {
	dir := t.TempDir()
	runCmd(t, dir, "git", "init", "-q")
	runCmd(t, dir, "git", "config", "user.email", "test@example.com")
	runCmd(t, dir, "git", "config", "user.name", "test")
	runCmd(t, dir, "git", "commit", "--allow-empty", "-q", "-m", "init")
	if err := os_WriteFile(dir, "b.txt", "uncommitted\n"); err != nil {
		t.Fatal(err)
	}
	runCmd(t, dir, "git", "add", "b.txt")
	if err := NewGitWorktree().IsDirty(context.Background(), dir); !errors.Is(err, ErrCheckpointDirty) {
		t.Fatalf("expected ErrCheckpointDirty, got %v", err)
	}
}

func TestGitWorktree_RealCheckpoint(t *testing.T) {
	dir := t.TempDir()
	runCmd(t, dir, "git", "init", "-q")
	// Make the test repository self-contained: commit identity is local
	// so the production Worktree.Commit can succeed without per-call
	// user.email/user.name flags.
	runCmd(t, dir, "git", "config", "user.email", "test@example.com")
	runCmd(t, dir, "git", "config", "user.name", "test")
	runCmd(t, dir, "git", "commit", "--allow-empty", "-q", "-m", "init")
	if err := os_WriteFile(dir, "a.txt", "first\n"); err != nil {
		t.Fatal(err)
	}
	runCmd(t, dir, "git", "add", "a.txt")
	runCmd(t, dir, "git", "commit", "-q", "-m", "seed")
	before, err := NewGitWorktree().ResolveHead(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os_WriteFile(dir, "a.txt", "second\n"); err != nil {
		t.Fatal(err)
	}
	runCmd(t, dir, "git", "add", "a.txt")
	cp, err := WriteCheckpoint(context.Background(), NewGitWorktree(), dir, "run-1", "step-1", "writer: step-1", "checkpoint body")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cp.DiffEmpty || cp.CommitSHA == before || cp.CommitSHA == "" {
		t.Fatalf("unexpected checkpoint: %+v (before=%s)", cp, before)
	}
}
