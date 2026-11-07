package git

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestClient_CreateWorktree(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		worktreeBaseDir string
		commit          string
		runErr          error
		wantErr         error
	}{
		{
			name:            "valid commit creates worktree and cleans up successfully",
			worktreeBaseDir: t.TempDir(),
			commit:          "HEAD",
		},
		{
			name:            "missing base dir returns error",
			worktreeBaseDir: filepath.Join(t.TempDir(), "missing"),
			commit:          "HEAD",
			wantErr:         fs.ErrNotExist,
		},
		{
			name:            "invalid commit returns error and cleans up immediately",
			worktreeBaseDir: t.TempDir(),
			commit:          "invalid-branch-name",
			runErr:          errors.New("git worktree failed"),
			wantErr:         cmpopts.AnyError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := t.Context()

			removeWorktreeCalled := false
			run := func(ctx context.Context, args ...string) ([]byte, error) {
				switch args[1] {
				case "add":
					wantArgs := []string{"worktree", "add", "--detach", args[3], tt.commit}
					if diff := cmp.Diff(wantArgs, args); diff != "" {
						t.Errorf("CreateWorktree() args mismatch (-want +got):\n%s", diff)
					}
				case "remove":
					removeWorktreeCalled = true
				}

				if tt.runErr != nil {
					return nil, tt.runErr
				}

				return nil, nil
			}

			client := &Client{
				worktreeBaseDir: tt.worktreeBaseDir,
				run:             run,
			}

			worktree, cleanup, err := client.CreateWorktree(ctx, tt.commit)

			if diff := cmp.Diff(tt.wantErr, err, cmpopts.EquateErrors()); diff != "" {
				t.Fatalf("CreateWorktree() error mismatch (-want +got):\n%s", diff)
			}

			if tt.runErr != nil && !errors.Is(err, tt.runErr) {
				t.Errorf("CreateWorktree() error = %v, want %v", err, tt.runErr)
			}

			if err != nil {
				files, err := os.ReadDir(tt.worktreeBaseDir)
				if err != nil && !errors.Is(err, fs.ErrNotExist) {
					t.Fatalf("failed to read worktree base dir: %v", err)
				}

				if len(files) != 0 {
					t.Errorf("expected %s to be empty after failure", tt.worktreeBaseDir)
				}

				return
			}

			if _, statErr := os.Stat(worktree); errors.Is(statErr, fs.ErrNotExist) {
				t.Fatalf("expected worktree dir %s to be created, but it does not exist", worktree)
			}

			cleanup()

			if !removeWorktreeCalled {
				t.Errorf("cleanup() failed to call git worktree remove")
			}

			if _, statErr := os.Stat(worktree); !errors.Is(statErr, fs.ErrNotExist) {
				t.Errorf("cleanup() failed to remove directory %s", worktree)
			}

			if !strings.HasPrefix(worktree, tt.worktreeBaseDir) {
				t.Errorf("expected worktree to be created inside %s, got %s", tt.worktreeBaseDir, worktree)
			}
		})
	}
}
