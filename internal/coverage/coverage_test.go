package coverage

import (
	"testing"

	"github.com/danrneal/go-tools/internal/git"
	"github.com/google/go-cmp/cmp"
	"golang.org/x/tools/cover"
)

func TestParse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		coverProfiles []*cover.Profile
		modulePath    string
		wantFiles     Files
	}{
		{
			name:          "empty coverProfiles",
			coverProfiles: []*cover.Profile{},
			modulePath:    "github.com/example/repo/",
			wantFiles:     Files{},
		},
		{
			name: "cover profile with no blocks",
			coverProfiles: []*cover.Profile{
				{
					FileName: "github.com/example/repo/main.go",
					Mode:     "set",
					Blocks:   []cover.ProfileBlock{},
				},
			},
			modulePath: "github.com/example/repo/",
			wantFiles: Files{
				"main.go": map[int]bool{},
			},
		},
		{
			name: "multi-line unrolling",
			coverProfiles: []*cover.Profile{
				{
					FileName: "github.com/example/repo/main.go",
					Mode:     "set",
					Blocks: []cover.ProfileBlock{
						{
							StartLine: 1,
							EndLine:   3,
							Count:     0,
						},
					},
				},
			},
			modulePath: "github.com/example/repo/",
			wantFiles: Files{
				"main.go": {
					1: false,
					2: false,
					3: false,
				},
			},
		},
		{
			name: "overlapping blocks preserve covered status",
			coverProfiles: []*cover.Profile{
				{
					FileName: "github.com/example/repo/main.go",
					Mode:     "set",
					Blocks: []cover.ProfileBlock{
						{
							StartLine: 5,
							EndLine:   5,
							Count:     1,
						},
						{
							StartLine: 5,
							EndLine:   5,
							Count:     0,
						},
					},
				},
			},
			modulePath: "github.com/example/repo/",
			wantFiles: Files{
				"main.go": {
					5: true,
				},
			},
		},
		{
			name: "module path without trailing slash is handled correctly",
			coverProfiles: []*cover.Profile{
				{
					FileName: "github.com/example/repo/main.go",
					Mode:     "set",
					Blocks: []cover.ProfileBlock{
						{
							StartLine: 1,
							EndLine:   1,
							Count:     1,
						},
					},
				},
			},
			modulePath: "github.com/example/repo",
			wantFiles: Files{
				"main.go": {
					1: true,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := Parse(tt.coverProfiles, tt.modulePath)

			if diff := cmp.Diff(tt.wantFiles, got); diff != "" {
				t.Errorf("Parse() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestOverallPercentage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		coverProfiles []*cover.Profile
		want          float64
	}{
		{
			name:          "empty coverProfiles returns 0.0 without panicking",
			coverProfiles: []*cover.Profile{},
			want:          0.0,
		},
		{
			name: "profile with no blocks returns 0.0 without panicking",
			coverProfiles: []*cover.Profile{
				{
					FileName: "github.com/example/repo/main.go",
					Mode:     "set",
					Blocks:   []cover.ProfileBlock{},
				},
			},
			want: 0.0,
		},
		{
			name: "0% coverage returns 0.0",
			coverProfiles: []*cover.Profile{
				{
					FileName: "github.com/example/repo/main.go",
					Mode:     "set",
					Blocks: []cover.ProfileBlock{
						{
							NumStmt: 5,
							Count:   0,
						},
					},
				},
			},
			want: 0.0,
		},
		{
			name: "multi-file partial coverage aggregates correctly (75%)",
			coverProfiles: []*cover.Profile{
				{
					FileName: "github.com/example/repo/main.go",
					Mode:     "set",
					Blocks: []cover.ProfileBlock{
						{
							NumStmt: 1,
							Count:   1,
						},
						{
							NumStmt: 2,
							Count:   1,
						},
					},
				},
				{
					FileName: "github.com/example/repo/utils.go",
					Mode:     "set",
					Blocks: []cover.ProfileBlock{
						{
							NumStmt: 1,
							Count:   0,
						},
					},
				},
			},
			want: 75.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := OverallPercentage(tt.coverProfiles)

			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("OverallPercentage() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestFindRegressions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		baseCoverage    Files
		currentCoverage Files
		fileDiffs       map[string]git.FileDiff
		want            []string
	}{
		{
			name:         "empty baseCoverage returns empty slice",
			baseCoverage: Files{},
			currentCoverage: Files{
				"main.go": {
					10: true,
				},
			},
			fileDiffs: map[string]git.FileDiff{},
			want:      []string{},
		},
		{
			name: "baseCoverage file with no lines returns empty slice",
			baseCoverage: Files{
				"main.go": {},
			},
			currentCoverage: Files{
				"main.go": {
					10: true,
				},
			},
			fileDiffs: map[string]git.FileDiff{},
			want:      []string{},
		},
		{
			name: "covered base line becomes uncovered (regression found)",
			baseCoverage: Files{
				"main.go": {
					10: true,
				},
			},
			currentCoverage: Files{
				"main.go": {
					10: false,
				},
			},
			fileDiffs: map[string]git.FileDiff{},
			want: []string{
				"main.go:10",
			},
		},
		{
			name: "uncovered base line is safely ignored",
			baseCoverage: Files{
				"main.go": {
					10: false,
					20: true,
				},
			},
			currentCoverage: Files{
				"main.go": {
					10: false,
					20: false,
				},
			},
			fileDiffs: map[string]git.FileDiff{},
			want: []string{
				"main.go:20",
			},
		},
		{
			name: "covered base line shifts and becomes uncovered (regression found at new line)",
			baseCoverage: Files{
				"main.go": {
					10: true,
				},
			},
			currentCoverage: Files{
				"main.go": {
					12: false,
				},
			},
			fileDiffs: map[string]git.FileDiff{
				"main.go": {
					NewFilepath: "main.go",
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
			want: []string{
				"main.go:12",
			},
		},
		{
			name: "file missing from current coverage is safely ignored",
			baseCoverage: Files{
				"a_missing.go": {
					10: true,
				},
				"z_regression.go": {
					10: true,
				},
			},
			currentCoverage: Files{
				"z_regression.go": {
					10: false,
				},
			},
			fileDiffs: map[string]git.FileDiff{},
			want: []string{
				"z_regression.go:10",
			},
		},
		{
			name: "covered line remains covered alongside regressions",
			baseCoverage: Files{
				"main.go": {
					10: true,
					20: true,
				},
			},
			currentCoverage: Files{
				"main.go": {
					10: true,
					20: false,
				},
			},
			fileDiffs: map[string]git.FileDiff{},
			want: []string{
				"main.go:20",
			},
		},
		{
			name: "line becomes non-executable (hits !ok for specific line)",
			baseCoverage: Files{
				"main.go": {
					10: true,
				},
			},
			currentCoverage: Files{
				"main.go": {
					11: true,
				},
			},
			fileDiffs: map[string]git.FileDiff{},
			want:      []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := FindRegressions(tt.baseCoverage, tt.currentCoverage, tt.fileDiffs)

			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("FindRegressions() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestFindNewUncoveredLines(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		coverage  Files
		fileDiffs map[string]git.FileDiff
		want      []string
	}{
		{
			name:     "empty coverage returns empty slice",
			coverage: Files{},
			fileDiffs: map[string]git.FileDiff{
				"main.go": {},
			},
			want: []string{},
		},
		{
			name: "non-empty coverage with no lines returns empty slice",
			coverage: Files{
				"main.go": {},
			},
			fileDiffs: map[string]git.FileDiff{
				"main.go": {
					NewFilepath: "main.go",
					Hunks: []git.Hunk{
						{
							OldStart: 1,
							OldCount: 0,
							NewStart: 1,
							NewCount: 2,
						},
					},
				},
			},
			want: []string{},
		},
		{
			name: "uncovered line outside of new hunks is ignored",
			coverage: Files{
				"main.go": {
					10: false,
				},
			},
			fileDiffs: map[string]git.FileDiff{
				"main.go": {
					NewFilepath: "main.go",
					Hunks: []git.Hunk{
						{
							OldStart: 20,
							OldCount: 0,
							NewStart: 20,
							NewCount: 5,
						},
					},
				},
			},
			want: []string{},
		},
		{
			name: "new uncovered code is found and reported",
			coverage: Files{
				"main.go": {
					10: false,
				},
			},
			fileDiffs: map[string]git.FileDiff{
				"main.go": {
					NewFilepath: "main.go",
					Hunks: []git.Hunk{
						{
							OldStart: 10,
							OldCount: 0,
							NewStart: 10,
							NewCount: 2,
						},
					},
				},
			},
			want: []string{
				"main.go:10",
			},
		},
		{
			name: "covered line inside new hunk is safely ignored",
			coverage: Files{
				"main.go": {
					10: true,
					20: false,
				},
			},
			fileDiffs: map[string]git.FileDiff{
				"main.go": {
					NewFilepath: "main.go",
					Hunks: []git.Hunk{
						{
							OldStart: 10,
							OldCount: 0,
							NewStart: 10,
							NewCount: 15,
						},
					},
				},
			},
			want: []string{
				"main.go:20",
			},
		},
		{
			name: "file missing from diff is safely ignored",
			coverage: Files{
				"a_missing.go": {
					10: false,
				},
				"z_uncovered.go": {
					10: false,
				},
			},
			fileDiffs: map[string]git.FileDiff{
				"z_uncovered.go": {
					NewFilepath: "z_uncovered.go",
					Hunks: []git.Hunk{
						{
							OldStart: 10,
							OldCount: 0,
							NewStart: 10,
							NewCount: 2,
						},
					},
				},
			},
			want: []string{
				"z_uncovered.go:10",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := FindNewUncoveredLines(tt.coverage, tt.fileDiffs)

			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("FindNewUncoveredLines() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
