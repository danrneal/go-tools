package coverage

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/danrneal/go-tools/internal/git"
	"golang.org/x/tools/cover"
)

// Files represents line-by-line test coverage across multiple files.
// The outer map key is the relative file path. The inner map key is the
// line number, and the boolean value indicates whether that line was executed.
type Files map[string]map[int]bool

// Parse converts a slice of parsed Go coverage profiles into a simpler,
// faster-to-query coverage map, dynamically stripping the module prefix from filenames.
func Parse(coverProfiles []*cover.Profile, modulePath string) Files {
	if !strings.HasSuffix(modulePath, "/") {
		modulePath += "/"
	}

	coverage := Files{}
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

// FindRegressions cross-references the base coverage against the current coverage.
// It returns a list of strings formatted as "filename:line" representing lines
// that were covered in the base commit but are no longer covered in the current code.
func FindRegressions(baseCoverage, currentCoverage Files, fileDiffs map[string]git.FileDiff) []string {
	regressions := []string{}

	baseFiles := slices.Sorted(maps.Keys(baseCoverage))
	for _, filename := range baseFiles {
		newLines, ok := currentCoverage[filename]
		if !ok {
			continue
		}

		oldLines := slices.Sorted(maps.Keys(baseCoverage[filename]))
		for _, oldLine := range oldLines {
			covered := baseCoverage[filename][oldLine]
			if !covered {
				continue
			}

			newLine := oldLine
			if fileDiff, ok := fileDiffs[filename]; ok {
				newLine = fileDiff.ToNewLine(oldLine)
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

// FindNewUncoveredLines iterates through uncovered lines in the current workspace
// and checks if they fall within newly inserted code blocks in the git diff.
func FindNewUncoveredLines(coverage Files, fileDiffs map[string]git.FileDiff) []string {
	newUncoveredLines := []string{}

	files := slices.Sorted(maps.Keys(coverage))
	for _, filename := range files {
		fileDiff, ok := fileDiffs[filename]
		if !ok {
			continue
		}

		lines := slices.Sorted(maps.Keys(coverage[filename]))
		for _, line := range lines {
			covered := coverage[filename][line]
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
