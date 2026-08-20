package recovery

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// WriteCheckpoint records a checkpoint commit for a successful writer step
// and returns the resulting Checkpoint. The caller is expected to persist
// CommitSHA (or, for no-op, BeforeSHA) as execution.result_commit.
//
// Behaviour:
//
//   - If the diff against HEAD is empty, ErrCheckpointNoOp is returned
//     alongside a Checkpoint with DiffEmpty=true and CommitSHA=BeforeSHA.
//     The caller records BeforeSHA as result_commit and does not create an
//     empty commit. DiffEmpty on a fresh worktree (no prior HEAD) is also
//     treated as a no-op.
//   - If the diff is non-empty, a checkpoint commit is created with the
//     given subject/body; the resulting SHA is returned in CommitSHA.
//   - If the worktree is not a Git repository, ErrCheckpointNotGitRepo is
//     returned and no commit is created.
//
// The workdir must be the writer's Git worktree. Branching (creating a
// per-step branch) is the controller's responsibility; recovery only commits
// into the current branch. Pre-checkpoint dirtiness (unrelated local changes
// vs HEAD) must be enforced by the controller via Worktree.IsDirty *before*
// staging the writer's changes; recovery does not recheck here because the
// staged/unstaged diff is exactly what the commit will capture.
func WriteCheckpoint(ctx context.Context, g Worktree, workdir, runID, stepID, subject, body string) (Checkpoint, error) {
	if g == nil {
		return Checkpoint{}, errors.New("recovery: Worktree is required")
	}
	if strings.TrimSpace(workdir) == "" {
		return Checkpoint{}, errors.New("recovery: workdir is required")
	}
	if strings.TrimSpace(subject) == "" {
		return Checkpoint{}, errors.New("recovery: subject is required")
	}

	cp := Checkpoint{
		Worktree: workdir,
		RunID:    runID,
		StepID:   stepID,
		Summary:  subject,
		Body:     body,
	}

	before, err := g.ResolveHead(ctx, workdir)
	switch {
	case errors.Is(err, ErrCheckpointNotGitRepo):
		return cp, err
	case err != nil:
		return cp, fmt.Errorf("recovery: resolve HEAD: %w", err)
	}
	cp.BeforeSHA = before

	empty, err := g.DiffEmpty(ctx, workdir)
	switch {
	case errors.Is(err, ErrCheckpointNotGitRepo):
		return cp, err
	case err != nil:
		return cp, fmt.Errorf("recovery: check diff: %w", err)
	}
	if empty {
		cp.DiffEmpty = true
		cp.CommitSHA = before
		return cp, ErrCheckpointNoOp
	}

	sha, err := g.Commit(ctx, workdir, subject, body)
	if err != nil {
		return cp, fmt.Errorf("recovery: commit: %w", err)
	}
	cp.CommitSHA = sha
	return cp, nil
}
