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

func TestFiles_Regressions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		files        Files
		baseCoverage Files
		combinedDiff git.CombinedDiff
		want         []FileReport
	}{
		{
			name:         "empty currentCoverage",
			files:        Files{},
			baseCoverage: Files{},
			combinedDiff: git.CombinedDiff{},
			want:         []FileReport{},
		},
		{
			name: "currentCoverage file with no lines",
			files: Files{
				"main.go": {},
			},
			baseCoverage: Files{
				"main.go": {
					10: true,
				},
			},
			combinedDiff: git.CombinedDiff{},
			want:         []FileReport{},
		},
		{
			name: "covered base line becomes uncovered (regression found)",
			files: Files{
				"main.go": {
					10: false,
				},
			},
			baseCoverage: Files{
				"main.go": {
					10: true,
				},
			},
			combinedDiff: git.CombinedDiff{},
			want: []FileReport{
				{
					RelPath: "main.go",
					Lines:   []int{10},
				},
			},
		},
		{
			name: "uncovered base line is safely ignored",
			files: Files{
				"main.go": {
					10: false,
					20: false,
				},
			},
			baseCoverage: Files{
				"main.go": {
					10: false,
					20: true,
				},
			},
			combinedDiff: git.CombinedDiff{},
			want: []FileReport{
				{
					RelPath: "main.go",
					Lines:   []int{20},
				},
			},
		},
		{
			name: "line completely missing from base coverage is safely ignored",
			files: Files{
				"main.go": {
					10: false,
					15: false,
				},
			},
			baseCoverage: Files{
				"main.go": {
					10: true,
				},
			},
			combinedDiff: git.CombinedDiff{},
			want: []FileReport{
				{
					RelPath: "main.go",
					Lines:   []int{10},
				},
			},
		},
		{
			name: "covered base line shifts and file renamed (regression found at new line)",
			files: Files{
				"new_main.go": {
					12: false,
				},
			},
			baseCoverage: Files{
				"main.go": {
					10: true,
				},
			},
			combinedDiff: git.CombinedDiff{
				ToFile: map[string]*git.FileDiff{
					"new_main.go": {
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
			want: []FileReport{
				{
					RelPath: "new_main.go",
					Lines:   []int{12},
				},
			},
		},
		{
			name: "covered line remains covered alongside regressions",
			files: Files{
				"main.go": {
					10: true,
					20: false,
				},
			},
			baseCoverage: Files{
				"main.go": {
					10: true,
					20: true,
				},
			},
			combinedDiff: git.CombinedDiff{},
			want: []FileReport{
				{
					RelPath: "main.go",
					Lines:   []int{20},
				},
			},
		},
		{
			name: "completely new file is safely ignored",
			files: Files{
				"new_file.go": {
					10: false,
				},
				"z_regression.go": {
					10: false,
				},
			},
			baseCoverage: Files{
				"z_regression.go": {
					10: true,
				},
			},
			combinedDiff: git.CombinedDiff{},
			want: []FileReport{
				{
					RelPath: "z_regression.go",
					Lines:   []int{10},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.files.Regressions(tt.baseCoverage, tt.combinedDiff)

			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("Regressions() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestFiles_UncoveredAdditions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		files        Files
		baseCoverage Files
		combinedDiff git.CombinedDiff
		want         []FileReport
	}{
		{
			name:         "empty coverage",
			files:        Files{},
			baseCoverage: Files{},
			combinedDiff: git.CombinedDiff{},
			want:         []FileReport{},
		},
		{
			name: "non-empty coverage with no lines",
			files: Files{
				"main.go": {},
			},
			baseCoverage: Files{},
			combinedDiff: git.CombinedDiff{},
			want:         []FileReport{},
		},
		{
			name: "untracked file missing from diff is reported as new",
			files: Files{
				"main.go": {
					10: false,
				},
			},
			baseCoverage: Files{},
			combinedDiff: git.CombinedDiff{},
			want: []FileReport{
				{
					RelPath: "main.go",
					Lines:   []int{10},
				},
			},
		},
		{
			name: "uncovered line outside of new hunks is ignored",
			files: Files{
				"main.go": {
					10: false,
					22: false,
				},
			},
			baseCoverage: Files{
				"main.go": {
					10: false,
				},
			},
			combinedDiff: git.CombinedDiff{
				ToFile: map[string]*git.FileDiff{
					"main.go": {
						NewRelPath: "main.go",
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
			},
			want: []FileReport{
				{
					RelPath: "main.go",
					Lines:   []int{22},
				},
			},
		},
		{
			name: "covered line inside new hunk is safely ignored",
			files: Files{
				"main.go": {
					10: true,
					20: false,
				},
			},
			baseCoverage: Files{
				"main.go": {
					5: true,
				},
			},
			combinedDiff: git.CombinedDiff{
				ToFile: map[string]*git.FileDiff{
					"main.go": {
						NewRelPath: "main.go",
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
			},
			want: []FileReport{
				{
					RelPath: "main.go",
					Lines:   []int{20},
				},
			},
		},
		{
			name: "unmodified file with uncovered lines is safely ignored alongside new code",
			files: Files{
				"a_unmodified.go": {
					10: false,
				},
				"z_new.go": {
					10: false,
				},
			},
			baseCoverage: Files{
				"a_unmodified.go": {
					10: false,
				},
			},
			combinedDiff: git.CombinedDiff{
				ToFile: map[string]*git.FileDiff{
					"z_new.go": {
						NewRelPath: "z_new.go",
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
			},
			want: []FileReport{
				{
					RelPath: "z_new.go",
					Lines:   []int{10},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.files.UncoveredAdditions(tt.baseCoverage, tt.combinedDiff)

			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("UncoveredAdditions() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestFiles_FormatLineRanges(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		files      Files
		fileReport FileReport
		want       []string
	}{
		{
			name: "report with only one line",
			files: Files{
				"main.go": {
					10: false,
				},
			},
			fileReport: FileReport{
				RelPath: "main.go",
				Lines:   []int{10},
			},
			want: []string{"10"},
		},
		{
			name: "report with gap >= 3",
			files: Files{
				"main.go": {
					10: false,
					11: true,
					12: false,
					16: false,
				},
			},
			fileReport: FileReport{
				RelPath: "main.go",
				Lines:   []int{10, 12, 16},
			},
			want: []string{"10", "12", "16"},
		},
		{
			name: "gap == 2 with unexecutable lines",
			files: Files{
				"main.go": {
					10: false,
					11: true,
					12: false,
					15: false,
				},
			},
			fileReport: FileReport{
				RelPath: "main.go",
				Lines:   []int{10, 12, 15},
			},
			want: []string{"10", "12-15"},
		},
		{
			name: "gap == 2 with covered second line",
			files: Files{
				"main.go": {
					10: false,
					12: true,
					13: false,
				},
			},
			fileReport: FileReport{
				RelPath: "main.go",
				Lines:   []int{10, 13},
			},
			want: []string{"10", "13"},
		},
		{
			name: "ends with bridged range",
			files: Files{
				"main.go": {
					10: false,
					11: false,
					15: false,
					16: false,
				},
			},
			fileReport: FileReport{
				RelPath: "main.go",
				Lines:   []int{10, 11, 15, 16},
			},
			want: []string{"10-11", "15-16"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.files.FormatLineRanges(tt.fileReport)

			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("FormatLineRanges() mismatch (-want +got):\n%s", diff)
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
			name:          "empty coverProfiles",
			coverProfiles: []*cover.Profile{},
			want:          0.0,
		},
		{
			name: "profile with no blocks",
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
			name: "0% coverage",
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
