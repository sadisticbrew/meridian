package cli

import (
	"encoding/json"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/sadisticbrew/meridian/internal/model"
)

func runCaptureStdin(stdin string, args ...string) (string, string, int) {
	var out, errW strings.Builder
	code := executeIOStdin(strings.NewReader(stdin), &out, &errW, args)
	return out.String(), errW.String(), code
}

func ptrStr(s string) *string { return &s }

func ptrFloat(f float64) *float64 { return &f }

// eventShape compares events ignoring id and created_at drift.
func eventShape(e model.Event) string {
	subject, dedup, raw, valueText := "", "", "", ""
	if e.Subject != nil {
		subject = *e.Subject
	}
	if e.DedupKey != nil {
		dedup = *e.DedupKey
	}
	if e.RawText != nil {
		raw = *e.RawText
	}
	if e.ValueText != nil {
		valueText = *e.ValueText
	}
	value := ""
	if e.ValueNum != nil {
		value = strconv.FormatFloat(*e.ValueNum, 'g', -1, 64)
	}
	return strings.Join([]string{e.Ts, e.Source, e.Type, subject, value, valueText,
		string(e.Payload), dedup, raw, strconv.Itoa(e.Quantified)}, "|")
}

func TestIngestMixedValidInvalid(t *testing.T) {
	db := filepath.Join(t.TempDir(), "m.db")
	seedLogThings(t, db)

	input := strings.Join([]string{
		`{"source":"manual","type":"session","subject":"course/ddco","ts":"2026-09-10T08:00:00Z","payload":{"minutes":45}}`,
		`not json`,
		`{"source":"manual","type":"session","subject":"course/nope","ts":"2026-09-10T08:00:00Z"}`,
		`{"source":"manual","type":"note","ts":"2026-09-11T08:00:00Z","raw_text":"hi"}`,
	}, "\n") + "\n"

	out, errW, code := runCaptureStdin(input, "ingest", "-", "--db", db)
	if code != 1 {
		t.Fatalf("exit = %d, want 1; stderr: %s", code, errW)
	}
	if out != "ingested 2 events, 2 rejected\n" {
		t.Errorf("stdout = %q", out)
	}
	for _, want := range []string{"line 2:", "line 3:", `unknown subject "course/nope"`} {
		if !strings.Contains(errW, want) {
			t.Errorf("stderr = %q, want %q", errW, want)
		}
	}
	if !strings.Contains(errW, "rejected lines 2, 3") {
		t.Errorf("stderr = %q, want rejected line summary", errW)
	}

	evs := logEvents(t, db)
	if len(evs) != 2 {
		t.Fatalf("events = %d, want 2", len(evs))
	}
	if evs[0].Subject == nil || *evs[0].Subject != "course/ddco" {
		t.Errorf("event 0 subject = %v", evs[0].Subject)
	}
	if evs[1].Type != "note" || evs[1].Subject != nil {
		t.Errorf("event 1 = %+v, want note with NULL subject", evs[1])
	}
}

func TestIngestFieldValidation(t *testing.T) {
	cases := []struct {
		name    string
		line    string
		wantErr string
	}{
		{"bad type", `{"source":"manual","type":"bogus","ts":"2026-09-10T08:00:00Z"}`, `invalid type "bogus"`},
		{"missing source", `{"type":"note","ts":"2026-09-10T08:00:00Z"}`, "missing source"},
		{"missing ts", `{"source":"manual","type":"note"}`, "missing ts"},
		{"array line", `[1,2,3]`, "invalid JSON"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := filepath.Join(t.TempDir(), "m.db")
			out, errW, code := runCaptureStdin(tc.line+"\n", "ingest", "-", "--db", db)
			if code != 1 {
				t.Fatalf("exit = %d, want 1; stderr: %s", code, errW)
			}
			if !strings.Contains(errW, "line 1:") || !strings.Contains(errW, tc.wantErr) {
				t.Errorf("stderr = %q, want line 1 and %q", errW, tc.wantErr)
			}
			if out != "ingested 0 events, 1 rejected\n" {
				t.Errorf("stdout = %q", out)
			}
		})
	}
}

