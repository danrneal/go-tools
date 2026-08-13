package mutesting

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
var mutatorPattern = regexp.MustCompile(`^([^:]+):(\d+):([^:]+)$`)

// IgnoreFile represents the state of ignored mutations.
type IgnoreFile struct {
	Mutations map[Mutation]bool
}

// ParseIgnoreFile reads and parses an ignore file, extracting the ignored mutations.
func ParseIgnoreFile(r io.Reader) (*IgnoreFile, error) {
	ignoreFile := &IgnoreFile{
		Mutations: map[Mutation]bool{},
	}

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
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

// Shift applies a git diff to the ignore file, dynamically recalculating the
// starting line numbers and file paths of all recorded mutations to match the new state.
func (i *IgnoreFile) Shift(combinedDiff git.CombinedDiff) {
	updatedMutations := map[Mutation]bool{}
	for mutation := range i.Mutations {
		if fileDiff, ok := combinedDiff.FromFile[mutation.RelPath]; ok {
			mutation.StartLine = fileDiff.ToNewLine(mutation.StartLine)
			mutation.RelPath = fileDiff.NewRelPath
		}

		updatedMutations[mutation] = true
	}

	i.Mutations = updatedMutations
}

// Filter prunes the ignore file, safely removing any recorded mutations that
// no longer exist in the provided list of currently valid mutations.
func (i *IgnoreFile) Filter(mutations map[Mutation][]string) {
	filteredMutations := map[Mutation]bool{}
	for mutation := range i.Mutations {
		if _, ok := mutations[mutation]; ok {
			filteredMutations[mutation] = true
		}
	}

	i.Mutations = filteredMutations
}

// WriteIgnoreFile writes the updated ignore configuration, including sorted mutations, to the specified writer.
func (i *IgnoreFile) WriteIgnoreFile(path string) error {
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to open ignore file for writing: %w", err)
	}

	defer file.Close()

	if _, err := file.WriteString("# format: filepath:line:mutatorName\n\n"); err != nil {
		return fmt.Errorf("failed to write format instruction: %w", err)
	}

	mutations := slices.Collect(maps.Keys(i.Mutations))
	slices.SortFunc(mutations, compareMutation)

	for _, mutation := range mutations {
		if _, err := fmt.Fprintf(file, "%s:%d:%s\n", mutation.RelPath, mutation.StartLine, mutation.Name); err != nil {
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
