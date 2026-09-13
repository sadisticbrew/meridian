package cli

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/sadisticbrew/meridian/internal/model"
	"github.com/sadisticbrew/meridian/internal/store"
	"github.com/spf13/cobra"
)

// importTypes is the closed set a mapping may name; notes are excluded because
// an imported note has no raw_text to carry (spec phase-4 task 5).
var importTypes = map[string]bool{
	"session":    true,
	"occurrence": true,
	"milestone":  true,
	"sample":     true,
}

// importMap is the mapping JSON shape from spec phase-4 task 5.
type importMap struct {
	Type       string `json:"type"`
	Subject    string `json:"subject"`
	TsField    string `json:"ts_field"`
	ValueField string `json:"value_field"`
	Format     string `json:"format"`
}

func newImportCmd(state *rt) *cobra.Command {
	var file, mapPath string
	cmd := &cobra.Command{
		Use:   "import",
		Short: "import events from a file via a mapping",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return usagef("import takes no arguments")
			}
			if file == "" || mapPath == "" {
				return usagef("import requires --file and --map")
			}
			return run(cmd, func() error {
				return importFile(state, cmd.OutOrStdout(), cmd.ErrOrStderr(), file, mapPath)
			})
		},
	}
	cmd.Flags().StringVar(&file, "file", "", "data file to import")
	cmd.Flags().StringVar(&mapPath, "map", "", "mapping JSON file")
	return cmd
}

func importFile(state *rt, out, errOut io.Writer, file, mapPath string) error {
	st, err := state.open()
	if err != nil {
		return err
	}
	defer st.Close()

	m, err := loadImportMap(st, mapPath)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return fmt.Errorf("import: read %s: %w", file, err)
	}

	imported, skipped, rejected := 0, 0, 0
	handle := func(lineNo int, row map[string]string, raw string, rowErr error) error {
		if rowErr != nil {
			rejected++
			fmt.Fprintf(errOut, "line %d: %v\n", lineNo, rowErr)
			return nil
		}
		e, err := buildImportEvent(m, file, row, raw)
		if err != nil {
			rejected++
			fmt.Fprintf(errOut, "line %d: %v\n", lineNo, err)
			return nil
		}
		if _, err := st.AddEvent(e); err != nil {
			if !store.IsDedupConflict(err) {
				return err
			}
			skipped++
			return nil
		}
		imported++
		return nil
	}

	if m.Format == "csv" {
		if err := importCSV(data, file, handle); err != nil {
			return err
		}
	} else if err := importNDJSON(data, file, handle); err != nil {
		return err
	}

	fmt.Fprintf(out, "imported %d events, %d skipped (duplicates/rejected)\n", imported, skipped+rejected)
	return nil
}

// loadImportMap validates the mapping before any data row is read; the subject
// must already exist in the registry.
func loadImportMap(st *store.Store, path string) (importMap, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return importMap{}, fmt.Errorf("import: read map %s: %w", path, err)
	}
	var m importMap
	if err := json.Unmarshal(data, &m); err != nil {
		return importMap{}, fmt.Errorf("import: parse map %s: %w", path, err)
	}
	if !importTypes[m.Type] {
		return importMap{}, fmt.Errorf("import: map type %q must be session, occurrence, milestone, or sample", m.Type)
	}
	if m.Subject == "" {
		return importMap{}, errors.New("import: map subject required")
	}
	if _, err := st.Thing(m.Subject); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return importMap{}, fmt.Errorf("import: unknown subject %q", m.Subject)
		}
		return importMap{}, err
	}
	if m.TsField == "" {
		return importMap{}, errors.New("import: map ts_field required")
	}
	if m.Format != "ndjson" && m.Format != "csv" {
		return importMap{}, fmt.Errorf("import: map format %q must be ndjson or csv", m.Format)
	}
	return m, nil
}

func importNDJSON(data []byte, file string, handle func(int, map[string]string, string, error) error) error {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		row := map[string]any{}
		dec := json.NewDecoder(strings.NewReader(line))
		dec.UseNumber()
		if err := dec.Decode(&row); err != nil {
			if err := handle(lineNo, nil, line, fmt.Errorf("invalid JSON: %w", err)); err != nil {
				return err
			}
			continue
		}
		if err := handle(lineNo, flattenRow(row), line, nil); err != nil {
			return err
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("import: read %s: %w", file, err)
	}
	return nil
}

func importCSV(data []byte, file string, handle func(int, map[string]string, string, error) error) error {
	r := csv.NewReader(bytes.NewReader(data))
	r.FieldsPerRecord = -1
	header, err := r.Read()
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("import: read %s: %w", file, err)
	}
	for {
		record, err := r.Read()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("import: read %s: %w", file, err)
		}
		lineNo := 1
		if len(record) > 0 {
			lineNo, _ = r.FieldPos(0)
		}
		row := make(map[string]string, len(header))
		for i, name := range header {
			if i < len(record) {
				row[name] = record[i]
			}
		}
		// Canonical record bytes: stable across re-reads and free of the
		// separator collisions a plain Join would allow.
		raw, _ := json.Marshal(record)
		if err := handle(lineNo, row, string(raw), nil); err != nil {
			return err
		}
	}
}

// flattenRow keeps only scalar values; anything else (null, object, array)
// reads as an absent field and the row gets rejected per-field.
func flattenRow(obj map[string]any) map[string]string {
	row := make(map[string]string, len(obj))
	for k, v := range obj {
		switch x := v.(type) {
		case string:
			row[k] = x
		case json.Number:
			row[k] = x.String()
		case bool:
			row[k] = strconv.FormatBool(x)
		}
	}
	return row
}

// buildImportEvent turns one row into an import event; dedup_key hashes the
// file path plus the raw row so re-importing the same file is a no-op.
func buildImportEvent(m importMap, file string, row map[string]string, raw string) (model.Event, error) {
	tsText, ok := row[m.TsField]
	if !ok || tsText == "" {
		return model.Event{}, fmt.Errorf("missing ts field %q", m.TsField)
	}
	ts, err := parseImportTs(tsText)
	if err != nil {
		return model.Event{}, err
	}
	var value *float64
	if m.ValueField != "" {
		text, ok := row[m.ValueField]
		if !ok {
			return model.Event{}, fmt.Errorf("missing value field %q", m.ValueField)
		}
		v, err := strconv.ParseFloat(text, 64)
		if err != nil {
			return model.Event{}, fmt.Errorf("invalid value %q for field %q", text, m.ValueField)
		}
		value = &v
	}
	sum := sha256.Sum256([]byte(file + "\x00" + raw))
	key := "import/" + hex.EncodeToString(sum[:])
	return model.Event{
		Ts:        ts,
		Source:    "import",
		Type:      m.Type,
		Subject:   &m.Subject,
		ValueNum:  value,
		DedupKey:  &key,
		CreatedAt: nowRFC3339(),
	}, nil
}

// parseImportTs accepts RFC 3339 or date-only YYYY-MM-DD (midnight UTC) and
// stores the result as UTC RFC 3339.
func parseImportTs(s string) (string, error) {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC().Format(time.RFC3339), nil
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t.UTC().Format(time.RFC3339), nil
	}
	return "", fmt.Errorf("invalid ts %q (want RFC 3339 or YYYY-MM-DD)", s)
}
