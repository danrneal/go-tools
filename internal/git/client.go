package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// Client provides a cohesive interface for executing and parsing Git commands.
type Client struct {
	dir string
	run func(ctx context.Context, arg ...string) ([]byte, error)
}

// NewClient initializes a new Git Client. It optionally accepts a single directory
// string to execute all underlying Git commands within.
func NewClient(dir ...string) (*Client, error) {
	if len(dir) > 1 {
		return nil, errors.New("NewClient accepts a maximum of one directory string")
	}

	repoDir := ""
	if len(dir) == 1 {
		repoDir = dir[0]
	}

	run := func(ctx context.Context, args ...string) ([]byte, error) {
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Dir = repoDir
		out, err := cmd.CombinedOutput()
		if err != nil {
			return out, fmt.Errorf("git command failed: %w", err)
		}

		return out, nil
	}

	client := &Client{
		dir: repoDir,
		run: run,
	}

	return client, nil
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

// Head executes `git rev-parse HEAD` and returns the full commit hash
// of the current HEAD.
func (c *Client) Head(ctx context.Context) (string, error) {
	out, err := c.run(ctx, "rev-parse", "HEAD")
	if err != nil {
		return "", fmt.Errorf("failed to run git rev-parse HEAD: %w", err)
	}

	head := strings.TrimSpace(string(out))

	return head, nil
}

// status executes `git status -z` and returns the raw null-terminated output string.
func (c *Client) status(ctx context.Context) (string, error) {
	out, err := c.run(ctx, "status", "-z")
	if err != nil {
		return "", fmt.Errorf("failed to run git status: %w", err)
	}

	status := string(out)

	return status, nil
}
