package normalizer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/sadisticbrew/meridian/internal/model"
	"github.com/sadisticbrew/meridian/internal/store"
)

const noteTs = "2026-09-10T10:00:00Z"

func ptr[T any](v T) *T { return &v }

// fakeProvider replays scripted responses in call order; the last response
// repeats, and an empty script yields the empty annotation.
type fakeProvider struct {
	name      string
	responses []string
	err       error
	prompts   []string
	i         int
}

func (f *fakeProvider) Name() string {
	if f.name == "" {
		return "fake"
	}
	return f.name
}

func (f *fakeProvider) Complete(_ context.Context, prompt string) (string, error) {
	f.prompts = append(f.prompts, prompt)
	if f.err != nil {
		return "", f.err
	}
	if len(f.responses) == 0 {
		return `{"tags":{},"fields":{}}`, nil
	}
	if f.i >= len(f.responses) {
		return f.responses[len(f.responses)-1], nil
	}
	r := f.responses[f.i]
	f.i++
	return r, nil
}

func openTest(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "normalizer.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func seedRegistry(t *testing.T, s *store.Store) {
	t.Helper()
	things := []model.Thing{
		{ID: "course/ddco", Kind: "course", DisplayName: "DDCO", Active: true, CreatedAt: "2026-09-01T00:00:00Z"},
		{ID: "language/german", Kind: "language", DisplayName: "German", Active: true, CreatedAt: "2026-09-01T00:00:00Z"},
		{ID: "pattern/monotonic-stack", Kind: "pattern", DisplayName: "Monotonic Stack", Active: true, CreatedAt: "2026-09-01T00:00:00Z"},
		{ID: "habit/instagram", Kind: "habit", DisplayName: "Instagram", Archived: true, CreatedAt: "2026-09-01T00:00:00Z"},
	}
	for _, thing := range things {
		if err := s.UpsertThing(thing); err != nil {
			t.Fatalf("UpsertThing(%s): %v", thing.ID, err)
		}
	}
}

func addNote(t *testing.T, s *store.Store, text string) int64 {
	t.Helper()
	raw := text
	id, err := s.AddEvent(model.Event{
		Ts:        noteTs,
		Source:    "manual",
		Type:      "note",
		RawText:   &raw,
		CreatedAt: "2026-09-10T10:00:01Z",
	})
	if err != nil {
		t.Fatalf("AddEvent(note): %v", err)
	}
	return id
}

