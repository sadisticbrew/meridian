package cli

import (
	"strings"
	"testing"

	"github.com/sadisticbrew/meridian/internal/config"
)

func TestProviderFor(t *testing.T) {
	const keyEnv = "MERIDIAN_TEST_LLM_KEY"
	httpCfg := func() config.HTTPConfig {
		return config.HTTPConfig{URL: "http://127.0.0.1:1/v1/chat/completions", APIKeyEnv: keyEnv, Timeout: "5s"}
	}

	tests := []struct {
		name        string
		cfg         config.Config
		key         string
		override    string
		wantName    string
		wantErr     bool
		errContains []string
	}{
		{
			name:        "no provider configured",
			cfg:         config.Config{},
			wantErr:     true,
			errContains: []string{"no quantify provider configured", "[quantify]"},
		},
		{
			name:        "unknown provider",
			cfg:         config.Config{Quantify: config.QuantifyConfig{Provider: "openai"}},
			wantErr:     true,
			errContains: []string{"no quantify provider configured"},
		},
		{
			name: "http without the env var",
			cfg: config.Config{Quantify: config.QuantifyConfig{
				Provider: "http",
				HTTP:     httpCfg(),
			}},
			wantErr:     true,
			errContains: []string{keyEnv, "not set"},
		},
		{
			name: "http with the env var",
			cfg: config.Config{Quantify: config.QuantifyConfig{
				Provider: "http",
				HTTP:     httpCfg(),
			}},
			key:      "sk-secret",
			wantName: "http",
		},
		{
			name: "provider flag overrides config",
			cfg: config.Config{Quantify: config.QuantifyConfig{
				Provider: "opencode",
				Opencode: config.OpencodeConfig{Command: "opencode run", Timeout: "5s"},
				HTTP:     httpCfg(),
			}},
			key:      "sk-secret",
			override: "http",
			wantName: "http",
		},
		{
			name: "opencode from config",
			cfg: config.Config{Quantify: config.QuantifyConfig{
				Provider: "opencode",
				Opencode: config.OpencodeConfig{Command: "opencode run", Timeout: "5s"},
			}},
			wantName: "opencode",
		},
		{
			name: "invalid opencode timeout",
			cfg: config.Config{Quantify: config.QuantifyConfig{
				Provider: "opencode",
				Opencode: config.OpencodeConfig{Command: "opencode run", Timeout: "soon"},
			}},
			wantErr:     true,
			errContains: []string{"soon"},
		},
		{
			name: "invalid http timeout",
			cfg: config.Config{Quantify: config.QuantifyConfig{
				Provider: "http",
				HTTP:     config.HTTPConfig{URL: "http://127.0.0.1:1", APIKeyEnv: keyEnv, Timeout: "never"},
			}},
			key:         "sk-secret",
			wantErr:     true,
			errContains: []string{"never"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(keyEnv, tt.key)
			prov, err := providerFor(&rt{}, tt.cfg, tt.override, "")
			if tt.wantErr {
				if err == nil {
					t.Fatal("providerFor returned nil error")
				}
				for _, want := range tt.errContains {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("error %q does not contain %q", err, want)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("providerFor: %v", err)
			}
			if prov.Name() != tt.wantName {
				t.Errorf("provider = %q, want %q", prov.Name(), tt.wantName)
			}
		})
	}
}
