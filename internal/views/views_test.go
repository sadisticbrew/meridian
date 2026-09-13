package views

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sadisticbrew/meridian/internal/model"
	"github.com/sadisticbrew/meridian/internal/render"
	"github.com/sadisticbrew/meridian/internal/store"
)

var (
	testZone    = time.FixedZone("FIX", 2*3600)
	testNow     = time.Date(2026, 9, 13, 18, 0, 0, 0, testZone)
	fixTS       = "2026-09-13T15:00:00Z"
	ruleCourse  = "book one evening block before the next test"
	rulePattern = "reviews ≥ solves this week → re-drill that pattern's trigger cards"
	ruleGerman  = "0 min this week → 15 min Nicos Weg tomorrow; B1 cuts PR 27→21 months"
	ruleOstep   = "book one audio-study block this week"
)

func openViewDB(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "views.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func seedThing(t *testing.T, st *store.Store, th model.Thing) {
	t.Helper()
	th.CreatedAt = fixTS
	if err := st.UpsertThing(th); err != nil {
		t.Fatalf("seed thing %s: %v", th.ID, err)
	}
}

func seedEvent(t *testing.T, st *store.Store, e model.Event) {
	t.Helper()
	e.CreatedAt = fixTS
	if _, err := st.AddEvent(e); err != nil {
		t.Fatalf("seed event %s/%s: %v", e.Type, deref(e.Subject), err)
	}
}

func deref(p *string) string {
	if p == nil {
		return "<nil>"
	}
	return *p
}

func sessionPayload(started, ended string, minutes int) string {
	return fmt.Sprintf(`{"started_at":%q,"ended_at":%q,"minutes":%d,"tags":[]}`, started, ended, minutes)
}

func seedBasic(t *testing.T) *store.Store {
	t.Helper()
	st := openViewDB(t)
	seedThing(t, st, model.Thing{ID: "course/ddco", Kind: "course", DisplayName: "Digital Design & Computer Organization",
		Active: true, GoalJSON: `{"weekly_minutes":90}`, DecisionRule: ruleCourse})
	seedThing(t, st, model.Thing{ID: "language/german", Kind: "language", DisplayName: "German (Nicos Weg)",
		Active: true, GoalJSON: `{"weekly_minutes":90,"occurrences_per_week":3}`, DecisionRule: ruleGerman})
	seedThing(t, st, model.Thing{ID: "self-study/ostep", Kind: "self-study", DisplayName: "OSTEP (audio study)",
		Active: true, GoalJSON: `{"weekly_minutes":90}`, DecisionRule: ruleOstep})
	seedThing(t, st, model.Thing{ID: "pattern/monotonic-stack", Kind: "pattern", DisplayName: "Monotonic Stack",
		Active: true, DecisionRule: rulePattern})
	seedThing(t, st, model.Thing{ID: "pattern/stack", Kind: "pattern", DisplayName: "Stack",
		Active: true, DecisionRule: rulePattern})

	ddco := "course/ddco"
	german := "language/german"
	mono := "pattern/monotonic-stack"
	stack := "pattern/stack"
	seedEvent(t, st, model.Event{Ts: "2026-09-13T07:00:00Z", Source: "manual", Type: "session",
		Subject: &ddco, Payload: jsonRaw(sessionPayload("2026-09-13T07:00:00Z", "2026-09-13T08:05:00Z", 65))})
	seedEvent(t, st, model.Event{Ts: "2026-09-13T07:30:00Z", Source: "manual", Type: "session",
		Subject: &german, Payload: jsonRaw(sessionPayload("2026-09-13T07:30:00Z", "2026-09-13T08:10:00Z", 40))})
	lesson46 := 46.0
	seedEvent(t, st, model.Event{Ts: "2026-09-13T08:20:00Z", Source: "manual", Type: "occurrence",
		Subject: &german, ValueNum: &lesson46})
	lesson44 := 44.0
	seedEvent(t, st, model.Event{Ts: "2026-09-12T15:00:00Z", Source: "manual", Type: "occurrence",
		Subject: &german, ValueNum: &lesson44})
	seedEvent(t, st, model.Event{Ts: "2026-09-13T07:00:00Z", Source: "manual", Type: "occurrence",
		Subject: &mono, Payload: jsonRaw(`{"problem":"car-fleet","outcome":"solved"}`)})
	seedEvent(t, st, model.Event{Ts: "2026-09-13T07:35:00Z", Source: "manual", Type: "occurrence",
		Subject: &mono, Payload: jsonRaw(`{"problem":"daily-temperatures","outcome":"solved"}`)})
	seedEvent(t, st, model.Event{Ts: "2026-09-11T08:00:00Z", Source: "manual", Type: "session",
		Subject: &ddco, Payload: jsonRaw(sessionPayload("2026-09-11T08:00:00Z", "2026-09-11T09:30:00Z", 90))})
	seedEvent(t, st, model.Event{Ts: "2026-09-10T07:00:00Z", Source: "manual", Type: "session",
		Subject: &ddco, Payload: jsonRaw(sessionPayload("2026-09-10T07:00:00Z", "2026-09-10T07:40:00Z", 40))})
	lesson42 := 42.0
	seedEvent(t, st, model.Event{Ts: "2026-09-10T14:00:00Z", Source: "manual", Type: "occurrence",
		Subject: &german, ValueNum: &lesson42})
	score := 21.0
	seedEvent(t, st, model.Event{Ts: "2026-09-12T07:00:00Z", Source: "manual", Type: "milestone",
		Subject: &ddco, ValueNum: &score, Payload: jsonRaw(`{"exam":"t1","max":25}`)})
	seedEvent(t, st, model.Event{Ts: "2026-09-11T08:00:00Z", Source: "manual", Type: "occurrence",
		Subject: &stack, Payload: jsonRaw(`{"problem":"largest-rectangle","outcome":"reviewed"}`)})
	seedEvent(t, st, model.Event{Ts: "2026-09-12T08:00:00Z", Source: "manual", Type: "occurrence",
		Subject: &stack, Payload: jsonRaw(`{"problem":"largest-rectangle","outcome":"struggled"}`)})
	return st
}

