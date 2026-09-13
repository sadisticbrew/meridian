package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

const (
	DefaultNeetCodeURL     = "https://github.com/sadisticbrew/neetcode-submissions.git"
	DefaultOpencodeCommand = "opencode run"
	DefaultProviderTimeout = "60s"
	DefaultHTTPKeyEnv      = "MERIDIAN_LLM_KEY"
)

type Config struct {
	Collectors CollectorsConfig `toml:"collectors"`
	Quantify   QuantifyConfig   `toml:"quantify"`
	Hooks      HooksConfig      `toml:"hooks"`
}

type CollectorsConfig struct {
	NeetCode NeetCodeConfig `toml:"neetcode"`
}

type NeetCodeConfig struct {
	URL string `toml:"url"`
}

// QuantifyConfig selects the normalizer provider and holds its settings
// (spec 04 "Providers").
type QuantifyConfig struct {
	Provider string         `toml:"provider"`
	Opencode OpencodeConfig `toml:"opencode"`
	HTTP     HTTPConfig     `toml:"http"`
}

type OpencodeConfig struct {
	Command string `toml:"command"`
	Model   string `toml:"model"`
	Timeout string `toml:"timeout"`
}

type HTTPConfig struct {
	URL       string `toml:"url"`
	APIKeyEnv string `toml:"api_key_env"`
	Model     string `toml:"model"`
	Timeout   string `toml:"timeout"`
}

// HooksConfig maps repo paths to project/* subjects for hook ingestion.
type HooksConfig struct {
	Repos map[string]string `toml:"repos"`
}

func defaultConfig() Config {
	return Config{
		Collectors: CollectorsConfig{NeetCode: NeetCodeConfig{URL: DefaultNeetCodeURL}},
		Quantify: QuantifyConfig{
			Opencode: OpencodeConfig{Command: DefaultOpencodeCommand, Timeout: DefaultProviderTimeout},
			HTTP:     HTTPConfig{APIKeyEnv: DefaultHTTPKeyEnv, Timeout: DefaultProviderTimeout},
		},
	}
}

// defaultPath mirrors internal/cli's db() resolution, in the config XDG slot.
func defaultPath() (string, error) {
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, "meridian", "meridian.toml"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "meridian", "meridian.toml"), nil
}

// Load reads the TOML config at path. An empty path resolves the XDG default,
// where a missing file is not an error; an explicit path must exist.
func Load(path string) (Config, error) {
	explicit := path != ""
	if !explicit {
		p, err := defaultPath()
		if err != nil {
			// No home dir means no config file to read; defaults still apply.
			return defaultConfig(), nil
		}
		path = p
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if !explicit && errors.Is(err, os.ErrNotExist) {
			return defaultConfig(), nil
		}
		return Config{}, fmt.Errorf("load config %s: %w", path, err)
	}
	var cfg Config
	if _, err := toml.Decode(string(data), &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config %s: %w", path, err)
	}
	if cfg.Collectors.NeetCode.URL == "" {
		cfg.Collectors.NeetCode.URL = DefaultNeetCodeURL
	}
	if cfg.Quantify.Opencode.Command == "" {
		cfg.Quantify.Opencode.Command = DefaultOpencodeCommand
	}
	if cfg.Quantify.Opencode.Timeout == "" {
		cfg.Quantify.Opencode.Timeout = DefaultProviderTimeout
	}
	if cfg.Quantify.HTTP.APIKeyEnv == "" {
		cfg.Quantify.HTTP.APIKeyEnv = DefaultHTTPKeyEnv
	}
	if cfg.Quantify.HTTP.Timeout == "" {
		cfg.Quantify.HTTP.Timeout = DefaultProviderTimeout
	}
	return cfg, nil
}
