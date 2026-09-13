package cli

import (
	"io"
	"time"

	"github.com/sadisticbrew/meridian/internal/views"
	"github.com/spf13/cobra"
)

func newGraphCmd(state *rt) *cobra.Command {
	var days int
	cmd := &cobra.Command{
		Use:   "graph [minutes|dsa|german]",
		Short: "plot terminal sparklines",
		RunE: func(cmd *cobra.Command, args []string) error {
			kind := "minutes"
			switch {
			case len(args) > 1:
				return usagef("graph takes at most one kind argument")
			case len(args) == 1:
				kind = args[0]
			}
			switch kind {
			case "minutes", "dsa", "german":
			default:
				return usagef("unknown graph kind %q (want minutes, dsa or german)", kind)
			}
			if days < 1 {
				return usagef("--days must be >= 1")
			}
			return run(cmd, func() error {
				return viewGraph(state, cmd.OutOrStdout(), kind, days)
			})
		},
	}
	cmd.Flags().IntVar(&days, "days", 30, "days to include")
	return cmd
}

func viewGraph(state *rt, out io.Writer, kind string, days int) error {
	st, err := state.open()
	if err != nil {
		return err
	}
	defer st.Close()
	now := time.Now()
	switch kind {
	case "dsa":
		return views.GraphDSA(out, now, st, days)
	case "german":
		return views.GraphGerman(out, now, st, days)
	default:
		return views.GraphMinutes(out, now, st, days)
	}
}
