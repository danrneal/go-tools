package git

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/google/go-cmp/cmp"
)

type runMock struct {
	wantArgs []string
	out      string
	err      error
}

func TestNewClient(t *testing.T) {
	t.Parallel()

	testDir := t.TempDir()
	testWorktreeBaseDir := t.TempDir()

	tests := []struct {
		name                string
		opts                []Option
		wantDir             string
		wantWorktreeBaseDir string
		wantErr             bool
	}{
		{
			name: "successful initialization",
			opts: []Option{
				WithDir(testDir),
				withRun(func(ctx context.Context, args ...string) ([]byte, error) {
					return nil, nil
				}),
			},
			wantDir:             testDir,
			wantWorktreeBaseDir: testWorktreeBaseDir,
			wantErr:             false,
		},
		{
			name: "prune worktrees fails",
			opts: []Option{
				withRun(func(ctx context.Context, args ...string) ([]byte, error) {
					if len(args) >= 2 && args[0] == "worktree" && args[1] == "prune" {
						return nil, errors.New("simulated git prune failure")
					}

					return nil, nil
				}),
			},
			wantDir:             "",
			wantWorktreeBaseDir: "",
			wantErr:             true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client, err := NewClient(t.Context(), testWorktreeBaseDir, tt.opts...)
			if (err != nil) != tt.wantErr {
				t.Fatalf("NewClient() error = %v, wantErr %v", err, tt.wantErr)
			}

			if tt.wantErr {
				return
			}

			if client.dir != tt.wantDir {
				t.Errorf("NewClient() dir = %v, want %v", client.dir, tt.wantDir)
			}

			if client.worktreeBaseDir != tt.wantWorktreeBaseDir {
				t.Errorf("NewClient() worktreeBaseDir = %v, want %v", client.worktreeBaseDir, tt.wantWorktreeBaseDir)
			}
		})
	}
}

func TestClient_LastCommit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		relPath  string
		runMock  *runMock
		wantHead string
		wantErr  bool
	}{
		{
			name:    "successfully retrieves and trims commit",
			relPath: ".go-mutesting-ignore",
			runMock: &runMock{
				wantArgs: []string{"log", "-1", "--format=%H", "--", ".go-mutesting-ignore"},
				out:      " a1b2c3d4e5f6 \n",
			},
			wantHead: "a1b2c3d4e5f6",
			wantErr:  false,
		},
		{
			name:    "git command fails",
			relPath: ".go-mutesting-ignore",
			runMock: &runMock{
				wantArgs: []string{"log", "-1", "--format=%H", "--", ".go-mutesting-ignore"},
				err:      errors.New("git crashed"),
			},
			wantHead: "",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := t.Context()
			client := newMockClient(ctx, t, tt.runMock)
			got, err := client.LastCommit(ctx, tt.relPath)

			if (err != nil) != tt.wantErr {
				t.Fatalf("LatestCommitHash() error = %v, wantErr %v", err, tt.wantErr)
			}

			if got != tt.wantHead {
				t.Errorf("LatestCommitHash() got = %v, want %v", got, tt.wantHead)
			}
		})
	}
}

func TestClient_Show(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		commit  string
		relPath string
		runMock *runMock
		wantOut string
		wantErr bool
	}{
		{
			name:    "valid execution",
			commit:  "a1b2c3",
			relPath: "main.go",
			runMock: &runMock{
				wantArgs: []string{"show", "a1b2c3:main.go"},
				out:      "file contents",
			},
			wantOut: "file contents",
			wantErr: false,
		},
		{
			name:    "git command fails",
			commit:  "HEAD",
			relPath: "missing.go",
			runMock: &runMock{
				wantArgs: []string{"show", "HEAD:missing.go"},
				err:      errors.New("git crashed"),
			},
			wantOut: "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := t.Context()
			client := newMockClient(ctx, t, tt.runMock)
			reader, err := client.Show(ctx, tt.commit, tt.relPath)

			if (err != nil) != tt.wantErr {
				t.Fatalf("Show() error = %v, wantErr %v", err, tt.wantErr)
			}

			if tt.wantErr {
				return
			}

			gotOut, err := io.ReadAll(reader)
			if err != nil {
				t.Fatalf("failed to read from Show() reader: %v", err)
			}

			if string(gotOut) != tt.wantOut {
				t.Errorf("Show() got = %v, want %v", string(gotOut), tt.wantOut)
			}
		})
	}
}

func newMockClient(ctx context.Context, t *testing.T, runMock *runMock, opts ...Option) *Client {
	t.Helper()

	run := func(ctx context.Context, args ...string) ([]byte, error) {
		return nil, nil
	}

	opts = append(opts, withRun(run))

	client, err := NewClient(ctx, t.TempDir(), opts...)
	if err != nil {
		t.Fatalf("failed to create mock client: %v", err)
	}

	run = func(ctx context.Context, args ...string) ([]byte, error) {
		if diff := cmp.Diff(runMock.wantArgs, args); diff != "" {
			t.Errorf("Show() args mismatch (-want +got):\n%s", diff)
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
