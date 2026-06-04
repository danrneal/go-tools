package diffparser

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestToNewLine(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		fileDiff    FileDiff
		oldLine     int
		wantNewLine int
	}{
		{
			name:        "no hunks in file diff (returns original line)",
			fileDiff:    FileDiff{},
			oldLine:     5,
			wantNewLine: 5,
		},
		{
			name: "insertions and modifications strictly before target line",
			fileDiff: FileDiff{
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
			oldLine:     5,
			wantNewLine: 7,
		},
		{
			name: "pure insertion strictly after target line (triggers left side of first if statement)",
			fileDiff: FileDiff{
				{
					OldStart: 10,
					OldCount: 0,
					NewStart: 10,
					NewCount: 2,
				},
			},
			oldLine:     5,
			wantNewLine: 5,
		},
		{
			name: "pure insertion exactly at target line (triggers right side of first if statement)",
			fileDiff: FileDiff{
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
			oldLine:     10,
			wantNewLine: 10,
		},
		{
			name: "target line deleted at exact start boundary (triggers second if statement)",
			fileDiff: FileDiff{
				{
					OldStart: 5,
					OldCount: 3,
					NewStart: 4,
					NewCount: 0,
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

func TestIsAddition(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		fileDiff FileDiff
		line     int
		want     bool
	}{
		{
			name: "line exactly at exclusive end boundary",
			fileDiff: FileDiff{
				{
					OldStart: 10,
					OldCount: 2,
					NewStart: 10,
					NewCount: 2,
				},
			},
			line: 12,
			want: false,
		},
		{
			name: "line strictly before hunk",
			fileDiff: FileDiff{
				{
					OldStart: 10,
					OldCount: 2,
					NewStart: 10,
					NewCount: 2,
				},
			},
			line: 5,
			want: false,
		},
		{
			name: "line exactly at start boundary",
			fileDiff: FileDiff{
				{
					OldStart: 10,
					OldCount: 2,
					NewStart: 10,
					NewCount: 2,
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

func TestParse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		diffOutput    string
		wantFileDiffs map[string]FileDiff
		wantErr       bool
	}{
		{
			name:          "empty diff output",
			diffOutput:    "",
			wantFileDiffs: map[string]FileDiff{},
			wantErr:       false,
		},
		{
			name: "file mode change only (no hunks)",
			diffOutput: `
				diff --git a/script.sh b/script.sh
				old mode 100644
				new mode 100755
			`,
			wantFileDiffs: map[string]FileDiff{},
			wantErr:       false,
		},
		{
			name: "single file with one hunk (omitted counts default to 1)",
			diffOutput: `
				diff --git a/main.go b/main.go
				index 8a1218a..87c9307 100644
				--- a/main.go
				+++ b/main.go
				@@ -10 +12 @@
				+func main() {
				+	fmt.Println("hello")
				+}
			`,
			wantFileDiffs: map[string]FileDiff{
				"main.go": {
					{
						OldStart: 10,
						OldCount: 1,
						NewStart: 12,
						NewCount: 1,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "omitted new count only",
			diffOutput: `
				diff --git a/main.go b/main.go
				index 8a1218a..87c9307 100644
				--- a/main.go
				+++ b/main.go
				@@ -10,3 +12 @@
				+func main() {
			`,
			wantFileDiffs: map[string]FileDiff{
				"main.go": {
					{
						OldStart: 10,
						OldCount: 3,
						NewStart: 12,
						NewCount: 1,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "omitted old count only",
			diffOutput: `
				diff --git a/main.go b/main.go
				index 8a1218a..87c9307 100644
				--- a/main.go
				+++ b/main.go
				@@ -10 +12,3 @@
				+func main() {
				+	fmt.Println("hello")
				+}
			`,
			wantFileDiffs: map[string]FileDiff{
				"main.go": {
					{
						OldStart: 10,
						OldCount: 1,
						NewStart: 12,
						NewCount: 3,
					},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cleanOutput := trimIndent(tt.diffOutput)
			got, err := Parse(strings.NewReader(cleanOutput))

			if (err != nil) != tt.wantErr {
				t.Fatalf("Parse() error = %v, wantErr %v", err, tt.wantErr)
			}

			if diff := cmp.Diff(tt.wantFileDiffs, got); diff != "" {
				t.Errorf("Parse() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func trimIndent(s string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimLeft(line, "\t ")
	}

	return strings.Join(lines, "\n")
}
