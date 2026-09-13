package cli

import (
	"io"
	"time"

	"github.com/sadisticbrew/meridian/internal/store"
	"github.com/sadisticbrew/meridian/internal/views"
	"github.com/spf13/cobra"
)

// Read commands (spec 03 "Reading"): human views via internal/views, --json
// windows emitted as dump NDJSON. Views never call time.Now themselves.

func localMidnight(now time.Time) time.Time {
	y, m, d := now.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, now.Location())
}

// weekWindow mirrors the views' Monday-start boundary: [from, from+7d) local,
// shifted back one week for --last.
func weekWindow(now time.Time, last bool) (time.Time, time.Time) {
	from := localMidnight(now).AddDate(0, 0, -(int(now.Weekday())+6)%7)
	if last {
		from = from.AddDate(0, 0, -7)
	}
	return from, from.AddDate(0, 0, 7)
}

func newTodayCmd(state *rt) *cobra.Command {
	return &cobra.Command{
		Use:   "today",
		Short: "show today's wins and gaps",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return usagef("today takes no arguments")
			}
			return run(cmd, func() error {
				return viewToday(state, cmd.OutOrStdout())
			})
		},
	}
}

func viewToday(state *rt, out io.Writer) error {
	st, err := state.open()
	if err != nil {
		return err
	}
	defer st.Close()
	now := time.Now()
	if state.jsonOut {
		from := localMidnight(now)
		to := from.AddDate(0, 0, 1)
		return emitEvents(out, st, store.EventFilter{
			From: from.UTC().Format(time.RFC3339),
			To:   to.UTC().Format(time.RFC3339),
		})
	}
	return views.Today(out, now, st)
}

func newWeekCmd(state *rt) *cobra.Command {
	var last bool
	cmd := &cobra.Command{
		Use:   "week",
		Short: "show this week's totals and gaps",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return usagef("week takes no arguments")
			}
			return run(cmd, func() error {
				return viewWeek(state, cmd.OutOrStdout(), last)
			})
		},
	}
	cmd.Flags().BoolVar(&last, "last", false, "show the previous week")
	return cmd
}

func viewWeek(state *rt, out io.Writer, last bool) error {
	st, err := state.open()
	if err != nil {
		return err
	}
	defer st.Close()
	now := time.Now()
	if state.jsonOut {
		from, to := weekWindow(now, last)
		return emitEvents(out, st, store.EventFilter{
			From: from.UTC().Format(time.RFC3339),
			To:   to.UTC().Format(time.RFC3339),
		})
	}
	return views.Week(out, now, st, last)
}

func newSubjectCmd(state *rt) *cobra.Command {
	var days int
	cmd := &cobra.Command{
		Use:   "subject <id>",
		Short: "show one subject's history",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return usagef("subject requires exactly one <id> argument")
			}
			if days < 1 {
				return usagef("--days must be >= 1")
			}
			return run(cmd, func() error {
				return viewSubject(state, cmd.OutOrStdout(), args[0], days)
			})
		},
	}
	cmd.Flags().IntVar(&days, "days", 14, "days to include")
	return cmd
}

func viewSubject(state *rt, out io.Writer, input string, days int) error {
	st, err := state.open()
	if err != nil {
		return err
	}
	defer st.Close()
	t, err := resolveThing(st, input)
	if err != nil {
		return err
	}
	now := time.Now()
	if state.jsonOut {
		today := localMidnight(now)
		from := today.AddDate(0, 0, -(days - 1))
		to := today.AddDate(0, 0, 1)
		return emitEvents(out, st, store.EventFilter{
			From:    from.UTC().Format(time.RFC3339),
			To:      to.UTC().Format(time.RFC3339),
			Subject: t.ID,
		})
	}
	return views.Subject(out, now, st, t.ID, days)
}
