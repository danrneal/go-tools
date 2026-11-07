package gomutants

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/danrneal/go-tools/internal/git"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestParseIgnoreFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
		want    *IgnoreFile
		wantErr error
	}{
		{
			name:    "empty file returns empty ignore struct",
			content: "",
			want: &IgnoreFile{
				Mutations: map[Mutation]bool{},
			},
		},
		{
			name: "invalid lines are safely ignored and loop continues",
			content: `
				some random text that is not a comment or match
				another invalid line
				main.go:10:5:test/mutator
			`,
			want: &IgnoreFile{
				Mutations: map[Mutation]bool{
					{
						path:        "main.go",
						mutatorName: "test/mutator",
						line:        10,
						column:      5,
					}: true,
				},
			},
		},
		{
			name: "successfully parses valid ignore file with mutations",
			content: `
				# format: path:line:column:mutatorName

				main.go:10:5:branch/if
				internal/utils.go:42:8:condition/negated
			`,
			want: &IgnoreFile{
				Mutations: map[Mutation]bool{
					{
						path:        "main.go",
						mutatorName: "branch/if",
						line:        10,
						column:      5,
					}: true,
					{
						path:        "internal/utils.go",
						mutatorName: "condition/negated",
						line:        42,
						column:      8,
					}: true,
				},
			},
		},
		{
			name:    "returns error when scanner fails",
			content: strings.Repeat("a", bufio.MaxScanTokenSize),
			want:    nil,
			wantErr: bufio.ErrTooLong,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			reader := strings.NewReader(trimIndent(tt.content))
			got, err := ParseIgnoreFile(reader)

			if diff := cmp.Diff(tt.wantErr, err, cmpopts.EquateErrors()); diff != "" {
				t.Fatalf("ParseIgnoreFile() error mismatch (-want +got):\n%s", diff)
			}

			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("ParseIgnoreFile() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestIgnoreFile_Shift(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		ignoreFile   *IgnoreFile
		combinedDiff git.CombinedDiff
		want         *IgnoreFile
	}{
		{
			name: "empty inputs result in empty mutations",
			ignoreFile: &IgnoreFile{
				Mutations: map[Mutation]bool{},
			},
			combinedDiff: git.CombinedDiff{},
			want: &IgnoreFile{
				Mutations: map[Mutation]bool{},
			},
		},
		{
			name: "mutation is retained unmodified when not in diff",
			ignoreFile: &IgnoreFile{
				Mutations: map[Mutation]bool{
					{
						path:        "main.go",
						mutatorName: "branch/if",
						line:        10,
						column:      5,
					}: true,
				},
			},
			combinedDiff: git.CombinedDiff{},
			want: &IgnoreFile{
				Mutations: map[Mutation]bool{
					{
						path:        "main.go",
						mutatorName: "branch/if",
						line:        10,
						column:      5,
					}: true,
				},
			},
		},
		{
			name: "mutation is updated with new path and shifted line number",
			ignoreFile: &IgnoreFile{
				Mutations: map[Mutation]bool{
					{
						path:        "main.go",
						mutatorName: "branch/if",
						line:        10,
						column:      5,
					}: true,
				},
			},
			combinedDiff: git.CombinedDiff{
				FromFile: map[string]*git.FileDiff{
					"main.go": {
						OldPath: "main.go",
						NewPath: "new_main.go",
						Hunks: []git.Hunk{
							{
								OldStart: 5,
								OldCount: 0,
								NewStart: 5,
								NewCount: 2,
							},
						},
					},
				},
			},
			want: &IgnoreFile{
				Mutations: map[Mutation]bool{
					{
						path:        "new_main.go",
						mutatorName: "branch/if",
						line:        12,
						column:      5,
					}: true,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tt.ignoreFile.Shift(tt.combinedDiff)

			if diff := cmp.Diff(tt.want, tt.ignoreFile); diff != "" {
				t.Errorf("Ignore.Shift() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestIgnoreFile_Filter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		ignoreFile *IgnoreFile
		mutations  map[Mutation]bool
		want       *IgnoreFile
	}{
		{
			name: "empty inputs result in empty mutations",
			ignoreFile: &IgnoreFile{
				Mutations: map[Mutation]bool{},
			},
			mutations: map[Mutation]bool{},
			want: &IgnoreFile{
				Mutations: map[Mutation]bool{},
			},
		},
		{
			name: "mutation is dropped when it is missing from new mutations",
			ignoreFile: &IgnoreFile{
				Mutations: map[Mutation]bool{
					{
						path:        "main.go",
						mutatorName: "branch/if",
						line:        10,
						column:      5,
					}: true,
				},
			},
			mutations: map[Mutation]bool{},
			want: &IgnoreFile{
				Mutations: map[Mutation]bool{},
			},
		},
		{
			name: "mutation is retained when it exists in new mutations",
			ignoreFile: &IgnoreFile{
				Mutations: map[Mutation]bool{
					{
						path:        "main.go",
						mutatorName: "branch/if",
						line:        10,
						column:      5,
					}: true,
				},
			},
			mutations: map[Mutation]bool{
				{
					path:        "main.go",
					mutatorName: "branch/if",
					line:        10,
					column:      5,
				}: true,
			},
			want: &IgnoreFile{
				Mutations: map[Mutation]bool{
					{
						path:        "main.go",
						mutatorName: "branch/if",
						line:        10,
						column:      5,
					}: true,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tt.ignoreFile.Filter(tt.mutations)

			if diff := cmp.Diff(tt.want, tt.ignoreFile); diff != "" {
				t.Errorf("Ignore.Filter() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestIgnoreFile_Write(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		ignoreFile *IgnoreFile
		path       string
		wantOut    string
		wantErr    error
	}{
		{
			name: "empty mutations writes only headers",
			path: ".test-ignore",
			ignoreFile: &IgnoreFile{
				Mutations: map[Mutation]bool{},
			},
			wantOut: `
				# format: path:line:column:mutatorName

			`,
		},
		{
			name: "populated mutations are sorted by path, line, column, then name",
			path: ".test-ignore",
			ignoreFile: &IgnoreFile{
				Mutations: map[Mutation]bool{
					{
						path:        "internal/file.go",
						mutatorName: "b_mutator",
						line:        20,
						column:      8,
					}: true,
					{
						path:        "main.go",
						mutatorName: "c_mutator",
						line:        30,
						column:      12,
					}: true,
					{
						path:        "main.go",
						mutatorName: "a_mutator",
						line:        10,
						column:      9,
					}: true,
					{
						path:        "main.go",
						mutatorName: "a_mutator",
						line:        10,
						column:      5,
					}: true,
					{
						path:        "main.go",
						mutatorName: "mutator",
						line:        10,
						column:      5,
					}: true,
				},
			},
			wantOut: `
				# format: path:line:column:mutatorName

				internal/file.go:20:8:b_mutator
				main.go:10:5:a_mutator
				main.go:10:5:mutator
				main.go:10:9:a_mutator
				main.go:30:12:c_mutator
			`,
		},
		{
			name:       "returns error when file creation fails",
			path:       "",
			ignoreFile: &IgnoreFile{},
			wantErr:    syscall.EISDIR,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(t.TempDir(), tt.path)

			err := tt.ignoreFile.Write(path)
			if diff := cmp.Diff(tt.wantErr, err, cmpopts.EquateErrors()); diff != "" {
				t.Fatalf("Write() error mismatch (-want +got):\n%s", diff)
			}

			if tt.wantErr != nil {
				return
			}

			out, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("failed to read test ignore file: %v", err)
			}

			gotOut := string(out)
			if gotOut != trimIndent(tt.wantOut) {
				t.Errorf("IgnoreFile.Write() got = %v, want %v", gotOut, trimIndent(tt.wantOut))
			}
		})
	}
}

func Test_compareMutation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		a    Mutation
		b    Mutation
		want int
	}{
		{
			name: "different path",
			a: Mutation{
				path:        "a.go",
				mutatorName: "mut",
				line:        10,
				column:      5,
			},
			b: Mutation{
				path:        "b.go",
				mutatorName: "mut",
				line:        10,
				column:      5,
			},
			want: -1,
		},
		{
			name: "different start line",
			a: Mutation{
				path:        "a.go",
				mutatorName: "mut",
				line:        20,
				column:      5,
			},
			b: Mutation{
				path:        "a.go",
				mutatorName: "mut",
				line:        10,
				column:      5,
			},
			want: 1,
		},
		{
			name: "different column",
			a: Mutation{
				path:        "a.go",
				mutatorName: "mut",
				line:        10,
				column:      5,
			},
			b: Mutation{
				path:        "a.go",
				mutatorName: "mut",
				line:        10,
				column:      9,
			},
			want: -1,
		},
		{
			name: "different name",
			a: Mutation{
				path:        "a.go",
				mutatorName: "a_mut",
				line:        10,
				column:      5,
			},
			b: Mutation{
				path:        "a.go",
				mutatorName: "b_mut",
				line:        10,
				column:      5,
			},
			want: -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := compareMutation(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("compareMutation() = %v, want %v", got, tt.want)
			}
		})
	}
}

// trimIndent strips a leading newline and any leading horizontal whitespace from each line of s.
func trimIndent(s string) string {
	s = strings.TrimPrefix(s, "\n")

	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimLeft(line, "\t ")
	}

	return strings.Join(lines, "\n")
}
