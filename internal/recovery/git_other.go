//go:build !unix

package recovery

import "context"

// GitWorktree is a non-functional stub on non-Unix platforms. The build
// constraint intentionally does not enumerate every GOOS; the production
// Unix implementation lives in git_unix.go. Callers should treat empty
// results as "git not available" rather than crash on Windows builds that
// lack the Git CLI or runtime integration.
type GitWorktree struct{}

// NewGitWorktree returns a stub Worktree on non-Unix platforms.
func NewGitWorktree() *GitWorktree { return &GitWorktree{} }

// ResolveHead always reports ErrCheckpointNotGitRepo.
func (g *GitWorktree) ResolveHead(ctx context.Context, workdir string) (string, error) {
	return "", ErrCheckpointNotGitRepo
}

// IsDirty always reports ErrCheckpointNotGitRepo.
func (g *GitWorktree) IsDirty(ctx context.Context, workdir string) error {
	return ErrCheckpointNotGitRepo
}

// DiffEmpty always returns (true, ErrCheckpointNotGitRepo) so the caller
// treats the worktree as a no-op checkpoint.
func (g *GitWorktree) DiffEmpty(ctx context.Context, workdir string) (bool, error) {
	return true, ErrCheckpointNotGitRepo
}

// Commit always returns ErrCheckpointNotGitRepo.
func (g *GitWorktree) Commit(ctx context.Context, workdir, subject, body string) (string, error) {
	return "", ErrCheckpointNotGitRepo
}
