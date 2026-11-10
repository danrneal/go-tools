package gomutants

import (
	"bufio"
	"cmp"
	"fmt"
	"io"
	"maps"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/danrneal/go-tools/internal/git"
)

// mutatorPattern is a regex used to parse individual mutations from the ignore file.
var mutatorPattern = regexp.MustCompile(`^([^:]+):(\d+):(\d+):([^:]+)$`)

// IgnoreFile represents the state of ignored mutations.
type IgnoreFile struct {
	Mutations Mutations
}

// Mutations represents a set of unique source code mutations.
type Mutations map[Mutation]bool

// Mutation defines the attributes of a single source code mutation, including its file path,
// mutator name, line, and column.
type Mutation struct {
	path        string
	mutatorName string
	line        int
	column      int
}

// ParseIgnoreFile reads and parses an ignore file, extracting the ignored mutations.
func ParseIgnoreFile(r io.Reader) (*IgnoreFile, error) {
	ignoreFile := &IgnoreFile{
		Mutations: make(Mutations),
	}

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		matches := mutatorPattern.FindStringSubmatch(line)
		if matches == nil {
			continue
		}

		//nolint:errcheck // mutatorPattern guarantees digits
		mutationLine, _ := strconv.Atoi(matches[2])
		//nolint:errcheck // mutatorPattern guarantees digits
		mutationColumn, _ := strconv.Atoi(matches[3])

		mutation := Mutation{
			path:        matches[1],
			mutatorName: matches[4],
			line:        mutationLine,
			column:      mutationColumn,
		}

		ignoreFile.Mutations[mutation] = true
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to scan ignore file: %w", err)
	}

	return ignoreFile, nil
}

// Shift applies a git diff to the ignore file, dynamically recalculating the
// starting line numbers and file paths of all recorded mutations to match the new state.
func (i *IgnoreFile) Shift(combinedDiff git.CombinedDiff) {
	updatedMutations := make(Mutations, len(i.Mutations))
	for mutation := range i.Mutations {
		if fileDiff, ok := combinedDiff.FromFile[mutation.path]; ok {
			mutation.line = fileDiff.ToNewLine(mutation.line)
			mutation.path = fileDiff.NewPath
		}

		updatedMutations[mutation] = true
	}

	i.Mutations = updatedMutations
}

// Filter prunes the ignore file, safely removing any recorded mutations that
// no longer exist in the provided list of currently valid mutations.
func (i *IgnoreFile) Filter(mutations Mutations) {
	filteredMutations := make(Mutations, len(i.Mutations))
	for mutation := range i.Mutations {
		if mutations[mutation] {
			filteredMutations[mutation] = true
		}
	}

	i.Mutations = filteredMutations
}

// Write writes the updated ignore configuration, including sorted mutations, to the specified path.
func (i *IgnoreFile) Write(path string) error {
	mutations := slices.Collect(maps.Keys(i.Mutations))
	slices.SortFunc(mutations, compareMutation)

	data := []byte("# format: path:line:column:mutatorName\n\n")
	for _, mutation := range mutations {
		data = fmt.Appendf(
			data,
			"%s:%d:%d:%s\n",
			mutation.path,
			mutation.line,
			mutation.column,
			mutation.mutatorName,
		)
	}

	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("failed to write ignore file: %w", err)
	}

	return nil
}

// compareMutation provides a deterministic sorting order for Mutation objects.
// It sorts by file path first, then by start line, then by column, and finally by mutation name.
func compareMutation(a, b Mutation) int {
	if c := cmp.Compare(a.path, b.path); c != 0 {
		return c
	}

	if c := cmp.Compare(a.line, b.line); c != 0 {
		return c
	}

	if c := cmp.Compare(a.column, b.column); c != 0 {
		return c
	}

	return cmp.Compare(a.mutatorName, b.mutatorName)
}
