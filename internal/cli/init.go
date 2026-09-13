package cli

import (
	"errors"
	"fmt"
	"io"

	"github.com/sadisticbrew/meridian/internal/seed"
	"github.com/sadisticbrew/meridian/internal/store"
	"github.com/spf13/cobra"
)

func newInitCmd(state *rt) *cobra.Command {
	var seeding bool
	cmd := &cobra.Command{
		Use:   "init",
		Short: "create the database and seed the registry",
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(cmd, func() error { return runInit(state, cmd.OutOrStdout(), seeding) })
		},
	}
	cmd.Flags().BoolVar(&seeding, "seed", true, "seed the registry")
	return cmd
}

func runInit(state *rt, out io.Writer, seeding bool) error {
	st, err := state.open()
	if err != nil {
		return err
	}
	defer st.Close()
	if !seeding {
		return nil
	}
	created := nowRFC3339()
	n := 0
	for _, t := range seed.Rows(created) {
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
