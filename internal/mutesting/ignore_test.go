package mutesting

import (
	"bytes"
	"strings"
	"testing"

	"github.com/danrneal/go-tools/internal/git"
	"github.com/google/go-cmp/cmp"
)

func TestParseIgnoreFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
		want    *IgnoreFile
		wantErr bool
	}{
		{
			name:    "empty file returns empty ignore struct",
			content: "",
			want: &IgnoreFile{
				Mutations: map[Mutation]bool{},
			},
			wantErr: false,
		},
		{
			name: "invalid lines are safely ignored and loop continues",
			content: `
				some random text that is not a comment or match
				another invalid line
				main.go:10:test/mutator
			`,
			want: &IgnoreFile{
				Mutations: map[Mutation]bool{
					{
						Name:      "test/mutator",
						RelPath:   "main.go",
						StartLine: 10,
					}: true,
				},
			},
			wantErr: false,
		},
		{
			name: "file with only Last-Synced-Commit is parsed successfully",
			content: `
				# Last-Synced-Commit: a1b2c3d4e5f6
			`,
			want: &IgnoreFile{
				LastSyncedCommit: "a1b2c3d4e5f6",
				Mutations:        map[Mutation]bool{},
			},
			wantErr: false,
		},
		{
			name: "successfully parses valid ignore file with mutations",
			content: `
				# Last-Synced-Commit: a1b2c3d4e5f6
				# format: filepath:line:mutatorName

				main.go:10:branch/if
				internal/utils.go:42:condition/negated
			`,
			want: &IgnoreFile{
				LastSyncedCommit: "a1b2c3d4e5f6",
				Mutations: map[Mutation]bool{
					{
						Name:      "branch/if",
						RelPath:   "main.go",
						StartLine: 10,
					}: true,
					{
						Name:      "condition/negated",
						RelPath:   "internal/utils.go",
						StartLine: 42,
					}: true,
				},
			},
			wantErr: false,
		},
		{
			name: "malformed Last-Synced-Commit header",
			content: `
				# Last-Synced-Commit:
			`,
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			reader := strings.NewReader(trimIndent(tt.content))
			got, err := ParseIgnoreFile(reader)

			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseIgnoreFile() error = %v, wantErr %v", err, tt.wantErr)
			}

			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("ParseIgnoreFile() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestIgnoreFile_Update(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		ignoreFile      *IgnoreFile
		ignoreMutations map[Mutation]bool
		combinedDiff    git.CombinedDiff
		mutations       map[Mutation]string
		commit          string
		want            *IgnoreFile
	}{
		{
			name: "empty inputs result in updated commit and empty mutations",
			ignoreFile: &IgnoreFile{
				LastSyncedCommit: "old-commit",
				Mutations:        map[Mutation]bool{},
			},
			ignoreMutations: map[Mutation]bool{},
			combinedDiff:    git.CombinedDiff{},
			mutations:       map[Mutation]string{},
			commit:          "new-commit",
			want: &IgnoreFile{
				LastSyncedCommit: "new-commit",
				Mutations:        map[Mutation]bool{},
			},
		},
		{
			name: "mutation missing from ignoreMutations is retained immediately",
			ignoreFile: &IgnoreFile{
				LastSyncedCommit: "old-commit",
				Mutations: map[Mutation]bool{
					{
						Name:      "branch/if",
						RelPath:   "main.go",
						StartLine: 10,
					}: true,
				},
			},
			ignoreMutations: map[Mutation]bool{},
			combinedDiff:    git.CombinedDiff{},
			mutations:       map[Mutation]string{},
			commit:          "new-commit",
			want: &IgnoreFile{
				LastSyncedCommit: "new-commit",
				Mutations: map[Mutation]bool{
					{
						Name:      "branch/if",
						RelPath:   "main.go",
						StartLine: 10,
					}: true,
				},
			},
		},
		{
			name: "mutation is dropped when it is missing from new mutations",
			ignoreFile: &IgnoreFile{
				LastSyncedCommit: "old-commit",
				Mutations: map[Mutation]bool{
					{
						Name:      "branch/if",
						RelPath:   "main.go",
						StartLine: 10,
					}: true,
				},
			},
			ignoreMutations: map[Mutation]bool{
				{
					Name:      "branch/if",
					RelPath:   "main.go",
					StartLine: 10,
				}: true,
			},
			combinedDiff: git.CombinedDiff{},
			mutations:    map[Mutation]string{},
			commit:       "new-commit",
			want: &IgnoreFile{
				LastSyncedCommit: "new-commit",
				Mutations:        map[Mutation]bool{},
			},
		},
		{
			name: "mutation is retained when it exists in new mutations",
			ignoreFile: &IgnoreFile{
				LastSyncedCommit: "old-commit",
				Mutations: map[Mutation]bool{
					{
						Name:      "branch/if",
						RelPath:   "main.go",
						StartLine: 10,
					}: true,
				},
			},
			ignoreMutations: map[Mutation]bool{
				{
					Name:      "branch/if",
					RelPath:   "main.go",
					StartLine: 10,
				}: true,
			},
			combinedDiff: git.CombinedDiff{},
			mutations: map[Mutation]string{
				{
					Name:      "branch/if",
					RelPath:   "main.go",
					StartLine: 10,
				}: "checksum123",
			},
			commit: "new-commit",
			want: &IgnoreFile{
				LastSyncedCommit: "new-commit",
				Mutations: map[Mutation]bool{
					{
						Name:      "branch/if",
						RelPath:   "main.go",
						StartLine: 10,
					}: true,
				},
			},
		},
		{
			name: "mutation is updated with new path and shifted line number",
			ignoreFile: &IgnoreFile{
				LastSyncedCommit: "old-commit",
				Mutations: map[Mutation]bool{
					{
						Name:      "branch/if",
						RelPath:   "main.go",
						StartLine: 10,
					}: true,
				},
			},
			ignoreMutations: map[Mutation]bool{
				{
					Name:      "branch/if",
					RelPath:   "main.go",
					StartLine: 10,
				}: true,
			},
			combinedDiff: git.CombinedDiff{
				FromFile: map[string]*git.FileDiff{
					"main.go": {
						OldRelPath: "main.go",
						NewRelPath: "new_main.go",
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
			mutations: map[Mutation]string{
				{
					Name:      "branch/if",
					RelPath:   "new_main.go",
					StartLine: 12,
				}: "checksum123",
			},
			commit: "new-commit",
			want: &IgnoreFile{
				LastSyncedCommit: "new-commit",
				Mutations: map[Mutation]bool{
					{
						Name:      "branch/if",
						RelPath:   "new_main.go",
						StartLine: 12,
					}: true,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tt.ignoreFile.Update(tt.ignoreMutations, tt.combinedDiff, tt.mutations, tt.commit)

			if diff := cmp.Diff(tt.want, tt.ignoreFile); diff != "" {
				t.Errorf("Ignore.Update() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestIgnoreFile_WriteIgnoreFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		ignoreFile *IgnoreFile
		wantOut    string
	}{
		{
			name: "empty mutations writes only headers",
			ignoreFile: &IgnoreFile{
				LastSyncedCommit: "a1b2c3d4e5f6",
				Mutations:        map[Mutation]bool{},
			},
			wantOut: `
				# Last-Synced-Commit: a1b2c3d4e5f6
				# format: filepath:line:mutatorName

			`,
		},
		{
			name: "populated mutations are sorted by path, line, then name",
			ignoreFile: &IgnoreFile{
				LastSyncedCommit: "a1b2c3d4e5f6",
				Mutations: map[Mutation]bool{
					{
						Name:      "b_mutator",
						RelPath:   "main.go",
						StartLine: 10,
					}: true,
					{
						Name:      "a_mutator",
						RelPath:   "main.go",
						StartLine: 20,
					}: true,
					{
						Name:      "a_mutator",
						RelPath:   "main.go",
						StartLine: 10,
					}: true,
					{
						Name:      "mutator",
						RelPath:   "internal/file.go",
						StartLine: 20,
					}: true,
				},
			},
			wantOut: `
				# Last-Synced-Commit: a1b2c3d4e5f6
				# format: filepath:line:mutatorName

				internal/file.go:20:mutator
				main.go:10:a_mutator
				main.go:10:b_mutator
				main.go:20:a_mutator
			`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer
			err := tt.ignoreFile.WriteIgnoreFile(&buf)
			if err != nil {
				t.Fatalf("unexpected error writing ignore file: %v", err)
			}

			gotOut := buf.String()
			if gotOut != trimIndent(tt.wantOut) {
				t.Errorf("Ignore.WriteIgnoreFile() got = %v, want %v", gotOut, trimIndent(tt.wantOut))
			}
		})
	}
}
