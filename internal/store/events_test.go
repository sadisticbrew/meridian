package store

import (
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/sadisticbrew/meridian/internal/model"
)

func ptr[T any](v T) *T { return &v }

func addEvent(t *testing.T, s *Store, e model.Event) int64 {
	t.Helper()
	id, err := s.AddEvent(e)
	if err != nil {
		t.Fatalf("AddEvent(%+v): %v", e, err)
	}
	return id
}

func TestAddEventRoundTrip(t *testing.T) {
	cases := []struct {
		name  string
		event model.Event
	}{
		{
			name: "all fields",
			event: model.Event{
				Ts:         "2026-09-13T09:12:00Z",
				Source:     "manual",
				Type:       "session",
				Subject:    ptr("course/ddco"),
				ValueNum:   ptr(65.0),
				ValueText:  ptr("sixty-five"),
				Payload:    json.RawMessage(`{"started_at":"2026-09-13T09:12:00Z","ended_at":"2026-09-13T10:17:00Z","minutes":65,"tags":["focus"]}`),
				DedupKey:   ptr("neetcode/car-fleet"),
				RawText:    ptr("verbatim note"),
				Quantified: 1,
				CreatedAt:  "2026-09-13T18:32:11Z",
			},
		},
		{
			name: "nullables absent",
			event: model.Event{
				Ts:        "2026-09-13T20:00:00Z",
				Source:    "manual",
				Type:      "note",
				CreatedAt: "2026-09-13T20:00:01Z",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := openTest(t)
			id := addEvent(t, s, tc.event)
			if id <= 0 {
				t.Fatalf("id = %d, want > 0", id)
			}
			want := tc.event
			want.ID = id
			got, err := s.Events(EventFilter{})
			if err != nil {
				t.Fatalf("Events: %v", err)
			}
			if !reflect.DeepEqual(got, []model.Event{want}) {
				t.Errorf("round trip mismatch:\n got %+v\nwant %+v", got, []model.Event{want})
			}
		})
	}
}

func TestEventWindowAndFilters(t *testing.T) {
	s := openTest(t)
	seed := []model.Event{
		{Ts: "2026-09-12T07:00:00Z", Source: "manual", Type: "session", Subject: ptr("course/ddco"), CreatedAt: "2026-09-12T07:01:00Z"},
		{Ts: "2026-09-13T08:00:00Z", Source: "manual", Type: "session", Subject: ptr("course/ddco"), CreatedAt: "2026-09-13T08:01:00Z"},
		{Ts: "2026-09-13T09:00:00Z", Source: "manual", Type: "occurrence", Subject: ptr("language/german"), ValueNum: ptr(44.0), CreatedAt: "2026-09-13T09:01:00Z"},
		{Ts: "2026-09-13T12:00:00Z", Source: "manual", Type: "session", Subject: ptr("course/ddco"), CreatedAt: "2026-09-13T12:01:00Z"},
		{Ts: "2026-09-14T08:00:00Z", Source: "manual", Type: "milestone", Subject: ptr("course/ddco"), ValueNum: ptr(22.0), CreatedAt: "2026-09-14T08:01:00Z"},
	}
	e0 := addEvent(t, s, seed[0])
	e1 := addEvent(t, s, seed[1])
	e2 := addEvent(t, s, seed[2])
	e3 := addEvent(t, s, seed[3])
	e4 := addEvent(t, s, seed[4])

	cases := []struct {
		name   string
		filter EventFilter
		want   []int64
	}{
		{"empty unbounded", EventFilter{}, []int64{e0, e1, e2, e3, e4}},
		{"half-open excludes to", EventFilter{From: "2026-09-13T08:00:00Z", To: "2026-09-13T12:00:00Z"}, []int64{e1, e2}},
		{"from only unbounded to", EventFilter{From: "2026-09-13T09:00:00Z"}, []int64{e2, e3, e4}},
		{"to only unbounded from", EventFilter{To: "2026-09-13T09:00:00Z"}, []int64{e0, e1}},
		{"type filter", EventFilter{Type: "session"}, []int64{e0, e1, e3}},
		{"subject filter", EventFilter{Subject: "course/ddco"}, []int64{e0, e1, e3, e4}},
		{"type and subject", EventFilter{Type: "milestone", Subject: "course/ddco"}, []int64{e4}},
		{"no match", EventFilter{Subject: "course/nope"}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := s.Events(tc.filter)
			if err != nil {
				t.Fatalf("Events: %v", err)
			}
			var gotIDs []int64
			for _, e := range got {
				gotIDs = append(gotIDs, e.ID)
			}
			if !slices.Equal(gotIDs, tc.want) {
				t.Errorf("ids = %v, want %v", gotIDs, tc.want)
			}
		})
	}
}

