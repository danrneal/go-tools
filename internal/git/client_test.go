package git

import (
	"context"
	"errors"
	"testing"
)

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

func TestHead(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		runOut   string
		runErr   error
		wantHead string
		wantErr  bool
	}{
		{
			name:     "successfully retrieves and trims HEAD",
			runOut:   " a1b2c3d4e5f6 \n",
			runErr:   nil,
			wantHead: "a1b2c3d4e5f6",
			wantErr:  false,
		},
		{
			name:     "returns error if git command fails",
			runOut:   "",
			runErr:   errors.New("git crashed"),
			wantHead: "",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client := &Client{
				run: func(ctx context.Context, args ...string) ([]byte, error) {
					if tt.runErr != nil {
						return nil, tt.runErr
					}

					return []byte(tt.runOut), nil
				},
			}

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
