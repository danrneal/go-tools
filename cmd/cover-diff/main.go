package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"

	"github.com/danrneal/go-tools/internal/diff"
	"github.com/danrneal/go-tools/internal/git"
	"golang.org/x/tools/cover"
)

// Coverage represents line-by-line test coverage across multiple files.
// The outer map key is the relative file path. The inner map key is the
// line number, and the boolean value indicates whether that line was executed.
type Coverage map[string]map[int]bool

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

	coverProfiles, err := cover.ParseProfiles(coverProfile)
	if err != nil {
		return fmt.Errorf("error parsing coverage profile: %w", err)
	}

	baseCoverProfiles, err := getCoverProfile(ctx, baseCommit)
	if err != nil {
		return err
	}

	coverage := parseCoverage(ctx, coverProfiles)
	baseCoverage := parseCoverage(ctx, baseCoverProfiles)

	fileDiffs, err := parseGitDiff(ctx, baseCommit)
	if err != nil {
		return err
	}

	baseOverallCoverage := calculateOverallCoverage(baseCoverProfiles)
	overallCoverage := calculateOverallCoverage(coverProfiles)
	regressions := findRegressions(baseCoverage, coverage, fileDiffs)
	newUncoveredLines := findNewUncoveredLines(coverage, fileDiffs)
	printReport(baseOverallCoverage, overallCoverage, regressions, newUncoveredLines)

	return nil
}

// parseCoverage converts a slice of parsed Go coverage profiles into a simpler,
// faster-to-query Coverage map, dynamically stripping the module prefix from filenames.
func parseCoverage(ctx context.Context, coverProfiles []*cover.Profile) Coverage {
	coverage := Coverage{}
	modulePath, err := getModulePath(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not get module path for prefix stripping: %v\n", err)
	}

	for _, coverProfile := range coverProfiles {
		filename := strings.TrimPrefix(coverProfile.FileName, modulePath)
		if _, ok := coverage[filename]; !ok {
			coverage[filename] = map[int]bool{}
		}

		for _, block := range coverProfile.Blocks {
			for line := block.StartLine; line <= block.EndLine; line++ {
				coverage[filename][line] = coverage[filename][line] || block.Count > 0
			}
		}
	}

	return coverage
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
func getCoverProfile(ctx context.Context, baseCommit string) ([]*cover.Profile, error) {
	worktree, cleanup, err := git.CreateDetachedWorktree(ctx, ".", baseCommit)
	if err != nil {
		return nil, fmt.Errorf("failed to create detached worktree: %w", err)
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
		_ = os.Remove(baseCoverProfile)
		errMsg = "failed to generate base coverage profile" + errMsg

		return nil, fmt.Errorf("%s\nTest Output:\n%s", errMsg, string(out))
	}

	coverProfiles, err := cover.ParseProfiles(baseCoverProfilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse generated base profiles: %w", err)
	}

	return coverProfiles, nil
}

// parseGitDiff executes a strict git diff against the base commit and parses
// the unified output into a collection of FileDiffs that map line shifts.
func parseGitDiff(ctx context.Context, baseCommit string) (map[string]diff.FileDiff, error) {
	cmd := exec.CommandContext(ctx, "git", "diff", "--no-ext-diff", "-U0", baseCommit)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to generate git diff: %w\nOutput: %s", err, string(out))
	}

	gitDiff, err := diff.Parse(bytes.NewReader(out))
	if err != nil {
		return nil, fmt.Errorf("failed to parse git diff: %w", err)
	}

	return gitDiff, nil
}

func calculateOverallCoverage(coverProfiles []*cover.Profile) float64 {
	coveredStatements := 0
	totalStatements := 0

	for _, coverProfile := range coverProfiles {
		for _, block := range coverProfile.Blocks {
			totalStatements += block.NumStmt
			if block.Count > 0 {
				coveredStatements += block.NumStmt
			}
		}
	}

	if totalStatements == 0 {
		return 0.0
	}

	return (float64(coveredStatements) / float64(totalStatements)) * 100.0
}

// findRegressions cross-references the base coverage against the current coverage.
// It returns a list of strings formatted as "filename:line" representing lines
// that were covered in the base commit but are no longer covered in the current code.
func findRegressions(baseCoverage, currentCoverage Coverage, fileDiffs map[string]diff.FileDiff) []string {
	regressions := []string{}
	for filename, oldLines := range baseCoverage {
		for oldLine, covered := range oldLines {
			if !covered {
				continue
			}

			newLine := oldLine
			if fileDiff, ok := fileDiffs[filename]; ok {
				newLine = fileDiff.ToNewLine(oldLine)
			}

			if newLine == -1 {
				continue
			}

			newLines, ok := currentCoverage[filename]
			if !ok {
				continue
			}

			if covered, ok := newLines[newLine]; !ok || covered {
				continue
			}

			regression := fmt.Sprintf("%s:%d", filename, newLine)
			regressions = append(regressions, regression)
		}
	}

	return regressions
}

// findNewUncoveredLines iterates through uncovered lines in the current workspace
// and checks if they fall within newly inserted code blocks in the git diff.
func findNewUncoveredLines(coverage Coverage, fileDiffs map[string]diff.FileDiff) []string {
	var newUncoveredLines []string
	for filename, lines := range coverage {
		fileDiff, ok := fileDiffs[filename]
		if !ok {
			continue
		}

		for line, covered := range lines {
			if covered {
				continue
			}

			if fileDiff.IsAddition(line) {
				newUncoveredLine := fmt.Sprintf("%s:%d", filename, line)
				newUncoveredLines = append(newUncoveredLines, newUncoveredLine)
			}
		}
	}

	return newUncoveredLines
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
