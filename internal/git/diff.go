package git

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// hunkRegex parses the unified diff hunk header format (e.g., "@@ -10,3 +12,5 @@").
// The line counts are optional in unified diffs if they equal 1.
var hunkRegex = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@`)

// FileDiff contains all the modifications made to a single file, including
// potential renames and a list of line-shifting hunks.
type FileDiff struct {
	NewRelPath string
	Hunks      []Hunk
}

// Hunk represents a single chunk of changes in a file as reported by a git diff.
// It tracks the starting line and the number of lines affected in both the
// original (old) and the modified (new) states.
type Hunk struct {
	OldStart int
	OldCount int
	NewStart int
	NewCount int
}

// Diff executes `git diff` against the provided commit(s) using strict formatting
// flags (-U0, -M, --no-ext-diff) and parses the output into FileDiff structs.
// It accepts one commit (to compare against dirty state) or two commits.
func (c *Client) Diff(ctx context.Context, commitA string, commitB ...string) (map[string]FileDiff, error) {
	if len(commitB) > 1 {
		return nil, errors.New("git diff accepts a maximum of two commits")
	}

	diff, err := c.runDiff(ctx, commitA, commitB...)
	if err != nil {
		return nil, err
	}

	fileDiffs := map[string]FileDiff{}

	reader := bytes.NewReader(diff)

	var oldRelPath, newRelPath string
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := scanner.Text()

		if suffix, ok := strings.CutPrefix(line, "rename from "); ok {
			oldRelPath = suffix
			continue
		}

		if suffix, ok := strings.CutPrefix(line, "rename to "); ok {
			newRelPath = suffix
			fileDiffs[oldRelPath] = FileDiff{
				NewRelPath: newRelPath,
			}

			continue
		}

		if suffix, ok := strings.CutPrefix(line, "--- a/"); ok {
			oldRelPath = suffix
			continue
		}

		if suffix, ok := strings.CutPrefix(line, "+++ b/"); ok {
			newRelPath = suffix
			if _, ok := fileDiffs[oldRelPath]; !ok {
				fileDiffs[oldRelPath] = FileDiff{
					NewRelPath: newRelPath,
				}
			}

			continue
		}

		matches := hunkRegex.FindStringSubmatch(line)
		if matches == nil {
			continue
		}

		hunk := parseHunk(matches)
		fileDiff := fileDiffs[oldRelPath]
		fileDiff.Hunks = append(fileDiff.Hunks, hunk)
		fileDiffs[oldRelPath] = fileDiff
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to scan diff: %w", err)
	}

	return fileDiffs, nil
}

// runDiff builds the argument list and executes the git diff command via the client's runner.
func (c *Client) runDiff(ctx context.Context, commitA string, commitB ...string) ([]byte, error) {
	if len(commitB) == 1 {
		out, err := c.run(ctx, "diff", "-U0", "-M", "--no-ext-diff", commitA, commitB[0])
		return out, err
	}

	out, err := c.run(ctx, "diff", "-U0", "-M", "--no-ext-diff", commitA)

	return out, err
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

// ToNewLine translates a line number from the original file to its corresponding
// line number in the modified file. If the line was deleted or directly modified,
// it returns -1.
func (fd FileDiff) ToNewLine(oldLine int) int {
	newLine := oldLine
	for _, hunk := range fd.Hunks {
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
	isAddition := slices.ContainsFunc(fd.Hunks, func(hunk Hunk) bool {
		return line >= hunk.NewStart && line < hunk.NewStart+hunk.NewCount
	})

	return isAddition
}
