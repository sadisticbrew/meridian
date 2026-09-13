package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/sadisticbrew/meridian/internal/model"
	"github.com/sadisticbrew/meridian/internal/store"
	"github.com/spf13/cobra"
)

var kinds = map[string]bool{
	"project":    true,
	"course":     true,
	"self-study": true,
	"language":   true,
	"pattern":    true,
	"habit":      true,
}

// goalSpec marshals with fixed key order (weekly_minutes first, matching
// spec 02's shape).
type goalSpec struct {
	WeeklyMinutes      *int `json:"weekly_minutes,omitempty"`
	OccurrencesPerWeek *int `json:"occurrences_per_week,omitempty"`
}

func goalJSON(weeklyMin, perWeek *int) (string, error) {
	if weeklyMin == nil && perWeek == nil {
		return "", nil
	}
	b, err := json.Marshal(goalSpec{WeeklyMinutes: weeklyMin, OccurrencesPerWeek: perWeek})
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func newTrackCmd(state *rt) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "track",
		Short: "manage the tracked-thing registry",
	}
	cmd.AddCommand(
		newTrackAddCmd(state),
		newTrackListCmd(state),
		newTrackSetCmd(state, "archive", true),
		newTrackSetCmd(state, "restore", false),
		newTrackRuleCmd(state),
		newTrackGoalCmd(state),
	)
	return cmd
}

func newTrackAddCmd(state *rt) *cobra.Command {
	var name, rule string
	var weeklyMin, perWeek int
	var archived bool
	cmd := &cobra.Command{
		Use:   "add <kind>/<name>",
		Short: "add a tracked thing",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return usagef("track add requires exactly one <kind>/<name> argument")
			}
			var weekly, per *int
			if cmd.Flags().Changed("weekly-min") {
				v := weeklyMin
				weekly = &v
			}
			if cmd.Flags().Changed("per-week") {
				v := perWeek
				per = &v
			}
			return run(cmd, func() error {
				return trackAdd(state, cmd.OutOrStdout(), cmd.ErrOrStderr(),
					args[0], name, rule, weekly, per, archived)
			})
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "display name")
	cmd.Flags().StringVar(&rule, "rule", "", "decision rule (required unless --archived)")
	cmd.Flags().IntVar(&weeklyMin, "weekly-min", 0, "goal weekly minutes")
	cmd.Flags().IntVar(&perWeek, "per-week", 0, "goal occurrences per week")
	cmd.Flags().BoolVar(&archived, "archived", false, "create as archived")
	return cmd
}

func trackAdd(state *rt, out, errW io.Writer, id, name, rule string, weeklyMin, perWeek *int, archived bool) error {
	kind, rest, ok := strings.Cut(id, "/")
	if !ok || kind == "" || rest == "" {
		return usagef("invalid id %q — want <kind>/<name>", id)
	}
	if !kinds[kind] {
		return usagef("invalid kind %q — want project|course|self-study|language|pattern|habit", kind)
	}
	if name == "" {
		return usagef("--name is required")
	}
	if rule == "" && !archived {
		return errors.New("decision rule required unless --archived")
	}
	goal, err := goalJSON(weeklyMin, perWeek)
	if err != nil {
		return err
	}
	st, err := state.open()
	if err != nil {
		return err
	}
	defer st.Close()
	if _, err := st.Thing(id); err == nil {
		return fmt.Errorf("%s already exists", id)
	} else if !errors.Is(err, store.ErrNotFound) {
		return err
	}
	t := model.Thing{
		ID:           id,
		Kind:         kind,
		DisplayName:  name,
		Active:       !archived,
		Archived:     archived,
		DecisionRule: rule,
		GoalJSON:     goal,
		CreatedAt:    nowRFC3339(),
	}
	if err := st.UpsertThing(t); err != nil {
		return err
	}
	// Anti-vanity budget: warning only, never fails the command.
	if !archived && kind != "pattern" {
		active, err := st.SubjectsActive()
		if err != nil {
			return err
		}
		n := 0
		for _, x := range active {
			if x.Kind != "pattern" {
				n++
			}
		}
		if n > 5 {
			fmt.Fprintln(errW, "metric budget exceeded — retire something or accept this is a vanity metric.")
		}
	}
	fmt.Fprintf(out, "added %s\n", id)
	return nil
}

func newTrackListCmd(state *rt) *cobra.Command {
	var all, archived bool
	var kind string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "list tracked things",
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(cmd, func() error {
				return trackList(state, cmd.OutOrStdout(), all, archived, kind)
			})
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "include archived")
	cmd.Flags().BoolVar(&archived, "archived", false, "show archived only")
	cmd.Flags().StringVar(&kind, "kind", "", "filter by kind")
	return cmd
}