func jsonRaw(s string) json.RawMessage { return json.RawMessage(s) }

func seedPassiveSolve(t *testing.T, st *store.Store, subject, slug string, attempts int, ts string) {
	t.Helper()
	payload := fmt.Sprintf(`{"slug":%q,"attempts":%d,"language":"py","topic_folder":"Data Structures & Algorithms"}`,
		slug, attempts)
	dedup := "neetcode/" + slug
	seedEvent(t, st, model.Event{Ts: ts, Source: "neetcode", Type: "occurrence",
		Subject: &subject, Payload: jsonRaw(payload), DedupKey: &dedup})
}

func seedDSA(t *testing.T) *store.Store {
	t.Helper()
	st := openViewDB(t)
	seedThing(t, st, model.Thing{ID: "pattern/monotonic-stack", Kind: "pattern", DisplayName: "Monotonic Stack",
		Active: true, DecisionRule: rulePattern})
	seedThing(t, st, model.Thing{ID: "pattern/unclassified", Kind: "pattern", DisplayName: "Unclassified",
		Active: true})
	seedThing(t, st, model.Thing{ID: "pattern/stack", Kind: "pattern", DisplayName: "Stack",
		Active: true, DecisionRule: rulePattern})

	mono := "pattern/monotonic-stack"
	stack := "pattern/stack"
	seedPassiveSolve(t, st, mono, "car-fleet", 1, "2026-09-11T08:00:00Z")
	seedPassiveSolve(t, st, mono, "daily-temperatures", 1, "2026-09-12T08:00:00Z")
	seedPassiveSolve(t, st, mono, "sliding-window-maximum", 3, "2026-09-13T08:00:00Z")
	seedEvent(t, st, model.Event{Ts: "2026-09-13T09:00:00Z", Source: "manual", Type: "occurrence",
		Subject: &stack, Payload: jsonRaw(`{"problem":"largest-rectangle","outcome":"reviewed"}`)})
	return st
}

func seedDSAUnclassified(t *testing.T) *store.Store {
	t.Helper()
	st := seedDSA(t)
	seedPassiveSolve(t, st, "pattern/unclassified", "contains-duplicate", 2, "2026-09-10T08:00:00Z")
	seedPassiveSolve(t, st, "pattern/unclassified", "valid-anagram", 3, "2026-09-11T09:00:00Z")
	return st
}

func seedGermanDelta(t *testing.T) *store.Store {
	t.Helper()
	st := openViewDB(t)
	seedThing(t, st, model.Thing{ID: "language/german", Kind: "language", DisplayName: "German (Nicos Weg)",
		Active: true})
	german := "language/german"
	before := 42.0
	seedEvent(t, st, model.Event{Ts: "2026-09-05T08:00:00Z", Source: "manual", Type: "occurrence",
		Subject: &german, ValueNum: &before})
	seedEvent(t, st, model.Event{Ts: "2026-09-12T08:00:00Z", Source: "manual", Type: "session",
		Subject: &german, Payload: jsonRaw(sessionPayload("2026-09-12T08:00:00Z", "2026-09-12T08:40:00Z", 40))})
	latest := 46.0
	seedEvent(t, st, model.Event{Ts: "2026-09-13T08:00:00Z", Source: "manual", Type: "occurrence",
		Subject: &german, ValueNum: &latest})
	return st
}