func TestEventWrappers(t *testing.T) {
	s := openTest(t)
	seed := []model.Event{
		{Ts: "2026-09-01T08:00:00Z", Source: "manual", Type: "session", Subject: ptr("course/ddco"), Payload: json.RawMessage(`{"minutes":40}`), CreatedAt: "2026-09-01T08:01:00Z"},
		{Ts: "2026-09-10T08:00:00Z", Source: "manual", Type: "occurrence", Subject: ptr("course/ddco"), ValueNum: ptr(7.0), CreatedAt: "2026-09-10T08:01:00Z"},
		{Ts: "2026-09-11T08:00:00Z", Source: "manual", Type: "milestone", Subject: ptr("course/ddco"), ValueNum: ptr(22.0), Payload: json.RawMessage(`{"exam":"t1","max":25}`), CreatedAt: "2026-09-11T08:01:00Z"},
		{Ts: "2026-09-12T08:00:00Z", Source: "manual", Type: "session", Subject: ptr("course/ddco"), Payload: json.RawMessage(`{"minutes":30}`), CreatedAt: "2026-09-12T08:01:00Z"},
		{Ts: "2026-09-13T08:00:00Z", Source: "manual", Type: "session", Subject: ptr("language/german"), Payload: json.RawMessage(`{"minutes":15}`), CreatedAt: "2026-09-13T08:01:00Z"},
	}
	ids := make([]int64, len(seed))
	for i, e := range seed {
		ids[i] = addEvent(t, s, e)
	}

	const from, to = "2026-09-10T00:00:00Z", "2026-09-13T00:00:00Z"
	cases := []struct {
		name string
		call func() ([]model.Event, error)
		want []int64
	}{
		{"events for window", func() ([]model.Event, error) { return s.EventsForWindow("course/ddco", from, to) }, []int64{ids[1], ids[2], ids[3]}},
		{"sessions for window", func() ([]model.Event, error) { return s.SessionsForWindow("course/ddco", from, to) }, []int64{ids[3]}},
		{"occurrences for window", func() ([]model.Event, error) { return s.OccurrencesForWindow("course/ddco", from, to) }, []int64{ids[1]}},
		{"milestones for subject", func() ([]model.Event, error) { return s.MilestonesForSubject("course/ddco") }, []int64{ids[2]}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.call()
			if err != nil {
				t.Fatalf("call: %v", err)
			}
			var gotIDs []int64
			for _, e := range got {
				gotIDs = append(gotIDs, e.ID)
			}
			if !slices.Equal(gotIDs, tc.want) {
				t.Errorf("ids = %v, want %v", gotIDs, tc.want)
			}
		})
	}

	t.Run("minutes for window", func(t *testing.T) {
		got, err := s.MinutesForWindow("course/ddco", from, to)
		if err != nil {
			t.Fatalf("MinutesForWindow: %v", err)
		}
		if got != 30 {
			t.Errorf("minutes = %d, want 30", got)
		}
	})
}

