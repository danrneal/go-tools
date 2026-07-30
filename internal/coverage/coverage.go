package coverage

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/danrneal/go-tools/internal/git"
	"golang.org/x/tools/cover"
)

// maxGapSize defines the maximum number of consecutive unexecutable lines
// (like blank lines or closing braces) that can be bridged into a single line range.
const maxGapSize = 2

// Files represents line-by-line test coverage across multiple files.
// The outer map key is the relative file path. The inner map key is the
// line number, and the boolean value indicates whether that line was executed.
type Files map[string]map[int]bool

// FileReport encapsulates a relative file path and a list of specific
// line numbers within that file that require attention (e.g., regressions).
type FileReport struct {
	RelPath string
	Lines   []int
}

// Parse converts a slice of parsed Go coverage profiles into a simpler,
// faster-to-query coverage map, dynamically stripping the module prefix from relative paths.
func Parse(coverProfiles []*cover.Profile, modulePath string) Files {
	if !strings.HasSuffix(modulePath, "/") {
		modulePath += "/"
	}

	coverage := Files{}
	for _, coverProfile := range coverProfiles {
		relPath := strings.TrimPrefix(coverProfile.FileName, modulePath)
		if _, ok := coverage[relPath]; !ok {
			coverage[relPath] = map[int]bool{}
		}

		for _, block := range coverProfile.Blocks {
			for line := block.StartLine; line <= block.EndLine; line++ {
				coverage[relPath][line] = coverage[relPath][line] || block.Count > 0
			}
		}
	}

	return coverage
}

// Regressions cross-references the base coverage against the current coverage.
// It returns a list of FileReports detailing specific lines that were covered
// in the base commit but are no longer covered in the current code.
func (f Files) Regressions(baseCoverage Files, combinedDiff git.CombinedDiff) []FileReport {
	regressions := []FileReport{}

	currentFiles := slices.Sorted(maps.Keys(f))
	for _, newRelPath := range currentFiles {
		oldRelPath := newRelPath
		fileDiff, ok := combinedDiff.ToFile[newRelPath]
		if ok {
			oldRelPath = fileDiff.OldRelPath
		}

		oldLines, ok := baseCoverage[oldRelPath]
		if !ok {
			continue
		}

		regressionLines := []int{}
		coveredLines := f[newRelPath]
		newLines := slices.Sorted(maps.Keys(coveredLines))
		for _, newLine := range newLines {
			if coveredLines[newLine] {
				continue
			}

			oldLine := newLine
			if fileDiff != nil {
				oldLine = fileDiff.ToOldLine(newLine)
			}

			if covered, ok := oldLines[oldLine]; !ok || !covered {
				continue
			}

			regressionLines = append(regressionLines, newLine)
		}

		if len(regressionLines) > 0 {
			regression := FileReport{
				RelPath: newRelPath,
				Lines:   regressionLines,
			}

			regressions = append(regressions, regression)
		}
	}

	return regressions
}

// UncoveredAdditions iterates through uncovered lines in the current workspace
// and checks if they fall within newly inserted code blocks in the git diff.
func (f Files) UncoveredAdditions(baseCoverage Files, combinedDiff git.CombinedDiff) []FileReport {
	uncoveredAdditions := []FileReport{}

	currentFiles := slices.Sorted(maps.Keys(f))
	for _, newRelPath := range currentFiles {
		fileDiff, ok := combinedDiff.ToFile[newRelPath]
		if !ok && baseCoverage[newRelPath] != nil {
			continue
		}

		uncoveredAdditionLines := []int{}
		coveredLines := f[newRelPath]
		newLines := slices.Sorted(maps.Keys(coveredLines))
		for _, newLine := range newLines {
			if coveredLines[newLine] {
				continue
			}

			if ok && !fileDiff.IsAddition(newLine) {
				continue
			}

			uncoveredAdditionLines = append(uncoveredAdditionLines, newLine)
		}

		if len(uncoveredAdditionLines) > 0 {
			uncoveredAddition := FileReport{
				RelPath: newRelPath,
				Lines:   uncoveredAdditionLines,
			}

			uncoveredAdditions = append(uncoveredAdditions, uncoveredAddition)
		}
	}

	return uncoveredAdditions
}

// FormatLineRanges groups consecutive line numbers into strings (e.g., "30-35").
// It intelligently bridges up to a 2-line gap if the missing lines are unexecutable
// (like blank lines or closing braces), but splits the range if any gap line is covered.
func (f Files) FormatLineRanges(fileReport FileReport) []string {
	fileLineRanges := []string{}

	fileCoverage := f[fileReport.RelPath]
	startLine := fileReport.Lines[0]
	prevLine := startLine
	for _, line := range fileReport.Lines {
		gap := line - prevLine - 1
		if gap <= maxGapSize {
			canBridge := true
			for i := range gap {
				canBridge = canBridge && !fileCoverage[prevLine+i+1]
			}

			if canBridge {
				prevLine = line
				continue
			}
		}

		if startLine == prevLine {
			fileLineRanges = append(fileLineRanges, strconv.Itoa(startLine))
		} else {
			fileLineRanges = append(fileLineRanges, fmt.Sprintf("%d-%d", startLine, prevLine))
		}

		startLine = line
		prevLine = line
	}

	if startLine == prevLine {
		fileLineRanges = append(fileLineRanges, strconv.Itoa(startLine))
	} else {
		fileLineRanges = append(fileLineRanges, fmt.Sprintf("%d-%d", startLine, prevLine))
	}

	return fileLineRanges
}

// OverallPercentage computes the total statement coverage percentage.
func OverallPercentage(coverProfiles []*cover.Profile) float64 {
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
