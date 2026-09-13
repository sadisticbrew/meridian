package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

const DefaultNeetCodeURL = "https://github.com/sadisticbrew/neetcode-submissions.git"

type Config struct {
	Collectors CollectorsConfig `toml:"collectors"`
}

type CollectorsConfig struct {
	NeetCode NeetCodeConfig `toml:"neetcode"`
}

type NeetCodeConfig struct {
	URL string `toml:"url"`
}

func defaultConfig() Config {
	return Config{Collectors: CollectorsConfig{NeetCode: NeetCodeConfig{URL: DefaultNeetCodeURL}}}
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
	return cfg, nil
}
