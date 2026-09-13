package cli

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sadisticbrew/meridian/internal/model"
	"github.com/sadisticbrew/meridian/internal/store"
)

func seedLogThings(t *testing.T, db string) {
	t.Helper()
	st := openTestStore(t, db)
	defer st.Close()
	for _, thing := range []model.Thing{
		{ID: "course/ddco", Kind: "course", DisplayName: "DDCO", Active: true},
		{ID: "language/german", Kind: "language", DisplayName: "German (Nicos Weg)", Active: true},
		{ID: "pattern/monotonic-stack", Kind: "pattern", DisplayName: "Monotonic Stack", Active: true},
	} {
		thing.CreatedAt = nowRFC3339()
		if err := st.UpsertThing(thing); err != nil {
			t.Fatalf("seed %s: %v", thing.ID, err)
		}
	}
}

func logEvents(t *testing.T, db string) []model.Event {
	t.Helper()
	st := openTestStore(t, db)
	evs, err := st.Events(store.EventFilter{})
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	return evs
}

func TestLogBackfillSession(t *testing.T) {
	db := filepath.Join(t.TempDir(), "m.db")
	seedLogThings(t, db)

	out, errW, code := runCapture("log", "ddco", "--minutes", "90", "--date", "2026-09-10", "--at", "20:00", "--db", db)
	if code != 0 {
		t.Fatalf("exit = %d, stderr: %s", code, errW)
	}
	if out != "logged 1h30m on DDCO.\n" {
		t.Errorf("stdout = %q, want %q", out, "logged 1h30m on DDCO.\n")
	}
	endedAt := time.Date(2026, 9, 10, 20, 0, 0, 0, time.Local)
	startedAt := endedAt.Add(-90 * time.Minute)
	wantTS := startedAt.UTC().Format(time.RFC3339)
	wantEnd := endedAt.UTC().Format(time.RFC3339)

	evs := logEvents(t, db)
	if len(evs) != 1 {
		t.Fatalf("events = %d, want 1", len(evs))
	}
	e := evs[0]
	if e.Type != "session" || e.Source != "manual" {
		t.Errorf("type/source = %q/%q, want session/manual", e.Type, e.Source)
	}
	if e.Subject == nil || *e.Subject != "course/ddco" {
		t.Errorf("subject = %v, want course/ddco", e.Subject)
	}
	if e.Ts != wantTS {
		t.Errorf("ts = %q, want %q", e.Ts, wantTS)
	}
	if e.ValueNum != nil {
		t.Errorf("value_num = %v, want nil", *e.ValueNum)
	}
	if e.DedupKey != nil {
		t.Errorf("dedup_key = %v, want NULL", *e.DedupKey)
	}
	wantPayload := fmt.Sprintf(`{"started_at":"%s","ended_at":"%s","minutes":90,"tags":[],"manual":true}`, wantTS, wantEnd)
	if string(e.Payload) != wantPayload {
		t.Errorf("payload = %s, want %s", e.Payload, wantPayload)
	}
	var p struct {
		StartedAt string   `json:"started_at"`
		EndedAt   string   `json:"ended_at"`
		Minutes   int      `json:"minutes"`
		Tags      []string `json:"tags"`
		Manual    bool     `json:"manual"`
	}
	if err := json.Unmarshal(e.Payload, &p); err != nil {
		t.Fatalf("payload %s: %v", e.Payload, err)
	}
	if p.Minutes != 90 || !p.Manual || p.Tags == nil || len(p.Tags) != 0 {
		t.Errorf("payload fields = %+v", p)
	}
	if p.StartedAt != e.Ts {
		t.Errorf("payload started_at = %q, ts = %q, want equal", p.StartedAt, e.Ts)
	}
	if _, err := time.Parse(time.RFC3339, e.CreatedAt); err != nil {
		t.Errorf("created_at %q not RFC 3339: %v", e.CreatedAt, err)
	}
}

