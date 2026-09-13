package cli

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/sadisticbrew/meridian/internal/views"
	"github.com/spf13/cobra"
)

func newExportCmd(state *rt) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "export",
		Short: "export data to other formats",
	}
	cmd.AddCommand(newExportHTMLCmd(state))
	return cmd
}

func newExportHTMLCmd(state *rt) *cobra.Command {
	var out string
	var days int
	cmd := &cobra.Command{
		Use:   "html",
		Short: "write a self-contained HTML dashboard",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return usagef("export html takes no arguments")
			}
			if days < 1 {
				return usagef("--days must be >= 1")
			}
			return run(cmd, func() error {
				return viewExportHTML(state, cmd.OutOrStdout(), out, days)
			})
		},
	}
	cmd.Flags().StringVar(&out, "out", "", "output file path")
	cmd.Flags().IntVar(&days, "days", 30, "days to include")
	return cmd
}

// exportPath resolves the HTML export target: --out if set, else the XDG
// default ($XDG_DATA_HOME/meridian/export.html, else ~/.local/share/...).
func (r *rt) exportPath(out string) string {
	if out != "" {
		return out
	}
	if dir := os.Getenv("XDG_DATA_HOME"); dir != "" {
		return filepath.Join(dir, "meridian", "export.html")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".local", "share", "meridian", "export.html")
}

func viewExportHTML(state *rt, out io.Writer, path string, days int) error {
	path = state.exportPath(path)
	if path == "" {
		return fmt.Errorf("cannot resolve export path: no home directory")
	}
	st, err := state.open()
	if err != nil {
		return err
	}
	defer st.Close()

	var buf bytes.Buffer
	if err := views.HTML(&buf, time.Now(), st, days); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create export dir: %w", err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		return fmt.Errorf("write export: %w", err)
	}
	fmt.Fprintln(out, path)
	return nil
}
