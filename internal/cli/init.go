package cli

import (
	"errors"
	"fmt"
	"io"

	"github.com/sadisticbrew/meridian/internal/model"
	"github.com/sadisticbrew/meridian/internal/store"
	"github.com/spf13/cobra"
)

func newInitCmd(state *rt) *cobra.Command {
	var seed bool
	cmd := &cobra.Command{
		Use:   "init",
		Short: "create the database and seed the registry",
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(cmd, func() error { return runInit(state, cmd.OutOrStdout(), seed) })
		},
	}
	cmd.Flags().BoolVar(&seed, "seed", true, "seed the registry")
	return cmd
}

func runInit(state *rt, out io.Writer, seed bool) error {
	st, err := state.open()
	if err != nil {
		return err
	}
	defer st.Close()
	if !seed {
		return nil
	}
	created := nowRFC3339()
	n := 0
	for _, t := range seedRows(created) {
		// Idempotent by id: existing rows are never touched, so
		// user-edited rules and goals survive re-init (spec 03).
		_, err := st.Thing(t.ID)
		switch {
		case err == nil:
			continue
		case !errors.Is(err, store.ErrNotFound):
			return err
		}
		if err := st.UpsertThing(t); err != nil {
			return err
		}
		n++
	}
	fmt.Fprintf(out, "created %d seed rows\n", n)
	return nil
}

// seedRows returns the phase-0 catalog verbatim (spec/phase-0.md); no
// project/* rows are seeded.
func seedRows(created string) []model.Thing {
	const (
		courseRule  = "0 study hours this week → book one evening block before the next test"
		patternRule = "reviews ≥ solves this week → re-drill that pattern's trigger cards"
	)
	rows := []model.Thing{
		{ID: "course/ddco", Kind: "course", DisplayName: "Digital Design & Computer Organization",
			Active: true, GoalJSON: `{"weekly_minutes":90}`, DecisionRule: courseRule, CreatedAt: created},
		{ID: "course/dav", Kind: "course", DisplayName: "Data Analytics & Visualization",
			Active: true, GoalJSON: `{"weekly_minutes":60}`, DecisionRule: courseRule, CreatedAt: created},
		{ID: "course/oop-java", Kind: "course", DisplayName: "Object Oriented Programming with Java",
			Active: true, GoalJSON: `{"weekly_minutes":60}`, DecisionRule: courseRule, CreatedAt: created},
		{ID: "course/cdd", Kind: "course", DisplayName: "Collaborative Development & DevOps",
			Active: true, GoalJSON: `{"weekly_minutes":30}`,
			DecisionRule: "home-turf subject — don't let it cannibalize DDCO evenings", CreatedAt: created},
		{ID: "self-study/ostep", Kind: "self-study", DisplayName: "OSTEP (audio study)",
			Active: true, GoalJSON: `{"weekly_minutes":90}`,
			DecisionRule: "book one audio-study block this week", CreatedAt: created},
		{ID: "self-study/rust", Kind: "self-study", DisplayName: "Rust",
			Active: true, GoalJSON: `{"weekly_minutes":60}`,
			DecisionRule: "pair practice with a -rs port; if it stalls two weeks, cut it", CreatedAt: created},
		{ID: "language/german", Kind: "language", DisplayName: "German (Nicos Weg)",
			Active: true, GoalJSON: `{"weekly_minutes":90,"occurrences_per_week":3}`,
			DecisionRule: "0 min this week → 15 min Nicos Weg tomorrow; B1 cuts PR 27→21 months", CreatedAt: created},
	}
	patterns := []struct{ id, name string }{
		{"arrays-hashing", "Arrays & Hashing"},
		{"two-pointer", "Two Pointer"},
		{"sliding-window", "Sliding Window"},
		{"stack", "Stack"},
		{"monotonic-stack", "Monotonic Stack"},
		{"binary-search", "Binary Search"},
		{"linked-list", "Linked List"},
		{"trees", "Trees"},
		{"trie", "Trie"},
		{"heap", "Heap"},
		{"backtracking", "Backtracking"},
		{"intervals", "Intervals"},
		{"greedy", "Greedy"},
		{"dynamic-programming", "Dynamic Programming"},
		{"unclassified", "Unclassified"},
	}
	for _, p := range patterns {
		rows = append(rows, model.Thing{ID: "pattern/" + p.id, Kind: "pattern",
			DisplayName: p.name, Active: true, DecisionRule: patternRule, CreatedAt: created})
	}
	rows = append(rows, model.Thing{ID: "habit/instagram", Kind: "habit",
		DisplayName: "Instagram cycle", Archived: true,
		DecisionRule: "reinstall logged → note the trigger, don't spiral", CreatedAt: created})
	return rows
}
