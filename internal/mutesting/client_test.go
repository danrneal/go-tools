package mutesting

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

type runMock struct {
	wantEnv  []string
	wantArgs []string
	out      string
	err      error
}

func TestNewClient(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		opts    []Option
		wantDir string
		wantErr bool
	}{
		{
			name:    "successful initialization",
			opts:    []Option{WithDir("/tmp/workspace")},
			wantDir: "/tmp/workspace",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client, err := NewClient(tt.opts...)
			if (err != nil) != tt.wantErr {
				t.Fatalf("NewClient() error = %v, wantErr %v", err, tt.wantErr)
			}

			if tt.wantErr {
				return
			}

			if client.dir != tt.wantDir {
				t.Errorf("NewClient() dir = %v, want %v", client.dir, tt.wantDir)
			}
		})
	}
}

func TestClient_Mutest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		disabledMutators []string
		runMock          *runMock
		want             string
		wantErr          bool
	}{
		{
			name:             "valid execution",
			disabledMutators: []string{"foo", "bar"},
			runMock: &runMock{
				wantEnv: []string{"PATH", "GO_BIN"},
				wantArgs: []string{
					"--disable=foo",
					"--disable=bar",
					"--html-output",
					"--blacklist=go-mutesting.blacklist",
					"./...",
				},
				out: `
					PASS "..."
					PASS "..."
					The mutation score is 0.850000 (85 / 100)
				`,
				err: nil,
			},
			want:    "The mutation score is 0.850000 (85 / 100)",
			wantErr: false,
		},
		{
			name: "command fails",
			runMock: &runMock{
				wantEnv: []string{"PATH", "GO_BIN"},
				wantArgs: []string{
					"--html-output",
					"--blacklist=go-mutesting.blacklist",
					"./...",
				},
				out: "some error output",
				err: errors.New("command failed"),
			},
			want:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client := newMockClient(t, tt.runMock)
			got, err := client.Mutest(t.Context(), tt.disabledMutators)

			if (err != nil) != tt.wantErr {
				t.Fatalf("Mutest() error = %v, wantErr %v", err, tt.wantErr)
			}

			if got != tt.want {
				t.Errorf("Mutest() got = %v, want %v", got, tt.want)
			}

			for _, envVar := range tt.runMock.wantEnv {
				path, ok := strings.CutPrefix(envVar, "PATH=")
				if !ok {
					continue
				}

				goTestWrapperDir, _, _ := strings.Cut(path, ":")
				if _, err := os.Stat(goTestWrapperDir); err == nil || !errors.Is(err, fs.ErrNotExist) {
					t.Errorf(
						"expected go test wrapper directory %s to be deleted by cleanup(), but it exists",
						goTestWrapperDir,
					)
				}
			}
		})
	}
}

func TestProgressWriter_Write(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		p               string
		onProgress      func(int)
		onProgressCalls int
		wantProgress    int
	}{
		{
			name:         "nil onProgress callback does not panic",
			p:            "some output with checksum to ensure it processes safely",
			onProgress:   nil,
			wantProgress: 0,
		},
		{
			name:            "no checksums present",
			p:               "PASS \"arithmetic/assign_invert\" for \"file.go:10\"\n",
			onProgress:      func(int) {},
			onProgressCalls: 0,
			wantProgress:    0,
		},
		{
			name:            "single checksum",
			p:               "some output\nchecksum: abcdef123\nmore output",
			onProgress:      func(int) {},
			onProgressCalls: 1,
			wantProgress:    1,
		},
		{
			name:            "multiple checksums in one chunk",
			p:               "checksum: 123\nchecksum: 456\nchecksum: 789",
			onProgress:      func(int) {},
			onProgressCalls: 1,
			wantProgress:    3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var gotProgress, gotOnProgressCalls int
			if tt.onProgress != nil {
				tt.onProgress = func(progress int) {
					gotProgress += progress
					gotOnProgressCalls++
				}
			}

			pw := &progressWriter{
				onProgress: tt.onProgress,
			}

			inputBytes := []byte(tt.p)
			n, err := pw.Write(inputBytes)
			if err != nil {
				t.Fatalf("Write() unexpected error: %v", err)
			}

			if n != len(inputBytes) {
				t.Errorf("Write() returned n=%d, want %d", n, len(inputBytes))
			}

			if gotProgress != tt.wantProgress {
				t.Errorf("onProgress callback invoked for %d progress, want %d progress", gotProgress, tt.wantProgress)
			}

			if gotOnProgressCalls != tt.onProgressCalls {
				t.Errorf("onProgress called %d times, want %d times", gotOnProgressCalls, tt.onProgressCalls)
			}
		})
	}
}

func newMockClient(t *testing.T, runMock *runMock, opts ...Option) *Client {
	t.Helper()

	client, err := NewClient(opts...)
	if err != nil {
		t.Fatalf("failed to create mock client: %v", err)
	}

	run := func(ctx context.Context, env []string, args ...string) ([]byte, error) {
		gotEnv := make([]string, 0, len(env))
		for _, envVar := range env {
			gotEnvVar, _, _ := strings.Cut(envVar, "=")
			gotEnv = append(gotEnv, gotEnvVar)
		}

		if diff := cmp.Diff(runMock.wantEnv, gotEnv); diff != "" {
			t.Errorf("runMock env mismatch (-want +got):\n%s", diff)
		}

		runMock.wantEnv = env

		if diff := cmp.Diff(runMock.wantArgs, args); diff != "" {
			t.Errorf("runMock args mismatch (-want +got):\n%s", diff)
		}

		if runMock.err != nil {
			return nil, runMock.err
		}

		out := []byte(trimIndent(runMock.out))

		return out, nil
	}

	client.run = run

	return client
}

func trimIndent(s string) string {
	s = strings.TrimPrefix(s, "\n")

	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimLeft(line, "\t ")
	}

	return strings.Join(lines, "\n")
}
