package git

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// CreateWorktree creates a temporary detached git worktree at the specified commit.
// It returns the path to the worktree, a cleanup function to remove it, and an error if it fails.
func (c *Client) CreateWorktree(ctx context.Context, commit string) (string, func(), error) {
	worktree, err := os.MkdirTemp("", "worktree-*")
	if err != nil {
		return "", nil, fmt.Errorf("failed to create temp dir for worktree: %w", err)
	}

	if err := c.addWorktree(ctx, worktree, commit); err != nil {
		_ = os.RemoveAll(worktree)
		return worktree, nil, err
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
	_, err := c.run(ctx, "worktree", "add", "--detach", worktree, commit)
	return err
}

// removeWorktree executes the git command to forcefully remove a worktree.
func (c *Client) removeWorktree(ctx context.Context, worktree string) {
	_, _ = c.run(ctx, "worktree", "remove", "--force", worktree)
}

// SyncDirtyFiles reads the current git status and synchronizes the dirty working
// directory state into the provided worktree by copying, renaming, or deleting files.
func (c *Client) SyncDirtyFiles(ctx context.Context, worktree string) error {
	status, err := c.status(ctx)
	if err != nil {
		return err
	}

	fields := strings.Split(status, "\x00")
	for i := 0; i < len(fields)-1; i++ {
		field := fields[i]
		status, filename := field[:2], field[3:]
		filePath := filepath.Join(worktree, filename)

		switch {
		case strings.Contains(status, "D"):
			if err = os.Remove(filePath); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return fmt.Errorf("failed to delete %s: %w", filePath, err)
			}
		case strings.Contains(status, "R"):
			oldFilePath := filepath.Join(worktree, fields[i+1])
			if err = os.Remove(oldFilePath); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return fmt.Errorf("failed to delete %s: %w", filePath, err)
			}

			fallthrough
		case strings.Contains(status, "C"):
			i++
			fallthrough
		default:
			if err := os.MkdirAll(filepath.Dir(filePath), 0o750); err != nil {
				return fmt.Errorf("failed to create directory for %s: %w", filePath, err)
			}

			repoPath := filepath.Join(c.dir, filename)
			if err := copyFile(repoPath, filePath); err != nil {
				return fmt.Errorf("failed to copy %s: %w", repoPath, err)
			}
		}
	}

	return nil
}

// copyFile is a helper utility that copies a file from src to dst.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open src: %w", err)
	}

	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("failed to create dst: %w", err)
	}

	defer out.Close()

	if _, err = io.Copy(out, in); err != nil {
		return fmt.Errorf("failed to copy data: %w", err)
	}

	if err = out.Sync(); err != nil {
		return fmt.Errorf("failed to sync file: %w", err)
	}

	return nil
}