func trackList(state *rt, out io.Writer, all, archived bool, kind string) error {
	mode := store.ArchivedExclude
	if archived {
		mode = store.ArchivedOnly
	} else if all {
		mode = store.ArchivedAll
	}
	st, err := state.open()
	if err != nil {
		return err
	}
	defer st.Close()
	things, err := st.Things(store.ListFilter{Kind: kind, Archived: mode})
	if err != nil {
		return err
	}
	if state.jsonOut {
		enc := json.NewEncoder(out)
		for _, t := range things {
			if err := enc.Encode(t); err != nil {
				return err
			}
		}
		return nil
	}
	w := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	fmt.Fprintln(w, "id\tname\tkind\tgoal\trule")
	for _, t := range things {
		goal, rule := t.GoalJSON, t.DecisionRule
		if goal == "" {
			goal = "-"
		}
		if rule == "" {
			rule = "-"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", t.ID, t.DisplayName, t.Kind, goal, rule)
	}
	return w.Flush()
}

func newTrackSetCmd(state *rt, verb string, archived bool) *cobra.Command {
	return &cobra.Command{
		Use:   verb + " <id>",
		Short: "archive or restore a tracked thing",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return usagef("track %s requires exactly one <id> argument", verb)
			}
			return run(cmd, func() error {
				return trackSetArchived(state, cmd.OutOrStdout(), verb, args[0], archived)
			})
		},
	}
}

func trackSetArchived(state *rt, out io.Writer, verb, input string, archived bool) error {
	st, err := state.open()
	if err != nil {
		return err
	}
	defer st.Close()
	t, err := resolveThing(st, input)
	if err != nil {
		return err
	}
	if err := st.ArchiveThing(t.ID, archived); err != nil {
		return err
	}
	fmt.Fprintf(out, "%s %s\n", verb, t.ID)
	return nil
}

func newTrackRuleCmd(state *rt) *cobra.Command {
	return &cobra.Command{
		Use:   "rule <id> <text>",
		Short: "replace a thing's decision rule",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 2 {
				return usagef("track rule requires <id> and <rule text>")
			}
			return run(cmd, func() error {
				return trackRule(state, cmd.OutOrStdout(), args[0], args[1])
			})
		},
	}
}

func trackRule(state *rt, out io.Writer, input, rule string) error {
	st, err := state.open()
	if err != nil {
		return err
	}
	defer st.Close()
	t, err := resolveThing(st, input)
	if err != nil {
		return err
	}
	t.DecisionRule = rule
	if err := st.UpsertThing(*t); err != nil {
		return err
	}
	fmt.Fprintf(out, "rule set for %s\n", t.ID)
	return nil
}

func newTrackGoalCmd(state *rt) *cobra.Command {
	var weeklyMin, perWeek int
	var clear bool
	cmd := &cobra.Command{
		Use:   "goal <id> [--weekly-min N | --per-week N | --clear]",
		Short: "set or clear a thing's goal",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return usagef("track goal requires exactly one <id> argument")
			}
			actions := 0
			for _, flag := range []string{"weekly-min", "per-week", "clear"} {
				if cmd.Flags().Changed(flag) {
					actions++
				}
			}
			if actions != 1 {
				return usagef("exactly one of --weekly-min, --per-week, --clear is required")
			}
			var weekly, per *int
			if cmd.Flags().Changed("weekly-min") {
				v := weeklyMin
				weekly = &v
			}
			if cmd.Flags().Changed("per-week") {
				v := perWeek
				per = &v
			}
			return run(cmd, func() error {
				return trackGoal(state, cmd.OutOrStdout(), args[0], weekly, per, clear)
			})
		},
	}
	cmd.Flags().IntVar(&weeklyMin, "weekly-min", 0, "goal weekly minutes")
	cmd.Flags().IntVar(&perWeek, "per-week", 0, "goal occurrences per week")
	cmd.Flags().BoolVar(&clear, "clear", false, "remove the goal")
	return cmd
}

func trackGoal(state *rt, out io.Writer, input string, weeklyMin, perWeek *int, clear bool) error {
	st, err := state.open()
	if err != nil {
		return err
	}
	defer st.Close()
	t, err := resolveThing(st, input)
	if err != nil {
		return err
	}
	if clear {
		t.GoalJSON = ""
	} else {
		goal, err := goalJSON(weeklyMin, perWeek)
		if err != nil {
			return err
		}
		t.GoalJSON = goal
	}
	if err := st.UpsertThing(*t); err != nil {
		return err
	}
	if clear {
		fmt.Fprintf(out, "goal cleared for %s\n", t.ID)
	} else {
		fmt.Fprintf(out, "goal set for %s\n", t.ID)
	}
	return nil
}
