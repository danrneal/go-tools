package mutesting

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestClient_GenerateMutations(t *testing.T) {
	t.Parallel()

	mockErr := errors.New("command failed")

	tests := []struct {
		name             string
		disabledMutators []string
		runMock          *runMock
		report           string
		want             map[Mutation][]string
		wantErr          bool
		errTarget        error
	}{
		{
			name:             "success with empty escaped mutants",
			disabledMutators: []string{"foo", "bar"},
			runMock: &runMock{
				wantEnv:  []string{"PATH", "GO_BIN"},
				wantArgs: []string{"--disable=foo", "--disable=bar", "./..."},
			},
			report: `
				{
					"escaped": []
				}
			`,
			want:    map[Mutation][]string{},
			wantErr: false,
		},
		{
			name: "success with populated escaped mutants",
			runMock: &runMock{
				wantEnv:  []string{"PATH", "GO_BIN"},
				wantArgs: []string{"./..."},
			},
			report: `
				{
					"escaped": [
						{
							"mutator": {
								"mutatorName": "testMutator",
								"originalFilePath": "main.go",
								"originalStartLine": 42
							},
							"processOutput": "some long output\nwith multiple lines\nand a checksum: abcdef123"
						}
					]
				}
			`,
			want: map[Mutation][]string{
				{
					Name:      "testMutator",
					RelPath:   "main.go",
					StartLine: 42,
				}: {"abcdef123"},
			},
			wantErr: false,
		},
		{
			name: "go-mutesting pre-run fails",
			runMock: &runMock{
				wantEnv:  []string{"PATH", "GO_BIN"},
				wantArgs: []string{"./..."},
				err:      mockErr,
			},
			report:    "",
			want:      nil,
			wantErr:   true,
			errTarget: mockErr,
		},
		{
			name: "ParseReport fails",
			runMock: &runMock{
				wantEnv:  []string{"PATH", "GO_BIN"},
				wantArgs: []string{"./..."},
			},
			report:    "",
			want:      nil,
			wantErr:   true,
			errTarget: fs.ErrNotExist,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()

			if tt.report != "" {
				reportPath := filepath.Join(dir, "report.json")
				if err := os.WriteFile(reportPath, []byte(trimIndent(tt.report)), 0o644); err != nil {
					t.Fatalf("failed to write mock report.json: %v", err)
				}
			}

			client := newMockClient(t, tt.runMock, WithDir(dir))

			got, err := client.GenerateMutations(t.Context(), tt.disabledMutators)

			if (err != nil) != tt.wantErr {
				t.Fatalf("GenerateMutations() error = %v, wantErr %v", err, tt.wantErr)
			}

			if tt.errTarget != nil && !errors.Is(err, tt.errTarget) {
				t.Fatalf("GenerateMutations() error = %v, does not match target %v", err, tt.errTarget)
			}

			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("GenerateMutations() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestClient_ParseReport(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		report    string
		want      map[Mutation][]string
		wantErr   bool
		errTarget error
	}{
		{
			name: "successfully parses valid report with multiple checksums",
			report: `
				{
                   "escaped": [
                       {
                           "mutator": {
                               "mutatorName": "branch/if",
                               "originalFilePath": "main.go",
                               "originalStartLine": 10
                           },
                           "processOutput": "PASS \"/tmp/path\" with checksum 123abc456def"
                       },
                       {
                           "mutator": {
                               "mutatorName": "branch/if",
                               "originalFilePath": "main.go",
                               "originalStartLine": 10
                           },
                           "processOutput": "PASS \"/tmp/path\" with checksum 789ghi012jkl"
                       }
                   ]
               }
			`,
			want: map[Mutation][]string{
				{
					Name:      "branch/if",
					RelPath:   "main.go",
					StartLine: 10,
				}: {"123abc456def", "789ghi012jkl"},
			},
			wantErr: false,
		},
		{
			name:      "report.json is missing",
			report:    "",
			want:      nil,
			wantErr:   true,
			errTarget: fs.ErrNotExist,
		},
		{
			name:    "JSON decoding fails",
			report:  `{ "invalid": json }`,
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()

			if tt.report != "" {
				reportPath := filepath.Join(dir, "report.json")
				if err := os.WriteFile(reportPath, []byte(trimIndent(tt.report)), 0o644); err != nil {
					t.Fatalf("failed to write mock report.json: %v", err)
				}
			}

			client := &Client{
				dir: dir,
			}

			got, err := client.ParseReport()

			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseReport() error = %v, wantErr %v", err, tt.wantErr)
			}

			if tt.errTarget != nil && !errors.Is(err, tt.errTarget) {
				t.Errorf("ParseReport() error = %v, does not match target %v", err, tt.errTarget)
			}

			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("ParseReport() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
