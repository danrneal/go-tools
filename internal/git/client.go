package git

import (
	"context"
	"errors"
	"fmt"
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

// Head executes `git rev-parse HEAD` and returns the full commit hash
// of the current HEAD.
func (c *Client) Head(ctx context.Context) (string, error) {
	out, err := c.run(ctx, "rev-parse", "HEAD")
	head := strings.TrimSpace(string(out))

	return head, err
}

// status executes `git status -z` and returns the raw null-terminated output string.
func (c *Client) status(ctx context.Context) (string, error) {
	out, err := c.run(ctx, "status", "-z")
	status := string(out)

	return status, err
}
