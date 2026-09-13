package cli

import (
	"encoding/json"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/sadisticbrew/meridian/internal/model"
	"github.com/sadisticbrew/meridian/internal/store"
)

func seedViewThings(t *testing.T, db string) {
	t.Helper()
	st := openTestStore(t, db)
	for _, thing := range []model.Thing{
		{ID: "course/ddco", Kind: "course", DisplayName: "DDCO", Active: true},
		{ID: "language/german", Kind: "language", DisplayName: "German", Active: true},
	} {
		thing.CreatedAt = nowRFC3339()
		if err := st.UpsertThing(thing); err != nil {
			t.Fatalf("seed %s: %v", thing.ID, err)
		}
	}
	st.Close()
}

func addViewEvent(t *testing.T, st *store.Store, e model.Event) {
	t.Helper()
	if e.Source == "" {
		e.Source = "manual"
	}
	if e.Type == "" {
		e.Type = "session"
	}
	if e.CreatedAt == "" {
		e.CreatedAt = nowRFC3339()
	}
	if _, err := st.AddEvent(e); err != nil {
		t.Fatalf("add event: %v", err)
	}
}

func parseNDJSON(t *testing.T, out string) []map[string]any {
	t.Helper()
	var evs []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("not JSON: %q: %v", line, err)
		}
		evs = append(evs, m)
	}
	return evs
}

var dumpKeys = []string{
	"id", "ts", "source", "type", "subject", "value_num", "value_text",
	"payload", "dedup_key", "raw_text", "quantified", "subject_name",
	"created_at",
}

func TestTodayJSONWindow(t *testing.T) {
	db := filepath.Join(t.TempDir(), "m.db")
	seedViewThings(t, db)
	now := time.Now()
	ddco := "course/ddco"
	st := openTestStore(t, db)
	addViewEvent(t, st, model.Event{
		Ts:      now.UTC().Format(time.RFC3339),
		Type:    "session",
		Subject: &ddco,
		Payload: json.RawMessage(`{"minutes":65}`),
	})
	addViewEvent(t, st, model.Event{
		Ts:      now.Add(-48 * time.Hour).UTC().Format(time.RFC3339),
		Type:    "session",
		Subject: &ddco,
	})
	st.Close()

	out, errW, code := runCapture("today", "--json", "--db", db)
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errW)
	}
	evs := parseNDJSON(t, out)
	if len(evs) != 1 {
		t.Fatalf("today events = %d, want only today's 1:\n%s", len(evs), out)
	}
	e := evs[0]
	for _, key := range dumpKeys {
		if _, ok := e[key]; !ok {
			t.Errorf("missing key %q in %s", key, out)
		}
	}
	if e["subject_name"] != "DDCO" {
		t.Errorf("subject_name = %v, want DDCO", e["subject_name"])
	}
	p, ok := e["payload"].(map[string]any)
	if !ok {
		t.Fatalf("payload = %#v, want JSON object", e["payload"])
	}
	if p["minutes"] != float64(65) {
		t.Errorf("payload.minutes = %v, want 65", p["minutes"])
	}
	if want := now.UTC().Format(time.RFC3339); e["ts"] != want {
		t.Errorf("ts = %v, want %s", e["ts"], want)
	}
}

func TestTodayTextMode(t *testing.T) {
	db := filepath.Join(t.TempDir(), "m.db")
	seedViewThings(t, db)
	out, errW, code := runCapture("today", "--db", db)
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errW)
	}
	if !strings.Contains(out, "Today —") {
		t.Errorf("output missing header:\n%s", out)
	}
}

func TestWeekJSONCurrentAndLast(t *testing.T) {
	db := filepath.Join(t.TempDir(), "m.db")
	seedViewThings(t, db)
	now := time.Now()
	ddco := "course/ddco"
	thisWeek := now
	lastWeek := now.AddDate(0, 0, -7)
	st := openTestStore(t, db)
	addViewEvent(t, st, model.Event{Ts: thisWeek.UTC().Format(time.RFC3339), Subject: &ddco})
	addViewEvent(t, st, model.Event{Ts: lastWeek.UTC().Format(time.RFC3339), Subject: &ddco})
	st.Close()

	out, errW, code := runCapture("week", "--json", "--db", db)
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errW)
	}
	evs := parseNDJSON(t, out)
	if len(evs) != 1 {
		t.Fatalf("current week events = %d, want 1:\n%s", len(evs), out)
	}
	if want := thisWeek.UTC().Format(time.RFC3339); evs[0]["ts"] != want {
		t.Errorf("current week ts = %v, want %s", evs[0]["ts"], want)
	}

	out, errW, code = runCapture("week", "--last", "--json", "--db", db)
	if code != 0 {
		t.Fatalf("last exit %d, stderr: %s", code, errW)
	}
	evs = parseNDJSON(t, out)
	if len(evs) != 1 {
		t.Fatalf("last week events = %d, want 1:\n%s", len(evs), out)
	}
	if want := lastWeek.UTC().Format(time.RFC3339); evs[0]["ts"] != want {
		t.Errorf("last week ts = %v, want %s", evs[0]["ts"], want)
	}
}

