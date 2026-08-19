package git

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestClient_Diff(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		commitA  string
		commitB  string
		runMock  *runMock
		wantDiff CombinedDiff
		wantErr  bool
	}{
		{
			name:    "two commits",
			commitA: "base-commit",
			commitB: "HEAD",
			runMock: &runMock{
				wantArgs: []string{"diff", "-U0", "-M", "--no-ext-diff", "base-commit", "HEAD"},
				out:      "",
			},
			wantDiff: CombinedDiff{FromFile: map[string]*FileDiff{}, ToFile: map[string]*FileDiff{}},
			wantErr:  false,
		},
		{
			name:    "underlying git command fails",
			commitA: "HEAD",
			runMock: &runMock{
				wantArgs: []string{"diff", "-U0", "-M", "--no-ext-diff", "HEAD"},
				err:      errors.New("fatal: not a git repository"),
			},
			wantDiff: CombinedDiff{},
			wantErr:  true,
		},
		{
			name:    "empty diff output",
			commitA: "HEAD",
			runMock: &runMock{
				wantArgs: []string{"diff", "-U0", "-M", "--no-ext-diff", "HEAD"},
				out:      "",
			},
			wantDiff: CombinedDiff{FromFile: map[string]*FileDiff{}, ToFile: map[string]*FileDiff{}},
			wantErr:  false,
		},
		{
			name:    "file mode change only (no hunks)",
			commitA: "HEAD",
			runMock: &runMock{
				wantArgs: []string{"diff", "-U0", "-M", "--no-ext-diff", "HEAD"},
				out: trimIndent(`
					diff --git a/script.sh b/script.sh
					old mode 100644
					new mode 100755
				`),
			},
			wantDiff: CombinedDiff{FromFile: map[string]*FileDiff{}, ToFile: map[string]*FileDiff{}},
			wantErr:  false,
		},
		{
			name:    "pure file rename (no hunks)",
			commitA: "HEAD",
			runMock: &runMock{
				wantArgs: []string{"diff", "-U0", "-M", "--no-ext-diff", "HEAD"},
				out: trimIndent(`
					diff --git a/file.go b/newfile.go
					similarity index 100%
					rename from file.go
					rename to newfile.go
					diff --git a/other.go b/other.go
					--- a/other.go
					+++ b/other.go
					@@ -1 +1 @@
					+foo
				`),
			},
			wantDiff: CombinedDiff{
				FromFile: map[string]*FileDiff{
					"file.go": {
						OldRelPath: "file.go",
						NewRelPath: "newfile.go",
					},
					"other.go": {
						OldRelPath: "other.go",
						NewRelPath: "other.go",
						Hunks: []Hunk{
							{
								OldStart: 1,
								OldCount: 1,
								NewStart: 1,
								NewCount: 1,
							},
						},
					},
				},
				ToFile: map[string]*FileDiff{
					"newfile.go": {
						OldRelPath: "file.go",
						NewRelPath: "newfile.go",
					},
					"other.go": {
						OldRelPath: "other.go",
						NewRelPath: "other.go",
						Hunks: []Hunk{
							{
								OldStart: 1,
								OldCount: 1,
								NewStart: 1,
								NewCount: 1,
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name:    "single file with one hunk (omitted counts default to 1)",
			commitA: "HEAD",
			runMock: &runMock{
				wantArgs: []string{"diff", "-U0", "-M", "--no-ext-diff", "HEAD"},
				out: trimIndent(`
					diff --git a/main.go b/main.go
					index 8a1218a..87c9307 100644
					--- a/main.go
					+++ b/main.go
					@@ -10 +12 @@
					+func main() {
					+	fmt.Println("hello")
					+}
				`),
			},
			wantDiff: CombinedDiff{
				FromFile: map[string]*FileDiff{
					"main.go": {
						OldRelPath: "main.go",
						NewRelPath: "main.go",
						Hunks: []Hunk{
							{
								OldStart: 10,
								OldCount: 1,
								NewStart: 12,
								NewCount: 1,
							},
						},
					},
				},
				ToFile: map[string]*FileDiff{
					"main.go": {
						OldRelPath: "main.go",
						NewRelPath: "main.go",
						Hunks: []Hunk{
							{
								OldStart: 10,
								OldCount: 1,
								NewStart: 12,
								NewCount: 1,
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name:    "newly added file (from /dev/null)",
			commitA: "HEAD",
			runMock: &runMock{
				wantArgs: []string{"diff", "-U0", "-M", "--no-ext-diff", "HEAD"},
				out: trimIndent(`
					diff --git a/new.go b/new.go
					new file mode 100644
					index 0000000..8a1218a
					--- /dev/null
					+++ b/new.go
					@@ -0,0 +1,2 @@
					+func main() {
					+}
				`),
			},
			wantDiff: CombinedDiff{
				FromFile: map[string]*FileDiff{},
				ToFile: map[string]*FileDiff{
					"new.go": {
						OldRelPath: "",
						NewRelPath: "new.go",
						Hunks: []Hunk{
							{
								OldStart: 0,
								OldCount: 0,
								NewStart: 1,
								NewCount: 2,
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name:    "omitted new count only",
			commitA: "HEAD",
			runMock: &runMock{
				wantArgs: []string{"diff", "-U0", "-M", "--no-ext-diff", "HEAD"},
				out: trimIndent(`
					diff --git a/main.go b/main.go
					index 8a1218a..87c9307 100644
					--- a/main.go
					+++ b/main.go
					@@ -10,3 +12 @@
					+func main() {
				`),
			},
			wantDiff: CombinedDiff{
				FromFile: map[string]*FileDiff{
					"main.go": {
						OldRelPath: "main.go",
						NewRelPath: "main.go",
						Hunks: []Hunk{
							{
								OldStart: 10,
								OldCount: 3,
								NewStart: 12,
								NewCount: 1,
							},
						},
					},
				},
				ToFile: map[string]*FileDiff{
					"main.go": {
						OldRelPath: "main.go",
						NewRelPath: "main.go",
						Hunks: []Hunk{
							{
								OldStart: 10,
								OldCount: 3,
								NewStart: 12,
								NewCount: 1,
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name:    "omitted old count only",
			commitA: "HEAD",
			runMock: &runMock{
				wantArgs: []string{"diff", "-U0", "-M", "--no-ext-diff", "HEAD"},
				out: trimIndent(`
					diff --git a/main.go b/main.go
					index 8a1218a..87c9307 100644
					--- a/main.go
					+++ b/main.go
					@@ -10 +12,3 @@
					+func main() {
					+	fmt.Println("hello")
					+}
				`),
			},
			wantDiff: CombinedDiff{
				FromFile: map[string]*FileDiff{
					"main.go": {
						OldRelPath: "main.go",
						NewRelPath: "main.go",
						Hunks: []Hunk{
							{
								OldStart: 10,
								OldCount: 1,
								NewStart: 12,
								NewCount: 3,
							},
						},
					},
				},
				ToFile: map[string]*FileDiff{
					"main.go": {
						OldRelPath: "main.go",
						NewRelPath: "main.go",
						Hunks: []Hunk{
							{
								OldStart: 10,
								OldCount: 1,
								NewStart: 12,
								NewCount: 3,
							},
						},
					},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := t.Context()
			client := newMockClient(ctx, t, tt.runMock)
			gotDiff, err := client.Diff(ctx, tt.commitA, tt.commitB)

			if (err != nil) != tt.wantErr {
				t.Fatalf("Diff() error = %v, wantErr %v", err, tt.wantErr)
			}

			if diff := cmp.Diff(tt.wantDiff, gotDiff); diff != "" {
				t.Errorf("Diff() parsed struct mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestFileDiff_ToNewLine(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		fileDiff    FileDiff
		oldLine     int
		wantNewLine int
	}{
		{
			name:        "no hunks in file diff",
			fileDiff:    FileDiff{},
			oldLine:     5,
			wantNewLine: 5,
		},
		{
			name: "insertions and modifications before target line",
			fileDiff: FileDiff{
				Hunks: []Hunk{
					{
						OldStart: 1,
						OldCount: 0,
						NewStart: 1,
						NewCount: 2,
					},
					{
						OldStart: 3,
						OldCount: 2,
						NewStart: 5,
						NewCount: 2,
					},
				},
			},
			oldLine:     5,
			wantNewLine: 7,
		},
		{
			name: "insertion after target line",
			fileDiff: FileDiff{
				Hunks: []Hunk{
					{
						OldStart: 10,
						OldCount: 0,
						NewStart: 10,
						NewCount: 2,
					},
				},
			},
			oldLine:     5,
			wantNewLine: 5,
		},
		{
			name: "insertion exactly at target line",
			fileDiff: FileDiff{
				Hunks: []Hunk{
					{
						OldStart: 10,
						OldCount: 0,
						NewStart: 10,
						NewCount: 2,
					},
					{
						OldStart: 10,
						OldCount: 2,
						NewStart: 12,
						NewCount: 0,
					},
				},
			},
			oldLine:     10,
			wantNewLine: 10,
		},
		{
			name: "target line deleted at start of hunk",
			fileDiff: FileDiff{
				Hunks: []Hunk{
					{
						OldStart: 5,
						OldCount: 3,
						NewStart: 4,
						NewCount: 0,
					},
				},
			},
			oldLine:     5,
			wantNewLine: -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.fileDiff.ToNewLine(tt.oldLine)
			if diff := cmp.Diff(tt.wantNewLine, got); diff != "" {
				t.Errorf("ToNewLine() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestFileDiff_ToOldLine(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		fileDiff    FileDiff
		newLine     int
		wantOldLine int
	}{
		{
			name:        "no hunks in file diff",
			fileDiff:    FileDiff{},
			newLine:     5,
			wantOldLine: 5,
		},
		{
			name: "deletions and modifications before target line",
			fileDiff: FileDiff{
				Hunks: []Hunk{
					{
						OldStart: 1,
						OldCount: 2,
						NewStart: 1,
						NewCount: 0,
					},
					{
						OldStart: 5,
						OldCount: 2,
						NewStart: 3,
						NewCount: 2,
					},
				},
			},
			newLine:     5,
			wantOldLine: 7,
		},
		{
			name: "insertion after target line",
			fileDiff: FileDiff{
				Hunks: []Hunk{
					{
						OldStart: 10,
						OldCount: 0,
						NewStart: 10,
						NewCount: 2,
					},
				},
			},
			newLine:     5,
			wantOldLine: 5,
		},
		{
			name: "deletion exactly at target line",
			fileDiff: FileDiff{
				Hunks: []Hunk{
					{
						OldStart: 10,
						OldCount: 2,
						NewStart: 10,
						NewCount: 0,
					},
					{
						OldStart: 12,
						OldCount: 0,
						NewStart: 10,
						NewCount: 2,
					},
				},
			},
			newLine:     10,
			wantOldLine: 10,
		},
		{
			name: "target line added at start of hunk",
			fileDiff: FileDiff{
				Hunks: []Hunk{
					{
						OldStart: 5,
						OldCount: 0,
						NewStart: 5,
						NewCount: 3,
					},
				},
			},
			newLine:     5,
			wantOldLine: -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.fileDiff.ToOldLine(tt.newLine)
			if diff := cmp.Diff(tt.wantOldLine, got); diff != "" {
				t.Errorf("ToOldLine() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestFileDiff_IsAddition(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		fileDiff FileDiff
		line     int
		want     bool
	}{
		{
			name: "line at end boundary",
			fileDiff: FileDiff{
				Hunks: []Hunk{
					{
						OldStart: 10,
						OldCount: 2,
						NewStart: 10,
						NewCount: 2,
					},
				},
			},
			line: 12,
			want: false,
		},
		{
			name: "line before hunk",
			fileDiff: FileDiff{
				Hunks: []Hunk{
					{
						OldStart: 10,
						OldCount: 2,
						NewStart: 10,
						NewCount: 2,
					},
				},
			},
			line: 5,
			want: false,
		},
		{
			name: "line at start boundary",
			fileDiff: FileDiff{
				Hunks: []Hunk{
					{
						OldStart: 10,
						OldCount: 2,
						NewStart: 10,
						NewCount: 2,
					},
				},
			},
			line: 10,
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.fileDiff.IsAddition(tt.line)
			if got != tt.want {
				t.Errorf("IsAddition() = %v, want %v", got, tt.want)
			}
		})
	}
}

func trimIndent(s string) string {
	s = strings.TrimPrefix(s, "\n")

	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimLeft(line, "\t ")
	}

	return strings.Join(lines, "\n")
}
