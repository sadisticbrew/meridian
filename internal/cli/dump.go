package cli

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/sadisticbrew/meridian/internal/model"
	"github.com/sadisticbrew/meridian/internal/store"
	"github.com/spf13/cobra"
)

// dumpEvent is the shared NDJSON event shape for `dump` and every read
// command's --json mode (spec 02 "Dump format").
type dumpEvent struct {
	ID          int64           `json:"id"`
	Ts          string          `json:"ts"`
	Source      string          `json:"source"`
	Type        string          `json:"type"`
	Subject     *string         `json:"subject"`
	ValueNum    *float64        `json:"value_num"`
	ValueText   *string         `json:"value_text"`
	Payload     json.RawMessage `json:"payload"`
	DedupKey    *string         `json:"dedup_key"`
	RawText     *string         `json:"raw_text"`
	Quantified  int             `json:"quantified"`
	SubjectName *string         `json:"subject_name"`
	CreatedAt   string          `json:"created_at"`
}

// subjectNames loads the whole registry once so event output never queries
// per event.
func subjectNames(st *store.Store) (map[string]string, error) {
	things, err := st.Things(store.ListFilter{Archived: store.ArchivedAll})
	if err != nil {
		return nil, err
	}
	names := make(map[string]string, len(things))
	for _, t := range things {
		names[t.ID] = t.DisplayName
	}
	return names, nil
}

func toDumpEvent(e model.Event, names map[string]string) dumpEvent {
	d := dumpEvent{
		ID:         e.ID,
		Ts:         e.Ts,
		Source:     e.Source,
		Type:       e.Type,
		Subject:    e.Subject,
		ValueNum:   e.ValueNum,
		ValueText:  e.ValueText,
		Payload:    e.Payload,
		DedupKey:   e.DedupKey,
		RawText:    e.RawText,
		Quantified: e.Quantified,
		CreatedAt:  e.CreatedAt,
	}
	if e.Subject != nil {
		if name, ok := names[*e.Subject]; ok {
			d.SubjectName = &name
		}
	}
	return d
}

// emitEvents writes one filtered event per line as dump JSON (NDJSON).
func emitEvents(out io.Writer, st *store.Store, f store.EventFilter) error {
	evs, err := st.Events(f)
	if err != nil {
		return err
	}
	names, err := subjectNames(st)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(out)
	// Spec 02's sample line shows a raw "&" in display names — no HTML
	// escaping in dump output.
	enc.SetEscapeHTML(false)
	for _, e := range evs {
		if err := enc.Encode(toDumpEvent(e, names)); err != nil {
			return err
		}
	}
	return nil
}

func newDumpCmd(state *rt) *cobra.Command {
	var since, typ, subject string
	cmd := &cobra.Command{
		Use:   "dump",
		Short: "emit events as NDJSON",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return usagef("dump takes no arguments")
			}
			return run(cmd, func() error {
				return dumpEvents(state, cmd.OutOrStdout(), since, typ, subject)
			})
		},
	}
	cmd.Flags().StringVar(&since, "since", "", "only events at or after this timestamp")
	cmd.Flags().StringVar(&typ, "type", "", "only events of this type")
	cmd.Flags().StringVar(&subject, "subject", "", "only events for this subject")
	return cmd
}

func dumpEvents(state *rt, out io.Writer, since, typ, input string) error {
	st, err := state.open()
	if err != nil {
		return err
	}
	defer st.Close()
	f := store.EventFilter{From: since, Type: typ}
	if input != "" {
		t, err := resolveThing(st, input)
		if err != nil {
			return fmt.Errorf("dump: %w", err)
		}
		f.Subject = t.ID
	}
	return emitEvents(out, st, f)
}
