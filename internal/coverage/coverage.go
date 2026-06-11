package coverage

import (
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
// It returns a list of strings formatted as "relPath:line" representing lines
// that were covered in the base commit but are no longer covered in the current code.
func FindRegressions(baseCoverage, currentCoverage Files, fileDiffs map[string]git.FileDiff) map[string][]int {
	regressions := map[string][]int{}
	for oldRelPath, oldLines := range baseCoverage {
		newRelPath := oldRelPath
		if fileDiff, ok := fileDiffs[oldRelPath]; ok {
			newRelPath = fileDiff.NewRelPath
		}

		newLines, ok := currentCoverage[newRelPath]
		if !ok {
			continue
		}

		lines := slices.Sorted(maps.Keys(oldLines))
		for _, oldLine := range lines {
			covered := oldLines[oldLine]
			if !covered {
				continue
			}

			newLine := oldLine
			if fileDiff, ok := fileDiffs[oldRelPath]; ok {
				newLine = fileDiff.ToNewLine(oldLine)
			}

			if covered, ok := newLines[newLine]; !ok || covered {
				continue
			}

			regressions[newRelPath] = append(regressions[newRelPath], newLine)
		}
	}

	return regressions
}

// FindNewUncoveredLines iterates through uncovered lines in the current workspace
// and checks if they fall within newly inserted code blocks in the git diff.
func FindNewUncoveredLines(baseCoverage, currentCoverage Files, fileDiffs map[string]git.FileDiff) map[string][]int {
	newFileDiffs := map[string]git.FileDiff{}
	for _, fileDiff := range fileDiffs {
		newFileDiffs[fileDiff.NewRelPath] = fileDiff
	}

	newUncoveredLines := map[string][]int{}
	for newRelPath, newLines := range currentCoverage {
		fileDiff, ok := newFileDiffs[newRelPath]
		if !ok && baseCoverage[newRelPath] != nil {
			continue
		}

		lines := slices.Sorted(maps.Keys(newLines))
		for _, newLine := range lines {
			covered := newLines[newLine]
			if covered {
				continue
			}

			if ok && !fileDiff.IsAddition(newLine) {
				continue
			}

			newUncoveredLines[newRelPath] = append(newUncoveredLines[newRelPath], newLine)
		}
	}

	return newUncoveredLines
}
