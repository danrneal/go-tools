package git

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// hunkRegex parses the unified diff hunk header format (e.g., "@@ -10,3 +12,5 @@").
// The line counts are optional in unified diffs if they equal 1.
var hunkRegex = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@`)

// CombinedDiff contains the output of a git diff, pre-indexed for fast lookups
// by either the original file path (FromFile) or the current file path (ToFile).
type CombinedDiff struct {
	FromFile map[string]*FileDiff
	ToFile   map[string]*FileDiff
}

// FileDiff contains all the modifications made to a single file, including
// potential renames and a list of line-shifting hunks.
type FileDiff struct {
	OldRelPath string
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
func (c *Client) Diff(ctx context.Context, commitA string, commitB ...string) (CombinedDiff, error) {
	if len(commitB) > 1 {
		return CombinedDiff{}, errors.New("git diff accepts a maximum of two commits")
	}

	diff, err := c.runDiff(ctx, commitA, commitB...)
	if err != nil {
		return CombinedDiff{}, err
	}

	reader := bytes.NewReader(diff)
	combinedDiff, err := parseDiff(reader)

	return combinedDiff, err
}

// runDiff builds the argument list and executes the git diff command via the client's runner.
func (c *Client) runDiff(ctx context.Context, commitA string, commitB ...string) ([]byte, error) {
	if len(commitB) == 1 {
		out, err := c.run(ctx, "diff", "-U0", "-M", "--no-ext-diff", commitA, commitB[0])
		if err != nil {
			return nil, fmt.Errorf("failed to run git diff: %w", err)
		}

		return out, nil
	}

	out, err := c.run(ctx, "diff", "-U0", "-M", "--no-ext-diff", commitA)
	if err != nil {
		return nil, fmt.Errorf("failed to run git diff: %w", err)
	}

	return out, nil
}

// parseDiff reads a unified git diff from the provided reader and parses it
// into a CombinedDiff struct, organizing the changes by their old and new file paths.
func parseDiff(r io.Reader) (CombinedDiff, error) {
	combinedDiff := CombinedDiff{
		FromFile: map[string]*FileDiff{},
		ToFile:   map[string]*FileDiff{},
	}

	var fileDiff *FileDiff
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()

		fileDiff = parseHeader(line, fileDiff)
		if fileDiff == nil {
			continue
		}

		oldRelPath := fileDiff.OldRelPath
		if _, ok := combinedDiff.FromFile[oldRelPath]; !ok && oldRelPath != "" {
			combinedDiff.FromFile[oldRelPath] = fileDiff
		}

		newRelPath := fileDiff.NewRelPath
		if _, ok := combinedDiff.ToFile[newRelPath]; !ok && newRelPath != "" {
			combinedDiff.ToFile[newRelPath] = fileDiff
		}

		matches := hunkRegex.FindStringSubmatch(line)
		if matches == nil {
			continue
		}

		hunk := parseHunk(matches)
		fileDiff.Hunks = append(fileDiff.Hunks, hunk)
	}

	if err := scanner.Err(); err != nil {
		return CombinedDiff{}, fmt.Errorf("failed to scan diff: %w", err)
	}

	return combinedDiff, nil
}

// parseHeader examines a diff output line to determine if it is a file boundary
// or path change indicator (e.g., "--- a/", "rename to "). It updates the provided
// FileDiff object or initializes a new one, returning the current active pointer.
func parseHeader(line string, fileDiff *FileDiff) *FileDiff {
	switch {
	case strings.HasPrefix(line, "rename from "):
		oldRelPath := strings.TrimPrefix(line, "rename from ")
		fileDiff = &FileDiff{
			OldRelPath: oldRelPath,
		}
	case strings.HasPrefix(line, "--- a/"):
		oldRelPath := strings.TrimPrefix(line, "--- a/")
		if fileDiff == nil || fileDiff.OldRelPath != oldRelPath {
			fileDiff = &FileDiff{
				OldRelPath: oldRelPath,
			}
		}
	case strings.HasPrefix(line, "--- /dev/null"):
		fileDiff = &FileDiff{}
	case strings.HasPrefix(line, "rename to "):
		newRelPath := strings.TrimPrefix(line, "rename to ")
		fileDiff.NewRelPath = newRelPath
	case strings.HasPrefix(line, "+++ b/"):
		newRelPath := strings.TrimPrefix(line, "+++ b/")
		fileDiff.NewRelPath = newRelPath
	}

	return fileDiff
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

// ToOldLine translates a line number from the modified file to its corresponding
// line number in the original file. If the line was newly added or directly modified,
// it returns -1.
func (fd FileDiff) ToOldLine(newLine int) int {
	oldLine := newLine
	for _, hunk := range fd.Hunks {
		if newLine < hunk.NewStart || (hunk.NewCount == 0 && newLine == hunk.NewStart) {
			break
		}

		if newLine < hunk.NewStart+hunk.NewCount {
			return -1
		}

		oldLine -= hunk.NewCount - hunk.OldCount
	}

	return oldLine
}

// IsAddition returns true if the given line number falls within a newly added
// or modified block of code in this file diff.
func (fd FileDiff) IsAddition(line int) bool {
	isAddition := slices.ContainsFunc(fd.Hunks, func(hunk Hunk) bool {
		return line >= hunk.NewStart && line < hunk.NewStart+hunk.NewCount
	})

	return isAddition
}