func TestSubjectJSONFiltersAndDays(t *testing.T) {
	db := filepath.Join(t.TempDir(), "m.db")
	seedViewThings(t, db)
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	ddco := "course/ddco"
	german := "language/german"
	st := openTestStore(t, db)
	addViewEvent(t, st, model.Event{Ts: now.UTC().Format(time.RFC3339), Subject: &ddco})
	addViewEvent(t, st, model.Event{Ts: now.UTC().Format(time.RFC3339), Type: "occurrence", Subject: &german})
	addViewEvent(t, st, model.Event{
		Ts:      today.AddDate(0, 0, -14).Add(12 * time.Hour).UTC().Format(time.RFC3339),
		Subject: &ddco,
	})
	addViewEvent(t, st, model.Event{
		Ts:      today.AddDate(0, 0, -15).Add(12 * time.Hour).UTC().Format(time.RFC3339),
		Subject: &ddco,
	})
	st.Close()

	cases := []struct {
		name string
		days int
		want int
	}{
		{"window start is today-(days-1)", 14, 1},
		{"includes boundary day", 15, 2},
		{"includes one more day", 16, 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, errW, code := runCapture("subject", "ddco", "--days", strconv.Itoa(tc.days), "--json", "--db", db)
			if code != 0 {
				t.Fatalf("exit %d, stderr: %s", code, errW)
			}
			evs := parseNDJSON(t, out)
			if len(evs) != tc.want {
				t.Fatalf("events = %d, want %d:\n%s", len(evs), tc.want, out)
			}
			for _, e := range evs {
				if e["subject"] != ddco {
					t.Errorf("subject = %v, want %s", e["subject"], ddco)
				}
			}
		})
	}
}

func TestSubjectUsageAndUnknown(t *testing.T) {
	db := filepath.Join(t.TempDir(), "m.db")
	seedViewThings(t, db)
	if _, errW, code := runCapture("subject", "ddco", "--days", "0", "--db", db); code != 2 {
		t.Errorf("--days 0 exit = %d, want 2; stderr: %s", code, errW)
	}
	if _, errW, code := runCapture("subject", "nope", "--db", db); code != 1 {
		t.Errorf("unknown exit = %d, want 1; stderr: %s", code, errW)
	}
}

func TestDumpNDJSON(t *testing.T) {
	db := filepath.Join(t.TempDir(), "m.db")
	seedViewThings(t, db)
	now := time.Now()
	ddco := "course/ddco"
	german := "language/german"
	raw := "freeform"
	st := openTestStore(t, db)
	addViewEvent(t, st, model.Event{
		Ts:      now.Add(-2 * time.Hour).UTC().Format(time.RFC3339),
		Type:    "session",
		Subject: &ddco,
		Payload: json.RawMessage(`{"minutes":65}`),
	})
	addViewEvent(t, st, model.Event{
		Ts:      now.Add(-time.Hour).UTC().Format(time.RFC3339),
		Type:    "occurrence",
		Subject: &german,
	})
	addViewEvent(t, st, model.Event{
		Ts:      now.UTC().Format(time.RFC3339),
		Type:    "note",
		RawText: &raw,
	})
	seeded, err := st.Events(store.EventFilter{})
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	st.Close()

	out, errW, code := runCapture("dump", "--db", db)
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errW)
	}
	if lines := strings.Count(out, "\n"); lines != len(seeded) {
		t.Errorf("dump lines = %d, want %d (event count)", lines, len(seeded))
	}
	evs := parseNDJSON(t, out)
	if len(evs) != 3 {
		t.Fatalf("events = %d, want 3:\n%s", len(evs), out)
	}
	for i, e := range evs {
		for _, key := range dumpKeys {
			if _, ok := e[key]; !ok {
				t.Errorf("event %d missing key %q", i, key)
			}
		}
	}
	byType := map[string]map[string]any{}
	for _, e := range evs {
		byType[e["type"].(string)] = e
	}
	session := byType["session"]
	if session["subject_name"] != "DDCO" {
		t.Errorf("session subject_name = %v, want DDCO", session["subject_name"])
	}
	if _, ok := session["payload"].(map[string]any); !ok {
		t.Errorf("session payload = %#v, want JSON object", session["payload"])
	}
	note := byType["note"]
	if note["subject"] != nil || note["subject_name"] != nil {
		t.Errorf("note subject/subject_name = %v/%v, want null/null", note["subject"], note["subject_name"])
	}
	if note["raw_text"] != "freeform" {
		t.Errorf("note raw_text = %v, want freeform", note["raw_text"])
	}

	out, errW, code = runCapture("dump", "--type", "session", "--db", db)
	if code != 0 {
		t.Fatalf("--type exit %d, stderr: %s", code, errW)
	}
	if evs := parseNDJSON(t, out); len(evs) != 1 || evs[0]["type"] != "session" {
		t.Errorf("--type session = %v", out)
	}

	out, errW, code = runCapture("dump", "--subject", "german", "--db", db)
	if code != 0 {
		t.Fatalf("--subject exit %d, stderr: %s", code, errW)
	}
	if evs := parseNDJSON(t, out); len(evs) != 1 || evs[0]["subject"] != german {
		t.Errorf("--subject german = %v", out)
	}

	out, errW, code = runCapture("dump", "--since", now.Add(-90*time.Minute).UTC().Format(time.RFC3339), "--db", db)
	if code != 0 {
		t.Fatalf("--since exit %d, stderr: %s", code, errW)
	}
	if evs := parseNDJSON(t, out); len(evs) != 2 {
		t.Errorf("--since events = %d, want 2:\n%s", len(evs), out)
	}

	_, errW, code = runCapture("dump", "--subject", "nope", "--db", db)
	if code != 1 {
		t.Errorf("unknown subject exit = %d, want 1; stderr: %s", code, errW)
	}
	if !strings.Contains(errW, "dump: unknown subject") {
		t.Errorf("stderr = %q, want dump-prefixed unknown subject", errW)
	}
}
