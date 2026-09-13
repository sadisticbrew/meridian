package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"time"

	"github.com/sadisticbrew/meridian/internal/collectors"
	"github.com/sadisticbrew/meridian/internal/config"
	"github.com/sadisticbrew/meridian/internal/store"
	"github.com/spf13/cobra"
)

func newSyncCmd(state *rt) *cobra.Command {
	var only, timeoutText string
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "run collectors",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return usagef("sync takes no arguments")
			}
			timeout, err := time.ParseDuration(timeoutText)
			if err != nil {
				return usagef("invalid --timeout %q — want a duration like 30s", timeoutText)
			}
			return run(cmd, func() error {
				return syncCollectors(state, cmd.OutOrStdout(), only, timeout)
			})
		},
	}
	cmd.Flags().StringVar(&only, "only", "", "run only this collector")
	cmd.Flags().StringVar(&timeoutText, "timeout", "30s", "per-collector timeout")
	return cmd
}

func syncCollectors(state *rt, out io.Writer, only string, timeout time.Duration) error {
	st, err := state.open()
	if err != nil {
		return err
	}
	defer st.Close()
	cfg, err := config.Load(state.configPath)
	if err != nil {
		return err
	}
	reg := collectors.Registry()
	if nc, ok := reg["neetcode"].(*collectors.NeetCode); ok {
		nc.RepoURL = cfg.Collectors.NeetCode.URL
	}
	names := make([]string, 0, len(reg))
	if only != "" {
		if _, ok := reg[only]; !ok {
			return usagef("unknown collector %q", only)
		}
		names = append(names, only)
	} else {
		for name := range reg {
			names = append(names, name)
		}
		sort.Strings(names)
	}
	for _, name := range names {
		c := reg[name]
		res, err := syncOne(c, st, timeout)
		if err != nil {
			var ue *collectors.UnavailableError
			if errors.As(err, &ue) {
				fmt.Fprintf(out, "%s: unavailable (offline?) — skipped\n", c.Name())
				continue
			}
			return err
		}
		line := fmt.Sprintf("%s: %d new, %d updated", c.Name(), res.New, res.Updated)
		if res.Details != "" {
			line += fmt.Sprintf(" (%s)", res.Details)
		}
		fmt.Fprintln(out, line)
	}
	return nil
}

// syncOne gives each collector its own deadline so one slow source never
// starves the rest.
func syncOne(c collectors.Collector, st *store.Store, timeout time.Duration) (collectors.Result, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return c.Sync(ctx, st)
}