func TestMinutesForWindow(t *testing.T) {
	s := openTest(t)
	seed := []model.Event{
		{Ts: "2026-09-13T08:00:00Z", Source: "manual", Type: "session", Subject: ptr("course/ddco"), Payload: json.RawMessage(`{"minutes":65}`), CreatedAt: "2026-09-13T08:01:00Z"},
		{Ts: "2026-09-13T09:00:00Z", Source: "manual", Type: "session", Subject: ptr("course/ddco"), Payload: json.RawMessage(`{"minutes":30,"tags":["focus"]}`), CreatedAt: "2026-09-13T09:01:00Z"},
		{Ts: "2026-09-13T10:00:00Z", Source: "manual", Type: "session", Subject: ptr("course/ddco"), Payload: json.RawMessage(`{"tags":["focus"]}`), CreatedAt: "2026-09-13T10:01:00Z"},
		{Ts: "2026-09-13T11:00:00Z", Source: "manual", Type: "session", Subject: ptr("course/ddco"), CreatedAt: "2026-09-13T11:01:00Z"},
		{Ts: "2026-09-13T12:00:00Z", Source: "manual", Type: "session", Subject: ptr("course/ddco"), Payload: json.RawMessage(`{"minutes":100}`), CreatedAt: "2026-09-13T12:01:00Z"},
		{Ts: "2026-09-13T13:00:00Z", Source: "manual", Type: "occurrence", Subject: ptr("course/ddco"), Payload: json.RawMessage(`{"minutes":99}`), CreatedAt: "2026-09-13T13:01:00Z"},
		{Ts: "2026-09-13T14:00:00Z", Source: "manual", Type: "session", Subject: ptr("language/german"), Payload: json.RawMessage(`{"minutes":7}`), CreatedAt: "2026-09-13T14:01:00Z"},
	}
	for _, e := range seed {
		addEvent(t, s, e)
	}

	got, err := s.MinutesForWindow("course/ddco", "2026-09-13T00:00:00Z", "2026-09-13T12:00:00Z")
	if err != nil {
		t.Fatalf("MinutesForWindow: %v", err)
	}
	if got != 95 {
		t.Errorf("minutes = %d, want 95", got)
	}
}

func TestLatestOccurrence(t *testing.T) {
	s := openTest(t)
	if _, err := s.LatestOccurrence("language/german"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("empty: err = %v, want ErrNotFound", err)
	}

	first := addEvent(t, s, model.Event{Ts: "2026-09-01T08:00:00Z", Source: "manual", Type: "occurrence", Subject: ptr("language/german"), ValueNum: ptr(40.0), CreatedAt: "2026-09-01T08:01:00Z"})
	second := addEvent(t, s, model.Event{Ts: "2026-09-08T08:00:00Z", Source: "manual", Type: "occurrence", Subject: ptr("language/german"), ValueNum: ptr(44.0), CreatedAt: "2026-09-08T08:01:00Z"})
	addEvent(t, s, model.Event{Ts: "2026-09-15T08:00:00Z", Source: "manual", Type: "session", Subject: ptr("language/german"), CreatedAt: "2026-09-15T08:01:00Z"})
	addEvent(t, s, model.Event{Ts: "2026-09-12T08:00:00Z", Source: "manual", Type: "occurrence", Subject: ptr("course/ddco"), ValueNum: ptr(9.0), CreatedAt: "2026-09-12T08:01:00Z"})

	cases := []struct {
		name string
		call func() (*model.Event, error)
		want int64
	}{
		{"latest", func() (*model.Event, error) { return s.LatestOccurrence("language/german") }, second},
		{"before latest", func() (*model.Event, error) {
			return s.LatestOccurrenceBefore("language/german", "2026-09-08T08:00:00Z")
		}, first},
		{"before bound after latest", func() (*model.Event, error) {
			return s.LatestOccurrenceBefore("language/german", "2026-09-09T00:00:00Z")
		}, second},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.call()
			if err != nil {
				t.Fatalf("call: %v", err)
			}
			if got.ID != tc.want {
				t.Errorf("id = %d, want %d", got.ID, tc.want)
			}
		})
	}

	if _, err := s.LatestOccurrenceBefore("language/german", "2026-09-01T08:00:00Z"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("before first: err = %v, want ErrNotFound", err)
	}
	if _, err := s.LatestOccurrence("course/nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown subject: err = %v, want ErrNotFound", err)
	}
}

