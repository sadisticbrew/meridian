package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var (
	dbPath     string
	configPath string
	jsonOut    bool

	rootCmd = &cobra.Command{
		Use:           "meridian",
		Short:         "personal tracking cli",
		SilenceErrors: true,
		SilenceUsage:  false,
	}
)

func init() {
	rootCmd.PersistentFlags().StringVar(&dbPath, "db", "", "database path")
	rootCmd.PersistentFlags().StringVar(&configPath, "config", "", "config file path")
	rootCmd.PersistentFlags().BoolVar(&jsonOut, "json", false, "machine-readable output")
}

func Execute() int {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	return 0
}

// DBFlag returns the effective database path: --db if set, else the XDG
// default ($XDG_DATA_HOME/meridian/meridian.db, else ~/.local/share/...).
func DBFlag() string {
	if dbPath != "" {
		return dbPath
	}
	if dir := os.Getenv("XDG_DATA_HOME"); dir != "" {
		return filepath.Join(dir, "meridian", "meridian.db")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".local", "share", "meridian", "meridian.db")
}

// ConfigFlag returns the raw --config value; default resolution is a later task.
func ConfigFlag() string { return configPath }

// JSONFlag reports whether --json was passed.
func JSONFlag() bool { return jsonOut }
