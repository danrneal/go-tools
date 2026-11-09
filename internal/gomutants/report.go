// Package gomutants parses and filters Stryker HTML mutation testing reports using ignore files.
package gomutants

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"slices"
)

const (
	// scriptOpenTag marks the opening HTML script tag containing the embedded JSON report.
	scriptOpenTag = `<script id="mutation-report-data" type="application/json">`
	// scriptCloseTag marks the closing HTML script tag following the embedded JSON report.
	scriptCloseTag = `</script>`
)

// enabledFiltersPattern matches the viewer's enabled filter array.
var enabledFiltersPattern = regexp.MustCompile(`enabled:\[[^\]]*\]`)

// Report represents a Stryker mutation testing HTML report and its surrounding HTML chrome.
type Report struct {
	SchemaVersion string          `json:"schemaVersion"`
	Thresholds    thresholds      `json:"thresholds"`
	ProjectRoot   string          `json:"projectRoot,omitempty"`
	Framework     *framework      `json:"framework,omitempty"`
	Files         map[string]file `json:"files"`
	prefix        []byte
	suffix        []byte
}

// thresholds defines the high and low mutation score percentage boundaries in the report.
type thresholds struct {
	High int `json:"high"`
	Low  int `json:"low"`
}

// framework identifies the mutation testing tool and version that generated the report.
type framework struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

// file contains the source text, programming language, and discovered mutants for a single file.
type file struct {
	Language string   `json:"language"`
	Source   string   `json:"source"`
	Mutants  []mutant `json:"mutants"`
}

// mutant describes an individual code mutation, its source location, and its test outcome status.
type mutant struct {
	ID          string   `json:"id"`
	MutatorName string   `json:"mutatorName"`
	Location    location `json:"location"`
	Status      string   `json:"status"`
	Replacement string   `json:"replacement,omitempty"`
}

// location specifies the inclusive start and end source positions of a mutant.
type location struct {
	Start position `json:"start"`
	End   position `json:"end"`
}

// position represents a 1-indexed line and column offset within a source file.
type position struct {
	Line   int `json:"line"`
	Column int `json:"column"`
}

// ParseHTMLReport reads an HTML mutation report from path and unmarshals its embedded JSON payload.
func ParseHTMLReport(path string) (*Report, error) {
	htmlReport, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read HTML report: %w", err)
	}

	prefix, jsonReport, _ := bytes.Cut(htmlReport, []byte(scriptOpenTag))
	jsonReport, suffix, ok := bytes.Cut(jsonReport, []byte(scriptCloseTag))
	if !ok {
		return nil, errors.New("failed to find mutation-report-data script tag")
	}

	var report Report
	report.prefix = prefix
	report.suffix = suffix

	err = json.Unmarshal(jsonReport, &report)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal report: %w", err)
	}

	return &report, nil
}

// SurvivedMutations extracts and returns the set of all mutations that currently have a Survived status.
func (r *Report) SurvivedMutations() Mutations {
	mutations := make(Mutations)
	for path, file := range r.Files {
		for _, mutant := range file.Mutants {
			if mutant.Status != "Survived" {
				continue
			}

			mutation := Mutation{
				path:        path,
				mutatorName: mutant.MutatorName,
				line:        mutant.Location.Start.Line,
				column:      mutant.Location.Start.Column,
			}

			mutations[mutation] = true
		}
	}

	return mutations
}

// Ignore updates the status of any surviving mutants matching the provided mutations set to Ignored.
func (r *Report) Ignore(mutations Mutations) {
	for path, file := range r.Files {
		for i, mutant := range file.Mutants {
			if mutant.Status != "Survived" {
				continue
			}

			mutation := Mutation{
				path:        path,
				mutatorName: mutant.MutatorName,
				line:        mutant.Location.Start.Line,
				column:      mutant.Location.Start.Column,
			}

			if !mutations[mutation] {
				continue
			}

			mutant.Status = "Ignored"
			file.Mutants[i] = mutant
		}
	}
}

// Write marshals the updated report JSON and writes the reassembled HTML document to path.
func (r *Report) Write(path string) error {
	jsonReport, err := json.Marshal(r)
	if err != nil {
		return fmt.Errorf("failed to marshal report: %w", err)
	}

	filters := enabledFiltersPattern.Find(r.prefix)
	filters = bytes.ReplaceAll(filters, []byte(",`NoCoverage`"), nil)
	filters = bytes.ReplaceAll(filters, []byte(",`Timeout`"), nil)
	prefix := enabledFiltersPattern.ReplaceAllLiteral(r.prefix, filters)

	htmlReport := slices.Concat(prefix, []byte(scriptOpenTag), jsonReport, []byte(scriptCloseTag), r.suffix)

	if err := os.WriteFile(path, htmlReport, 0o600); err != nil {
		return fmt.Errorf("failed to write HTML report: %w", err)
	}

	return nil
}
