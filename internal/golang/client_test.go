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
		dirs    []string
		wantDir string
		wantErr bool
	}{
		{
			name:    "zero arguments",
			dirs:    nil,
			wantDir: "",
			wantErr: false,
		},
		{
			name:    "one argument",
			dirs:    []string{"/tmp/workspace"},
			wantDir: "/tmp/workspace",
			wantErr: false,
		},
		{
			name:    "two arguments returns error",
			dirs:    []string{"/tmp/workspace", "/var/lib"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client, err := NewClient(tt.dirs...)
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

func TestModulePath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		runMock *runMock
		want    string
		wantErr bool
	}{
		{
			name: "successfully returns module path",
			runMock: &runMock{
				wantArgs: []string{"list", "-m"},
				out:      "github.com/example/repo\n",
				err:      nil,
			},
			want:    "github.com/example/repo",
			wantErr: false,
		},
		{
			name: "returns error if command fails",
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
			got, err := client.ModulePath(context.Background())

			if (err != nil) != tt.wantErr {
				t.Fatalf("ModulePath() error = %v, wantErr %v", err, tt.wantErr)
			}

			if got != tt.want {
				t.Errorf("ModulePath() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGenerateCoverProfile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		runMock *runMock
		wantErr bool
	}{
		{
			name: "successfully generates and returns coverage profile path",
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

			client := newMockClient(t, tt.runMock, dir)
			got, err := client.GenerateCoverProfile(context.Background())

			if (err != nil) != tt.wantErr {
				t.Fatalf("GenerateCoverProfile() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr && got != filepath.Join(dir, "coverage.out") {
				t.Errorf("GenerateCoverProfile() got = %v, want %v", got, filepath.Join(dir, "coverage.out"))
			}
		})
	}
}

func newMockClient(t *testing.T, runMock *runMock, dir ...string) *Client {
	t.Helper()

	if len(dir) > 1 {
		t.Fatalf("")
	}

	workDir := ""
	if len(dir) > 0 {
		workDir = dir[0]
	}

	run := func(ctx context.Context, args ...string) ([]byte, error) {
		if diff := cmp.Diff(runMock.wantArgs, args); diff != "" {
			t.Errorf("Show() args mismatch (-want +got):\n%s", diff)
		}

		if runMock.coverProfiles != nil {
			coverProfile := filepath.Join(workDir, "coverage.out")
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

	client := &Client{
		dir: workDir,
		run: run,
	}

	return client
}
