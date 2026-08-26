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
	dir             string
	worktreeBaseDir string
	run             func(ctx context.Context, args ...string) ([]byte, error)
}

// NewClient initializes a new Git Client, applying any provided configuration Options.
func NewClient(ctx context.Context, worktreeBaseDir string, opts ...Option) (*Client, error) {
	client := &Client{
		worktreeBaseDir: worktreeBaseDir,
	}

	for _, opt := range opts {
		opt(client)
	}

	if err := os.RemoveAll(client.worktreeBaseDir); err != nil {
		return nil, fmt.Errorf("failed to clean worktree base dir: %w", err)
	}

	if err := os.MkdirAll(client.worktreeBaseDir, 0o750); err != nil {
		return nil, fmt.Errorf("failed to create worktree base dir: %w", err)
	}

	if client.run == nil {
		run := func(ctx context.Context, args ...string) ([]byte, error) {
			cmd := exec.CommandContext(ctx, "git", args...)
			cmd.Dir = client.dir
			out, err := cmd.CombinedOutput()
			if err != nil {
				return out, fmt.Errorf("git command failed: %w", err)
			}

			return out, nil
		}

		client.run = run
	}

	if err := client.pruneWorktrees(ctx); err != nil {
		return nil, err
	}

	return client, nil
}

// Option defines a functional configuration parameter for the Git Client.
type Option func(*Client)

// WithDir sets the working directory where the Git commands will be executed.
// If not provided, commands run in the current working directory.
func WithDir(dir string) Option {
	setDir := func(c *Client) {
		c.dir = dir
	}

	return setDir
}

// withRun allows tests to inject a mock execution function, bypassing the real os/exec call.
// It is unexported to prevent external callers from manipulating the internal command runner.
func withRun(run func(ctx context.Context, args ...string) ([]byte, error)) Option {
	setRun := func(c *Client) {
		c.run = run
	}

	return setRun
}

// AddAll executes `git add -A` to stage all changes (including untracked files)
// within the specified directory.
func (c *Client) AddAll(ctx context.Context, dir string) error {
	if _, err := c.run(ctx, "-C", dir, "add", "-A"); err != nil {
		return fmt.Errorf("failed to add all files in %s: %w", dir, err)
	}

	return nil
}

// LastCommit executes `git log -1 --format=%H -- <relPath>` and returns the
// hash of the commit that most recently modified the specified file.
func (c *Client) LastCommit(ctx context.Context, relPath string) (string, error) {
	out, err := c.run(ctx, "log", "-1", "--format=%H", "--", relPath)
	if err != nil {
		return "", fmt.Errorf("failed to run git log: %w", err)
	}

	commitHash := strings.TrimSpace(string(out))

	return commitHash, nil
}

// Show executes `git show` for a specific commit and file path, returning the output as an [io.Reader].
func (c *Client) Show(ctx context.Context, commit, relPath string) (io.Reader, error) {
	out, err := c.run(ctx, "show", fmt.Sprintf("%s:%s", commit, relPath))
	if err != nil {
		return nil, fmt.Errorf("failed to run git show: %w", err)
	}

	reader := bytes.NewReader(out)

	return reader, nil
}

// status executes `git status -z` and returns the raw null-terminated output string.
func (c *Client) status(ctx context.Context) (string, error) {
	out, err := c.run(ctx, "status", "-z", "--untracked-files=all")
	if err != nil {
		return "", fmt.Errorf("failed to run git status: %w", err)
	}

	status := string(out)

	return status, nil
}
