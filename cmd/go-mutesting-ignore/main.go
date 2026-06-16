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
	"github.com/schollz/progressbar/v3"
	"golang.org/x/tools/cover"
)

// ignoreFilename is the default name for the mutation testing ignore file.
const ignoreFilename = ".go-mutesting-ignore"

func main() {
	coverProfile := flag.String("coverprofile", "", "Path to the coverage.out file")

	flag.Parse()

	// These checks are disabled since they are duplicates 1:1 of checks in gremlins.
	disabledMutators := []string{
		"arithmetic/assign_invert",
		"arithmetic/assignment",
		"arithmetic/base",
		"arithmetic/bitwise",
		"loop/break",
		"conditional/negated",
		"expression/comparison",
		"expression/remove",
	}

	if err := run(*coverProfile, disabledMutators); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// run orchestrates the execution of go-mutesting-ignore, syncing the workspace, evaluating test coverage,
// and producing a mutation testing report.
func run(coverProfile string, disabledMutators []string) error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	progressBar := newProgressBar()

	gitClient, err := git.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create git client: %w", err)
	}

	goClient, err := golang.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create go client: %w", err)
	}

	ignoreFile, err := parseIgnoreFile()
	if err != nil {
		return err
	}

	worktree, cleanup, err := createDirtyWorktree(ctx, gitClient)
	if err != nil {
		return err
	}

	defer cleanup()

	mutestingClient, err := mutesting.NewClient(worktree)
	if err != nil {
		return fmt.Errorf("failed to create mutantesting client: %w", err)
	}

	mutations, err := mutestingClient.GenerateMutations(ctx, disabledMutators)
	if err != nil {
		return fmt.Errorf("mutesting pre-run failed: %w", err)
	}

	if err = updateIgnoreFile(ctx, gitClient, ignoreFile, mutations, disabledMutators); err != nil {
		return err
	}

	fileCoverage, err := parseCoverProfile(ctx, goClient, coverProfile)
	if err != nil {
		return err
	}

	blacklist, err := createBlacklist(ignoreFile, mutations, fileCoverage, worktree)
	if err != nil {
		return err
	}

	totalChecksums := 0
	for _, checksums := range mutations {
		totalChecksums += len(checksums)
	}

	progressBar.Describe("Testing mutations...")
	progressBar.ChangeMax(totalChecksums - len(blacklist))
	progressbar.OptionShowCount()(progressBar)

	mutestingClient.OnProgress = func(progress int) {
		_ = progressBar.Add(progress)
	}

	summary, err := mutest(ctx, mutestingClient, gitClient, worktree, disabledMutators)
	if err != nil {
		return err
	}

	fmt.Fprintln(os.Stdout)
	fmt.Fprintln(os.Stdout, summary)

	return nil
}

// newProgressBar initializes and returns a progress bar configured for mutation testing.
func newProgressBar() *progressbar.ProgressBar {
	progressBarTheme := progressbar.ThemeDefault
	progressBarTheme.BarEnd = "| [Elapsed:ETA]"
	progressBarTheme.BarEndFilled = "|"

	progressBarOpts := []progressbar.Option{
		progressbar.OptionSetDescription("Generating mutations..."),
		progressbar.OptionSetTheme(progressBarTheme),
		progressbar.OptionShowElapsedTimeOnFinish(),
	}

	progressBar := progressbar.NewOptions(-1, progressBarOpts...)

	return progressBar
}

// parseIgnoreFile opens and parses the dirty ignore file from the current working directory.
func parseIgnoreFile() (*mutesting.IgnoreFile, error) {
	file, err := os.OpenFile(ignoreFilename, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, fmt.Errorf("failed to open or create .go-mutesting-ignore: %w", err)
	}

	defer file.Close()

	ignoreFile, err := mutesting.ParseIgnoreFile(file)
	if err != nil {
		return nil, fmt.Errorf("failed to parse ignore file: %w", err)
	}

	return ignoreFile, nil
}

// createDirtyWorktree creates a new git worktree based on HEAD and syncs any dirty files into it.
func createDirtyWorktree(ctx context.Context, gitClient *git.Client) (string, func(), error) {
	worktree, cleanup, err := gitClient.CreateWorktree(ctx, "HEAD")
	if err != nil {
		return "", nil, fmt.Errorf("failed to setup worktree: %w", err)
	}

	if err = gitClient.SyncDirtyFiles(ctx, worktree); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("failed to sync dirty files: %w", err)
	}

	return worktree, cleanup, nil
}

