package git

import (
	"context"
	"errors"
	"io"
	"strings"
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
			name:    "two arguments",
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
				err:      nil,
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
				out:      "",
				err:      errors.New("git crashed"),
			},
			wantOut: "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client := newMockClient(t, tt.runMock)
			reader, err := client.Show(context.Background(), tt.commit, tt.relPath)

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

func TestClient_Head(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		runMock  *runMock
		wantHead string
		wantErr  bool
	}{
		{
			name: "successfully retrieves and trims HEAD",
			runMock: &runMock{
				wantArgs: []string{"rev-parse", "HEAD"},
				out:      " a1b2c3d4e5f6 \n",
				err:      nil,
			},
			wantHead: "a1b2c3d4e5f6",
			wantErr:  false,
		},
		{
			name: "git command fails",
			runMock: &runMock{
				wantArgs: []string{"rev-parse", "HEAD"},
				out:      "",
				err:      errors.New("git crashed"),
			},
			wantHead: "",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client := newMockClient(t, tt.runMock)
			got, err := client.Head(context.Background())

			if (err != nil) != tt.wantErr {
				t.Fatalf("Head() error = %v, wantErr %v", err, tt.wantErr)
			}

			if got != tt.wantHead {
				t.Errorf("Head() got = %v, want %v", got, tt.wantHead)
			}
		})
	}
}

func newMockClient(t *testing.T, runMock *runMock) *Client {
	t.Helper()

	run := func(ctx context.Context, args ...string) ([]byte, error) {
		if diff := cmp.Diff(runMock.wantArgs, args); diff != "" {
			t.Errorf("Show() args mismatch (-want +got):\n%s", diff)
		}

		if runMock.err != nil {
			return nil, runMock.err
		}

		out := []byte(trimIndent(runMock.out))

		return out, nil
	}

	client := &Client{
		run: run,
	}

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