func TestLogBackfillDefaults(t *testing.T) {
	db := filepath.Join(t.TempDir(), "m.db")
	seedLogThings(t, db)

	out, errW, code := runCapture("log", "german", "--minutes", "30", "--db", db)
	if code != 0 {
		t.Fatalf("exit = %d, stderr: %s", code, errW)
	}
	if out != "logged 30m on German (Nicos Weg).\n" {
		t.Errorf("stdout = %q", out)
	}
	now := time.Now()
	endedAt := time.Date(now.Year(), now.Month(), now.Day(), 20, 0, 0, 0, time.Local)
	startedAt := endedAt.Add(-30 * time.Minute)
	wantTS := startedAt.UTC().Format(time.RFC3339)
	wantEnd := endedAt.UTC().Format(time.RFC3339)

	evs := logEvents(t, db)
	if len(evs) != 1 {
		t.Fatalf("events = %d, want 1", len(evs))
	}
	var p struct {
		StartedAt string `json:"started_at"`
		EndedAt   string `json:"ended_at"`
		Minutes   int    `json:"minutes"`
	}
	if err := json.Unmarshal(evs[0].Payload, &p); err != nil {
		t.Fatalf("payload %s: %v", evs[0].Payload, err)
	}
	if p.Minutes != 30 {
		t.Errorf("minutes = %d, want 30", p.Minutes)
	}
	if p.EndedAt != wantEnd {
		t.Errorf("ended_at = %q, want today 20:00 local %q", p.EndedAt, wantEnd)
	}
	if p.StartedAt != wantTS {
		t.Errorf("started_at = %q, want %q", p.StartedAt, wantTS)
	}
}

func TestLogBackfillWithNote(t *testing.T) {
	db := filepath.Join(t.TempDir(), "m.db")
	seedLogThings(t, db)

	out, errW, code := runCapture("log", "german", "--minutes", "45", "--note", "Nicos Weg 12",
		"--date", "2026-09-11", "--at", "18:30", "--db", db)
	if code != 0 {
		t.Fatalf("exit = %d, stderr: %s", code, errW)
	}
	if out != "logged 45m on German (Nicos Weg).\n" {
		t.Errorf("stdout = %q", out)
	}
	endedAt := time.Date(2026, 9, 11, 18, 30, 0, 0, time.Local)
	startedAt := endedAt.Add(-45 * time.Minute)
	wantPayload := fmt.Sprintf(`{"started_at":"%s","ended_at":"%s","minutes":45,"tags":[],"note":"Nicos Weg 12","manual":true}`,
		startedAt.UTC().Format(time.RFC3339), endedAt.UTC().Format(time.RFC3339))
	evs := logEvents(t, db)
	if len(evs) != 1 {
		t.Fatalf("events = %d, want 1", len(evs))
	}
	if string(evs[0].Payload) != wantPayload {
		t.Errorf("payload = %s, want %s", evs[0].Payload, wantPayload)
	}
}

func TestLogLesson(t *testing.T) {
	db := filepath.Join(t.TempDir(), "m.db")
	seedLogThings(t, db)

	out, errW, code := runCapture("log", "german", "--lesson", "44", "--db", db)
	if code != 0 {
		t.Fatalf("exit = %d, stderr: %s", code, errW)
	}
	if out != "logged lesson 44 on German (Nicos Weg).\n" {
		t.Errorf("stdout = %q", out)
	}
	evs := logEvents(t, db)
	if len(evs) != 1 {
		t.Fatalf("events = %d, want 1", len(evs))
	}
	e := evs[0]
	if e.Type != "occurrence" || e.Source != "manual" {
		t.Errorf("type/source = %q/%q, want occurrence/manual", e.Type, e.Source)
	}
	if e.Subject == nil || *e.Subject != "language/german" {
		t.Errorf("subject = %v, want language/german", e.Subject)
	}
	if e.ValueNum == nil || *e.ValueNum != 44 {
		t.Errorf("value_num = %v, want 44", e.ValueNum)
	}
	if len(e.Payload) != 0 {
		t.Errorf("payload = %s, want nil", e.Payload)
	}
	if e.DedupKey != nil {
		t.Errorf("dedup_key = %v, want NULL", *e.DedupKey)
	}
	if _, err := time.Parse(time.RFC3339, e.Ts); err != nil {
		t.Errorf("ts %q not RFC 3339: %v", e.Ts, err)
	}
}

