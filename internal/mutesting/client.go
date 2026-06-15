package mutesting

import (
	"context"
	"errors"
	"fmt"
	"go/build"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

// Client provides a mockable interface for executing go-mutesting commands.
type Client struct {
	dir string
	run func(ctx context.Context, args ...string) ([]byte, error)
}

// NewClient initializes a new Mutant Client. It optionally accepts a single directory
// string to execute all underlying go-mutesting commands within.
func NewClient(dir ...string) (*Client, error) {
	if len(dir) > 1 {
		return nil, errors.New("NewClient accepts a maximum of one directory string")
	}

	workDir := ""
	if len(dir) > 0 {
		workDir = dir[0]
	}

	run := func(ctx context.Context, args ...string) ([]byte, error) {
		var exitErr *exec.ExitError

		cmd := exec.CommandContext(ctx, "go-mutesting", args...)
		if cmd.Err != nil && errors.Is(cmd.Err, exec.ErrNotFound) {
			if gopaths := build.Default.GOPATH; gopaths != "" {
				gopath := filepath.SplitList(gopaths)[0]
				mutestingPath := filepath.Join(gopath, "bin", "go-mutesting")
				if _, err := os.Stat(mutestingPath); err == nil {
					cmd.Path = mutestingPath
					cmd.Err = nil
				}
			}
		}

		cmd.Dir = workDir

		out, err := cmd.CombinedOutput()
		if err != nil && !errors.As(err, &exitErr) {
			return out, fmt.Errorf("go-mutesting command failed: %w", err)
		}

		return out, nil
	}

	client := &Client{
		dir: workDir,
		run: run,
	}

	return client, nil
}

// Mutest runs the mutation testing process, outputting HTML results and utilizing a blacklist.
func (c *Client) Mutest(ctx context.Context, disabledMutators []string) (string, error) {
	disableFlags := make([]string, 0, len(disabledMutators))
	for _, disabledMutator := range disabledMutators {
		disableFlag := fmt.Sprintf("--disable=%s", disabledMutator)
		disableFlags = append(disableFlags, disableFlag)
	}

	args := slices.Concat(disableFlags, []string{"--html-output", "--blacklist=go-mutesting.blacklist", "./..."})
	out, err := c.run(ctx, args...)
	if err != nil {
		return "", fmt.Errorf("mutation testing failed: %w\nOutput:\n%s", err, string(out))
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	summary := lines[len(lines)-1]

	return summary, nil
}
