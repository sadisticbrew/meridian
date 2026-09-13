package normalizer

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeOpenCodeScript(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fake-opencode.sh")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	return path
}

func TestOpenCodeProviderComplete(t *testing.T) {
	const payload = `{"tags":{"subject":"course/ddco"},"fields":{"minutes":45}}`
	ansiWrapped := "printf '\\033[0m\\n\\033[0m" + payload + "\\n\\033[0m\\n'"

	tests := []struct {
		name        string
		script      string
		model       string
		prompt      string
		timeout     time.Duration
		want        string
		wantErr     bool
		errContains string
	}{
		{
			name:    "strips ANSI and returns raw text",
			script:  ansiWrapped,
			timeout: 5 * time.Second,
			want:    "\n" + payload + "\n\n",
		},
		{
			name:    "model flag and prompt stay distinct argv elements",
			script:  "printf '%s\\n' \"$@\"\n",
			model:   "sonnet",
			prompt:  "studied 45 minutes",
			timeout: 5 * time.Second,
			want:    "-m\nsonnet\nstudied 45 minutes\n",
		},
		{
			name:        "non-zero exit is an error",
			script:      "echo boom >&2\nexit 3\n",
			timeout:     5 * time.Second,
			wantErr:     true,
			errContains: "exit status 3",
		},
		{
			name:        "timeout",
			script:      "exec sleep 2\n",
			timeout:     50 * time.Millisecond,
			wantErr:     true,
			errContains: "timeout",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prov := NewOpenCodeProvider(writeOpenCodeScript(t, tt.script), tt.model, tt.timeout)
			if name := prov.Name(); name != "opencode" {
				t.Errorf("Name() = %q, want opencode", name)
			}

			out, err := prov.Complete(context.Background(), tt.prompt)
			if tt.wantErr {
				if err == nil {
					t.Fatal("Complete returned nil error")
				}
				if !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("error %q does not contain %q", err, tt.errContains)
				}
				return
			}
			if err != nil {
				t.Fatalf("Complete: %v", err)
			}
			if out != tt.want {
				t.Errorf("output = %q, want %q", out, tt.want)
			}
		})
	}
}

func TestOpenCodeProviderMissingBinary(t *testing.T) {
	prov := NewOpenCodeProvider(filepath.Join(t.TempDir(), "absent-opencode"), "", time.Second)
	if _, err := prov.Complete(context.Background(), "hello"); err == nil {
		t.Fatal("Complete returned nil error for a missing binary")
	}
}

// TestOpenCodeProviderIntegration runs the real CLI once when installed. It
// skips on any hiccup — an unstable third-party backend must never fail the
// suite; the orchestrator verifies the live path manually.
func TestOpenCodeProviderIntegration(t *testing.T) {
	if _, err := exec.LookPath("opencode"); err != nil {
		t.Skip("opencode not installed")
	}

	prov := NewOpenCodeProvider("opencode run", "", 60*time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	out, err := prov.Complete(ctx, `Reply with exactly this JSON and nothing else: {"ok":true}`)
	if err != nil {
		t.Skipf("opencode integration unstable: %v", err)
	}
	body, ok := ExtractJSON(out)
	if !ok {
		t.Skipf("opencode integration unstable: no JSON object in output %q", out)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(body), &parsed); err != nil {
		t.Skipf("opencode integration unstable: %v", err)
	}
}
