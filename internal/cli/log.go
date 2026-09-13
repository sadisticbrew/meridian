package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/sadisticbrew/meridian/internal/model"
	"github.com/spf13/cobra"
)

// Manual logging (spec 03 "Timer & logging"): back-filled sessions, German
// lessons, DSA outcomes, exam scores, and freeform notes.

// logModes in selection order. Each mode owns a disjoint group of flags, so
// an invocation touching two groups is a usage error.
const (
	logModeSession = iota
	logModeLesson
	logModeProblem
	logModeScore
)

// logFlagGroups must stay parallel to the logMode* constants.
var logFlagGroups = [][]string{
	{"minutes", "note", "date", "at"},
	{"lesson"},
	{"problem", "outcome"},
	{"score", "exam", "max"},
}

var logOutcomes = map[string]bool{"solved": true, "reviewed": true, "struggled": true}
var logExams = map[string]bool{"t1": true, "t2": true, "quiz1": true, "quiz2": true, "see": true}

type logFlags struct {
	minutes int
	note    string
	date    string
	at      string
	lesson  int
	problem string
	outcome string
	score   int
	exam    string
	max     int
}

// logOptions carries one validated, ready-to-write log invocation.
type logOptions struct {
	mode    int
	minutes int
	note    string
	endedAt time.Time // session mode only; local, at/date already applied
	lesson  int
	problem string
	outcome string
	score   int
	exam    string
	max     *int // score mode only; nil when --max was not given
}

// logSessionPayload marshals with a fixed key order: started_at, ended_at,
// minutes, tags, note (only when non-empty), manual (spec 02 back-fill shape).
type logSessionPayload struct {
	StartedAt string   `json:"started_at"`
	EndedAt   string   `json:"ended_at"`
	Minutes   int      `json:"minutes"`
	Tags      []string `json:"tags"`
	Note      string   `json:"note,omitempty"`
	Manual    bool     `json:"manual"`
}

type logProblemPayload struct {
	Problem string `json:"problem"`
	Outcome string `json:"outcome"`
}

type logScorePayload struct {
	Exam string `json:"exam"`
	Max  *int   `json:"max,omitempty"`
}

func newLogCmd(state *rt) *cobra.Command {
	var f logFlags
	cmd := &cobra.Command{
		Use:   "log <subject>",
		Short: "log a manual event",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return usagef("log requires exactly one <subject> argument")
			}
			opts, err := logOptionsFromFlags(cmd, f)
			if err != nil {
				return err
			}
			return run(cmd, func() error {
				return logEvent(state, cmd.OutOrStdout(), args[0], opts)
			})
		},
	}
	cmd.Flags().IntVar(&f.minutes, "minutes", 0, "back-filled session length")
	cmd.Flags().StringVar(&f.note, "note", "", "back-filled session note")
	cmd.Flags().StringVar(&f.date, "date", "", "back-fill date YYYY-MM-DD (default today)")
	cmd.Flags().StringVar(&f.at, "at", "", "back-fill end time HH:MM (default 20:00)")
	cmd.Flags().IntVar(&f.lesson, "lesson", 0, "German lesson number")
	cmd.Flags().StringVar(&f.problem, "problem", "", "DSA problem slug")
	cmd.Flags().StringVar(&f.outcome, "outcome", "", "solved|reviewed|struggled")
	cmd.Flags().IntVar(&f.score, "score", 0, "exam marks obtained")
	cmd.Flags().StringVar(&f.exam, "exam", "", "t1|t2|quiz1|quiz2|see")
	cmd.Flags().IntVar(&f.max, "max", 0, "exam maximum marks")
	return cmd
}

