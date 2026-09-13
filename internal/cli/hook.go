package cli

import (
	"encoding/json"
	"errors"
	"os/exec"
	"strings"

	"github.com/sadisticbrew/meridian/internal/config"
	"github.com/sadisticbrew/meridian/internal/model"
	"github.com/sadisticbrew/meridian/internal/store"
	"github.com/spf13/cobra"
)

// gitToplevel resolves the repository root containing dir; an empty dir means
// the process working directory. Tests pass their fixture repo explicitly.
var gitToplevel = func(dir string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func newHookCmd(state *rt) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "hook",
		Short: "git hook entry points",
	}
	cmd.AddCommand(newHookPostCommitCmd(state))
	return cmd
}

func newHookPostCommitCmd(state *rt) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "post-commit",
		Short: "record a commit occurrence for the mapped repo",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return usagef("hook post-commit takes no arguments")
			}
			return run(cmd, func() error {
				return hookPostCommit(state, "", gitToplevel)
			})
		},
	}
	return cmd
}

// hookPostCommit writes one occurrence event when the repo root resolves to a
// [hooks.repos] entry whose subject exists and is a project/* thing (spec 04:
// hook subjects must be project/*). Unmapped repos, unknown subjects, non-
// project subjects, and "not a git repo" are dropped silently: a commit hook
// must never fail a commit. Hook events are archived-tier only (spec 00) and
// never render in today/week.
func hookPostCommit(state *rt, dir string, toplevel func(string) (string, error)) error {
	root, err := toplevel(dir)
	if err != nil || root == "" {
		return nil
	}
	cfg, err := config.Load(state.configPath)
	if err != nil {
		return err
	}
	subject, ok := cfg.Hooks.Repos[root]
	if !ok || subject == "" || !strings.HasPrefix(subject, "project/") {
		return nil
	}
	st, err := state.open()
	if err != nil {
		return err
	}
	defer st.Close()
	if _, err := st.Thing(subject); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil
		}
		return err
	}
	now := nowRFC3339()
	_, err = st.AddEvent(model.Event{
		Ts:        now,
		Source:    "hook",
		Type:      "occurrence",
		Subject:   &subject,
		Payload:   json.RawMessage(`{"source":"hook"}`),
		CreatedAt: now,
	})
	return err
}
