package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"

	"github.com/danrneal/go-tools/internal/git"
	"github.com/danrneal/go-tools/internal/gomutants"
)

const (
	// ignoreFilename is the default name for the mutation testing ignore file.
	ignoreFilename = ".gomutants-ignore"

	colorReset = "\033[0m"
	colorRed   = "\033[31m"
	colorGreen = "\033[32m"
)

func main() {
	htmlReport := flag.String("htmlreport", "mutation-report.html", "Path to current HTML mutation report")

	flag.Parse()

	if err := run(*htmlReport); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// run orchestrates the mutation report post-processing. It parses the ignore file and HTML report,
// shifts and self-heals ignored mutations across git diffs, and rewrites the filtered HTML report.
func run(htmlReport string) error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	gitClient, err := git.NewClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to create git client: %w", err)
	}

	ignoreFile, err := parseIgnoreFile()
	if err != nil {
		return err
	}

	report, err := gomutants.ParseHTMLReport(htmlReport)
	if err != nil {
		return fmt.Errorf("failed to parse HTML report: %w", err)
	}

	survivedMutations := report.SurvivedMutations()

	if err = updateIgnoreFile(ctx, gitClient, ignoreFile, survivedMutations); err != nil {
		return err
	}

	if err = updateHTMLReport(report, htmlReport, ignoreFile.Mutations); err != nil {
		return err
	}

	ignoredCount := len(ignoreFile.Mutations)
	remainingCount := len(survivedMutations) - ignoredCount
	color := colorGreen
	if remainingCount > 0 {
		color = colorRed
	}

	fmt.Fprintf(
		os.Stdout,
		"%sIgnored %d mutations (%d unignored survived mutations remaining).%s\n",
		color,
		ignoredCount,
		remainingCount,
		colorReset,
	)

	return nil
}

// parseIgnoreFile opens and parses the dirty ignore file from the current working directory.
func parseIgnoreFile() (*gomutants.IgnoreFile, error) {
	file, err := os.OpenFile(ignoreFilename, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, fmt.Errorf("failed to open or create %s: %w", ignoreFilename, err)
	}

	defer file.Close()

	ignoreFile, err := gomutants.ParseIgnoreFile(file)
	if err != nil {
		return nil, fmt.Errorf("failed to parse ignore file: %w", err)
	}

	return ignoreFile, nil
}

// updateIgnoreFile synchronizes the given ignore file with the latest diffs and mutations.
func updateIgnoreFile(
	ctx context.Context,
	gitClient *git.Client,
	ignoreFile *gomutants.IgnoreFile,
	mutations gomutants.Mutations,
) error {
	lastCommit, err := gitClient.LastCommit(ctx, ignoreFilename)
	if err != nil || lastCommit == "" {
		ignoreFile.Filter(mutations)
		if err = ignoreFile.Write(ignoreFilename); err != nil {
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

	combinedDiff, err := gitClient.Diff(ctx, "HEAD", "")
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
	if err = ignoreFile.Write(ignoreFilename); err != nil {
		return fmt.Errorf("failed to save updated ignore file: %w", err)
	}

	return nil
}

// parseIgnoreFileAtCommit retrieves and parses the ignore file from the specified commit.
func parseIgnoreFileAtCommit(ctx context.Context, gitClient *git.Client, commit string) (*gomutants.IgnoreFile, error) {
	file, err := gitClient.Show(ctx, commit, ignoreFilename)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch ignore file from %s: %w", commit, err)
	}

	ignoreFile, err := gomutants.ParseIgnoreFile(file)
	if err != nil {
		return nil, fmt.Errorf("failed to parse ignore file from %s: %w", commit, err)
	}

	return ignoreFile, nil
}

// updateHTMLReport marks matching surviving mutations as ignored in the report and writes it to path.
func updateHTMLReport(report *gomutants.Report, path string, mutations gomutants.Mutations) error {
	report.Ignore(mutations)

	if err := report.Write(path); err != nil {
		return fmt.Errorf("failed to write HTML report: %w", err)
	}

	return nil
}
