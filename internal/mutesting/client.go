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
	run func(ctx context.Context, env []string, args ...string) ([]byte, error)
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

	run := func(ctx context.Context, env []string, args ...string) ([]byte, error) {
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

		cmd.Env = append(os.Environ(), env...)
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
	env, cleanup, err := setupGoTestWrapper("shift && exec $GO_BIN test -trimpath \"$@\"")
	if err != nil {
		return "", err
	}

	defer cleanup()

	disableFlags := make([]string, 0, len(disabledMutators))
	for _, disabledMutator := range disabledMutators {
		disableFlag := fmt.Sprintf("--disable=%s", disabledMutator)
		disableFlags = append(disableFlags, disableFlag)
	}

	args := slices.Concat(disableFlags, []string{"--html-output", "--blacklist=go-mutesting.blacklist", "./..."})
	out, err := c.run(ctx, env, args...)
	if err != nil {
		return "", fmt.Errorf("mutation testing failed: %w\nOutput:\n%s", err, string(out))
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	summary := lines[len(lines)-1]

	return summary, nil
}

func setupGoTestWrapper(goTestLogic string) ([]string, func(), error) {
	goBin, err := exec.LookPath("go")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to locate go binary: %w", err)
	}

	goTestWrapperDir, err := os.MkdirTemp("", "go-test-wrapper-")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create temp dir for go test wrapper: %w", err)
	}

	cleanup := func() {
		_ = os.RemoveAll(goTestWrapperDir)
	}

	goTestWrapper := fmt.Sprintf(`#!/bin/sh
		if [ "$1" = "test" ]; then
			%s
		fi
		exec "$GO_BIN" "$@"
	`, goTestLogic)

	goTestWrapperPath := filepath.Join(goTestWrapperDir, "go")

	//nolint:gosec // This file must be executable as it acts as a fake go binary wrapper.
	if err := os.WriteFile(goTestWrapperPath, []byte(goTestWrapper), 0o700); err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("failed to write go test wrapper: %w", err)
	}

	env := []string{
		fmt.Sprintf("PATH=%s:%s", goTestWrapperDir, os.Getenv("PATH")),
		fmt.Sprintf("GO_BIN=%s", goBin),
	}

	return env, cleanup, nil
}
