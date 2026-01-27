package common

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestGetLfsCustomTransferInt(t *testing.T) {
	configDir := t.TempDir()
	configPath := filepath.Join(configDir, "gitconfig")

	setConfig := func(t *testing.T, key, value string) {
		t.Helper()
		cmd := exec.Command("git", "config", "--file", configPath, key, value)
		if err := cmd.Run(); err != nil {
			t.Fatalf("set git config %s=%s: %v", key, value, err)
		}
	}

	setEnv := func(t *testing.T) {
		t.Helper()
		t.Setenv("GIT_CONFIG_GLOBAL", configPath)
		t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)
		t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	}

	const key = "lfs.customtransfer.drs.multipart-min-chunk-size"

	tests := []struct {
		name       string
		value      string
		defaultVal int64
		want       int64
		wantErr    bool
		setValue   bool
	}{
		{
			name:       "missing uses default",
			defaultVal: 10,
			want:       10,
			wantErr:    false,
			setValue:   false,
		},
		{
			name:       "valid value",
			value:      "25",
			defaultVal: 10,
			want:       25,
			wantErr:    false,
			setValue:   true,
		},
		{
			name:       "negative value",
			value:      "-3",
			defaultVal: 10,
			want:       10,
			wantErr:    true,
			setValue:   true,
		},
		{
			name:       "zero value",
			value:      "0",
			defaultVal: 10,
			want:       10,
			wantErr:    true,
			setValue:   true,
		},
		{
			name:       "over max",
			value:      "501",
			defaultVal: 10,
			want:       10,
			wantErr:    true,
			setValue:   true,
		},
		{
			name:       "non-integer",
			value:      "abc",
			defaultVal: 10,
			want:       10,
			wantErr:    true,
			setValue:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := os.WriteFile(configPath, nil, 0o600); err != nil {
				t.Fatalf("reset git config: %v", err)
			}
			if tt.setValue {
				setConfig(t, key, tt.value)
			}
			setEnv(t)

			got, err := GetLfsCustomTransferInt(key, tt.defaultVal)
			if tt.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("value = %d, want %d", got, tt.want)
			}
		})
	}
}
