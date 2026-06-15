package mutesting

import (
	"context"
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
				wantArgs: []string{"--disable=foo", "--disable=bar", "--exec", "false", "./..."},
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
				wantArgs: []string{"--exec", "false", "./..."},
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
				wantArgs: []string{"--exec", "false", "./..."},
				err:      mockErr,
			},
			report:    "",
			want:      nil,
			wantErr:   true,
			errTarget: mockErr,
		},
		{
			name: "report.json is missing",
			runMock: &runMock{
				wantArgs: []string{"--exec", "false", "./..."},
			},
			report:    "",
			want:      nil,
			wantErr:   true,
			errTarget: fs.ErrNotExist,
		},
		{
			name: "JSON decoding fails",
			runMock: &runMock{
				wantArgs: []string{"--exec", "false", "./..."},
			},
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

			client := newMockClient(t, tt.runMock)
			client.dir = dir

			got, err := client.GenerateMutations(context.Background(), tt.disabledMutators)

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
