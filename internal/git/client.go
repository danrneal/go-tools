package git

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// Client provides a cohesive interface for executing and parsing Git commands.
type Client struct {
	worktreeBaseDir string
	run             func(ctx context.Context, args ...string) ([]byte, error)
}

// NewClient initializes a new Git Client, applying any provided configuration Options.
func NewClient(ctx context.Context, opts ...Option) (*Client, error) {
	client := &Client{}

	for _, opt := range opts {
		opt(client)
	}

	if client.run == nil {
		run := func(ctx context.Context, args ...string) ([]byte, error) {
			cmd := exec.CommandContext(ctx, "git", args...)
			out, err := cmd.CombinedOutput()
			if err != nil {
				return out, fmt.Errorf("git command failed: %w", err)
			}

			return out, nil
		}

		client.run = run
	}

	if client.worktreeBaseDir == "" {
		return client, nil
	}

	if err := os.RemoveAll(client.worktreeBaseDir); err != nil {
		return nil, fmt.Errorf("failed to clean worktree base dir: %w", err)
	}

	if err := os.MkdirAll(client.worktreeBaseDir, 0o750); err != nil {
		return nil, fmt.Errorf("failed to create worktree base dir: %w", err)
	}

	if err := client.pruneWorktrees(ctx); err != nil {
		return nil, err
	}

	return client, nil
}

// Option defines a functional configuration parameter for the Git Client.
type Option func(*Client)

// WithWorktreeBaseDir sets the base directory where temporary worktrees will be created.
func WithWorktreeBaseDir(dir string) Option {
	setWorktreeBaseDir := func(c *Client) {
		c.worktreeBaseDir = dir
	}

	return setWorktreeBaseDir
}

// withRun allows tests to inject a mock execution function, bypassing the real os/exec call.
// It is unexported to prevent external callers from manipulating the internal command runner.
func withRun(run func(ctx context.Context, args ...string) ([]byte, error)) Option {
	setRun := func(c *Client) {
		c.run = run
	}

	return setRun
}

// LastCommit executes `git log -1 --format=%H -- <path>` and returns the
// hash of the commit that most recently modified the specified file.
func (c *Client) LastCommit(ctx context.Context, path string) (string, error) {
	out, err := c.run(ctx, "log", "-1", "--format=%H", "--", path)
	if err != nil {
		return "", fmt.Errorf("failed to run git log: %w", err)
	}

	commitHash := strings.TrimSpace(string(out))

	return commitHash, nil
}

// Show executes `git show` for a specific commit and file path, returning the output as an [io.Reader].
func (c *Client) Show(ctx context.Context, commit, path string) (io.Reader, error) {
	out, err := c.run(ctx, "show", fmt.Sprintf("%s:%s", commit, path))
	if err != nil {
		return nil, fmt.Errorf("failed to run git show: %w", err)
	}

	reader := bytes.NewReader(out)

	return reader, nil
}
