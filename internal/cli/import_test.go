package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func importWrite(t *testing.T, name, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return path
}

func TestImportNDJSONIdempotent(t *testing.T) {
	db := filepath.Join(t.TempDir(), "m.db")
	seedLogThings(t, db)
	data := importWrite(t, "data.ndjson", strings.Join([]string{
		`{"date":"2026-09-10T08:00:00Z","minutes":45}`,
		`{"date":"2026-09-11","minutes":30}`,
	}, "\n")+"\n")
	mapping := importWrite(t, "map.json",
		`{"type":"session","subject":"course/ddco","ts_field":"date","value_field":"minutes","format":"ndjson"}`)

	out, errW, code := runCapture("import", "--file", data, "--map", mapping, "--db", db)
	if code != 0 {
		t.Fatalf("exit = %d, stderr: %s", code, errW)
	}
	if out != "imported 2 events, 0 skipped (duplicates/rejected)\n" {
		t.Errorf("stdout = %q", out)
	}
	evs := logEvents(t, db)
	if len(evs) != 2 {
		t.Fatalf("events = %d, want 2", len(evs))
	}
	for i, want := range []struct {
		ts    string
		value float64
	}{{"2026-09-10T08:00:00Z", 45}, {"2026-09-11T00:00:00Z", 30}} {
		e := evs[i]
		if e.Source != "import" || e.Type != "session" {
			t.Errorf("event %d source/type = %q/%q, want import/session", i, e.Source, e.Type)
		}
		if e.Subject == nil || *e.Subject != "course/ddco" {
			t.Errorf("event %d subject = %v", i, e.Subject)
		}
		if e.Ts != want.ts {
			t.Errorf("event %d ts = %q, want %q", i, e.Ts, want.ts)
		}
		if e.ValueNum == nil || *e.ValueNum != want.value {
			t.Errorf("event %d value_num = %v, want %v", i, e.ValueNum, want.value)
		}
		if e.DedupKey == nil || !strings.HasPrefix(*e.DedupKey, "import/") {
			t.Errorf("event %d dedup_key = %v, want import/ prefix", i, e.DedupKey)
		}
	}

	out, errW, code = runCapture("import", "--file", data, "--map", mapping, "--db", db)
	if code != 0 {
		t.Fatalf("re-import exit = %d, stderr: %s", code, errW)
	}
	if out != "imported 0 events, 2 skipped (duplicates/rejected)\n" {
		t.Errorf("re-import stdout = %q", out)
	}
	if evs := logEvents(t, db); len(evs) != 2 {
		t.Errorf("events after re-import = %d, want 2", len(evs))
	}
}

func TestImportCSV(t *testing.T) {
	db := filepath.Join(t.TempDir(), "m.db")
	seedLogThings(t, db)
	data := importWrite(t, "data.csv", "date,minutes,note\n2026-09-12T09:00:00Z,20,first\n2026-09-13,10,second\n")
	mapping := importWrite(t, "map.json",
		`{"type":"occurrence","subject":"course/ddco","ts_field":"date","value_field":"minutes","format":"csv"}`)

	out, errW, code := runCapture("import", "--file", data, "--map", mapping, "--db", db)
	if code != 0 {
		t.Fatalf("exit = %d, stderr: %s", code, errW)
	}
	if out != "imported 2 events, 0 skipped (duplicates/rejected)\n" {
		t.Errorf("stdout = %q", out)
	}
	evs := logEvents(t, db)
	if len(evs) != 2 {
		t.Fatalf("events = %d, want 2", len(evs))
	}
	if evs[0].Ts != "2026-09-12T09:00:00Z" || evs[1].Ts != "2026-09-13T00:00:00Z" {
		t.Errorf("ts = %q, %q", evs[0].Ts, evs[1].Ts)
	}
	if evs[1].ValueNum == nil || *evs[1].ValueNum != 10 {
		t.Errorf("value_num = %v, want 10", evs[1].ValueNum)
	}
}

