package git

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestCreateWorktree(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		commit  string
		runErr  error
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
			runErr:  errors.New("git worktree failed"),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

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
				run: run,
			}

			ctx := context.Background()
			worktree, cleanup, err := client.CreateWorktree(ctx, tt.commit)

			if (err != nil) != tt.wantErr {
				t.Fatalf("CreateWorktree() error = %v, wantErr %v", err, tt.wantErr)
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

			if !removeWorktreeCalled {
				t.Errorf("cleanup() failed to call git worktree remove")
			}

			if _, statErr := os.Stat(worktree); !errors.Is(statErr, fs.ErrNotExist) {
				t.Errorf("cleanup() failed to remove directory %s", worktree)
			}
		})
	}
}

func TestSyncDirtyFiles(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		status        string
		runErr        error
		setupRepo     map[string]string
		setupWorktree map[string]string
		wantWorktree  map[string]string
		wantErr       bool
	}{
		{
			name:   "empty status string performs no actions",
			status: "",
			setupRepo: map[string]string{
				"foo.go": "repo content",
			},
			setupWorktree: map[string]string{
				"bar.go": "worktree content",
			},
			wantWorktree: map[string]string{
				"bar.go": "worktree content",
			},
			wantErr: false,
		},
		{
			name:      "deleted file is removed from worktree",
			status:    " D deleted.go\x00",
			setupRepo: map[string]string{},
			setupWorktree: map[string]string{
				"deleted.go": "worktree content",
				"kept.go":    "keep me",
			},
			wantWorktree: map[string]string{
				"kept.go": "keep me",
			},
			wantErr: false,
		},
		{
			name:          "deleted file ignores error if already missing from worktree",
			status:        "D  already_missing.go\x00",
			setupRepo:     map[string]string{},
			setupWorktree: map[string]string{},
			wantWorktree:  map[string]string{},
			wantErr:       false,
		},
		{
			name:   "modified file is copied from repo to worktree",
			status: " M modified.go\x00",
			setupRepo: map[string]string{
				"modified.go": "new content",
			},
			setupWorktree: map[string]string{
				"modified.go": "old content",
			},
			wantWorktree: map[string]string{
				"modified.go": "new content",
			},
			wantErr: false,
		},
		{
			name:   "copied file is created in worktree without deleting original",
			status: "C  copied.go\x00original.go\x00",
			setupRepo: map[string]string{
				"copied.go": "new content",
			},
			setupWorktree: map[string]string{
				"original.go": "old content",
			},
			wantWorktree: map[string]string{
				"original.go": "old content",
				"copied.go":   "new content",
			},
			wantErr: false,
		},
		{
			name:   "renamed file deletes old and copies new",
			status: "R  new.go\x00old.go\x00",
			setupRepo: map[string]string{
				"new.go": "new content",
			},
			setupWorktree: map[string]string{
				"old.go":  "old content",
				"kept.go": "keep me",
			},
			wantWorktree: map[string]string{
				"new.go":  "new content",
				"kept.go": "keep me",
			},
			wantErr: false,
		},
		{
			name:   "renamed file ignores error if old file is already missing",
			status: "R  new.go\x00old.go\x00",
			setupRepo: map[string]string{
				"new.go": "new content",
			},
			setupWorktree: map[string]string{},
			wantWorktree: map[string]string{
				"new.go": "new content",
			},
			wantErr: false,
		},
		{
			name:         "returns error if git status fails",
			status:       "",
			runErr:       errors.New("git status failed"),
			wantWorktree: map[string]string{},
			wantErr:      true,
		},
		{
			name:         "returns error if copyFile fails",
			status:       "M  missing.go\x00",
			setupRepo:    map[string]string{},
			wantWorktree: map[string]string{},
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			repo := t.TempDir()
			worktree := t.TempDir()

			for path, content := range tt.setupRepo {
				if err := os.MkdirAll(filepath.Join(repo, filepath.Dir(path)), 0o750); err != nil {
					t.Fatalf("failed to create repo dir for %s: %v", path, err)
				}

				if err := os.WriteFile(filepath.Join(repo, path), []byte(content), 0o644); err != nil {
					t.Fatalf("failed to write repo file %s: %v", path, err)
				}
			}

			for path, content := range tt.setupWorktree {
				if err := os.MkdirAll(filepath.Join(worktree, filepath.Dir(path)), 0o750); err != nil {
					t.Fatalf("failed to create worktree dir for %s: %v", path, err)
				}

				if err := os.WriteFile(filepath.Join(worktree, path), []byte(content), 0o644); err != nil {
					t.Fatalf("failed to write worktree file %s: %v", path, err)
				}
			}

			run := func(ctx context.Context, args ...string) ([]byte, error) {
				if tt.runErr != nil {
					return nil, tt.runErr
				}

				status := []byte(tt.status)

				return status, nil
			}

			client := &Client{
				dir: repo,
				run: run,
			}

			err := client.SyncDirtyFiles(context.Background(), worktree)

			if (err != nil) != tt.wantErr {
				t.Fatalf("SyncDirtyFiles() error = %v, wantErr %v", err, tt.wantErr)
			}

			gotWorktree := map[string]string{}
			walkFn := func(path string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				}

				if d.IsDir() {
					return nil
				}

				content, err := os.ReadFile(filepath.Join(worktree, path))
				if err != nil {
					return err
				}

				gotWorktree[path] = string(content)

				return nil
			}

			err = fs.WalkDir(os.DirFS(worktree), ".", walkFn)
			if err != nil {
				t.Fatalf("failed to walk worktree for assertions: %v", err)
			}

			if diff := cmp.Diff(tt.wantWorktree, gotWorktree); diff != "" {
				t.Errorf("SyncDirtyFiles() worktree mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
