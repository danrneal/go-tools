package mutesting

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Report represents the overall mutation testing results, containing a list of escaped mutants.
type Report struct {
	Escaped []Mutant `json:"escaped"`
}

// Mutant describes a specific code mutation and the output produced when testing it.
type Mutant struct {
	Mutation      Mutation `json:"mutator"`
	ProcessOutput string   `json:"processOutput"`
}

// Mutation defines the attributes of a single source code mutation, such as the mutator name, file path,
// and start line.
type Mutation struct {
	Name      string `json:"mutatorName"`
	RelPath   string `json:"originalFilePath"`
	StartLine int    `json:"originalStartLine"`
}

// GenerateMutations executes a fast mutation testing run without executing the test suite
// to generate the base report.json containing all potential mutations.
func (c *Client) GenerateMutations(ctx context.Context, disabledMutators []string) (map[Mutation][]string, error) {
	env, cleanup, err := setupGoTestWrapper("exit 0")
	if err != nil {
		return nil, err
	}

	defer cleanup()

	disableFlags := make([]string, 0, len(disabledMutators))
	for _, disabledMutator := range disabledMutators {
		disableFlag := fmt.Sprintf("--disable=%s", disabledMutator)
		disableFlags = append(disableFlags, disableFlag)
	}

	args := slices.Concat(disableFlags, []string{"./..."})

	if _, err = c.run(ctx, env, args...); err != nil {
		return nil, fmt.Errorf("failed to run go-mutesting pre-run: %w", err)
	}

	mutations, err := c.ParseReport()
	if err != nil {
		return nil, err
	}

	return mutations, nil
}

// ParseReport reads the report.json file from the work directory and decodes it,
// returning a map of escaped mutations and their associated checksums.
func (c *Client) ParseReport() (map[Mutation][]string, error) {
	reportPath := filepath.Join(c.dir, "report.json")
	reportFile, err := os.Open(reportPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open report.json: %w", err)
	}

	defer reportFile.Close()

	report := Report{}
	decoder := json.NewDecoder(reportFile)
	if err = decoder.Decode(&report); err != nil {
		return nil, fmt.Errorf("failed to decode JSON report: %w", err)
	}

	mutations := map[Mutation][]string{}
	for _, mutant := range report.Escaped {
		fields := strings.Fields(mutant.ProcessOutput)
		checksum := fields[len(fields)-1]
		mutations[mutant.Mutation] = append(mutations[mutant.Mutation], checksum)
	}

	return mutations, nil
}
