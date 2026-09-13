package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, path, body string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoad(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(t *testing.T) string
		wantURL string
		wantErr bool
	}{
		{
			name: "empty path with no default file",
			setup: func(t *testing.T) string {
				t.Setenv("XDG_CONFIG_HOME", t.TempDir())
				return ""
			},
			wantURL: DefaultNeetCodeURL,
		},
		{
			name: "empty path loads default file when present",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				t.Setenv("XDG_CONFIG_HOME", dir)
				writeConfig(t, filepath.Join(dir, "meridian", "meridian.toml"), "[collectors.neetcode]\nurl = \"https://example.com/xdg.git\"\n")
				return ""
			},
			wantURL: "https://example.com/xdg.git",
		},
		{
			name: "url override",
			setup: func(t *testing.T) string {
				return writeConfig(t, filepath.Join(t.TempDir(), "meridian.toml"), "[collectors.neetcode]\nurl = \"https://example.com/override.git\"\n")
			},
			wantURL: "https://example.com/override.git",
		},
		{
			name: "neetcode section without url",
			setup: func(t *testing.T) string {
				return writeConfig(t, filepath.Join(t.TempDir(), "meridian.toml"), "[collectors.neetcode]\n")
			},
			wantURL: DefaultNeetCodeURL,
		},
		{
			name: "explicit path missing",
			setup: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "absent.toml")
			},
			wantErr: true,
		},
		{
			name: "explicit path invalid toml",
			setup: func(t *testing.T) string {
				return writeConfig(t, filepath.Join(t.TempDir(), "meridian.toml"), "[collectors.neetcode\nurl = \n")
			},
			wantErr: true,
		},
		{
			name: "unrelated keys parse fine",
			setup: func(t *testing.T) string {
				body := strings.Join([]string{
					"[quantify]",
					`provider = "opencode"`,
					"",
					"[hooks.repos]",
					`"/home/caffeine/projects/x" = "project/x"`,
				}, "\n")
				return writeConfig(t, filepath.Join(t.TempDir(), "meridian.toml"), body)
			},
			wantURL: DefaultNeetCodeURL,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tt.setup(t)
			cfg, err := Load(path)
			if tt.wantErr {
				if err == nil {
					t.Fatal("Load returned nil error")
				}
				return
			}
			if err != nil {
				t.Fatalf("Load(%q): %v", path, err)
			}
			if cfg.Collectors.NeetCode.URL != tt.wantURL {
				t.Errorf("URL = %q, want %q", cfg.Collectors.NeetCode.URL, tt.wantURL)
			}
		})
	}
}
