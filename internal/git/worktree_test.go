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
)

func TestClient_CreateWorktree(t *testing.T) {
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

			ctx := t.Context()

			removeWorktreeCalled := false
			run := func(ctx context.Context, args ...string) ([]byte, error) {
				switch args[1] {
				case "prune":
					return nil, nil
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

			worktreeBaseTempDir := t.TempDir()

			opts := []Option{
				withWorktreeBaseDir(worktreeBaseTempDir),
				withRun(run),
			}

			client, err := NewClient(ctx, opts...)
			if err != nil {
				t.Fatalf("failed to create client: %v", err)
			}

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

			if !strings.HasPrefix(worktree, worktreeBaseTempDir) {
				t.Errorf("expected worktree to be created inside %s, got %s", worktreeBaseTempDir, worktree)
			}
		})
	}
}

func TestClient_SyncDirtyFiles(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		runMock       *runMock
		setupRepo     map[string]string
		setupWorktree map[string]string
		wantWorktree  map[string]string
		wantErr       bool
	}{
		{
			name: "empty status string performs no actions",
			runMock: &runMock{
				wantArgs: []string{"status", "-z"},
				out:      "",
			},
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
			name: "deleted file is removed from worktree",
			runMock: &runMock{
				wantArgs: []string{"status", "-z"},
				out:      " D deleted.go\x00",
			},
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
			name: "deleted file already missing from worktree",
			runMock: &runMock{
				wantArgs: []string{"status", "-z"},
				out:      "D  already_missing.go\x00",
			},
			setupRepo:     map[string]string{},
			setupWorktree: map[string]string{},
			wantWorktree:  map[string]string{},
			wantErr:       false,
		},
		{
			name: "modified file is copied from repo to worktree",
			runMock: &runMock{
				wantArgs: []string{"status", "-z"},
				out:      " M modified.go\x00",
			},
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
			name: "copied file is created in worktree without deleting original",
			runMock: &runMock{
				wantArgs: []string{"status", "-z"},
				out:      "C  copied.go\x00original.go\x00",
			},
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
			name: "renamed file deletes old and copies new",
			runMock: &runMock{
				wantArgs: []string{"status", "-z"},
				out:      "R  new.go\x00old.go\x00",
			},
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
			name: "renamed file old file already missing",
			runMock: &runMock{
				wantArgs: []string{"status", "-z"},
				out:      "R  new.go\x00old.go\x00",
			},
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
			name: "git status fails",
			runMock: &runMock{
				wantArgs: []string{"status", "-z"},
				err:      errors.New("git status failed"),
			},
			wantWorktree: map[string]string{},
			wantErr:      true,
		},
		{
			name: "copyFile fails",
			runMock: &runMock{
				wantArgs: []string{"status", "-z"},
				out:      "M  missing.go\x00",
			},
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

			ctx := t.Context()
			client := newMockClient(ctx, t, tt.runMock, WithDir(repo))
			err := client.SyncDirtyFiles(ctx, worktree)

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

func TestClient_CopyFromWorktree(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		relPath       string
		setupRepo     map[string]string
		setupWorktree map[string]string
		wantRepo      map[string]string
		wantErr       bool
	}{
		{
			name:    "successful copy overwrites file in repo",
			relPath: "main.go",
			setupRepo: map[string]string{
				"main.go": "old repo content",
			},
			setupWorktree: map[string]string{
				"main.go": "new worktree content",
			},
			wantRepo: map[string]string{
				"main.go": "new worktree content",
			},
			wantErr: false,
		},
		{
			name:          "file missing from worktree",
			relPath:       "missing.go",
			setupRepo:     map[string]string{},
			setupWorktree: map[string]string{},
			wantRepo:      map[string]string{},
			wantErr:       true,
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

			runMock := &runMock{}
			client := newMockClient(t.Context(), t, runMock, WithDir(repo))
			err := client.CopyFromWorktree(tt.relPath, worktree)

			if (err != nil) != tt.wantErr {
				t.Fatalf("CopyFromWorktree() error = %v, wantErr %v", err, tt.wantErr)
			}

			gotRepo := map[string]string{}
			walkFn := func(path string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				}

				if d.IsDir() {
					return nil
				}

				content, err := os.ReadFile(filepath.Join(repo, path))
				if err != nil {
					return err
				}

				gotRepo[path] = string(content)

				return nil
			}

			err = fs.WalkDir(os.DirFS(repo), ".", walkFn)
			if err != nil {
				t.Fatalf("failed to walk repo for assertions: %v", err)
			}

			if diff := cmp.Diff(tt.wantRepo, gotRepo); diff != "" {
				t.Errorf("CopyFromWorktree() repo mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
