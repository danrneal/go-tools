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

// ErrNoCoverage is returned when the go test command completes but fails to generate
// a coverage profile, typically because there were no testable packages found.
var ErrNoCoverage = errors.New("no coverage profile generated")

// Client provides a mockable interface for executing Go toolchain commands.
type Client struct {
	dir string
	run func(ctx context.Context, args ...string) ([]byte, error)
}

// NewClient initializes a new Go Client, applying any provided configuration Options.
func NewClient(opts ...Option) (*Client, error) {
	client := &Client{}

	for _, opt := range opts {
		opt(client)
	}

	run := func(ctx context.Context, args ...string) ([]byte, error) {
		cmd := exec.CommandContext(ctx, "go", args...)
		cmd.Dir = client.dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			return out, fmt.Errorf("go command failed: %w", err)
		}

		return out, nil
	}

	client.run = run

	return client, nil
}

// Option defines a functional configuration parameter for the Go Client.
type Option func(*Client)

// WithDir sets the working directory where the Go toolchain commands will be executed.
// If not provided, commands run in the current working directory.
func WithDir(dir string) Option {
	setDir := func(c *Client) {
		c.dir = dir
	}

	return setDir
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
		return "", fmt.Errorf("%w\nTest Exit Status: %w\nTest Output:\n%s", ErrNoCoverage, testErr, string(out))
	}

	return coverProfile, nil
}
