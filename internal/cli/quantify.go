package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/sadisticbrew/meridian/internal/config"
	"github.com/sadisticbrew/meridian/internal/normalizer"
	"github.com/sadisticbrew/meridian/internal/store"
	"github.com/spf13/cobra"
)

func newQuantifyCmd(state *rt) *cobra.Command {
	var (
		pending  bool
		redo     bool
		dryRun   bool
		provider string
		model    string
	)
	cmd := &cobra.Command{
		Use:   "quantify",
		Short: "annotate notes and derive events via a provider",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return usagef("quantify takes no arguments")
			}
			return run(cmd, func() error {
				return syncQuantify(state, cmd.OutOrStdout(), pending, redo, dryRun, provider, model)
			})
		},
	}
	cmd.Flags().BoolVar(&pending, "pending", false, "process only unquantified notes (default)")
	cmd.Flags().BoolVar(&redo, "redo", false, "delete derived events, reset notes, and re-run")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "annotate without writing")
	cmd.Flags().StringVar(&provider, "provider", "", "override the configured provider")
	cmd.Flags().StringVar(&model, "model", "", "override the provider model")
	return cmd
}

func syncQuantify(state *rt, out io.Writer, pending, redo, dryRun bool, providerName, model string) error {
	st, err := state.open()
	if err != nil {
		return err
	}
	defer st.Close()
	cfg, err := config.Load(state.configPath)
	if err != nil {
		return err
	}
	prov, err := providerFor(state, cfg, providerName, model)
	if err != nil {
		return err
	}
	return quantifyRun(out, st, prov, normalizer.Options{
		// --redo resets every note first, so pending-only equals all notes.
		OnlyUnquantified: pending || !redo,
		Redo:             redo,
		DryRun:           dryRun,
	})
}

// quantifyRun is the store-facing entry point; tests drive it directly with a
// fake provider.
func quantifyRun(out io.Writer, st *store.Store, prov normalizer.Provider, opts normalizer.Options) error {
	return normalizer.Run(context.Background(), out, st, prov, opts)
}

// providerFor resolves the configured provider ("opencode" or "http"), with the
// --provider and --model flags taking precedence. The http api key is read from
// the environment variable named in config — never from the config file.
func providerFor(state *rt, cfg config.Config, nameOverride, modelOverride string) (normalizer.Provider, error) {
	name := nameOverride
	if name == "" {
		name = cfg.Quantify.Provider
	}
	switch name {
	case "opencode":
		oc := cfg.Quantify.Opencode
		model := oc.Model
		if modelOverride != "" {
			model = modelOverride
		}
		timeout, err := time.ParseDuration(oc.Timeout)
		if err != nil {
			return nil, fmt.Errorf("quantify: invalid opencode timeout %q: %w", oc.Timeout, err)
		}
		return normalizer.NewOpenCodeProvider(oc.Command, model, timeout), nil
	case "http":
		hc := cfg.Quantify.HTTP
		key := os.Getenv(hc.APIKeyEnv)
		if key == "" {
			return nil, fmt.Errorf("api key env var %s not set", hc.APIKeyEnv)
		}
		model := hc.Model
		if modelOverride != "" {
			model = modelOverride
		}
		timeout, err := time.ParseDuration(hc.Timeout)
		if err != nil {
			return nil, fmt.Errorf("quantify: invalid http timeout %q: %w", hc.Timeout, err)
		}
		return normalizer.NewHTTPProvider(hc.URL, key, model, timeout), nil
	default:
		return nil, fmt.Errorf("no quantify provider configured — set [quantify] provider in config")
	}
}
