package golang

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
)

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

func TestClient_ModulePath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		runMock *runMock
		want    string
		wantErr bool
	}{
		{
			name: "valid execution",
			runMock: &runMock{
				wantArgs: []string{"list", "-m"},
				out:      "github.com/example/repo\n",
				err:      nil,
			},
			want:    "github.com/example/repo",
			wantErr: false,
		},
		{
			name: "command fails",
			runMock: &runMock{
				wantArgs: []string{"list", "-m"},
				out:      "",
				err:      errors.New("go command failed"),
			},
			want:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client := newMockClient(t, tt.runMock)
			got, err := client.ModulePath(t.Context())

			if (err != nil) != tt.wantErr {
				t.Fatalf("ModulePath() error = %v, wantErr %v", err, tt.wantErr)
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
		wantErr bool
	}{
		{
			name: "valid execution",
			runMock: &runMock{
				wantArgs:      []string{"test", "-coverprofile=coverage.out", "./..."},
				coverProfiles: new("dummy cover profiles"),
				out:           "ok\n",
				err:           nil,
			},
			wantErr: false,
		},
		{
			name: "tests failed and profile missing",
			runMock: &runMock{
				wantArgs: []string{"test", "-coverprofile=coverage.out", "./..."},
				out:      "build failed\n",
				err:      errors.New("exit status 1"),
			},
			wantErr: true,
		},
		{
			name: "tests passed but profile is empty",
			runMock: &runMock{
				wantArgs:      []string{"test", "-coverprofile=coverage.out", "./..."},
				coverProfiles: new(""),
				out:           "ok\n",
				err:           nil,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()

			client := newMockClient(t, tt.runMock, WithDir(dir))
			got, err := client.GenerateCoverProfile(t.Context())

			if (err != nil) != tt.wantErr {
				t.Fatalf("GenerateCoverProfile() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr && got != filepath.Join(dir, "coverage.out") {
				t.Errorf("GenerateCoverProfile() got = %v, want %v", got, filepath.Join(dir, "coverage.out"))
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
