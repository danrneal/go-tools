package mutesting

import (
	"bufio"
	"cmp"
	"errors"
	"fmt"
	"io"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/danrneal/go-tools/internal/git"
)

// mutatorPattern is a regex used to parse individual mutations from the ignore file.
var mutatorPattern = regexp.MustCompile(`^([^:]+):(\d+):([^:]+)$`)

// IgnoreFile represents the state of ignored mutations, tracking the commit they were last synced with.
type IgnoreFile struct {
	LastSyncedCommit string
	Mutations        map[Mutation]bool
}

// ParseIgnoreFile reads and parses an ignore file, extracting the last synced commit and ignored mutations.
func ParseIgnoreFile(r io.Reader) (*IgnoreFile, error) {
	ignoreFile := &IgnoreFile{
		Mutations: map[Mutation]bool{},
	}

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if lastSyncedCommit, ok := strings.CutPrefix(line, "# Last-Synced-Commit:"); ok {
			lastSyncedCommit = strings.TrimSpace(lastSyncedCommit)
			if lastSyncedCommit == "" {
				return nil, errors.New("missing commit hash in Last-Synced-Commit header")
			}

			ignoreFile.LastSyncedCommit = lastSyncedCommit
			continue
		}

		matches := mutatorPattern.FindStringSubmatch(line)
		if matches == nil {
			continue
		}

		startLine, _ := strconv.Atoi(matches[2])

		mutation := Mutation{
			Name:      matches[3],
			RelPath:   matches[1],
			StartLine: startLine,
		}

		ignoreFile.Mutations[mutation] = true
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to scan ignore file: %w", err)
	}

	return ignoreFile, nil
}

// Update refreshes the ignore file's state by applying file diffs to shift line numbers and prune invalid mutations.
func (i *IgnoreFile) Update(
	ignoreMutations map[Mutation]bool,
	combinedDiff git.CombinedDiff,
	mutations map[Mutation][]string,
	commit string,
) {
	updatedMutations := map[Mutation]bool{}
	for mutation := range i.Mutations {
		if !ignoreMutations[mutation] {
			updatedMutations[mutation] = true
		} else {
			if fileDiff, ok := combinedDiff.FromFile[mutation.RelPath]; ok {
				mutation.StartLine = fileDiff.ToNewLine(mutation.StartLine)
				mutation.RelPath = fileDiff.NewRelPath
			}

			if _, ok := mutations[mutation]; ok {
				updatedMutations[mutation] = true
			}
		}
	}

	i.LastSyncedCommit = commit
	i.Mutations = updatedMutations
}

// WriteIgnoreFile writes the updated ignore configuration, including the commit header and sorted mutations,
// to the specified writer.
func (i *IgnoreFile) WriteIgnoreFile(w io.Writer) error {
	if _, err := fmt.Fprintf(w, "# Last-Synced-Commit: %s\n", i.LastSyncedCommit); err != nil {
		return fmt.Errorf("failed to write header: %w", err)
	}

	if _, err := io.WriteString(w, "# format: filepath:line:mutatorName\n\n"); err != nil {
		return fmt.Errorf("failed to write format instruction: %w", err)
	}

	mutations := slices.Collect(maps.Keys(i.Mutations))
	slices.SortFunc(mutations, compareMutation)

	for _, mutation := range mutations {
		if _, err := fmt.Fprintf(w, "%s:%d:%s\n", mutation.RelPath, mutation.StartLine, mutation.Name); err != nil {
			return fmt.Errorf("failed to write mutation: %w", err)
		}
	}

	return nil
}

func compareMutation(a, b Mutation) int {
	if c := cmp.Compare(a.RelPath, b.RelPath); c != 0 {
		return c
	}

	if c := cmp.Compare(a.StartLine, b.StartLine); c != 0 {
		return c
	}

	return cmp.Compare(a.Name, b.Name)
}