func TestRunningSession(t *testing.T) {
	s := openTest(t)

	got, err := s.RunningSession()
	if err != nil {
		t.Fatalf("RunningSession: %v", err)
	}
	if got != nil {
		t.Fatalf("running = %+v, want nil", got)
	}

	want := RunningSession{Subject: "course/ddco", StartedAt: "2026-09-13T18:00:00Z", Note: "deep work"}
	if err := s.SetRunning(want); err != nil {
		t.Fatalf("SetRunning: %v", err)
	}
	got, err = s.RunningSession()
	if err != nil {
		t.Fatalf("RunningSession: %v", err)
	}
	if !reflect.DeepEqual(*got, want) {
		t.Errorf("round trip mismatch:\n got %+v\nwant %+v", *got, want)
	}

	second := RunningSession{Subject: "language/german", StartedAt: "2026-09-13T19:00:00Z", Note: ""}
	if err := s.SetRunning(second); err != nil {
		t.Fatalf("SetRunning second: %v", err)
	}
	got, err = s.RunningSession()
	if err != nil {
		t.Fatalf("RunningSession: %v", err)
	}
	if !reflect.DeepEqual(*got, second) {
		t.Errorf("replace mismatch:\n got %+v\nwant %+v", *got, second)
	}
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM state WHERE key = ?`, runningSessionKey).Scan(&n); err != nil {
		t.Fatalf("count state: %v", err)
	}
	if n != 1 {
		t.Errorf("state rows = %d, want 1", n)
	}

	if err := s.ClearRunning(); err != nil {
		t.Fatalf("ClearRunning: %v", err)
	}
	got, err = s.RunningSession()
	if err != nil {
		t.Fatalf("RunningSession after clear: %v", err)
	}
	if got != nil {
		t.Fatalf("running = %+v after clear, want nil", got)
	}
	if err := s.ClearRunning(); err != nil {
		t.Fatalf("ClearRunning when absent: %v", err)
	}
}

func TestUpsertByDedup(t *testing.T) {
	const dedup = "neetcode/car-fleet"
	seedEvent := func(ts, payload, createdAt string) model.Event {
		return model.Event{
			Ts:        ts,
			Source:    "neetcode",
			Type:      "occurrence",
			Subject:   ptr("pattern/monotonic-stack"),
			Payload:   json.RawMessage(payload),
			DedupKey:  ptr(dedup),
			CreatedAt: createdAt,
		}
	}
	unchanged := func(t *testing.T, got []model.Event, wantID int64, wantPayload string) {
		t.Helper()
		if len(got) != 1 {
			t.Fatalf("events = %d, want 1", len(got))
		}
		if got[0].ID != wantID {
			t.Errorf("id = %d, want %d", got[0].ID, wantID)
		}
		if string(got[0].Payload) != wantPayload {
			t.Errorf("payload = %s, want unchanged %s", got[0].Payload, wantPayload)
		}
	}

	cases := []struct {
		name       string
		seed       []model.Event
		upsert     model.Event
		wantAdded  bool
		wantUpdate bool
		wantErr    string
		check      func(t *testing.T, got []model.Event, upsert model.Event, seedIDs []int64)
	}{
		{
			name:      "insert new dedup key",
			upsert:    seedEvent("2026-09-13T09:00:00Z", `{"slug":"car-fleet","attempts":1,"language":"py"}`, "2026-09-13T09:00:01Z"),
			wantAdded: true,
			check: func(t *testing.T, got []model.Event, upsert model.Event, _ []int64) {
				if len(got) != 1 {
					t.Fatalf("events = %d, want 1", len(got))
				}
				want := upsert
				want.ID = got[0].ID
				if got[0].ID <= 0 || !reflect.DeepEqual(got[0], want) {
					t.Errorf("event mismatch:\n got %+v\nwant %+v", got[0], want)
				}
			},
		},
		{
			name: "higher attempts updates payload only",
			seed: []model.Event{seedEvent("2026-09-01T09:00:00Z", `{"slug":"car-fleet","attempts":1,"language":"py"}`, "2026-09-01T09:00:01Z")},
			upsert: model.Event{
				Ts:        "2026-09-02T10:00:00Z",
				Source:    "hook",
				Type:      "occurrence",
				Subject:   ptr("pattern/other"),
				Payload:   json.RawMessage(`{"slug":"car-fleet","attempts":2,"language":"go"}`),
				DedupKey:  ptr(dedup),
				CreatedAt: "2026-09-02T10:00:01Z",
			},
			wantUpdate: true,
			check: func(t *testing.T, got []model.Event, _ model.Event, seedIDs []int64) {
				if len(got) != 1 {
					t.Fatalf("events = %d, want 1", len(got))
				}
				e := got[0]
				if e.ID != seedIDs[0] {
					t.Errorf("id = %d, want %d (same row, no duplicate)", e.ID, seedIDs[0])
				}
				if e.Ts != "2026-09-01T09:00:00Z" || e.Source != "neetcode" || e.CreatedAt != "2026-09-01T09:00:01Z" {
					t.Errorf("immutable fields changed: ts=%q source=%q created_at=%q", e.Ts, e.Source, e.CreatedAt)
				}
				if e.Subject == nil || *e.Subject != "pattern/monotonic-stack" {
					t.Errorf("subject = %v, want unchanged", e.Subject)
				}
				if string(e.Payload) != `{"slug":"car-fleet","attempts":2,"language":"go"}` {
					t.Errorf("payload = %s, want enriched", e.Payload)
				}
			},
		},
		{
			name: "equal attempts no-op",
			seed: []model.Event{seedEvent("2026-09-01T09:00:00Z", `{"slug":"car-fleet","attempts":2}`, "2026-09-01T09:00:01Z")},
			upsert: model.Event{
				Ts:        "2026-09-02T10:00:00Z",
				Source:    "neetcode",
				Type:      "occurrence",
				Subject:   ptr("pattern/monotonic-stack"),
				Payload:   json.RawMessage(`{"slug":"car-fleet","attempts":2,"language":"go"}`),
				DedupKey:  ptr(dedup),
				CreatedAt: "2026-09-02T10:00:01Z",
			},
			check: func(t *testing.T, got []model.Event, _ model.Event, seedIDs []int64) {
				unchanged(t, got, seedIDs[0], `{"slug":"car-fleet","attempts":2}`)
			},
		},
		{
			name: "lower attempts no-op",
			seed: []model.Event{seedEvent("2026-09-01T09:00:00Z", `{"slug":"car-fleet","attempts":3}`, "2026-09-01T09:00:01Z")},
			upsert: model.Event{
				Ts:        "2026-09-02T10:00:00Z",
				Source:    "neetcode",
				Type:      "occurrence",
				Subject:   ptr("pattern/monotonic-stack"),
				Payload:   json.RawMessage(`{"slug":"car-fleet","attempts":2,"language":"go"}`),
				DedupKey:  ptr(dedup),
				CreatedAt: "2026-09-02T10:00:01Z",
			},
			check: func(t *testing.T, got []model.Event, _ model.Event, seedIDs []int64) {
				unchanged(t, got, seedIDs[0], `{"slug":"car-fleet","attempts":3}`)
			},
		},
		{
			name: "existing payload without attempts treated as zero",
			seed: []model.Event{seedEvent("2026-09-01T09:00:00Z", `{"slug":"car-fleet"}`, "2026-09-01T09:00:01Z")},
			upsert: model.Event{
				Ts:        "2026-09-02T10:00:00Z",
				Source:    "neetcode",
				Type:      "occurrence",
				Subject:   ptr("pattern/monotonic-stack"),
				Payload:   json.RawMessage(`{"slug":"car-fleet","attempts":1}`),
				DedupKey:  ptr(dedup),
				CreatedAt: "2026-09-02T10:00:01Z",
			},
			wantUpdate: true,
			check: func(t *testing.T, got []model.Event, _ model.Event, seedIDs []int64) {
				if len(got) != 1 {
					t.Fatalf("events = %d, want 1", len(got))
				}
				if got[0].ID != seedIDs[0] || string(got[0].Payload) != `{"slug":"car-fleet","attempts":1}` {
					t.Errorf("id/payload = %d/%s, want %d/attempts 1", got[0].ID, got[0].Payload, seedIDs[0])
				}
			},
		},
		{
			name: "unparseable existing payload treated as zero",
			seed: []model.Event{seedEvent("2026-09-01T09:00:00Z", `not-json`, "2026-09-01T09:00:01Z")},
			upsert: model.Event{
				Ts:        "2026-09-02T10:00:00Z",
				Source:    "neetcode",
				Type:      "occurrence",
				Subject:   ptr("pattern/monotonic-stack"),
				Payload:   json.RawMessage(`{"slug":"car-fleet","attempts":1}`),
				DedupKey:  ptr(dedup),
				CreatedAt: "2026-09-02T10:00:01Z",
			},
			wantUpdate: true,
			check: func(t *testing.T, got []model.Event, _ model.Event, seedIDs []int64) {
				if len(got) != 1 {
					t.Fatalf("events = %d, want 1", len(got))
				}
				if got[0].ID != seedIDs[0] || string(got[0].Payload) != `{"slug":"car-fleet","attempts":1}` {
					t.Errorf("id/payload = %d/%s, want %d/attempts 1", got[0].ID, got[0].Payload, seedIDs[0])
				}
			},
		},
		{
			name: "nil dedup key rejected",
			upsert: model.Event{
				Ts:        "2026-09-13T09:00:00Z",
				Source:    "neetcode",
				Type:      "occurrence",
				Subject:   ptr("pattern/monotonic-stack"),
				CreatedAt: "2026-09-13T09:00:01Z",
			},
			wantErr: "dedup_key required",
			check: func(t *testing.T, got []model.Event, _ model.Event, _ []int64) {
				if len(got) != 0 {
					t.Errorf("events = %d, want 0", len(got))
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := openTest(t)
			seedIDs := make([]int64, len(tc.seed))
			for i, e := range tc.seed {
				seedIDs[i] = addEvent(t, s, e)
			}
			added, updated, err := s.UpsertByDedup(tc.upsert)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tc.wantErr)
				}
			} else if err != nil {
				t.Fatalf("UpsertByDedup: %v", err)
			}
			if added != tc.wantAdded || updated != tc.wantUpdate {
				t.Errorf("added=%v updated=%v, want added=%v updated=%v", added, updated, tc.wantAdded, tc.wantUpdate)
			}
			got, err := s.Events(EventFilter{})
			if err != nil {
				t.Fatalf("Events: %v", err)
			}
			if tc.check != nil {
				tc.check(t, got, tc.upsert, seedIDs)
			}
		})
	}
}

func TestEventByDedup(t *testing.T) {
	s := openTest(t)
	seed := model.Event{
		Ts:        "2026-09-01T10:00:00Z",
		Source:    "neetcode",
		Type:      "occurrence",
		Subject:   ptr("pattern/monotonic-stack"),
		Payload:   json.RawMessage(`{"slug":"car-fleet","attempts":1,"language":"py"}`),
		DedupKey:  ptr("neetcode/car-fleet"),
		CreatedAt: "2026-09-01T10:00:01Z",
	}
	want := seed
	want.ID = addEvent(t, s, seed)

	cases := []struct {
		name    string
		key     string
		want    *model.Event
		wantErr error
	}{
		{"found", "neetcode/car-fleet", &want, nil},
		{"not found", "neetcode/nope", nil, ErrNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := s.EventByDedup(tc.key)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v, want %v", err, tc.wantErr)
				}
				if got != nil {
					t.Errorf("event = %+v, want nil", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("EventByDedup: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("event mismatch:\n got %+v\nwant %+v", *got, *tc.want)
			}
		})
	}
}

func TestAddEventNullDedupKeys(t *testing.T) {
	s := openTest(t)
	cases := []model.Event{
		{Ts: "2026-09-13T08:00:00Z", Source: "manual", Type: "note", RawText: ptr("first"), CreatedAt: "2026-09-13T08:00:01Z"},
		{Ts: "2026-09-13T09:00:00Z", Source: "manual", Type: "note", RawText: ptr("second"), CreatedAt: "2026-09-13T09:00:01Z"},
	}
	eventIDs := make([]int64, len(cases))
	for i, e := range cases {
		eventIDs[i] = addEvent(t, s, e)
	}
	if eventIDs[0] == eventIDs[1] {
		t.Fatalf("ids = %v, want distinct", eventIDs)
	}
	got, err := s.Events(EventFilter{})
	if err != nil {
		t.Fatalf("Events: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("events = %d, want 2 (multiple NULL dedup keys allowed)", len(got))
	}
	for i, e := range got {
		if e.DedupKey != nil {
			t.Errorf("event %d dedup_key = %q, want NULL", i, *e.DedupKey)
		}
	}
}
