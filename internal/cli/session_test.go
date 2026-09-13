package cli

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sadisticbrew/meridian/internal/model"
	"github.com/sadisticbrew/meridian/internal/store"
)

func openTestStore(t *testing.T, db string) *store.Store {
	t.Helper()
	st, err := store.Open(db)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func seedSessionThing(t *testing.T, st *store.Store, id, name string) {
	t.Helper()
	kind := id[:strings.Index(id, "/")]
	if err := st.UpsertThing(model.Thing{
		ID:          id,
		Kind:        kind,
		DisplayName: name,
		Active:      true,
		CreatedAt:   nowRFC3339(),
	}); err != nil {
		t.Fatalf("seed thing %s: %v", id, err)
	}
}

func TestSessionNoRunning(t *testing.T) {
	for _, verb := range []string{"status", "stop", "abort"} {
		t.Run(verb, func(t *testing.T) {
			db := filepath.Join(t.TempDir(), "m.db")
			out, errW, code := runCapture(verb, "--db", db)
			if code != 3 {
				t.Errorf("exit = %d, want 3; stderr: %s", code, errW)
			}
			if out != "no running session\n" {
				t.Errorf("stdout = %q, want %q", out, "no running session\n")
			}
		})
	}
}

func TestSessionStartThenStatus(t *testing.T) {
	db := filepath.Join(t.TempDir(), "m.db")
	st := openTestStore(t, db)
	seedSessionThing(t, st, "language/german", "German (Nicos Weg)")
	st.Close()

	out, errW, code := runCapture("start", "german", "--db", db)
	if code != 0 {
		t.Fatalf("start exit %d, stderr: %s", code, errW)
	}
	if out != "started language/german\n" {
		t.Errorf("start stdout = %q", out)
	}

	out, errW, code = runCapture("status", "--db", db)
	if code != 0 {
		t.Fatalf("status exit %d, stderr: %s", code, errW)
	}
	if !strings.HasPrefix(out, "language/german — ") {
		t.Errorf("status stdout = %q, want subject prefix", out)
	}
	if !strings.HasSuffix(strings.TrimRight(out, "\n"), "m") {
		t.Errorf("status stdout = %q, want compact minutes suffix", out)
	}

	st2 := openTestStore(t, db)
	rs, err := st2.RunningSession()
	if err != nil {
		t.Fatalf("running session: %v", err)
	}
	if rs == nil || rs.Subject != "language/german" {
		t.Fatalf("db running state = %+v, want language/german", rs)
	}
	if _, err := time.Parse(time.RFC3339, rs.StartedAt); err != nil {
		t.Errorf("started_at %q not RFC 3339: %v", rs.StartedAt, err)
	}
}

func TestSessionStartTwiceRefuses(t *testing.T) {
	db := filepath.Join(t.TempDir(), "m.db")
	st := openTestStore(t, db)
	seedSessionThing(t, st, "course/ddco", "DDCO")
	st.Close()

	if _, _, code := runCapture("start", "ddco", "--db", db); code != 0 {
		t.Fatal("first start failed")
	}
	_, errW, code := runCapture("start", "ddco", "--db", db)
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if !strings.Contains(errW, "session already running") || !strings.Contains(errW, "course/ddco") {
		t.Errorf("stderr = %q, want running subject report", errW)
	}
}

func TestSessionCrashRecovery(t *testing.T) {
	db := filepath.Join(t.TempDir(), "m.db")
	st := openTestStore(t, db)
	seedSessionThing(t, st, "course/ddco", "DDCO")
	started := time.Now().UTC().Add(-40 * time.Second).Format(time.RFC3339)
	if err := st.SetRunning(store.RunningSession{Subject: "course/ddco", StartedAt: started}); err != nil {
		t.Fatalf("set running: %v", err)
	}
	st.Close()

	out, errW, code := runCapture("status", "--db", db)
	if code != 0 {
		t.Fatalf("status exit %d, stderr: %s", code, errW)
	}
	if !strings.HasPrefix(out, "course/ddco — ") {
		t.Errorf("status stdout = %q, want course/ddco", out)
	}

	out, errW, code = runCapture("stop", "--db", db)
	if code != 0 {
		t.Fatalf("stop exit %d, stderr: %s", code, errW)
	}
	if !strings.Contains(out, "discarded 40s session — too short to be signal") {
		t.Errorf("stop stdout = %q", out)
	}

	st2 := openTestStore(t, db)
	rs, err := st2.RunningSession()
	if err != nil {
		t.Fatalf("running session: %v", err)
	}
	if rs != nil {
		t.Errorf("state not cleared after short stop: %+v", rs)
	}
	evs, err := st2.Events(store.EventFilter{})
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	if len(evs) != 0 {
		t.Errorf("short session wrote %d events", len(evs))
	}
}

func TestSessionStopLogsEvent(t *testing.T) {
	db := filepath.Join(t.TempDir(), "m.db")
	st := openTestStore(t, db)
	seedSessionThing(t, st, "course/ddco", "DDCO")
	started := time.Now().UTC().Add(-90 * time.Minute).Format(time.RFC3339)
	if err := st.SetRunning(store.RunningSession{Subject: "course/ddco", StartedAt: started}); err != nil {
		t.Fatalf("set running: %v", err)
	}
	st.Close()

	out, errW, code := runCapture("stop", "--db", db)
	if code != 0 {
		t.Fatalf("stop exit %d, stderr: %s", code, errW)
	}
	if out != "1h30m on DDCO — logged.\n" {
		t.Errorf("stdout = %q, want %q", out, "1h30m on DDCO — logged.\n")
	}

	st2 := openTestStore(t, db)
	evs, err := st2.Events(store.EventFilter{Type: "session"})
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	if len(evs) != 1 {
		t.Fatalf("session events = %d, want 1", len(evs))
	}
	e := evs[0]
	if e.Ts != started {
		t.Errorf("event ts = %q, want started_at %q", e.Ts, started)
	}
	if e.Source != "manual" || e.Subject == nil || *e.Subject != "course/ddco" {
		t.Errorf("event meta wrong: %+v", e)
	}
	var p struct {
		StartedAt string   `json:"started_at"`
		EndedAt   string   `json:"ended_at"`
		Minutes   int      `json:"minutes"`
		Tags      []string `json:"tags"`
		Note      string   `json:"note"`
	}
	if err := json.Unmarshal(e.Payload, &p); err != nil {
		t.Fatalf("payload %s: %v", e.Payload, err)
	}
	if p.StartedAt != started {
		t.Errorf("payload started_at = %q, want %q", p.StartedAt, started)
	}
	if p.Minutes != 90 {
		t.Errorf("payload minutes = %d, want 90", p.Minutes)
	}
	if p.Tags == nil {
		t.Error("payload tags = null, want []")
	}
	if p.Note != "" {
		t.Errorf("payload note = %q, want omitted", p.Note)
	}
	if _, err := time.Parse(time.RFC3339, p.EndedAt); err != nil {
		t.Errorf("payload ended_at %q not RFC 3339: %v", p.EndedAt, err)
	}
	rs, err := st2.RunningSession()
	if err != nil {
		t.Fatalf("running session: %v", err)
	}
	if rs != nil {
		t.Errorf("state not cleared after stop: %+v", rs)
	}
}

func TestSessionAbort(t *testing.T) {
	db := filepath.Join(t.TempDir(), "m.db")
	st := openTestStore(t, db)
	seedSessionThing(t, st, "course/ddco", "DDCO")
	if err := st.SetRunning(store.RunningSession{Subject: "course/ddco", StartedAt: nowRFC3339()}); err != nil {
		t.Fatalf("set running: %v", err)
	}
	st.Close()

	out, errW, code := runCapture("abort", "--db", db)
	if code != 0 {
		t.Fatalf("abort exit %d, stderr: %s", code, errW)
	}
	if out != "discarded.\n" {
		t.Errorf("stdout = %q, want %q", out, "discarded.\n")
	}

	st2 := openTestStore(t, db)
	rs, err := st2.RunningSession()
	if err != nil {
		t.Fatalf("running session: %v", err)
	}
	if rs != nil {
		t.Errorf("state not cleared after abort: %+v", rs)
	}
	evs, err := st2.Events(store.EventFilter{})
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	if len(evs) != 0 {
		t.Errorf("abort wrote %d events", len(evs))
	}
}

func TestSessionStartUsage(t *testing.T) {
	db := filepath.Join(t.TempDir(), "m.db")
	for _, args := range [][]string{
		{"start"},
		{"start", "a", "b"},
	} {
		if _, _, code := runCapture(append(args, "--db", db)...); code != 2 {
			t.Errorf("%v exit = %d, want 2", args, code)
		}
	}
}