// updateIgnoreFile synchronizes the given ignore file with the latest diffs and mutations.
func updateIgnoreFile(
	ctx context.Context,
	gitClient *git.Client,
	ignoreFile *mutesting.IgnoreFile,
	mutations map[mutesting.Mutation][]string,
	disabledMutators []string,
) error {
	headIgnoreFile, err := parseHeadIgnoreFile(ctx, gitClient)
	if err != nil {
		return nil
	}

	headMutations, err := generateHeadMutations(ctx, gitClient, disabledMutators)
	if err != nil {
		return err
	}

	combinedDiff, err := gitClient.Diff(ctx, "HEAD")
	if err != nil {
		return fmt.Errorf("failed to get HEAD...Dirty diffs: %w", err)
	}

	headCombinedDiff, err := gitClient.Diff(ctx, headIgnoreFile.LastSyncedCommit, "HEAD")
	if err != nil {
		return fmt.Errorf("failed to get ${LAST_SYNCED_COMMIT}...HEAD diffs: %w", err)
	}

	head, err := gitClient.Head(ctx)
	if err != nil {
		return fmt.Errorf("failed to get HEAD: %w", err)
	}

	file, err := os.Create(ignoreFilename)
	if err != nil {
		return fmt.Errorf("failed to open ignore file for writing: %w", err)
	}

	defer file.Close()

	ignoreFile.Update(headIgnoreFile.Mutations, headCombinedDiff, headMutations, head)
	if err = ignoreFile.WriteIgnoreFile(file); err != nil {
		return fmt.Errorf("failed to save updated ignore file: %w", err)
	}

	headIgnoreFile.Update(headIgnoreFile.Mutations, headCombinedDiff, headMutations, head)
	ignoreFile.Update(headIgnoreFile.Mutations, combinedDiff, mutations, "")

	return nil
}

// parseHeadIgnoreFile retrieves and parses the ignore file from the HEAD commit.
func parseHeadIgnoreFile(ctx context.Context, gitClient *git.Client) (*mutesting.IgnoreFile, error) {
	file, err := gitClient.Show(ctx, "HEAD", ignoreFilename)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch HEAD ignore file: %w", err)
	}

	headIgnoreFile, err := mutesting.ParseIgnoreFile(file)
	if err != nil {
		return nil, fmt.Errorf("failed to parse HEAD ignore file: %w", err)
	}

	return headIgnoreFile, nil
}

// generateHeadMutations prepares a worktree for the HEAD commit and generates its mutations.
func generateHeadMutations(
	ctx context.Context,
	gitClient *git.Client,
	disabledMutators []string,
) (map[mutesting.Mutation][]string, error) {
	headWorktree, cleanup, err := gitClient.CreateWorktree(ctx, "HEAD")
	if err != nil {
		return nil, fmt.Errorf("failed to setup head worktree: %w", err)
	}

	defer cleanup()

	headMutestingClient, err := mutesting.NewClient(headWorktree)
	if err != nil {
		return nil, fmt.Errorf("failed to create head mutant client: %w", err)
	}

	headMutations, err := headMutestingClient.GenerateMutations(ctx, disabledMutators)
	if err != nil {
		return nil, fmt.Errorf("head pre-run failed: %w", err)
	}

	return headMutations, nil
}

// parseCoverProfile reads the coverage profile and processes it into coverage ranges per file.
func parseCoverProfile(ctx context.Context, goClient *golang.Client, coverProfile string) (coverage.Files, error) {
	coverProfiles, err := cover.ParseProfiles(coverProfile)
	if err != nil {
		return nil, fmt.Errorf("error parsing coverage profile: %w", err)
	}

	modulePath, err := goClient.ModulePath(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get module path: %w", err)
	}

	fileCoverage := coverage.Parse(coverProfiles, modulePath)

	return fileCoverage, nil
}

// createBlacklist generates a list of mutation checksums to be blacklisted based on the existing mutations,
// ignore lists, and test coverage information.
func createBlacklist(
	ignoreFile *mutesting.IgnoreFile,
	mutations map[mutesting.Mutation][]string,
	fileCoverage coverage.Files,
	worktree string,
) ([]string, error) {
	blacklist := []string{}
	for mutation, checksums := range mutations {
		if covered, ok := fileCoverage[mutation.RelPath][mutation.StartLine]; !ok || !covered {
			blacklist = append(blacklist, checksums...)
		} else if ignoreFile.Mutations[mutation] {
			blacklist = append(blacklist, checksums...)
		}
	}

	blacklistData := []byte(strings.Join(blacklist, "\n"))
	blacklistPath := filepath.Join(worktree, "go-mutesting.blacklist")
	if err := os.WriteFile(blacklistPath, blacklistData, 0o600); err != nil {
		return nil, fmt.Errorf("failed to write blacklist: %w", err)
	}

	return blacklist, nil
}

// mutest executes the mutation testing command on the target worktree and fetches the summary report.
func mutest(
	ctx context.Context,
	mutestingClient *mutesting.Client,
	gitClient *git.Client,
	worktree string,
	disabledMutators []string,
) (string, error) {
	summary, err := mutestingClient.Mutest(ctx, disabledMutators)
	if err != nil {
		return "", fmt.Errorf("mutation testing failed: %w", err)
	}

	if err := gitClient.CopyFromWorktree("go-mutesting-report.html", worktree); err != nil {
		return "", fmt.Errorf("failed to copy report from worktree: %w", err)
	}

	return summary, nil
}