// logOptionsFromFlags validates flag groupings and mode-specific values,
// returning usage errors for anything malformed; no store access happens here.
func logOptionsFromFlags(cmd *cobra.Command, f logFlags) (logOptions, error) {
	mode := -1
	groups := 0
	for i, flags := range logFlagGroups {
		hit := false
		for _, name := range flags {
			if cmd.Flags().Changed(name) {
				hit = true
			}
		}
		if hit {
			mode = i
			groups++
		}
	}
	if groups != 1 {
		return logOptions{}, usagef("exactly one of --minutes, --lesson, --problem/--outcome, --score/--exam is required")
	}
	opts := logOptions{mode: mode}
	switch mode {
	case logModeSession:
		if !cmd.Flags().Changed("minutes") {
			return logOptions{}, usagef("--minutes is required for a back-filled session")
		}
		if f.minutes <= 0 {
			return logOptions{}, usagef("--minutes must be a positive integer")
		}
		day := time.Now()
		if cmd.Flags().Changed("date") {
			d, err := time.ParseInLocation("2006-01-02", f.date, time.Local)
			if err != nil {
				return logOptions{}, usagef("invalid --date %q — want YYYY-MM-DD", f.date)
			}
			day = d
		}
		hour, minute := 20, 0
		if cmd.Flags().Changed("at") {
			clock, err := time.Parse("15:04", f.at)
			if err != nil {
				return logOptions{}, usagef("invalid --at %q — want HH:MM (24h)", f.at)
			}
			hour, minute = clock.Hour(), clock.Minute()
		}
		opts.minutes = f.minutes
		opts.note = f.note
		opts.endedAt = time.Date(day.Year(), day.Month(), day.Day(), hour, minute, 0, 0, time.Local)
	case logModeLesson:
		if f.lesson <= 0 {
			return logOptions{}, usagef("--lesson must be a positive integer")
		}
		opts.lesson = f.lesson
	case logModeProblem:
		if !cmd.Flags().Changed("problem") || f.problem == "" {
			return logOptions{}, usagef("--problem requires a non-empty SLUG")
		}
		if !cmd.Flags().Changed("outcome") {
			return logOptions{}, usagef("--problem requires --outcome solved|reviewed|struggled")
		}
		if !logOutcomes[f.outcome] {
			return logOptions{}, usagef("invalid --outcome %q — want solved|reviewed|struggled", f.outcome)
		}
		opts.problem = f.problem
		opts.outcome = f.outcome
	case logModeScore:
		if !cmd.Flags().Changed("score") || !cmd.Flags().Changed("exam") {
			return logOptions{}, usagef("--score and --exam must be given together")
		}
		if !logExams[f.exam] {
			return logOptions{}, usagef("invalid --exam %q — want t1|t2|quiz1|quiz2|see", f.exam)
		}
		opts.score = f.score
		opts.exam = f.exam
		if cmd.Flags().Changed("max") {
			max := f.max
			opts.max = &max
		}
	}
	return opts, nil
}

func logEvent(state *rt, out io.Writer, input string, opts logOptions) error {
	st, err := state.open()
	if err != nil {
		return err
	}
	defer st.Close()
	thing, err := resolveThing(st, input)
	if err != nil {
		return err
	}
	ev := model.Event{Source: "manual", CreatedAt: nowRFC3339()}
	var line string
	switch opts.mode {
	case logModeSession:
		startedAt := opts.endedAt.Add(-time.Duration(opts.minutes) * time.Minute)
		ev.Ts = startedAt.UTC().Format(time.RFC3339)
		ev.Type = "session"
		ev.Subject = &thing.ID
		ev.Payload, err = json.Marshal(logSessionPayload{
			StartedAt: ev.Ts,
			EndedAt:   opts.endedAt.UTC().Format(time.RFC3339),
			Minutes:   opts.minutes,
			Tags:      []string{},
			Note:      opts.note,
			Manual:    true,
		})
		if err != nil {
			return err
		}
		line = fmt.Sprintf("logged %s on %s.", fmtCompact(opts.minutes*60), thing.DisplayName)
	case logModeLesson:
		value := float64(opts.lesson)
		ev.Ts = nowRFC3339()
		ev.Type = "occurrence"
		ev.Subject = &thing.ID
		ev.ValueNum = &value
		line = fmt.Sprintf("logged lesson %d on %s.", opts.lesson, thing.DisplayName)
	case logModeProblem:
		ev.Ts = nowRFC3339()
		ev.Type = "occurrence"
		ev.Subject = &thing.ID
		ev.Payload, err = json.Marshal(logProblemPayload{Problem: opts.problem, Outcome: opts.outcome})
		if err != nil {
			return err
		}
		line = fmt.Sprintf("logged %s %s on %s.", opts.outcome, opts.problem, thing.DisplayName)
	case logModeScore:
		value := float64(opts.score)
		ev.Ts = nowRFC3339()
		ev.Type = "milestone"
		ev.Subject = &thing.ID
		ev.ValueNum = &value
		ev.Payload, err = json.Marshal(logScorePayload{Exam: opts.exam, Max: opts.max})
		if err != nil {
			return err
		}
		if opts.max != nil {
			line = fmt.Sprintf("logged %s %d/%d on %s.", opts.exam, opts.score, *opts.max, thing.DisplayName)
		} else {
			line = fmt.Sprintf("logged %s %d on %s.", opts.exam, opts.score, thing.DisplayName)
		}
	default:
		return fmt.Errorf("log: unknown mode %d", opts.mode)
	}
	if _, err := st.AddEvent(ev); err != nil {
		return err
	}
	fmt.Fprintln(out, line)
	return nil
}

func newNoteCmd(state *rt) *cobra.Command {
	return &cobra.Command{
		Use:   `note "<text>"`,
		Short: "store a freeform note",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return usagef(`note requires exactly one "<text>" argument`)
			}
			return run(cmd, func() error {
				return noteStore(state, cmd.OutOrStdout(), args[0])
			})
		},
	}
}

func noteStore(state *rt, out io.Writer, text string) error {
	st, err := state.open()
	if err != nil {
		return err
	}
	defer st.Close()
	raw := text
	id, err := st.AddEvent(model.Event{
		Ts:        nowRFC3339(),
		Source:    "manual",
		Type:      "note",
		RawText:   &raw,
		CreatedAt: nowRFC3339(),
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "stored note #%d.\n", id)
	return nil
}
