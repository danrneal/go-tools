package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"

	"github.com/danrneal/go-tools/internal/coverage"
	"github.com/danrneal/go-tools/internal/git"
	"github.com/danrneal/go-tools/internal/golang"
	"golang.org/x/tools/cover"
)

func main() {
	coverProfile := flag.String("coverprofile", "coverage.out", "Path to current coverage profile")
	baseCommit := flag.String("base", "HEAD", "Base branch or commit to compare against")

	flag.Parse()

	worktreeBaseDir := filepath.Join(os.TempDir(), "cover-diff-worktrees")

	if err := run(*coverProfile, *baseCommit, worktreeBaseDir); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// run orchestrates the coverage diffing process. It parses the current coverage,
// generates and parses the base coverage, computes the git diff, and finally
// compares the data sets to report regressions or uncovered new code.
func run(coverProfile, baseCommit, worktreeBaseDir string) error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	gitClient, err := git.NewClient(ctx, worktreeBaseDir)
	if err != nil {
		return fmt.Errorf("failed to create git client: %w", err)
	}

	goClient, err := golang.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create go client: %w", err)
	}

	coverProfiles, err := cover.ParseProfiles(coverProfile)
	if err != nil {
		return fmt.Errorf("error parsing coverage profile: %w", err)
	}

	baseCoverProfiles, err := getCoverProfiles(ctx, gitClient, baseCommit)
	if err != nil {
		return err
	}

	modulePath, err := goClient.ModulePath(ctx)
	if err != nil {
		return fmt.Errorf("failed to get module path: %w", err)
	}

	coverageFiles := coverage.Parse(coverProfiles, modulePath)
	baseCoverageFiles := coverage.Parse(baseCoverProfiles, modulePath)

	combinedDiff, err := gitClient.Diff(ctx, baseCommit, "")
	if err != nil {
		return fmt.Errorf("failed to parse git diff: %w", err)
	}

	regressions := coverageFiles.Regressions(baseCoverageFiles, combinedDiff)
	uncoveredAdditions := coverageFiles.UncoveredAdditions(baseCoverageFiles, combinedDiff)
	baseOverallPercentage := coverage.OverallPercentage(baseCoverProfiles)
	overallPercentage := coverage.OverallPercentage(coverProfiles)
	printReport(coverageFiles, regressions, uncoveredAdditions, baseOverallPercentage, overallPercentage)

	return nil
}

// getCoverProfiles creates a temporary git worktree at the specified baseCommit,
// runs the test suite within that isolated environment to generate a coverage
// profile, and parses the resulting file before cleaning up the worktree.
func getCoverProfiles(ctx context.Context, gitClient *git.Client, commit string) ([]*cover.Profile, error) {
	worktree, cleanup, err := gitClient.CreateWorktree(ctx, commit)
	if err != nil {
		return nil, fmt.Errorf("failed to create worktree: %w", err)
	}

	defer cleanup()

	goClient, err := golang.NewClient(golang.WithDir(worktree))
	if err != nil {
		return nil, fmt.Errorf("failed to create go client in worktree: %w", err)
	}

	coverProfile, err := goClient.GenerateCoverProfile(ctx)
	if err != nil {
		if errors.Is(err, golang.ErrNoCoverage) {
			return nil, nil
		}

		return nil, fmt.Errorf("failed to get coverage profile: %w", err)
	}

	coverProfiles, err := cover.ParseProfiles(coverProfile)
	if err != nil {
		return nil, fmt.Errorf("failed to parse generated profiles: %w", err)
	}

	return coverProfiles, nil
}

// printReport formats and writes the identified regressions and uncovered lines
// to standard output. It does not return an error, making the tool purely informational.
func printReport(
	coverageFiles coverage.Files,
	regressions, uncoveredAdditions []coverage.FileReport,
	baseOverallCoverage, overallCoverage float64,
) {
	const (
		colorReset  = "\033[0m"
		colorRed    = "\033[31m"
		colorGreen  = "\033[32m"
		colorYellow = "\033[33m"
	)

	if len(regressions) > 0 {
		fmt.Fprintf(os.Stdout, "%sCoverage Regressions Found:%s\n", colorRed, colorReset)

		for _, regression := range regressions {
			lineRanges := coverageFiles.FormatLineRanges(regression)
			for _, lineRange := range lineRanges {
				fmt.Fprintf(os.Stdout, "%s  - %s:%s%s\n", colorRed, regression.RelPath, lineRange, colorReset)
			}
		}

		fmt.Fprintln(os.Stdout, "")
	}

	if len(uncoveredAdditions) > 0 {
		fmt.Fprintf(os.Stdout, "%sUncovered Code Additions Found (Please Review):%s\n", colorYellow, colorReset)

		for _, uncoveredAddition := range uncoveredAdditions {
			lineRanges := coverageFiles.FormatLineRanges(uncoveredAddition)
			for _, lineRange := range lineRanges {
				fmt.Fprintf(os.Stdout, "%s  - %s:%s%s\n", colorYellow, uncoveredAddition.RelPath, lineRange, colorReset)
			}
		}
	}

	if len(regressions) == 0 && len(uncoveredAdditions) == 0 {
		fmt.Fprintf(os.Stdout,
			"%sCoverage checks passed! No regressions or new uncovered code.%s\n",
			colorGreen, colorReset,
		)
	}

	var deltaStr string
	delta := overallCoverage - baseOverallCoverage
	if delta >= 0 {
		deltaStr = fmt.Sprintf("%s+%.2f%%%s", colorGreen, delta, colorReset)
	} else {
		deltaStr = fmt.Sprintf("%s%.2f%%%s", colorRed, delta, colorReset)
	}

	fmt.Fprintf(os.Stdout, "Base Coverage:    %.2f%%\n", baseOverallCoverage)
	fmt.Fprintf(os.Stdout, "Current Coverage: %.2f%% (%s)\n\n", overallCoverage, deltaStr)
}