func TestImportBadRowsRejectedNotFatal(t *testing.T) {
	db := filepath.Join(t.TempDir(), "m.db")
	seedLogThings(t, db)
	data := importWrite(t, "data.ndjson", strings.Join([]string{
		`{"date":"2026-09-10T08:00:00Z","minutes":45}`,
		`{"date":"not-a-date","minutes":30}`,
		`{"date":"2026-09-12","minutes":"lots"}`,
		`{"minutes":15}`,
	}, "\n")+"\n")
	mapping := importWrite(t, "map.json",
		`{"type":"session","subject":"course/ddco","ts_field":"date","value_field":"minutes","format":"ndjson"}`)

	out, errW, code := runCapture("import", "--file", data, "--map", mapping, "--db", db)
	if code != 0 {
		t.Fatalf("exit = %d, want 0; stderr: %s", code, errW)
	}
	if out != "imported 1 events, 3 skipped (duplicates/rejected)\n" {
		t.Errorf("stdout = %q", out)
	}
	for _, want := range []string{"line 2:", "line 3:", "line 4:"} {
		if !strings.Contains(errW, want) {
			t.Errorf("stderr = %q, want %q", errW, want)
		}
	}
	if evs := logEvents(t, db); len(evs) != 1 {
		t.Errorf("events = %d, want 1", len(evs))
	}
}

func TestImportMapValidation(t *testing.T) {
	cases := []struct {
		name    string
		mapping string
		wantErr string
	}{
		{"unknown subject", `{"type":"session","subject":"course/nope","ts_field":"date","format":"ndjson"}`, "unknown subject"},
		{"note type", `{"type":"note","subject":"course/ddco","ts_field":"date","format":"ndjson"}`, "must be session"},
		{"missing subject", `{"type":"session","ts_field":"date","format":"ndjson"}`, "subject required"},
		{"missing ts_field", `{"type":"session","subject":"course/ddco","format":"ndjson"}`, "ts_field required"},
		{"bad format", `{"type":"session","subject":"course/ddco","ts_field":"date","format":"xml"}`, "ndjson or csv"},
		{"bad json", `not json`, "parse map"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := filepath.Join(t.TempDir(), "m.db")
			seedLogThings(t, db)
			mapping := importWrite(t, "map.json", tc.mapping)
			missingData := filepath.Join(t.TempDir(), "does-not-exist.ndjson")

			_, errW, code := runCapture("import", "--file", missingData, "--map", mapping, "--db", db)
			if code != 1 {
				t.Fatalf("exit = %d, want 1; stderr: %s", code, errW)
			}
			if !strings.Contains(errW, tc.wantErr) {
				t.Errorf("stderr = %q, want %q", errW, tc.wantErr)
			}
			if strings.Contains(errW, "does-not-exist") {
				t.Errorf("stderr = %q, map must be validated before reading rows", errW)
			}
			if evs := logEvents(t, db); len(evs) != 0 {
				t.Errorf("events = %d, want 0", len(evs))
			}
		})
	}
}

func TestImportUsageAndIOErrors(t *testing.T) {
	db := filepath.Join(t.TempDir(), "m.db")
	for _, args := range [][]string{
		{"import"},
		{"import", "--file", "x"},
		{"import", "--map", "x"},
		{"import", "extra", "--file", "x", "--map", "y"},
	} {
		if _, _, code := runCapture(args...); code != 2 {
			t.Errorf("%v exit = %d, want 2", args, code)
		}
	}

	seedLogThings(t, db)
	mapping := importWrite(t, "map.json",
		`{"type":"session","subject":"course/ddco","ts_field":"date","format":"ndjson"}`)
	_, errW, code := runCapture("import", "--file", filepath.Join(t.TempDir(), "nope"), "--map", mapping, "--db", db)
	if code != 1 {
		t.Errorf("missing data file exit = %d, want 1; stderr: %s", code, errW)
	}
	_, errW, code = runCapture("import", "--file", mapping, "--map", filepath.Join(t.TempDir(), "nope"), "--db", db)
	if code != 1 {
		t.Errorf("missing map exit = %d, want 1; stderr: %s", code, errW)
	}
}
