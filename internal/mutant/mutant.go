package mutant

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/danrneal/go-tools/internal/diff"
)

type Report struct {
	Escaped []Mutant `json:"escaped"`
}

type Mutant struct {
	Mutator mutator `json:"mutator"`
}

type mutator struct {
	MutatorName string `json:"mutatorName"`
	FilePath    string `json:"originalFilePath"`
	StartLine   int    `json:"originalStartLine"`
}

func ParseReport(r io.Reader) (*Report, error) {
	var report Report
	decoder := json.NewDecoder(r)
	if err := decoder.Decode(&report); err != nil {
		return nil, fmt.Errorf("failed to decode JSON report: %w", err)
	}

	return &report, nil
}

func Diff(baseMutants, mutants []Mutant, fileDiffs map[string]diff.FileDiff) map[string][]bool {
	oldMutators := map[mutator]bool{}
	for _, baseMutant := range baseMutants {
		originalFilePath := baseMutant.Mutator.FilePath
		originalStartLine := baseMutant.Mutator.StartLine
		newStartLine := originalStartLine
		if fileDiff, ok := fileDiffs[originalFilePath]; ok {
			newStartLine = fileDiff.ToNewLine(originalStartLine)
		}

		if newStartLine == -1 {
			continue
		}

		oldMutator := mutator{
			MutatorName: baseMutant.Mutator.MutatorName,
			FilePath:    originalFilePath, // To be updated to newFilePath
			StartLine:   newStartLine,
		}

		oldMutators[oldMutator] = true
	}

	newMutants := map[string][]bool{}
	for _, mutant := range mutants {
		_, ok := oldMutators[mutant.Mutator]
		filePath := mutant.Mutator.FilePath
		newMutants[filePath] = append(newMutants[filePath], !ok)
	}

	return newMutants
}