func seedPatternSubject(t *testing.T) *store.Store {
	t.Helper()
	st := openViewDB(t)
	seedThing(t, st, model.Thing{ID: "pattern/monotonic-stack", Kind: "pattern", DisplayName: "Monotonic Stack",
		Active: true, DecisionRule: rulePattern})
	mono := "pattern/monotonic-stack"
	seedPassiveSolve(t, st, mono, "car-fleet", 2, "2026-09-12T08:00:00Z")
	seedEvent(t, st, model.Event{Ts: "2026-09-13T09:00:00Z", Source: "manual", Type: "occurrence",
		Subject: &mono, Payload: jsonRaw(`{"problem":"largest-rectangle","outcome":"reviewed"}`)})
	return st
}

func goldenCompare(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if os.Getenv("UPDATE_GOLDEN") != "" {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatalf("write golden %s: %v", name, err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v", name, err)
	}
	if string(want) != got {
		t.Errorf("golden %s mismatch\n--- want ---\n%s\n--- got ---\n%s", name, string(want), got)
	}
}

func TestTodayGolden(t *testing.T) {
	cases := []struct {
		name   string
		seed   func(*testing.T) *store.Store
		run    bool
		golden string
	}{
		{"basic", seedBasic, false, "today_basic.txt"},
		{"running", func(t *testing.T) *store.Store {
			st := seedBasic(t)
			if err := st.SetRunning(store.RunningSession{
				Subject:   "language/german",
				StartedAt: testNow.Add(-32 * time.Minute).UTC().Format(time.RFC3339),
			}); err != nil {
				t.Fatalf("SetRunning: %v", err)
			}
			return st
		}, false, "today_running.txt"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := tc.seed(t)
			var buf bytes.Buffer
			if err := Today(&buf, testNow, st); err != nil {
				t.Fatalf("Today: %v", err)
			}
			goldenCompare(t, tc.golden, buf.String())
		})
	}
}

func TestWeekGolden(t *testing.T) {
	cases := []struct {
		name   string
		last   bool
		seed   func(*testing.T) *store.Store
		golden string
	}{
		{"basic", false, seedBasic, "week_basic.txt"},
		{"last", true, seedBasic, "week_last.txt"},
		{"firsttry", false, seedDSA, "week_firsttry.txt"},
		{"unclassified", false, seedDSAUnclassified, "week_unclassified.txt"},
		{"german-delta", false, seedGermanDelta, "week_german_delta.txt"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := tc.seed(t)
			var buf bytes.Buffer
			if err := Week(&buf, testNow, st, tc.last); err != nil {
				t.Fatalf("Week: %v", err)
			}
			goldenCompare(t, tc.golden, buf.String())
		})
	}
}

func TestSubjectGolden(t *testing.T) {
	seedNoGoal := func(t *testing.T) *store.Store {
		st := openViewDB(t)
		seedThing(t, st, model.Thing{ID: "pattern/stack", Kind: "pattern", DisplayName: "Stack",
			Active: true, DecisionRule: rulePattern})
		return st
	}
	cases := []struct {
		name   string
		seed   func(*testing.T) *store.Store
		id     string
		golden string
	}{
		{"basic", seedBasic, "course/ddco", "subject_basic.txt"},
		{"empty", seedNoGoal, "pattern/stack", "subject_empty.txt"},
		{"pattern", seedPatternSubject, "pattern/monotonic-stack", "subject_pattern.txt"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := tc.seed(t)
			var buf bytes.Buffer
			if err := Subject(&buf, testNow, st, tc.id, 14); err != nil {
				t.Fatalf("Subject: %v", err)
			}
			goldenCompare(t, tc.golden, buf.String())
		})
	}
}

func TestSubjectUnknown(t *testing.T) {
	st := openViewDB(t)
	var buf bytes.Buffer
	err := Subject(&buf, testNow, st, "course/nope", 14)
	if err == nil {
		t.Fatal("want error for unknown subject")
	}
}

// empty-DB today/week render without sections and without error.
func TestEmptyTodayWeek(t *testing.T) {
	st := openViewDB(t)
	var buf bytes.Buffer
	if err := Today(&buf, testNow, st); err != nil {
		t.Fatalf("Today: %v", err)
	}
	wantToday := "Today — Sun Sep 13\n" + render.Underline("Today — Sun Sep 13") + "\n"
	if buf.String() != wantToday {
		t.Errorf("empty today = %q, want %q", buf.String(), wantToday)
	}
	buf.Reset()
	if err := Week(&buf, testNow, st, false); err != nil {
		t.Fatalf("Week: %v", err)
	}
	wantWeek := "Week — Sep 07 → Sep 13\n" + render.Underline("Week — Sep 07 → Sep 13") + "\n"
	if buf.String() != wantWeek {
		t.Errorf("empty week = %q, want %q", buf.String(), wantWeek)
	}
}
