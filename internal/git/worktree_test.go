package git

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"testing"
)

func TestCreateDetachedWorktree(t *testing.T) {
	t.Parallel()

	repoDir := setupRepo(t)

	tests := []struct {
		name    string
		commit  string
		wantErr bool
	}{
		{
			name:    "valid commit creates worktree and cleans up successfully",
			commit:  "HEAD",
			wantErr: false,
		},
		{
			name:    "invalid commit returns error and cleans up immediately",
			commit:  "invalid-branch-name",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			worktree, cleanup, err := CreateDetachedWorktree(ctx, repoDir, tt.commit)

			if (err != nil) != tt.wantErr {
				t.Fatalf("CreateDetachedWorktree() error = %v, wantErr %v", err, tt.wantErr)
			}

			if err != nil {
				if _, statErr := os.Stat(worktree); !errors.Is(statErr, fs.ErrNotExist) {
					t.Errorf("expected worktree dir %s to be cleaned up after failure, but it exists", worktree)
				}

				return
			}

			if _, statErr := os.Stat(worktree); errors.Is(statErr, fs.ErrNotExist) {
				t.Fatalf("expected worktree dir %s to be created, but it does not exist", worktree)
			}

			cleanup()

			if _, statErr := os.Stat(worktree); !errors.Is(statErr, fs.ErrNotExist) {
				t.Errorf("cleanup() failed to remove directory %s", worktree)
			}
		})
	}
}

func setupRepo(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()

	cmds := [][]string{
		{"git", "init"},
		{"git", "config", "user.name", "Test User"},
		{"git", "config", "user.email", "test@example.com"},
		{"git", "commit", "--allow-empty", "-m", "initial commit"},
	}

	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("failed to setup test repo: %s\n%s", err, out)
		}
	}

	return dir
}
