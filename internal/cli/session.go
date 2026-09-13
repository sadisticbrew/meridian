package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/sadisticbrew/meridian/internal/model"
	"github.com/sadisticbrew/meridian/internal/store"
	"github.com/spf13/cobra"
)

// fmtCompact renders a duration in whole minutes as "32m" or "1h10m".
// Local to avoid an import of internal/render (parallel work).
func fmtCompact(seconds int) string {
	minutes := seconds / 60
	if minutes < 60 {
		return fmt.Sprintf("%dm", minutes)
	}
	return fmt.Sprintf("%dh%02dm", minutes/60, minutes%60)
}

func parseStartedAt(rs *store.RunningSession) (time.Time, error) {
	started, err := time.Parse(time.RFC3339, rs.StartedAt)
	if err != nil {
		return time.Time{}, fmt.Errorf("running session: invalid started_at %q: %w", rs.StartedAt, err)
	}
	return started.UTC(), nil
}

func newStartCmd(state *rt) *cobra.Command {
	var note string
	cmd := &cobra.Command{
		Use:   "start <subject>",
		Short: "start a focus session",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return usagef("start requires exactly one <subject> argument")
			}
			return run(cmd, func() error {
				return sessionStart(state, cmd.OutOrStdout(), args[0], note)
			})
		},
	}
	cmd.Flags().StringVar(&note, "note", "", "session note")
	return cmd
}

func sessionStart(state *rt, out io.Writer, input, note string) error {
	st, err := state.open()
	if err != nil {
		return err
	}
	defer st.Close()
	t, err := resolveThing(st, input)
	if err != nil {
		return err
	}
	rs, err := st.RunningSession()
	if err != nil {
		return err
	}
	if rs != nil {
		started, err := parseStartedAt(rs)
		if err != nil {
			return err
		}
		return fmt.Errorf("session already running: %s — %s",
			rs.Subject, fmtCompact(int(time.Since(started).Seconds())))
	}
	if err := st.SetRunning(store.RunningSession{
		Subject:   t.ID,
		StartedAt: nowRFC3339(),
		Note:      note,
	}); err != nil {
		return err
	}
	fmt.Fprintf(out, "started %s\n", t.ID)
	return nil
}

func newStatusCmd(state *rt) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "show the running session",
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(cmd, func() error {
				return sessionStatus(state, cmd.OutOrStdout())
			})
		},
	}
}

func sessionStatus(state *rt, out io.Writer) error {
	st, err := state.open()
	if err != nil {
		return err
	}
	defer st.Close()
	rs, err := st.RunningSession()
	if err != nil {
		return err
	}
	if rs == nil {
		fmt.Fprintln(out, "no running session")
		return exitSilent(3)
	}
	started, err := parseStartedAt(rs)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "%s — %s\n", rs.Subject, fmtCompact(int(time.Since(started).Seconds())))
	return nil
}

// sessionPayload marshals with fixed key order (started_at, ended_at,
// minutes, tags, note).
type sessionPayload struct {
	StartedAt string   `json:"started_at"`
	EndedAt   string   `json:"ended_at"`
	Minutes   int      `json:"minutes"`
	Tags      []string `json:"tags"`
	Note      string   `json:"note,omitempty"`
}

func newStopCmd(state *rt) *cobra.Command {
	var note string
	cmd := &cobra.Command{
		Use:   "stop",
		Short: "stop the running session and log it",
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(cmd, func() error {
				return sessionStop(state, cmd.OutOrStdout(), note, cmd.Flags().Changed("note"))
			})
		},
	}
	cmd.Flags().StringVar(&note, "note", "", "session note")
	return cmd
}

func sessionStop(state *rt, out io.Writer, note string, noteChanged bool) error {
	st, err := state.open()
	if err != nil {
		return err
	}
	defer st.Close()
	rs, err := st.RunningSession()
	if err != nil {
		return err
	}
	if rs == nil {
		fmt.Fprintln(out, "no running session")
		return exitSilent(3)
	}
	started, err := parseStartedAt(rs)
	if err != nil {
		return err
	}
	dur := time.Now().UTC().Sub(started)
	secs := int(dur.Seconds())
	if secs < 60 {
		if err := st.ClearRunning(); err != nil {
			return err
		}
		fmt.Fprintf(out, "discarded %ds session — too short to be signal\n", secs)
		return nil
	}
	t, err := st.Thing(rs.Subject)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			if err := st.ClearRunning(); err != nil {
				return err
			}
			return fmt.Errorf("running session subject %s no longer exists", rs.Subject)
		}
		return err
	}
	effectiveNote := rs.Note
	if noteChanged {
		effectiveNote = note
	}
	payload, err := json.Marshal(sessionPayload{
		StartedAt: rs.StartedAt,
		EndedAt:   nowRFC3339(),
		Minutes:   int(dur.Minutes()),
		Tags:      []string{},
		Note:      effectiveNote,
	})
	if err != nil {
		return err
	}
	subject := rs.Subject
	if _, err := st.AddEvent(model.Event{
		Ts:        rs.StartedAt,
		Source:    "manual",
		Type:      "session",
		Subject:   &subject,
		Payload:   payload,
		CreatedAt: nowRFC3339(),
	}); err != nil {
		return err
	}
	// Clear only after the event is in: a crash between the two leaves the
	// session running (possibly double-logged later) but never lost.
	if err := st.ClearRunning(); err != nil {
		return err
	}
	fmt.Fprintf(out, "%s on %s — logged.\n", fmtCompact(int(dur.Minutes())*60), t.DisplayName)
	return nil
}

func newAbortCmd(state *rt) *cobra.Command {
	return &cobra.Command{
		Use:   "abort",
		Short: "discard the running session",
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(cmd, func() error {
				return sessionAbort(state, cmd.OutOrStdout())
			})
		},
	}
}

func sessionAbort(state *rt, out io.Writer) error {
	st, err := state.open()
	if err != nil {
		return err
	}
	defer st.Close()
	rs, err := st.RunningSession()
	if err != nil {
		return err
	}
	if rs == nil {
		fmt.Fprintln(out, "no running session")
		return exitSilent(3)
	}
	if err := st.ClearRunning(); err != nil {
		return err
	}
	fmt.Fprintln(out, "discarded.")
	return nil
}
