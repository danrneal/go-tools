package gomutants

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

type syntaxError struct{}

func (syntaxError) Error() string {
	return "json syntax error"
}

func (syntaxError) Is(target error) bool {
	_, ok := errors.AsType[*json.SyntaxError](target)
	return ok
}

func TestParseHTMLReport(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		path       string
		htmlReport string
		want       *Report
		wantErr    error
	}{
		{
			name: "successfully parses valid HTML report",
			path: filepath.Join(t.TempDir(), "mutation-report.html"),
			htmlReport: `
				<html>
					<body>
						<script id="mutation-report-data" type="application/json">
							{
								"schemaVersion": "2",
								"thresholds": {
									"high": 80,
									"low": 60
								},
								"projectRoot": "/repo",
								"framework": {
									"name": "gomutants",
									"version": "0.7.2"
								},
								"files": {
									"main.go": {
										"language": "go",
										"source": "package main\n",
										"mutants": [
											{
												"id": "main.go:main:BRANCH_IF#0",
												"mutatorName": "BRANCH_IF",
												"location": {
													"start": {
														"line": 10,
														"column": 5
													},
													"end": {
														"line": 12,
														"column": 2
													}
												},
												"status": "Survived",
												"replacement": "{ _ = 0 }"
											}
										]
									}
								}
							}
						</script>
					</body>
				</html>
			`,
			want: &Report{
				SchemaVersion: "2",
				Thresholds: thresholds{
					High: 80,
					Low:  60,
				},
				ProjectRoot: "/repo",
				Framework: &framework{
					Name:    "gomutants",
					Version: "0.7.2",
				},
				Files: map[string]file{
					"main.go": {
						Language: "go",
						Source:   "package main\n",
						Mutants: []mutant{
							{
								ID:          "main.go:main:BRANCH_IF#0",
								MutatorName: "BRANCH_IF",
								Location: location{
									Start: position{
										Line:   10,
										Column: 5,
									},
									End: position{
										Line:   12,
										Column: 2,
									},
								},
								Status:      "Survived",
								Replacement: "{ _ = 0 }",
							},
						},
					},
				},
				prefix: []byte("<html>\n<body>\n"),
				suffix: []byte("\n</body>\n</html>\n"),
			},
		},
		{
			name:    "returns error when HTML report is missing",
			path:    filepath.Join(t.TempDir(), "missing.html"),
			wantErr: fs.ErrNotExist,
		},
		{
			name:       "returns error when opening script tag is missing",
			path:       filepath.Join(t.TempDir(), "mutation-report.html"),
			htmlReport: `<html><body></body></html>`,
			wantErr:    cmpopts.AnyError,
		},
		{
			name:       "returns error when closing script tag is missing",
			path:       filepath.Join(t.TempDir(), "mutation-report.html"),
			htmlReport: "<html><body>" + scriptOpenTag + "{}",
			wantErr:    cmpopts.AnyError,
		},
		{
			name: "returns error when embedded JSON is invalid",
			path: filepath.Join(t.TempDir(), "mutation-report.html"),
			htmlReport: `
				<html>
					<body>
						<script id="mutation-report-data" type="application/json">
							{ "invalid": json }
						</script>
					</body>
				</html>
			`,
			wantErr: syntaxError{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if tt.htmlReport != "" {
				data := []byte(trimIndent(tt.htmlReport))
				if err := os.WriteFile(tt.path, data, 0o600); err != nil {
					t.Fatalf("failed to write HTML report: %v", err)
				}
			}

			got, err := ParseHTMLReport(tt.path)
			if diff := cmp.Diff(tt.wantErr, err, cmpopts.EquateErrors()); diff != "" {
				t.Fatalf("ParseHTMLReport() error mismatch (-want +got):\n%s", diff)
			}

			if tt.wantErr != nil {
				return
			}

			opts := cmp.AllowUnexported(Report{})
			if diff := cmp.Diff(tt.want, got, opts); diff != "" {
				t.Errorf("ParseHTMLReport() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestReport_SurvivedMutations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		report *Report
		want   Mutations
	}{
		{
			name: "returns empty mutations when report has no files",
			report: &Report{
				Files: map[string]file{},
			},
			want: Mutations{},
		},
		{
			name: "returns empty mutations when file has no mutants",
			report: &Report{
				Files: map[string]file{
					"main.go": {
						Mutants: []mutant{},
					},
				},
			},
			want: Mutations{},
		},
		{
			name: "returns survived mutation",
			report: &Report{
				Files: map[string]file{
					"main.go": {
						Mutants: []mutant{
							{
								MutatorName: "ARITHMETIC_BASE",
								Location: location{
									Start: position{
										Line:   5,
										Column: 3,
									},
								},
								Status: "Killed",
							},
							{
								MutatorName: "BRANCH_IF",
								Location: location{
									Start: position{
										Line:   10,
										Column: 5,
									},
								},
								Status: "Survived",
							},
						},
					},
				},
			},
			want: Mutations{
				{
					path:        "main.go",
					mutatorName: "BRANCH_IF",
					line:        10,
					column:      5,
				}: true,
			},
		},
		{
			name: "skips killed mutation",
			report: &Report{
				Files: map[string]file{
					"main.go": {
						Mutants: []mutant{
							{
								MutatorName: "BRANCH_IF",
								Location: location{
									Start: position{
										Line:   10,
										Column: 5,
									},
								},
								Status: "Killed",
							},
						},
					},
				},
			},
			want: Mutations{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.report.SurvivedMutations()

			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("Report.SurvivedMutations() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestReport_Ignore(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		report    *Report
		mutations Mutations
		want      *Report
	}{
		{
			name: "report with no files remains unchanged",
			report: &Report{
				Files: map[string]file{},
			},
			mutations: Mutations{},
			want: &Report{
				Files: map[string]file{},
			},
		},
		{
			name: "file with no mutants remains unchanged",
			report: &Report{
				Files: map[string]file{
					"main.go": {
						Mutants: []mutant{},
					},
				},
			},
			mutations: Mutations{},
			want: &Report{
				Files: map[string]file{
					"main.go": {
						Mutants: []mutant{},
					},
				},
			},
		},
		{
			name: "survived mutant in mutations is marked ignored",
			report: &Report{
				Files: map[string]file{
					"main.go": {
						Mutants: []mutant{
							{
								MutatorName: "ARITHMETIC_BASE",
								Location: location{
									Start: position{
										Line:   5,
										Column: 3,
									},
								},
								Status: "Killed",
							},
							{
								MutatorName: "CONDITIONAL_NEGATION",
								Location: location{
									Start: position{
										Line:   7,
										Column: 4,
									},
								},
								Status: "Survived",
							},
							{
								MutatorName: "BRANCH_IF",
								Location: location{
									Start: position{
										Line:   10,
										Column: 5,
									},
								},
								Status: "Survived",
							},
						},
					},
				},
			},
			mutations: Mutations{
				{
					path:        "main.go",
					mutatorName: "BRANCH_IF",
					line:        10,
					column:      5,
				}: true,
			},
			want: &Report{
				Files: map[string]file{
					"main.go": {
						Mutants: []mutant{
							{
								MutatorName: "ARITHMETIC_BASE",
								Location: location{
									Start: position{
										Line:   5,
										Column: 3,
									},
								},
								Status: "Killed",
							},
							{
								MutatorName: "CONDITIONAL_NEGATION",
								Location: location{
									Start: position{
										Line:   7,
										Column: 4,
									},
								},
								Status: "Survived",
							},
							{
								MutatorName: "BRANCH_IF",
								Location: location{
									Start: position{
										Line:   10,
										Column: 5,
									},
								},
								Status: "Ignored",
							},
						},
					},
				},
			},
		},
		{
			name: "killed mutant remains unchanged",
			report: &Report{
				Files: map[string]file{
					"main.go": {
						Mutants: []mutant{
							{
								MutatorName: "BRANCH_IF",
								Location: location{
									Start: position{
										Line:   10,
										Column: 5,
									},
								},
								Status: "Killed",
							},
						},
					},
				},
			},
			mutations: Mutations{
				{
					path:        "main.go",
					mutatorName: "BRANCH_IF",
					line:        10,
					column:      5,
				}: true,
			},
			want: &Report{
				Files: map[string]file{
					"main.go": {
						Mutants: []mutant{
							{
								MutatorName: "BRANCH_IF",
								Location: location{
									Start: position{
										Line:   10,
										Column: 5,
									},
								},
								Status: "Killed",
							},
						},
					},
				},
			},
		},
		{
			name: "survived mutant not in mutations remains unchanged",
			report: &Report{
				Files: map[string]file{
					"main.go": {
						Mutants: []mutant{
							{
								MutatorName: "BRANCH_IF",
								Location: location{
									Start: position{
										Line:   10,
										Column: 5,
									},
								},
								Status: "Survived",
							},
						},
					},
				},
			},
			mutations: Mutations{},
			want: &Report{
				Files: map[string]file{
					"main.go": {
						Mutants: []mutant{
							{
								MutatorName: "BRANCH_IF",
								Location: location{
									Start: position{
										Line:   10,
										Column: 5,
									},
								},
								Status: "Survived",
							},
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tt.report.Ignore(tt.mutations)

			opts := cmp.AllowUnexported(Report{})
			if diff := cmp.Diff(tt.want, tt.report, opts); diff != "" {
				t.Errorf("Report.Ignore() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestReport_Write(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		report  *Report
		path    string
		wantOut string
		wantErr error
	}{
		{
			name: "successfully writes HTML report",
			path: "mutation-report.html",
			report: &Report{
				SchemaVersion: "2",
				Thresholds: thresholds{
					High: 80,
					Low:  60,
				},
				Files: map[string]file{},
				prefix: []byte(
					"<html><script>" +
						"enabled:[...this.selectedMutantStates,`Survived`,`NoCoverage`,`Timeout`]" +
						"</script><body>",
				),
				suffix: []byte("</body></html>"),
			},
			wantOut: `<html><script>` +
				"enabled:[...this.selectedMutantStates,`Survived`]" +
				`</script><body>` +
				scriptOpenTag +
				`{"schemaVersion":"2","thresholds":{"high":80,"low":60},"files":{}}` +
				scriptCloseTag +
				`</body></html>`,
		},
		{
			name:    "returns error when file write fails",
			path:    "",
			report:  &Report{},
			wantErr: syscall.EISDIR,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(t.TempDir(), tt.path)

			err := tt.report.Write(path)
			if diff := cmp.Diff(tt.wantErr, err, cmpopts.EquateErrors()); diff != "" {
				t.Fatalf("Write() error mismatch (-want +got):\n%s", diff)
			}

			if tt.wantErr != nil {
				return
			}

			out, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("failed to read test HTML report: %v", err)
			}

			gotOut := string(out)
			if gotOut != tt.wantOut {
				t.Errorf("Report.Write() got = %v, want %v", gotOut, tt.wantOut)
			}
		})
	}
}
