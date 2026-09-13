package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/sadisticbrew/meridian/internal/store"
	"github.com/spf13/cobra"
)

// rt carries per-execution flag state; a fresh one is built for every
// Execute so commands never share mutable package state (table-driven
// CLI tests depend on this).
type rt struct {
	dbPath     string
	configPath string
	jsonOut    bool
	ran        bool
}

// db resolves the effective database path: --db if set, else the XDG
// default ($XDG_DATA_HOME/meridian/meridian.db, else ~/.local/share/...).
func (r *rt) db() string {
	if r.dbPath != "" {
		return r.dbPath
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

func (r *rt) open() (*store.Store, error) {
	return store.Open(r.db())
}

// usageError marks errors caused by bad command usage (exit 2). cobra
// still prints usage for these because run() only silences it for
// runtime errors.
type usageError struct{ err error }

func (e usageError) Error() string { return e.err.Error() }
func (e usageError) Unwrap() error { return e.err }

func usagef(format string, a ...any) error {
	return usageError{fmt.Errorf(format, a...)}
}

// exitError carries an explicit exit code; an empty msg is printed as
// nothing at all (e.g. exit 3 for "no running session", a normal state).
type exitError struct {
	code int
	msg  string
}

func (e *exitError) Error() string { return e.msg }

func exitSilent(code int) error { return &exitError{code: code} }

// run wraps a subcommand body: runtime errors suppress the usage dump,
// usage errors keep it (spec 01 exit-code semantics).
func run(cmd *cobra.Command, fn func() error) error {
	err := fn()
	if err != nil {
		var ue usageError
		if !errors.As(err, &ue) {
			cmd.SilenceUsage = true
		}
	}
	return err
}

// classify maps an execution error to the spec 01 exit code.
func classify(err error, ran bool) int {
	var ee *exitError
	if errors.As(err, &ee) {
		return ee.code
	}
	var ae *AmbiguousError
	if errors.As(err, &ae) {
		return 4
	}
	var ue usageError
	if errors.As(err, &ue) {
		return 2
	}
	if !ran {
		return 2
	}
	return 1
}

func Execute() int { return executeIO(os.Stdout, os.Stderr, os.Args[1:]) }

func executeIO(out, errW io.Writer, args []string) int {
	root, state := newRoot()
	root.SetOut(out)
	root.SetErr(errW)
	root.SetArgs(args)
	err := root.Execute()
	if err == nil {
		return 0
	}
	code := classify(err, state.ran)
	var ee *exitError
	if errors.As(err, &ee) && ee.msg == "" {
		return code
	}
	fmt.Fprintf(errW, "Error: %v\n", err)
	return code
}

func newRoot() (*cobra.Command, *rt) {
	state := &rt{}
	root := &cobra.Command{
		Use:           "meridian",
		Short:         "personal tracking cli",
		SilenceErrors: true,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			state.ran = true
		},
	}
	root.PersistentFlags().StringVar(&state.dbPath, "db", "", "database path")
	root.PersistentFlags().StringVar(&state.configPath, "config", "", "config file path")
	root.PersistentFlags().BoolVar(&state.jsonOut, "json", false, "machine-readable output")
	root.AddCommand(
		newTrackCmd(state),
		newInitCmd(state),
		newStartCmd(state),
		newStopCmd(state),
		newStatusCmd(state),
		newAbortCmd(state),
		newLogCmd(state),
		newNoteCmd(state),
		newTodayCmd(state),
		newWeekCmd(state),
		newSubjectCmd(state),
		newDumpCmd(state),
		newSyncCmd(state),
	)
	return root, state
}

func nowRFC3339() string { return time.Now().UTC().Format(time.RFC3339) }