func TestLogProblem(t *testing.T) {
	db := filepath.Join(t.TempDir(), "m.db")
	seedLogThings(t, db)

	out, errW, code := runCapture("log", "monotonic-stack", "--problem", "car-fleet", "--outcome", "struggled", "--db", db)
	if code != 0 {
		t.Fatalf("exit = %d, stderr: %s", code, errW)
	}
	if out != "logged struggled car-fleet on Monotonic Stack.\n" {
		t.Errorf("stdout = %q", out)
	}
	evs := logEvents(t, db)
	if len(evs) != 1 {
		t.Fatalf("events = %d, want 1", len(evs))
	}
	e := evs[0]
	if e.Type != "occurrence" || e.Source != "manual" {
		t.Errorf("type/source = %q/%q, want occurrence/manual", e.Type, e.Source)
	}
	if e.Subject == nil || *e.Subject != "pattern/monotonic-stack" {
		t.Errorf("subject = %v, want pattern/monotonic-stack", e.Subject)
	}
	if e.ValueNum != nil {
		t.Errorf("value_num = %v, want nil", *e.ValueNum)
	}
	want := `{"problem":"car-fleet","outcome":"struggled"}`
	if string(e.Payload) != want {
		t.Errorf("payload = %s, want %s", e.Payload, want)
	}
	if e.DedupKey != nil {
		t.Errorf("dedup_key = %v, want NULL", *e.DedupKey)
	}
}

func TestLogScore(t *testing.T) {
	cases := []struct {
		name        string
		args        []string
		wantPayload string
		wantOut     string
	}{
		{
			name:        "with max",
			args:        []string{"--score", "21", "--exam", "t1", "--max", "25"},
			wantPayload: `{"exam":"t1","max":25}`,
			wantOut:     "logged t1 21/25 on DDCO.\n",
		},
		{
			name:        "without max",
			args:        []string{"--score", "21", "--exam", "t1"},
			wantPayload: `{"exam":"t1"}`,
			wantOut:     "logged t1 21 on DDCO.\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := filepath.Join(t.TempDir(), "m.db")
			seedLogThings(t, db)

			out, errW, code := runCapture(append([]string{"log", "ddco", "--db", db}, tc.args...)...)
			if code != 0 {
				t.Fatalf("exit = %d, stderr: %s", code, errW)
			}
			if out != tc.wantOut {
				t.Errorf("stdout = %q, want %q", out, tc.wantOut)
			}
			evs := logEvents(t, db)
			if len(evs) != 1 {
				t.Fatalf("events = %d, want 1", len(evs))
			}
			e := evs[0]
			if e.Type != "milestone" || e.Source != "manual" {
				t.Errorf("type/source = %q/%q, want milestone/manual", e.Type, e.Source)
			}
			if e.Subject == nil || *e.Subject != "course/ddco" {
				t.Errorf("subject = %v, want course/ddco", e.Subject)
			}
			if e.ValueNum == nil || *e.ValueNum != 21 {
				t.Errorf("value_num = %v, want 21", e.ValueNum)
			}
			if string(e.Payload) != tc.wantPayload {
				t.Errorf("payload = %s, want %s", e.Payload, tc.wantPayload)
			}
			if e.DedupKey != nil {
				t.Errorf("dedup_key = %v, want NULL", *e.DedupKey)
			}
		})
	}
}

