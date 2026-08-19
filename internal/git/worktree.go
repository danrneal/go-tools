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

// worktreeNamespace defines the deterministic directory name used within the
// system's temporary directory to isolate and manage temporary git worktrees.
const worktreeNamespace = "go-tools-worktrees"

// CreateWorktree creates a temporary detached git worktree at the specified commit.
// It returns the path to the worktree, a cleanup function to remove it, and an error if it fails.
func (c *Client) CreateWorktree(ctx context.Context, commit string) (string, func(), error) {
	worktree, err := os.MkdirTemp(c.worktreeBaseDir, "worktree-*")
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
	if _, err := c.run(ctx, "worktree", "add", "--detach", worktree, commit); err != nil {
		return fmt.Errorf("failed to run git worktree add: %w", err)
	}

	return nil
}

// removeWorktree executes the git command to forcefully remove a worktree.
func (c *Client) removeWorktree(ctx context.Context, worktree string) {
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

		indexStatus, worktreeStatus, relPath := field[0], field[1], field[3:]
		filePath := filepath.Join(worktree, relPath)

		hasStatus := func(s byte) bool {
			return indexStatus == s || worktreeStatus == s
		}

		switch {
		case hasStatus('D'):
			if err = os.Remove(filePath); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return fmt.Errorf("failed to delete %s: %w", filePath, err)
			}
		case hasStatus('R'):
			oldFilePath := filepath.Join(worktree, fields[i+1])
			if err = os.Remove(oldFilePath); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return fmt.Errorf("failed to delete %s: %w", filePath, err)
			}

			fallthrough
		case hasStatus('C'):
			i++
			fallthrough
		default:
			if err := os.MkdirAll(filepath.Dir(filePath), 0o750); err != nil {
				return fmt.Errorf("failed to create directory for %s: %w", filePath, err)
			}

			if err := c.copyToWorktree(relPath, worktree); err != nil {
				return err
			}
		}
	}

	return nil
}

// CopyFromWorktree copies a file from the provided worktree back into the client's working directory.
func (c *Client) CopyFromWorktree(relPath, worktree string) error {
	src := filepath.Join(worktree, relPath)
	dst := filepath.Join(c.dir, relPath)

	return copyFile(src, dst)
}

// copyToWorktree copies a file from the client's working directory into the specified worktree.
func (c *Client) copyToWorktree(relPath, worktree string) error {
	src := filepath.Join(c.dir, relPath)
	dst := filepath.Join(worktree, relPath)

	return copyFile(src, dst)
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
