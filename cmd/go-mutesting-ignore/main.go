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

	if err = updateIgnoreFile(ctx, gitClient, ignoreFile, mutations); err != nil {
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

	mutestingClient.OnProgress = updateProgressBar(progressBar, mutations, blacklist)

	summary, err := mutest(ctx, mutestingClient, gitClient, worktree, disabledMutators)
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stdout, "\n%s\n", summary)

	if len(ignoreFile.Mutations) == 0 {
		return nil
	}

	invertedIgnoreFile := createInvertedIgnoreFile(ignoreFile, mutations)
	invertedBlacklist, err := createBlacklist(invertedIgnoreFile, mutations, fileCoverage, worktree)
	if err != nil {
		return fmt.Errorf("failed to create inverted blacklist for removing killed mutants from ignore file: %w", err)
	}

	fmt.Fprintln(os.Stdout, "Verifying ignored mutations are not killed...")
	progressBar = newProgressBar()
	mutestingClient.OnProgress = updateProgressBar(progressBar, mutations, invertedBlacklist)

	removedMutationCount, err := removeKilledMutations(ctx, mutestingClient, ignoreFile, disabledMutators)
	if err != nil {
		return err
	}

	if removedMutationCount > 0 {
		fmt.Fprintf(os.Stdout, "\nRemoved %d killed mutations from ignore file.\n", removedMutationCount)
	} else {
		fmt.Fprintln(os.Stdout, "\nNo killed mutations found in ignore file.")
	}

	return nil
}

// newProgressBar initializes and returns a progress bar configured for mutation testing.
func newProgressBar() *progressbar.ProgressBar {
	progressBarTheme := progressbar.ThemeDefault
	progressBarTheme.BarEnd = "| [Elapsed:ETA]"
	progressBarTheme.BarEndFilled = "|"

	progressBarOpts := []progressbar.Option{
		progressbar.OptionSetDescription("Generating mutations..."),
		progressbar.OptionSetRenderBlankState(true),
		progressbar.OptionSetTheme(progressBarTheme),
		progressbar.OptionShowElapsedTimeOnFinish(),
	}

	progressBar := progressbar.NewOptions(-1, progressBarOpts...)

	return progressBar
}

// updateProgressBar configures an existing progress bar with the correct maximum value
// based on the total number of mutations minus those in the blacklist. It also wires
// the mutesting client to update this progress bar as it executes.
func updateProgressBar(
	progressBar *progressbar.ProgressBar,
	mutations map[mutesting.Mutation][]string,
	blacklist []string,
) func(int) {
	totalChecksums := 0
	for _, checksums := range mutations {
		totalChecksums += len(checksums)
	}

	progressBar.Describe("Testing mutations...")
	progressBar.ChangeMax(totalChecksums - len(blacklist))
	progressbar.OptionShowCount()(progressBar)

	onProgress := func(progress int) {
		_ = progressBar.Add(progress)
	}

	return onProgress
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
) error {
	lastCommit, err := gitClient.LastCommit(ctx, ignoreFilename)
	if err != nil || lastCommit == "" {
		ignoreFile.Filter(mutations)
		if err = ignoreFile.WriteIgnoreFile(ignoreFilename); err != nil {
			return fmt.Errorf("failed to save updated ignore file: %w", err)
		}

		return nil
	}

	lastCommitIgnoreFile, err := parseIgnoreFileAtCommit(ctx, gitClient, lastCommit)
	if err != nil {
		return err
	}

	lastCommitCombinedDiff, err := gitClient.Diff(ctx, lastCommit, "HEAD")
	if err != nil {
		return fmt.Errorf("failed to get ${LAST_SYNCED_COMMIT}...HEAD diffs: %w", err)
	}

	lastCommitIgnoreFile.Shift(lastCommitCombinedDiff)

	combinedDiff, err := gitClient.Diff(ctx, "HEAD")
	if err != nil {
		return fmt.Errorf("failed to get HEAD...Dirty diffs: %w", err)
	}

	lastCommitIgnoreFile.Shift(combinedDiff)

	for mutation := range ignoreFile.Mutations {
		if !lastCommitIgnoreFile.Mutations[mutation] {
			lastCommitIgnoreFile.Mutations[mutation] = true
		}
	}

	lastCommitIgnoreFile.Filter(mutations)

	*ignoreFile = *lastCommitIgnoreFile
	if err = ignoreFile.WriteIgnoreFile(ignoreFilename); err != nil {
		return fmt.Errorf("failed to save updated ignore file: %w", err)
	}

	return nil
}

// parseAnchorIgnoreFile retrieves and parses the ignore file from the last commit.
func parseIgnoreFileAtCommit(ctx context.Context, gitClient *git.Client, commit string) (*mutesting.IgnoreFile, error) {
	file, err := gitClient.Show(ctx, commit, ignoreFilename)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch ignore file from %s: %w", commit, err)
	}

	ignoreFile, err := mutesting.ParseIgnoreFile(file)
	if err != nil {
		return nil, fmt.Errorf("failed to parse ignore file from %s: %w", commit, err)
	}

	return ignoreFile, nil
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

// createInvertedIgnoreFile generates a temporary IgnoreFile containing all mutations
// that are currently NOT in the given ignore file. This inverse state is used
// to generate a blacklist for the self-healing verification pass.
func createInvertedIgnoreFile(
	ignoreFile *mutesting.IgnoreFile,
	mutations map[mutesting.Mutation][]string,
) *mutesting.IgnoreFile {
	invertedIgnoreFile := &mutesting.IgnoreFile{
		Mutations: map[mutesting.Mutation]bool{},
	}

	for mutation := range mutations {
		if !ignoreFile.Mutations[mutation] {
			invertedIgnoreFile.Mutations[mutation] = true
		}
	}

	return invertedIgnoreFile
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

// removeKilledMutations performs a secondary verification pass by running go-mutesting
// with an inverted blacklist. It parses the results to determine which ignored mutations
// were successfully killed by the test suite, removes them from the ignore file, and saves it.
// It returns the number of mutations that were successfully self-healed.
func removeKilledMutations(
	ctx context.Context,
	mutestingClient *mutesting.Client,
	ignoreFile *mutesting.IgnoreFile,
	disabledMutators []string,
) (int, error) {
	if _, err := mutestingClient.Mutest(ctx, disabledMutators); err != nil {
		return 0, fmt.Errorf("failed to run mutation testing with inverted blacklist: %w", err)
	}

	escapedIgnoredMutations, err := mutestingClient.ParseReport()
	if err != nil {
		return 0, fmt.Errorf("failed to parse report for inverted blacklist: %w", err)
	}

	removedMutationCount := len(ignoreFile.Mutations) - len(escapedIgnoredMutations)
	if removedMutationCount > 0 {
		ignoreFile.Filter(escapedIgnoredMutations)
		if err = ignoreFile.WriteIgnoreFile(ignoreFilename); err != nil {
			return 0, fmt.Errorf("failed to save ignore file after removing killed ignored mutations: %w", err)
		}
	}

	return removedMutationCount, nil
}