func TestIngestRoundTrip(t *testing.T) {
	src := filepath.Join(t.TempDir(), "src.db")
	seedLogThings(t, src)
	st := openTestStore(t, src)
	ddco, german := "course/ddco", "language/german"
	seed := []model.Event{
		{Ts: "2026-09-10T08:00:00Z", Source: "manual", Type: "session", Subject: &ddco,
			Payload:   json.RawMessage(`{"started_at":"2026-09-10T08:00:00Z","ended_at":"2026-09-10T08:45:00Z","minutes":45}`),
			CreatedAt: "2026-09-10T08:45:01Z"},
		{Ts: "2026-09-11T09:00:00Z", Source: "neetcode", Type: "occurrence", Subject: &ddco,
			Payload:   json.RawMessage(`{"problem":"car-fleet","outcome":"solved"}`),
			DedupKey:  ptrStr("neetcode/car-fleet"),
			CreatedAt: "2026-09-11T09:00:01Z"},
		{Ts: "2026-09-12T07:00:00Z", Source: "manual", Type: "note",
			RawText:   ptrStr("struggled on k-maps"),
			CreatedAt: "2026-09-12T07:00:01Z"},
		{Ts: "2026-09-13T22:00:00Z", Source: "manual", Type: "sample", Subject: &german,
			ValueNum:  ptrFloat(7.5),
			CreatedAt: "2026-09-13T22:00:01Z"},
	}
	for _, e := range seed {
		if _, err := st.AddEvent(e); err != nil {
			t.Fatalf("seed event: %v", err)
		}
	}

	dumpOut, errW, code := runCapture("dump", "--db", src)
	if code != 0 {
		t.Fatalf("dump exit = %d, stderr: %s", code, errW)
	}
	if !strings.Contains(dumpOut, `"subject_name":"DDCO"`) {
		t.Fatalf("dump output missing subject_name: %s", dumpOut)
	}

	dst := filepath.Join(t.TempDir(), "dst.db")
	seedLogThings(t, dst)
	inOut, inErr, inCode := runCaptureStdin(dumpOut, "ingest", "-", "--db", dst)
	if inCode != 0 {
		t.Fatalf("ingest exit = %d, stderr: %s", inCode, inErr)
	}
	if inOut != "ingested 4 events.\n" {
		t.Errorf("ingest stdout = %q", inOut)
	}

	srcEvs := logEvents(t, src)
	dstEvs := logEvents(t, dst)
	if len(dstEvs) != len(srcEvs) {
		t.Fatalf("events = %d, want %d", len(dstEvs), len(srcEvs))
	}
	for i := range srcEvs {
		if got, want := eventShape(dstEvs[i]), eventShape(srcEvs[i]); got != want {
			t.Errorf("event %d mismatch:\n got %s\nwant %s", i, got, want)
		}
	}
}

func TestIngestToleratesDumpExtras(t *testing.T) {
	db := filepath.Join(t.TempDir(), "m.db")
	seedLogThings(t, db)

	line := `{"id":41,"ts":"2026-09-13T09:12:00Z","source":"manual","type":"note","subject":null,` +
		`"value_num":null,"value_text":null,"payload":null,"dedup_key":null,"raw_text":"verbatim",` +
		`"quantified":0,"subject_name":null,"created_at":"2026-09-13T18:32:11Z"}`
	out, errW, code := runCaptureStdin(line+"\n", "ingest", "-", "--db", db)
	if code != 0 {
		t.Fatalf("exit = %d, stderr: %s", code, errW)
	}
	if out != "ingested 1 events.\n" {
		t.Errorf("stdout = %q", out)
	}
	evs := logEvents(t, db)
	if len(evs) != 1 {
		t.Fatalf("events = %d, want 1", len(evs))
	}
	e := evs[0]
	if e.ID == 41 {
		t.Errorf("id = 41, want a fresh autoincrement id")
	}
	if e.Subject != nil {
		t.Errorf("subject = %v, want NULL", *e.Subject)
	}
	if len(e.Payload) != 0 {
		t.Errorf("payload = %s, want nil", e.Payload)
	}
	if e.RawText == nil || *e.RawText != "verbatim" {
		t.Errorf("raw_text = %v", e.RawText)
	}
	if e.CreatedAt != "2026-09-13T18:32:11Z" {
		t.Errorf("created_at = %q, want incoming value preserved", e.CreatedAt)
	}
}

func TestIngestDuplicateDedupRejected(t *testing.T) {
	db := filepath.Join(t.TempDir(), "m.db")
	seedLogThings(t, db)
	line := `{"ts":"2026-09-11T09:00:00Z","source":"neetcode","type":"occurrence",` +
		`"subject":"course/ddco","dedup_key":"neetcode/car-fleet","payload":{"problem":"car-fleet"}}`

	if out, errW, code := runCaptureStdin(line+"\n", "ingest", "-", "--db", db); code != 0 {
		t.Fatalf("first ingest exit = %d, stderr: %s", code, errW)
	} else if out != "ingested 1 events.\n" {
		t.Errorf("first ingest stdout = %q", out)
	}

	out, errW, code := runCaptureStdin(line+"\n", "ingest", "-", "--db", db)
	if code != 1 {
		t.Fatalf("second ingest exit = %d, want 1; stderr: %s", code, errW)
	}
	if !strings.Contains(errW, "line 1: duplicate dedup_key") {
		t.Errorf("stderr = %q, want duplicate line report", errW)
	}
	if out != "ingested 0 events, 1 rejected\n" {
		t.Errorf("stdout = %q", out)
	}
	if evs := logEvents(t, db); len(evs) != 1 {
		t.Errorf("events = %d, want 1", len(evs))
	}
}

func TestIngestUsage(t *testing.T) {
	cases := [][]string{
		{"ingest"},
		{"ingest", "file.ndjson"},
		{"ingest", "-", "extra"},
	}
	for _, args := range cases {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			if _, errW, code := runCaptureStdin("", args...); code != 2 {
				t.Errorf("exit = %d, want 2; stderr: %s", code, errW)
			}
		})
	}
}
