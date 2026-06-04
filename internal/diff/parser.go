package diff

import (
	"bufio"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

var hunkRegex = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@`)

// Hunk represents a single chunk of changes in a file as reported by a git diff.
// It tracks the starting line and the number of lines affected in both the
// original (old) and the modified (new) states.
type Hunk struct {
	OldStart int
	OldCount int
	NewStart int
	NewCount int
}

// FileDiff represents a collection of all hunks (changes) for a single file.
type FileDiff []Hunk

// ToNewLine translates a line number from the original file to its corresponding
// line number in the modified file. If the line was deleted or directly modified,
// it returns -1.
func (fd FileDiff) ToNewLine(oldLine int) int {
	newLine := oldLine
	for _, hunk := range fd {
		if oldLine < hunk.OldStart || (hunk.OldCount == 0 && oldLine == hunk.OldStart) {
			break
		}

		if oldLine < hunk.OldStart+hunk.OldCount {
			return -1
		}

		newLine += hunk.NewCount - hunk.OldCount
	}

	return newLine
}

// IsAddition returns true if the given line number falls within a newly added
// or modified block of code in this file diff.
func (fd FileDiff) IsAddition(line int) bool {
	isAddition := slices.ContainsFunc(fd, func(hunk Hunk) bool {
		return line >= hunk.NewStart && line < hunk.NewStart+hunk.NewCount
	})

	return isAddition
}

// Parse reads unified diff output (preferably generated with -U0 to omit context lines)
// from the provided reader and returns a map correlating filenames to their FileDiffs.
func Parse(r io.Reader) (map[string]FileDiff, error) {
	scanner := bufio.NewScanner(r)
	fileDiffs := map[string]FileDiff{}

	var filename string
	for scanner.Scan() {
		line := scanner.Text()
		if suffix, ok := strings.CutPrefix(line, "+++ b/"); ok {
			filename = suffix
			continue
		}

		if matches := hunkRegex.FindStringSubmatch(line); matches != nil {
			hunk := parseHunk(matches)
			fileDiffs[filename] = append(fileDiffs[filename], hunk)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to scan diff: %w", err)
	}

	return fileDiffs, nil
}

// parseHunk converts regex matches from a diff hunk header into a Hunk struct.
// It handles omitted counts in unified diffs by defaulting them to 1.
func parseHunk(matches []string) Hunk {
	oldStart, _ := strconv.Atoi(matches[1])

	oldCount := 1
	if matches[2] != "" {
		oldCount, _ = strconv.Atoi(matches[2])
	}

	newStart, _ := strconv.Atoi(matches[3])

	newCount := 1
	if matches[4] != "" {
		newCount, _ = strconv.Atoi(matches[4])
	}

	hunk := Hunk{
		OldStart: oldStart,
		OldCount: oldCount,
		NewStart: newStart,
		NewCount: newCount,
	}

	return hunk
}
