package golang

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

// runMock configures expected command arguments, optional coverage output, and simulated results for tests.
type runMock struct {
	wantArgs      []string
	coverProfiles *string
	out           string
	err           error
}

func TestNewClient(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		opts    []Option
		wantDir string
	}{
		{
			name: "successful initialization",
			opts: []Option{
				WithDir("/tmp/workspace"),
			},
			wantDir: "/tmp/workspace",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client := NewClient(tt.opts...)

			if client.dir != tt.wantDir {
				t.Errorf("NewClient() dir = %v, want %v", client.dir, tt.wantDir)
			}

			if client.run == nil {
				t.Error("NewClient() run is nil")
			}
		})
	}
}

func TestClient_ModulePath(t *testing.T) {
	t.Parallel()

	errMockRun := errors.New("go command failed")

	tests := []struct {
		name    string
		runMock *runMock
		want    string
		wantErr error
	}{
		{
			name: "valid execution",
			runMock: &runMock{
				wantArgs: []string{"list", "-m"},
				out:      "github.com/example/repo\n",
				err:      nil,
			},
			want: "github.com/example/repo",
		},
		{
			name: "command fails",
			runMock: &runMock{
				wantArgs: []string{"list", "-m"},
				out:      "",
				err:      errMockRun,
			},
			want:    "",
			wantErr: errMockRun,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client := newMockClient(t, tt.runMock)
			got, err := client.ModulePath(t.Context())

			if diff := cmp.Diff(tt.wantErr, err, cmpopts.EquateErrors()); diff != "" {
				t.Fatalf("ModulePath() error mismatch (-want +got):\n%s", diff)
			}

			if got != tt.want {
				t.Errorf("ModulePath() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestClient_GenerateCoverProfile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		runMock *runMock
		wantErr error
	}{
		{
			name: "valid execution",
			runMock: &runMock{
				wantArgs:      []string{"test", "-coverprofile=coverage.out", "./..."},
				coverProfiles: new("dummy cover profiles"),
				out:           "ok\n",
				err:           nil,
			},
		},
		{
			name: "tests failed and profile missing",
			runMock: &runMock{
				wantArgs: []string{"test", "-coverprofile=coverage.out", "./..."},
				out:      "build failed\n",
				err:      errors.New("exit status 1"),
			},
			wantErr: ErrNoCoverage,
		},
		{
			name: "tests passed but profile is empty",
			runMock: &runMock{
				wantArgs:      []string{"test", "-coverprofile=coverage.out", "./..."},
				coverProfiles: new(""),
				out:           "ok\n",
				err:           nil,
			},
			wantErr: ErrNoCoverage,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()

			client := newMockClient(t, tt.runMock, WithDir(dir))
			got, err := client.GenerateCoverProfile(t.Context())

			if diff := cmp.Diff(tt.wantErr, err, cmpopts.EquateErrors()); diff != "" {
				t.Fatalf("GenerateCoverProfile() error mismatch (-want +got):\n%s", diff)
			}

			if tt.wantErr != nil && tt.runMock.err != nil && !errors.Is(err, tt.runMock.err) {
				t.Errorf("GenerateCoverProfile() error = %v, want %v", err, tt.runMock.err)
			}

			if tt.wantErr == nil && got != filepath.Join(dir, "coverage.out") {
				t.Errorf("GenerateCoverProfile() got = %v, want %v", got, filepath.Join(dir, "coverage.out"))
			}
		})
	}
}

// newMockClient creates a Client wired to the provided runMock for unit testing without invoking go.
func newMockClient(t *testing.T, runMock *runMock, opts ...Option) *Client {
	t.Helper()

	client := NewClient(opts...)

	run := func(ctx context.Context, args ...string) ([]byte, error) {
		if diff := cmp.Diff(runMock.wantArgs, args); diff != "" {
			t.Errorf("Show() args mismatch (-want +got):\n%s", diff)
		}

		if runMock.coverProfiles != nil {
			coverProfile := filepath.Join(client.dir, "coverage.out")
			coverProfiles := []byte(*runMock.coverProfiles)
			if err := os.WriteFile(coverProfile, coverProfiles, 0o644); err != nil {
				t.Fatalf("mock failed to write file: %v", err)
			}
		}

		if runMock.err != nil {
			return nil, runMock.err
		}

		out := []byte(runMock.out)

		return out, nil
	}

	client.run = run

	return client
}
