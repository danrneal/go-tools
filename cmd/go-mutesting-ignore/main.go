package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"

	"github.com/danrneal/go-tools/internal/coverage"
	"github.com/danrneal/go-tools/internal/git"
	"github.com/danrneal/go-tools/internal/golang"
	"github.com/danrneal/go-tools/internal/mutesting"
	"golang.org/x/sync/errgroup"
	"golang.org/x/tools/cover"
)

// ignoreFilename is the default name for the mutation testing ignore file.
const ignoreFilename = ".go-mutesting-ignore"

func main() {
	coverProfile := flag.String("coverprofile", "", "Path to the coverage.out file")

	flag.Parse()

	if err := run(*coverProfile); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// run orchestrates the execution of go-mutesting-ignore, syncing the workspace, evaluating test coverage,
// and producing a mutation testing report.
func run(coverProfile string) error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	gitClient, err := git.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create git client: %w", err)
	}

	goClient, err := golang.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create go client: %w", err)
	}

	headIgnoreFile, err := gitClient.Show(ctx, "HEAD", ignoreFilename)
	if err != nil {
		return fmt.Errorf("failed to fetch HEAD ignore file: %w", err)
	}

	dirtyIgnoreFile, err := os.OpenFile(ignoreFilename, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return fmt.Errorf("failed to open or create .go-mutesting-ignore: %w", err)
	}

	defer dirtyIgnoreFile.Close()

	headIgnore, err := mutesting.ParseIgnoreFile(headIgnoreFile)
	if err != nil {
		return fmt.Errorf("failed to parse HEAD ignore file: %w", err)
	}

	dirtyIgnore, err := mutesting.ParseIgnoreFile(dirtyIgnoreFile)
	if err != nil {
		return fmt.Errorf("failed to parse ignore file: %w", err)
	}

	headWorktree, cleanup, err := gitClient.CreateWorktree(ctx, "HEAD")
	if err != nil {
		return fmt.Errorf("failed to setup head worktree: %w", err)
	}

	defer cleanup()

	dirtyWorktree, cleanup, err := gitClient.CreateWorktree(ctx, "HEAD")
	if err != nil {
		return fmt.Errorf("failed to setup dirty worktree: %w", err)
	}

	defer cleanup()

	if err = gitClient.SyncDirtyFiles(ctx, dirtyWorktree); err != nil {
		return fmt.Errorf("failed to sync dirty files: %w", err)
	}

	headMutestingClient, err := mutesting.NewClient(headWorktree)
	if err != nil {
		return fmt.Errorf("failed to create head mutant client: %w", err)
	}

	dirtyMutestingClient, err := mutesting.NewClient(dirtyWorktree)
	if err != nil {
		return fmt.Errorf("failed to create dirty mutant client: %w", err)
	}

	g, gCtx := errgroup.WithContext(ctx)

	var headMutations map[mutesting.Mutation]string
	g.Go(func() error {
		var headErr error
		if headMutations, headErr = headMutestingClient.GenerateMutations(gCtx); headErr != nil {
			return fmt.Errorf("head pre-run failed: %w", headErr)
		}

		return nil
	})

	var dirtyMutations map[mutesting.Mutation]string
	g.Go(func() error {
		var dirtyErr error
		if dirtyMutations, dirtyErr = dirtyMutestingClient.GenerateMutations(gCtx); dirtyErr != nil {
			return fmt.Errorf("dirty pre-run failed: %w", dirtyErr)
		}

		return nil
	})

	if err = g.Wait(); err != nil {
		return fmt.Errorf("parallel pre-runs failed: %w", err)
	}

	headDiffs, err := gitClient.Diff(ctx, headIgnore.LastSyncedCommit, "HEAD")
	if err != nil {
		return fmt.Errorf("failed to get ${LAST_SYNCED_COMMIT}...HEAD diffs: %w", err)
	}

	dirtyDiffs, err := gitClient.Diff(ctx, "HEAD")
	if err != nil {
		return fmt.Errorf("failed to get HEAD...Dirty diffs: %w", err)
	}

	head, err := gitClient.Head(ctx)
	if err != nil {
		return fmt.Errorf("failed to get HEAD: %w", err)
	}

	dirtyIgnoreFile, err = os.Create(ignoreFilename)
	if err != nil {
		return fmt.Errorf("failed to open ignore file for writing: %w", err)
	}

	defer dirtyIgnoreFile.Close()

	dirtyIgnore.Update(headIgnore.Mutations, headDiffs, headMutations, head)
	if err = dirtyIgnore.WriteIgnoreFile(dirtyIgnoreFile); err != nil {
		return fmt.Errorf("failed to save updated ignore file: %w", err)
	}

	headIgnore.Update(headIgnore.Mutations, headDiffs, headMutations, head)
	dirtyIgnore.Update(headIgnore.Mutations, dirtyDiffs, dirtyMutations, "")

	coverProfiles, err := cover.ParseProfiles(coverProfile)
	if err != nil {
		return fmt.Errorf("error parsing coverage profile: %w", err)
	}

	modulePath, err := goClient.ModulePath(ctx)
	if err != nil {
		return fmt.Errorf("failed to get module path: %w", err)
	}

	fileCoverage := coverage.Parse(coverProfiles, modulePath)

	blacklist := createBlacklist(dirtyMutations, dirtyIgnore.Mutations, fileCoverage)
	blacklistData := []byte(strings.Join(blacklist, "\n"))
	blacklistPath := filepath.Join(dirtyWorktree, "go-mutesting.blacklist")
	if err = os.WriteFile(blacklistPath, blacklistData, 0o600); err != nil {
		return fmt.Errorf("failed to write blacklist: %w", err)
	}

	summary, err := dirtyMutestingClient.Mutest(ctx)
	if err != nil {
		return fmt.Errorf("mutation testing failed: %w", err)
	}

	if err := gitClient.CopyFromWorktree("go-mutesting-report.html", dirtyWorktree); err != nil {
		return fmt.Errorf("failed to copy report from worktree: %w", err)
	}

	fmt.Fprintln(os.Stdout, summary)

	return nil
}

// createBlacklist generates a list of mutation checksums to be blacklisted based on the existing mutations,
// ignore lists, and test coverage information.
func createBlacklist(
	mutations map[mutesting.Mutation]string,
	ignoreMutations map[mutesting.Mutation]bool,
	fileCoverage coverage.Files,
) []string {
	blacklist := []string{}
	for mutation, checksum := range mutations {
		if covered, ok := fileCoverage[mutation.RelPath][mutation.StartLine]; !ok || !covered {
			blacklist = append(blacklist, checksum)
		} else if ignoreMutations[mutation] {
			blacklist = append(blacklist, checksum)
		}
	}

	return blacklist
}