func runQuantify(t *testing.T, s *store.Store, prov Provider, opts Options) string {
	t.Helper()
	var out strings.Builder
	if err := Run(context.Background(), &out, s, prov, opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	return out.String()
}

func noteByID(t *testing.T, s *store.Store, id int64) model.Event {
	t.Helper()
	notes, err := s.Notes(false)
	if err != nil {
		t.Fatalf("Notes: %v", err)
	}
	for _, n := range notes {
		if n.ID == id {
			return n
		}
	}
	t.Fatalf("note %d not found", id)
	return model.Event{}
}

func derivedEvents(t *testing.T, s *store.Store) []model.Event {
	t.Helper()
	evs, err := s.Events(store.EventFilter{})
	if err != nil {
		t.Fatalf("Events: %v", err)
	}
	var out []model.Event
	for _, e := range evs {
		if e.Source == "quantify" {
			out = append(out, e)
		}
	}
	return out
}

func TestExtractJSON(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
		ok   bool
	}{
		{"plain object", `{"a":1}`, `{"a":1}`, true},
		{"prose and fences", "Here you go:\n```json\n{\"a\":1}\n```", `{"a":1}`, true},
		{"first to last brace", "prefix {x} mid {y} suffix", `{x} mid {y}`, true},
		{"trailing prose", `answer: {"a":1} — done`, `{"a":1}`, true},
		{"no braces", "no json here", "", false},
		{"close before open", "} {", "", false},
		{"open only", `{"a":1`, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := ExtractJSON(tc.in)
			if ok != tc.ok || got != tc.want {
				t.Errorf("ExtractJSON(%q) = %q, %v; want %q, %v", tc.in, got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestQuantifyHappyPath(t *testing.T) {
	s := openTest(t)
	seedRegistry(t, s)
	const raw = "studied DDCO for 45 minutes"
	id := addNote(t, s, raw)
	prov := &fakeProvider{responses: []string{`{"tags":{"subject":"course/ddco"},"fields":{"minutes":45}}`}}

	out := runQuantify(t, s, prov, Options{OnlyUnquantified: true})
	if !strings.Contains(out, fmt.Sprintf("note #%d → subject:course/ddco, minutes:45", id)) {
		t.Errorf("output = %q, want accepted note line", out)
	}
	if len(prov.prompts) != 1 {
		t.Fatalf("provider calls = %d, want 1", len(prov.prompts))
	}
	prompt := prov.prompts[0]
	for _, want := range []string{
		"You are a data-extraction engine",
		"course/ddco",
		"pattern/monotonic-stack",
		raw,
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt missing %q", want)
		}
	}
	if strings.Contains(prompt, "<VOCAB>") {
		t.Errorf("prompt still contains <VOCAB>")
	}

	n := noteByID(t, s, id)
	if n.Quantified != 1 {
		t.Errorf("quantified = %d, want 1", n.Quantified)
	}
	if got, want := string(n.Payload), `{"tags":{"subject":"course/ddco"},"fields":{"minutes":45}}`; got != want {
		t.Errorf("payload = %s, want %s", got, want)
	}
	if n.RawText == nil || *n.RawText != raw {
		t.Errorf("raw_text = %v, want unchanged %q", n.RawText, raw)
	}

	ev, err := s.EventByDedup(fmt.Sprintf("quantify/%d", id))
	if err != nil {
		t.Fatalf("EventByDedup: %v", err)
	}
	if ev.Source != "quantify" || ev.Type != "session" || ev.Subject == nil || *ev.Subject != "course/ddco" {
		t.Errorf("derived event = %+v, want quantify session on course/ddco", ev)
	}
	if ev.Ts != noteTs {
		t.Errorf("derived ts = %q, want %q", ev.Ts, noteTs)
	}
	var payload struct {
		StartedAt string   `json:"started_at"`
		EndedAt   string   `json:"ended_at"`
		Minutes   int      `json:"minutes"`
		Tags      []string `json:"tags"`
		Manual    bool     `json:"manual"`
	}
	if err := json.Unmarshal(ev.Payload, &payload); err != nil {
		t.Fatalf("derived payload: %v", err)
	}
	if payload.StartedAt != noteTs || payload.EndedAt != "2026-09-10T10:45:00Z" || payload.Minutes != 45 || !payload.Manual {
		t.Errorf("session payload = %+v", payload)
	}
}

func TestQuantifyExtractsJSONFromProse(t *testing.T) {
	s := openTest(t)
	seedRegistry(t, s)
	id := addNote(t, s, "studied DDCO for 45 minutes")
	prov := &fakeProvider{responses: []string{
		"Here you go:\n```json\n{\"tags\": {\"subject\": \"course/ddco\"}, \"fields\": {\"minutes\": 45}}\n```",
	}}

	out := runQuantify(t, s, prov, Options{OnlyUnquantified: true})
	if !strings.Contains(out, fmt.Sprintf("note #%d → subject:course/ddco, minutes:45", id)) {
		t.Errorf("output = %q, want accepted note line", out)
	}
	if n := noteByID(t, s, id); n.Quantified != 1 {
		t.Errorf("quantified = %d, want 1", n.Quantified)
	}
	if got := derivedEvents(t, s); len(got) != 1 {
		t.Errorf("derived events = %d, want 1", len(got))
	}
}

func TestQuantifyRejectsInvalidOutput(t *testing.T) {
	cases := []struct {
		name  string
		reply string
	}{
		{"unknown subject", `{"tags":{"subject":"not-a-thing"},"fields":{}}`},
		{"unknown tag key", `{"tags":{"mood":"happy"},"fields":{}}`},
		{"pattern value not a pattern", `{"tags":{"pattern":"course/ddco"},"fields":{}}`},
		{"unknown field key", `{"tags":{},"fields":{"mood":"happy"}}`},
		{"bad outcome", `{"tags":{},"fields":{"outcome":"crushed"}}`},
		{"bad difficulty", `{"tags":{},"fields":{"difficulty":"insane"}}`},
		{"bad exam", `{"tags":{},"fields":{"exam":"final"}}`},
		{"minutes wrong type", `{"tags":{},"fields":{"minutes":"45"}}`},
		{"problem wrong type", `{"tags":{},"fields":{"problem":7}}`},
		{"not json", `sorry, no JSON today`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := openTest(t)
			seedRegistry(t, s)
			id := addNote(t, s, "some note")
			prov := &fakeProvider{responses: []string{tc.reply}}

			out := runQuantify(t, s, prov, Options{OnlyUnquantified: true})
			if !strings.Contains(out, fmt.Sprintf("note #%d rejected:", id)) {
				t.Errorf("output = %q, want rejection warning", out)
			}
			n := noteByID(t, s, id)
			if n.Quantified != 0 {
				t.Errorf("quantified = %d, want 0", n.Quantified)
			}
			if len(n.Payload) != 0 {
				t.Errorf("payload = %s, want empty", n.Payload)
			}
			if got := derivedEvents(t, s); len(got) != 0 {
				t.Errorf("derived events = %d, want 0", len(got))
			}
		})
	}
}

func TestQuantifyProviderErrorKeepsNotePending(t *testing.T) {
	s := openTest(t)
	seedRegistry(t, s)
	id := addNote(t, s, "some note")
	prov := &fakeProvider{err: errors.New("model offline")}

	out := runQuantify(t, s, prov, Options{OnlyUnquantified: true})
	if !strings.Contains(out, fmt.Sprintf("note #%d rejected: model offline", id)) {
		t.Errorf("output = %q, want provider-error warning", out)
	}
	if n := noteByID(t, s, id); n.Quantified != 0 {
		t.Errorf("quantified = %d, want 0", n.Quantified)
	}
	if got := derivedEvents(t, s); len(got) != 0 {
		t.Errorf("derived events = %d, want 0", len(got))
	}
}

func TestMaterialization(t *testing.T) {
	cases := []struct {
		name        string
		reply       string
		wantType    string
		wantSubject string
		wantValue   *float64
		wantPayload string
	}{
		{
			name:        "subject plus minutes to session",
			reply:       `{"tags":{"subject":"course/ddco"},"fields":{"minutes":30}}`,
			wantType:    "session",
			wantSubject: "course/ddco",
			wantPayload: `{"started_at":"2026-09-10T10:00:00Z","ended_at":"2026-09-10T10:30:00Z","minutes":30,"tags":[],"manual":true}`,
		},
		{
			name:        "lesson to german occurrence",
			reply:       `{"tags":{},"fields":{"lesson":44}}`,
			wantType:    "occurrence",
			wantSubject: "language/german",
			wantValue:   ptr(44.0),
		},
		{
			name:        "pattern problem outcome to occurrence",
			reply:       `{"tags":{"pattern":"pattern/monotonic-stack"},"fields":{"problem":"car-fleet","outcome":"reviewed"}}`,
			wantType:    "occurrence",
			wantSubject: "pattern/monotonic-stack",
			wantPayload: `{"problem":"car-fleet","outcome":"reviewed"}`,
		},
		{
			name:        "subject exam score to milestone",
			reply:       `{"tags":{"subject":"course/ddco"},"fields":{"exam":"t1","score":22}}`,
			wantType:    "milestone",
			wantSubject: "course/ddco",
			wantValue:   ptr(22.0),
			wantPayload: `{"exam":"t1"}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := openTest(t)
			seedRegistry(t, s)
			id := addNote(t, s, "note")
			prov := &fakeProvider{responses: []string{tc.reply}}

			runQuantify(t, s, prov, Options{OnlyUnquantified: true})
			ev, err := s.EventByDedup(fmt.Sprintf("quantify/%d", id))
			if err != nil {
				t.Fatalf("EventByDedup: %v", err)
			}
			if ev.Type != tc.wantType || ev.Ts != noteTs {
				t.Errorf("event type/ts = %s/%s, want %s/%s", ev.Type, ev.Ts, tc.wantType, noteTs)
			}
			if ev.Subject == nil || *ev.Subject != tc.wantSubject {
				t.Errorf("subject = %v, want %s", ev.Subject, tc.wantSubject)
			}
			if tc.wantValue != nil {
				if ev.ValueNum == nil || *ev.ValueNum != *tc.wantValue {
					t.Errorf("value_num = %v, want %v", ev.ValueNum, *tc.wantValue)
				}
			} else if ev.ValueNum != nil {
				t.Errorf("value_num = %v, want nil", *ev.ValueNum)
			}
			if tc.wantPayload != "" && string(ev.Payload) != tc.wantPayload {
				t.Errorf("payload = %s, want %s", ev.Payload, tc.wantPayload)
			}
		})
	}
}

func TestRedoIdempotent(t *testing.T) {
	s := openTest(t)
	seedRegistry(t, s)
	id := addNote(t, s, "lesson 44 with 30 minutes of German")
	reply := `{"tags":{"subject":"language/german"},"fields":{"lesson":44,"minutes":30}}`
	prov := &fakeProvider{responses: []string{reply}}

	snapshot := func() []model.Event {
		evs := derivedEvents(t, s)
		out := make([]model.Event, len(evs))
		for i, e := range evs {
			e.ID = 0
			e.CreatedAt = ""
			out[i] = e
		}
		return out
	}

	runQuantify(t, s, prov, Options{OnlyUnquantified: true})
	first := snapshot()
	if len(first) != 2 {
		t.Fatalf("first run derived %d events, want 2 (session + lesson)", len(first))
	}
	n := noteByID(t, s, id)
	rawBefore := *n.RawText
	payloadBefore := string(n.Payload)

	runQuantify(t, s, prov, Options{Redo: true})
	second := snapshot()
	runQuantify(t, s, prov, Options{Redo: true})
	third := snapshot()

	if len(second) != len(first) || len(third) != len(first) {
		t.Fatalf("event counts = %d/%d/%d, want %d each", len(first), len(second), len(third), len(first))
	}
	if !reflect.DeepEqual(first, second) || !reflect.DeepEqual(first, third) {
		t.Errorf("derived events changed across redo:\nfirst  %+v\nsecond %+v\nthird  %+v", first, second, third)
	}
	n = noteByID(t, s, id)
	if n.RawText == nil || *n.RawText != rawBefore {
		t.Errorf("raw_text = %v, want unchanged %q", n.RawText, rawBefore)
	}
	if got := string(n.Payload); got != payloadBefore {
		t.Errorf("note payload = %s, want unchanged %s", got, payloadBefore)
	}
}

func TestDryRunWritesNothing(t *testing.T) {
	s := openTest(t)
	seedRegistry(t, s)
	id := addNote(t, s, "studied DDCO for 45 minutes")
	prov := &fakeProvider{responses: []string{`{"tags":{"subject":"course/ddco"},"fields":{"minutes":45}}`}}

	out := runQuantify(t, s, prov, Options{OnlyUnquantified: true, DryRun: true})
	if !strings.Contains(out, fmt.Sprintf("note #%d → subject:course/ddco, minutes:45", id)) {
		t.Errorf("output = %q, want accepted note line", out)
	}
	n := noteByID(t, s, id)
	if n.Quantified != 0 || len(n.Payload) != 0 {
		t.Errorf("note mutated by dry run: quantified=%d payload=%s", n.Quantified, n.Payload)
	}
	all, err := s.Events(store.EventFilter{})
	if err != nil {
		t.Fatalf("Events: %v", err)
	}
	if len(all) != 1 {
		t.Errorf("events = %d, want only the note", len(all))
	}
}

func TestEmptyAnnotationAccepted(t *testing.T) {
	s := openTest(t)
	seedRegistry(t, s)
	id := addNote(t, s, "just a thought")
	prov := &fakeProvider{responses: []string{`{"tags":{},"fields":{}}`}}

	out := runQuantify(t, s, prov, Options{OnlyUnquantified: true})
	if !strings.Contains(out, fmt.Sprintf("note #%d →", id)) {
		t.Errorf("output = %q, want accepted note line", out)
	}
	n := noteByID(t, s, id)
	if n.Quantified != 1 {
		t.Errorf("quantified = %d, want 1", n.Quantified)
	}
	if got, want := string(n.Payload), `{"tags":{},"fields":{}}`; got != want {
		t.Errorf("payload = %s, want %s", got, want)
	}
	if got := derivedEvents(t, s); len(got) != 0 {
		t.Errorf("derived events = %d, want 0", len(got))
	}
}

func TestNoPendingNotes(t *testing.T) {
	s := openTest(t)
	seedRegistry(t, s)
	prov := &fakeProvider{}

	out := runQuantify(t, s, prov, Options{OnlyUnquantified: true})
	if out != "no pending notes.\n" {
		t.Errorf("output = %q, want %q", out, "no pending notes.\n")
	}
	if len(prov.prompts) != 0 {
		t.Errorf("provider calls = %d, want 0", len(prov.prompts))
	}
}
