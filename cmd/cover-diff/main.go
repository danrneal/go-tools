package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"

	"github.com/danrneal/go-tools/internal/coverage"
	"github.com/danrneal/go-tools/internal/git"
	"golang.org/x/tools/cover"
)

func main() {
	coverProfile := flag.String("coverprofile", "coverage.out", "Path to current coverage profile")
	baseCommit := flag.String("base", "main", "Base branch or commit to compare against")

	flag.Parse()

	if err := run(*coverProfile, *baseCommit); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// run orchestrates the coverage diffing process. It parses the current coverage,
// generates and parses the base coverage, computes the git diff, and finally
// compares the data sets to report regressions or uncovered new code.
func run(coverProfile, baseCommit string) error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	gc, err := git.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create git client: %w", err)
	}

	coverProfiles, err := cover.ParseProfiles(coverProfile)
	if err != nil {
		return fmt.Errorf("error parsing coverage profile: %w", err)
	}

	baseCoverProfiles, err := getCoverProfiles(ctx, gc, baseCommit)
	if err != nil {
		return err
	}

	modulePath, err := getModulePath(ctx)
	if err != nil {
		return err
	}

	fileCoverage := coverage.Parse(coverProfiles, modulePath)
	baseFileCoverage := coverage.Parse(baseCoverProfiles, modulePath)

	fileDiffs, err := gc.Diff(ctx, baseCommit)
	if err != nil {
		return fmt.Errorf("failed to parse git diff: %w", err)
	}

	baseOverallPercentage := coverage.OverallPercentage(baseCoverProfiles)
	overallPercentage := coverage.OverallPercentage(coverProfiles)
	regressions := coverage.FindRegressions(baseFileCoverage, fileCoverage, fileDiffs)
	newUncoveredLines := coverage.FindNewUncoveredLines(fileCoverage, fileDiffs)
	printReport(baseOverallPercentage, overallPercentage, regressions, newUncoveredLines)

	return nil
}

// getModulePath queries the local Go toolchain to determine the current module path.
// This is required because go test -cover prefixes filenames with the module path,
// whereas git diff uses relative paths.
func getModulePath(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "go", "list", "-m")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get module path: %w", err)
	}

	modulePath := strings.TrimSpace(string(out)) + "/"

	return modulePath, nil
}

// getCoverProfile creates a temporary git worktree at the specified baseCommit,
// runs the test suite within that isolated environment to generate a coverage
// profile, and parses the resulting file before cleaning up the worktree.
func getCoverProfiles(ctx context.Context, gc *git.Client, baseCommit string) ([]*cover.Profile, error) {
	worktree, cleanup, err := gc.CreateWorktree(ctx, baseCommit)
	if err != nil {
		return nil, fmt.Errorf("failed to create worktree: %w", err)
	}

	defer cleanup()

	errMsg := ""

	baseCoverProfile := "base-coverage.out"
	cmd := exec.CommandContext(ctx, "go", "test", "-coverprofile="+baseCoverProfile, "./...")
	cmd.Dir = worktree
	out, err := cmd.CombinedOutput()
	if err != nil {
		errMsg = fmt.Sprintf(" (tests failed: %v)", err)
	}

	baseCoverProfilePath := filepath.Join(worktree, baseCoverProfile)
	stat, err := os.Stat(baseCoverProfilePath)
	if err != nil || stat.Size() == 0 {
		errMsg = "failed to generate base coverage profile" + errMsg
		return nil, fmt.Errorf("%s\nTest Output:\n%s", errMsg, string(out))
	}

	coverProfiles, err := cover.ParseProfiles(baseCoverProfilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse generated base profiles: %w", err)
	}

	return coverProfiles, nil
}

// printReport formats and writes the identified regressions and uncovered lines
// to standard output. It does not return an error, making the tool purely informational.
func printReport(baseOverallCoverage, overallCoverage float64, regressions, newUncoveredLines []string) {
	const (
		colorReset  = "\033[0m"
		colorRed    = "\033[31m"
		colorGreen  = "\033[32m"
		colorYellow = "\033[33m"
	)

	if len(regressions) > 0 {
		fmt.Fprintf(os.Stdout, "%sCoverage Regressions Found:%s\n", colorRed, colorReset)

		for _, regression := range regressions {
			fmt.Fprintf(os.Stdout, "%s  - %s%s\n", colorRed, regression, colorReset)
		}

		fmt.Fprintln(os.Stdout, "")
	}

	if len(newUncoveredLines) > 0 {
		fmt.Fprintf(os.Stdout, "%sNew Uncovered Code Found (Please Review):%s\n", colorYellow, colorReset)

		for _, newUncoveredLine := range newUncoveredLines {
			fmt.Fprintf(os.Stdout, "%s  - %s%s\n", colorYellow, newUncoveredLine, colorReset)
		}
	}

	if len(regressions) == 0 && len(newUncoveredLines) == 0 {
		fmt.Fprintf(
			os.Stdout,
			"%sCoverage checks passed! No regressions or new uncovered code.%s\n",
			colorGreen,
			colorReset,
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
