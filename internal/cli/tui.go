package cli

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/sadisticbrew/meridian/internal/tui"
	"github.com/spf13/cobra"
)

func newTUICmd(state *rt) *cobra.Command {
	return &cobra.Command{
		Use:   "tui",
		Short: "full-screen read-only dashboard",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return usagef("tui takes no arguments")
			}
			return run(cmd, func() error {
				st, err := state.open()
				if err != nil {
					return err
				}
				defer st.Close()
				if _, err := tea.NewProgram(tui.NewModel(st, nil), tea.WithAltScreen()).Run(); err != nil {
					return err
				}
				return nil
			})
		},
	}
}
