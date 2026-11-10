package git

import (
	"context"
	"fmt"
	"os"
)

// CreateWorktree creates a temporary detached git worktree at the specified commit.
// It returns the path to the worktree, a cleanup function to remove it, and an error if it fails.
func (c *Client) CreateWorktree(ctx context.Context, commit string) (string, func(), error) {
	worktree, err := os.MkdirTemp(c.worktreeBaseDir, "worktree-*")
	if err != nil {
		return "", nil, fmt.Errorf("failed to create temp dir for worktree: %w", err)
	}

	if err := c.addWorktree(ctx, worktree, commit); err != nil {
		_ = os.RemoveAll(worktree)
		return "", nil, err
	}

	cleanup := func() {
		cleanupCtx := context.WithoutCancel(ctx)
		c.removeWorktree(cleanupCtx, worktree)
		_ = os.RemoveAll(worktree)
	}

	return worktree, cleanup, nil
}

// addWorktree executes the git command to add a new detached worktree.
func (c *Client) addWorktree(ctx context.Context, worktree, commit string) error {
	if _, err := c.run(ctx, "worktree", "add", "--detach", worktree, commit); err != nil {
		return fmt.Errorf("failed to run git worktree add: %w", err)
	}

	return nil
}

// removeWorktree executes the git command to forcefully remove a worktree.
func (c *Client) removeWorktree(ctx context.Context, worktree string) {
	//nolint:errcheck // best-effort cleanup
	_, _ = c.run(ctx, "worktree", "remove", "--force", worktree)
}

// pruneWorktrees executes git worktree prune to clean up any internal Git metadata
// that references directories that no longer exist on disk.
func (c *Client) pruneWorktrees(ctx context.Context) error {
	if _, err := c.run(ctx, "worktree", "prune"); err != nil {
		return fmt.Errorf("failed to run git worktree prune: %w", err)
	}

	return nil
}