func TestLogModeExclusivity(t *testing.T) {
	db := filepath.Join(t.TempDir(), "m.db")
	seedLogThings(t, db)

	cases := []struct {
		name string
		args []string
	}{
		{"two modes", []string{"log", "ddco", "--minutes", "30", "--lesson", "5"}},
		{"no mode", []string{"log", "ddco"}},
		{"problem without outcome", []string{"log", "ddco", "--problem", "x"}},
		{"outcome without problem", []string{"log", "ddco", "--outcome", "solved"}},
		{"score without exam", []string{"log", "ddco", "--score", "21"}},
		{"exam without score", []string{"log", "ddco", "--exam", "see"}},
		{"max alone", []string{"log", "ddco", "--max", "25"}},
		{"bad outcome", []string{"log", "ddco", "--problem", "x", "--outcome", "won"}},
		{"bad exam", []string{"log", "ddco", "--score", "21", "--exam", "midterm"}},
		{"empty problem", []string{"log", "ddco", "--problem", "", "--outcome", "solved"}},
		{"note alone", []string{"log", "ddco", "--note", "hi"}},
		{"note with lesson", []string{"log", "ddco", "--lesson", "3", "--note", "hi"}},
		{"zero minutes", []string{"log", "ddco", "--minutes", "0"}},
		{"negative lesson", []string{"log", "ddco", "--lesson", "-1"}},
		{"bad date", []string{"log", "ddco", "--minutes", "30", "--date", "10-09-2026"}},
		{"bad at", []string{"log", "ddco", "--minutes", "30", "--at", "8pm"}},
		{"no subject", []string{"log"}},
		{"two subjects", []string{"log", "ddco", "extra", "--minutes", "30"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, errW, code := runCapture(append(tc.args, "--db", db)...)
			if code != 2 {
				t.Errorf("exit = %d, want 2; stderr: %s", code, errW)
			}
		})
	}
}

func TestLogUnknownSubject(t *testing.T) {
	db := filepath.Join(t.TempDir(), "m.db")
	seedLogThings(t, db)

	_, errW, code := runCapture("log", "nope", "--minutes", "30", "--db", db)
	if code != 1 {
		t.Errorf("exit = %d, want 1; stderr: %s", code, errW)
	}
	if !strings.Contains(errW, "unknown subject") {
		t.Errorf("stderr = %q, want unknown subject", errW)
	}
}

func TestNoteStoresRawText(t *testing.T) {
	db := filepath.Join(t.TempDir(), "m.db")
	out, errW, code := runCapture("note", "struggled on k-maps", "--db", db)
	if code != 0 {
		t.Fatalf("exit = %d, stderr: %s", code, errW)
	}
	evs := logEvents(t, db)
	if len(evs) != 1 {
		t.Fatalf("events = %d, want 1", len(evs))
	}
	e := evs[0]
	if e.Type != "note" || e.Source != "manual" {
		t.Errorf("type/source = %q/%q, want note/manual", e.Type, e.Source)
	}
	if e.Subject != nil {
		t.Errorf("subject = %v, want NULL", *e.Subject)
	}
	if e.RawText == nil || *e.RawText != "struggled on k-maps" {
		t.Errorf("raw_text = %v, want verbatim note", e.RawText)
	}
	if e.Quantified != 0 {
		t.Errorf("quantified = %d, want 0", e.Quantified)
	}
	if e.DedupKey != nil {
		t.Errorf("dedup_key = %v, want NULL", *e.DedupKey)
	}
	if len(e.Payload) != 0 {
		t.Errorf("payload = %s, want nil", e.Payload)
	}
	if want := fmt.Sprintf("stored note #%d.\n", e.ID); out != want {
		t.Errorf("stdout = %q, want %q", out, want)
	}
}

func TestNoteUsage(t *testing.T) {
	db := filepath.Join(t.TempDir(), "m.db")
	for _, args := range [][]string{
		{"note", "--db", db},
		{"note", "a", "b", "--db", db},
	} {
		if _, _, code := runCapture(args...); code != 2 {
			t.Errorf("%v exit = %d, want 2", args, code)
		}
	}
}
