package cli

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/sadisticbrew/meridian/internal/model"
	"github.com/sadisticbrew/meridian/internal/store"
	"github.com/spf13/cobra"
)

// eventTypes is the closed set of event types an ingested row may carry
// (spec 02 "Event types").
var eventTypes = map[string]bool{
	"session":    true,
	"occurrence": true,
	"milestone":  true,
	"note":       true,
	"sample":     true,
}

func newIngestCmd(state *rt) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ingest -",
		Short: "insert events from stdin as NDJSON",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 || args[0] != "-" {
				return usagef("usage: meridian ingest -")
			}
			return run(cmd, func() error {
				return ingestEvents(state, cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr())
			})
		},
	}
	return cmd
}

// ingestEvents reads NDJSON dump lines; valid rows are inserted, invalid rows
// are reported with their 1-based line number and the run ends in an error
// (spec 03 "Output & export").
func ingestEvents(state *rt, in io.Reader, out, errOut io.Writer) error {
	st, err := state.open()
	if err != nil {
		return err
	}
	defer st.Close()

	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	var rejected []int
	ingested := 0
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		if strings.TrimSpace(scanner.Text()) == "" {
			continue
		}
		e, err := parseIngestLine(st, scanner.Text())
		if err != nil {
			rejected = append(rejected, lineNo)
			fmt.Fprintf(errOut, "line %d: %v\n", lineNo, err)
			continue
		}
		if _, err := st.AddEvent(e); err != nil {
			if !store.IsDedupConflict(err) {
				return err
			}
			rejected = append(rejected, lineNo)
			fmt.Fprintf(errOut, "line %d: duplicate dedup_key %s\n", lineNo, *e.DedupKey)
			continue
		}
		ingested++
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("ingest: read stdin: %w", err)
	}
	if len(rejected) == 0 {
		fmt.Fprintf(out, "ingested %d events.\n", ingested)
		return nil
	}
	fmt.Fprintf(out, "ingested %d events, %d rejected\n", ingested, len(rejected))
	return fmt.Errorf("ingest: rejected lines %s", formatLineNumbers(rejected))
}

// parseIngestLine decodes one dump line and validates the fields ingest
// promises to guard (subject id, type, source, ts). The incoming id is
// ignored; autoincrement assigns a new one.
func parseIngestLine(st *store.Store, line string) (model.Event, error) {
	var d dumpEvent
	if err := json.Unmarshal([]byte(line), &d); err != nil {
		return model.Event{}, fmt.Errorf("invalid JSON: %w", err)
	}
	if d.Source == "" {
		return model.Event{}, errors.New("missing source")
	}
	if !eventTypes[d.Type] {
		return model.Event{}, fmt.Errorf("invalid type %q", d.Type)
	}
	if d.Ts == "" {
		return model.Event{}, errors.New("missing ts")
	}
	subject := d.Subject
	if subject != nil && *subject == "" {
		subject = nil
	}
	if subject != nil {
		if _, err := st.Thing(*subject); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return model.Event{}, fmt.Errorf("unknown subject %q", *subject)
			}
			return model.Event{}, err
		}
	}
	createdAt := d.CreatedAt
	if createdAt == "" {
		createdAt = nowRFC3339()
	}
	return model.Event{
		Ts:         d.Ts,
		Source:     d.Source,
		Type:       d.Type,
		Subject:    subject,
		ValueNum:   d.ValueNum,
		ValueText:  d.ValueText,
		Payload:    ingestPayload(d.Payload),
		DedupKey:   d.DedupKey,
		RawText:    d.RawText,
		Quantified: d.Quantified,
		CreatedAt:  createdAt,
	}, nil
}

// ingestPayload keeps dump's "payload":null from becoming a literal "null"
// payload_json value; absent and null are the same thing to the store.
func ingestPayload(p json.RawMessage) json.RawMessage {
	if len(p) == 0 || string(bytes.TrimSpace(p)) == "null" {
		return nil
	}
	return p
}

func formatLineNumbers(lines []int) string {
	parts := make([]string, len(lines))
	for i, n := range lines {
		parts[i] = strconv.Itoa(n)
	}
	return strings.Join(parts, ", ")
}
