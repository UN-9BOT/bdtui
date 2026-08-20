//go:build unix

package recovery

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// GitWorktree implements Worktree using the git CLI on Unix. It assumes `git`
// is on PATH; callers (or main) should fail fast at startup if it is not.
type GitWorktree struct{}

// NewGitWorktree returns the production Worktree implementation.
func NewGitWorktree() *GitWorktree { return &GitWorktree{} }

// ResolveHead returns the current HEAD commit SHA. For an empty repository
// (no commits yet) it returns ("", nil) so the caller can treat it as a
// fresh worktree; the first checkpoint will create the initial commit.
func (g *GitWorktree) ResolveHead(ctx context.Context, workdir string) (string, error) {
	if workdir == "" {
		return "", ErrCheckpointNotGitRepo
	}
	out, stderr, err := runGitRaw(ctx, workdir, "rev-parse", "--verify", "HEAD")
	if err != nil {
		stderr = strings.TrimSpace(stderr)
		switch {
		case strings.Contains(stderr, "unknown revision"),
			strings.Contains(stderr, "ambiguous argument 'HEAD'"),
			strings.Contains(stderr, "Needed a single revision"),
			strings.Contains(stderr, "fatal: ambiguous argument"):
			return "", nil
		case stderr == "":
			return "", ErrCheckpointNotGitRepo
		default:
			return "", fmt.Errorf("%w: %s", ErrCheckpointNotGitRepo, stderr)
		}
	}
	return strings.TrimSpace(out), nil
}

// IsDirty returns ErrCheckpointDirty if the working tree has staged or
// unstaged changes vs HEAD. Untracked files are ignored — checkpoint
// metadata files written by the agent must not block the commit.
func (g *GitWorktree) IsDirty(ctx context.Context, workdir string) error {
	if workdir == "" {
		return ErrCheckpointNotGitRepo
	}
	out, stderr, err := runGitRaw(ctx, workdir, "status", "--porcelain", "--untracked-files=no")
	if err != nil {
		if strings.TrimSpace(stderr) == "" {
			return ErrCheckpointNotGitRepo
		}
		return err
	}
	if strings.TrimSpace(out) != "" {
		return ErrCheckpointDirty
	}
	return nil
}

// DiffEmpty reports whether the diff against HEAD (staged + unstaged) is
// empty. For an empty repository it returns (true, nil) because there is no
// prior HEAD to compare against.
func (g *GitWorktree) DiffEmpty(ctx context.Context, workdir string) (bool, error) {
	if workdir == "" {
		return true, ErrCheckpointNotGitRepo
	}
	// We need both unstaged (`git diff HEAD`) and staged (`git diff --cached HEAD`)
	// comparisons because `git diff` alone misses staged changes after `git add`.
	unstagedEmpty, err := diffExitClean(ctx, workdir, "diff", "HEAD")
	if err != nil {
		return false, err
	}
	stagedEmpty, err := diffExitClean(ctx, workdir, "diff", "--cached", "HEAD")
	if err != nil {
		return false, err
	}
	return unstagedEmpty && stagedEmpty, nil
}

// diffExitClean returns true when `git diff --exit-code` exits 0 (no diff).
// --exit-code guarantees 0 when the working tree matches HEAD, 1 when there
// is any difference, and propagates stderr for actual git errors.
func diffExitClean(ctx context.Context, workdir string, args ...string) (bool, error) {
	full := append(append([]string{}, args...), "--exit-code")
	_, _, err := runGitRaw(ctx, workdir, full...)
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			if len(ee.Stderr) == 0 {
				return false, nil
			}
			return false, fmt.Errorf("git diff: %s", strings.TrimSpace(string(ee.Stderr)))
		}
		return false, err
	}
	return true, nil
}

// Commit creates a checkpoint commit with the given subject and body, then
// returns the resulting commit SHA. The caller must have already verified
// the tree is clean (IsDirty) and that the diff is not empty (DiffEmpty)
// unless intentionally recording an empty commit.
func (g *GitWorktree) Commit(ctx context.Context, workdir, subject, body string) (string, error) {
	if workdir == "" {
		return "", ErrCheckpointNotGitRepo
	}
	if strings.TrimSpace(subject) == "" {
		return "", errors.New("recovery: checkpoint subject is required")
	}
	fullMsg := subject
	if strings.TrimSpace(body) != "" {
		fullMsg = subject + "\n\n" + body
	}
	if _, stderr, err := runGitRaw(ctx, workdir, "commit", "--allow-empty", "-m", fullMsg); err != nil {
		if strings.TrimSpace(stderr) == "" {
			return "", fmt.Errorf("%w: commit failed", ErrCheckpointNotGitRepo)
		}
		return "", err
	}
	return g.ResolveHead(ctx, workdir)
}

func runGitRaw(ctx context.Context, dir string, args ...string) (string, string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}
