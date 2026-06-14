package mutesting

import (
	"context"
	"errors"
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

func TestMutest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		runMock *runMock
		want    string
		wantErr bool
	}{
		{
			name: "successful mutation test returns summary",
			runMock: &runMock{
				wantArgs: []string{
					"--html-output",
					"--blacklist=go-mutesting.blacklist",
					"--disable=arithmetic/assign_invert",
					"--disable=arithmetic/assignment",
					"--disable=arithmetic/base",
					"--disable=arithmetic/bitwise",
					"--disable=loop/break",
					"--disable=conditional/negated",
					"--disable=expression/comparison",
					"./...",
				},
				out: `
					PASS "..."
					PASS "..."
					The mutation score is 0.850000 (85 / 100)
				`,
				err: nil,
			},
			want:    "The mutation score is 0.850000 (85 / 100)",
			wantErr: false,
		},
		{
			name: "returns error if command fails",
			runMock: &runMock{
				wantArgs: []string{
					"--html-output",
					"--blacklist=go-mutesting.blacklist",
					"--disable=arithmetic/assign_invert",
					"--disable=arithmetic/assignment",
					"--disable=arithmetic/base",
					"--disable=arithmetic/bitwise",
					"--disable=loop/break",
					"--disable=conditional/negated",
					"--disable=expression/comparison",
					"./...",
				},
				out: "some error output",
				err: errors.New("command failed"),
			},
			want:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client := newMockClient(t, tt.runMock)
			got, err := client.Mutest(context.Background())

			if (err != nil) != tt.wantErr {
				t.Fatalf("Mutest() error = %v, wantErr %v", err, tt.wantErr)
			}

			if got != tt.want {
				t.Errorf("Mutest() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func newMockClient(t *testing.T, runMock *runMock) *Client {
	t.Helper()

	run := func(ctx context.Context, args ...string) ([]byte, error) {
		if diff := cmp.Diff(runMock.wantArgs, args); diff != "" {
			t.Errorf("runMock args mismatch (-want +got):\n%s", diff)
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
