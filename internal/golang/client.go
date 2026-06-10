package golang

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Client provides a mockable interface for executing Go toolchain commands.
type Client struct {
	dir string
	run func(ctx context.Context, args ...string) ([]byte, error)
}

// NewClient initializes a new Go Client. It optionally accepts a single directory
// string to execute all underlying Go commands within.
func NewClient(dir ...string) (*Client, error) {
	if len(dir) > 1 {
		return nil, errors.New("NewClient accepts a maximum of one directory string")
	}

	workDir := ""
	if len(dir) > 0 {
		workDir = dir[0]
	}

	run := func(ctx context.Context, args ...string) ([]byte, error) {
		cmd := exec.CommandContext(ctx, "go", args...)
		cmd.Dir = workDir
		out, err := cmd.CombinedOutput()
		if err != nil {
			return out, fmt.Errorf("go command failed: %w", err)
		}

		return out, nil
	}

	client := &Client{
		dir: workDir,
		run: run,
	}

	return client, nil
}

// ModulePath executes `go list -m` and returns the current module path.
func (c *Client) ModulePath(ctx context.Context) (string, error) {
	out, err := c.run(ctx, "list", "-m")
	if err != nil {
		return "", fmt.Errorf("failed to get module path: %w", err)
	}

	modulePath := strings.TrimSpace(string(out))

	return modulePath, nil
}

// GenerateCoverProfile executes the test suite to generate a coverage profile.
// It returns the path to the profile, or an error only if the profile fails to generate, safely ignoring
// standard test failures.
func (c *Client) GenerateCoverProfile(ctx context.Context) (string, error) {
	out, testErr := c.run(ctx, "test", "-coverprofile=coverage.out", "./...")

	coverProfile := filepath.Join(c.dir, "coverage.out")
	stat, err := os.Stat(coverProfile)
	if err != nil || stat.Size() == 0 {
		return "", fmt.Errorf(
			"failed to generate coverage profile\nTest Exit Status: %w\nTest Output:\n%s",
			testErr,
			string(out),
		)
	}

	return coverProfile, nil
}
