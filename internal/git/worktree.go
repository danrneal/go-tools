package git

import (
	"context"
	"fmt"
	"os"
	"os/exec"
)

func CreateDetachedWorktree(ctx context.Context, repo, commit string) (string, func(), error) {
	worktree, err := os.MkdirTemp("", "worktree-*")
	if err != nil {
		return "", nil, fmt.Errorf("failed to create temp dir for worktree: %w", err)
	}

	cmd := exec.CommandContext(ctx, "git", "worktree", "add", "--detach", worktree, commit)
	cmd.Dir = repo
	if out, err := cmd.CombinedOutput(); err != nil {
		_ = os.RemoveAll(worktree)
		return "", nil, fmt.Errorf("failed to create git worktree: %w\nOutput: %s", err, string(out))
	}

	cleanup := func() {
		cleanupCtx := context.WithoutCancel(ctx)
		cmd := exec.CommandContext(cleanupCtx, "git", "worktree", "remove", "--force", worktree)
		cmd.Dir = repo
		_ = cmd.Run()
		_ = os.RemoveAll(worktree)
	}

	return worktree, cleanup, nil
}
