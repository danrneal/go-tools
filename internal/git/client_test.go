package git

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// runMock configures expected command arguments and simulated output for testing Client commands.
type runMock struct {
	wantArgs []string
	out      string
	err      error
}

func TestNewClient(t *testing.T) {
	t.Parallel()

	testDir := t.TempDir()
	testWorktreeBaseDir := t.TempDir()

	readOnlyDir := t.TempDir()
	if err := os.Chmod(readOnlyDir, 0o500); err != nil {
		t.Fatalf("failed to chmod read-only dir: %v", err)
	}

	tests := []struct {
		name                string
		opts                []Option
		runMock             *runMock
		wantWorktreeBaseDir string
		wantErr             bool
		errTarget           error
	}{
		{
			name:                "default initialization",
			opts:                nil,
			wantWorktreeBaseDir: "",
			wantErr:             false,
		},
		{
			name: "successful initialization",
			opts: []Option{
				WithWorktreeBaseDir(testWorktreeBaseDir),
			},
			runMock: &runMock{
				wantArgs: []string{"worktree", "prune"},
			},
			wantWorktreeBaseDir: testWorktreeBaseDir,
			wantErr:             false,
		},
		{
			name: "clean worktree base dir fails",
			opts: []Option{
				WithWorktreeBaseDir(t.TempDir() + "/."),
			},
			runMock: &runMock{
				wantArgs: []string{"worktree", "prune"},
			},
			wantErr:   true,
			errTarget: syscall.EINVAL,
		},
		{
			name: "create worktree base dir fails",
			opts: []Option{
				WithWorktreeBaseDir(filepath.Join(readOnlyDir, "worktrees")),
			},
			runMock: &runMock{
				wantArgs: []string{"worktree", "prune"},
			},
			wantErr:   true,
			errTarget: fs.ErrPermission,
		},
		{
			name: "prune worktrees fails",
			opts: []Option{
				WithWorktreeBaseDir(testDir),
			},
			runMock: &runMock{
				wantArgs: []string{"worktree", "prune"},
				err:      errors.New("simulated git prune failure"),
			},
			wantWorktreeBaseDir: "",
			wantErr:             true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			opts := tt.opts
			if tt.runMock != nil {
				run := func(ctx context.Context, args ...string) ([]byte, error) {
					if diff := cmp.Diff(tt.runMock.wantArgs, args); diff != "" {
						t.Errorf("NewClient() args mismatch (-want +got):\n%s", diff)
					}

					if tt.runMock.err != nil {
						return nil, tt.runMock.err
					}

					out := []byte(tt.runMock.out)

					return out, nil
				}

				opts = append(opts, withRun(run))
			}

			client, err := NewClient(t.Context(), opts...)
			if (err != nil) != tt.wantErr {
				t.Fatalf("NewClient() error = %v, wantErr %v", err, tt.wantErr)
			}

			if tt.errTarget != nil && !errors.Is(err, tt.errTarget) {
				t.Errorf("NewClient() error = %v, want %v", err, tt.errTarget)
			}

			if tt.runMock != nil && tt.runMock.err != nil && !errors.Is(err, tt.runMock.err) {
				t.Errorf("NewClient() error = %v, want %v", err, tt.runMock.err)
			}

			if tt.wantErr {
				return
			}

			if client.worktreeBaseDir != tt.wantWorktreeBaseDir {
				t.Errorf("NewClient() worktreeBaseDir = %v, want %v", client.worktreeBaseDir, tt.wantWorktreeBaseDir)
			}

			if client.run == nil {
				t.Error("NewClient() run is nil")
			}
		})
	}
}

func TestClient_LastCommit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		path     string
		runMock  *runMock
		wantHead string
		wantErr  bool
	}{
		{
			name: "successfully retrieves and trims commit",
			path: ".gomutants-ignore",
			runMock: &runMock{
				wantArgs: []string{"log", "-1", "--format=%H", "--", ".gomutants-ignore"},
				out:      " a1b2c3d4e5f6 \n",
			},
			wantHead: "a1b2c3d4e5f6",
			wantErr:  false,
		},
		{
			name: "git command fails",
			path: ".gomutants-ignore",
			runMock: &runMock{
				wantArgs: []string{"log", "-1", "--format=%H", "--", ".gomutants-ignore"},
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
			got, err := client.LastCommit(ctx, tt.path)

			if (err != nil) != tt.wantErr {
				t.Fatalf("LastCommit() error = %v, wantErr %v", err, tt.wantErr)
			}

			if tt.runMock.err != nil && !errors.Is(err, tt.runMock.err) {
				t.Errorf("LastCommit() error = %v, want %v", err, tt.runMock.err)
			}

			if got != tt.wantHead {
				t.Errorf("LastCommit() got = %v, want %v", got, tt.wantHead)
			}
		})
	}
}

func TestClient_Show(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		commit  string
		path    string
		runMock *runMock
		wantOut string
		wantErr bool
	}{
		{
			name:   "valid execution",
			commit: "a1b2c3",
			path:   "main.go",
			runMock: &runMock{
				wantArgs: []string{"show", "a1b2c3:main.go"},
				out:      "file contents",
			},
			wantOut: "file contents",
			wantErr: false,
		},
		{
			name:   "git command fails",
			commit: "HEAD",
			path:   "missing.go",
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
			reader, err := client.Show(ctx, tt.commit, tt.path)

			if (err != nil) != tt.wantErr {
				t.Fatalf("Show() error = %v, wantErr %v", err, tt.wantErr)
			}

			if tt.runMock.err != nil && !errors.Is(err, tt.runMock.err) {
				t.Errorf("Show() error = %v, want %v", err, tt.runMock.err)
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

// newMockClient creates a Client wired to the provided runMock for unit testing without invoking git.
func newMockClient(ctx context.Context, t *testing.T, runMock *runMock, opts ...Option) *Client {
	t.Helper()

	run := func(ctx context.Context, args ...string) ([]byte, error) {
		if diff := cmp.Diff(runMock.wantArgs, args); diff != "" {
			t.Errorf("Show() args mismatch (-want +got):\n%s", diff)
		}

		if runMock.err != nil {
			return nil, runMock.err
		}

		out := []byte(runMock.out)

		return out, nil
	}

	opts = append(opts, withRun(run))

	client, err := NewClient(ctx, opts...)
	if err != nil {
		t.Fatalf("failed to create mock client: %v", err)
	}

	return client
}
