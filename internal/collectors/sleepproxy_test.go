package collectors

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/sadisticbrew/meridian/internal/store"
)

// Fixture journal lines (journalctl -o short-iso shape), never a live journal.
const (
	suspendLine1 = "2026-09-13T23:00:00+0530 cachy-brew systemd-sleep[29532]: Performing sleep operation 'suspend'..."
	resumeLine1  = "2026-09-14T06:30:00+0530 cachy-brew systemd-sleep[29533]: System returned from sleep operation 'suspend'."
	suspendLine2 = "2026-09-14T08:30:00Z cachy-brew systemd-sleep[29600]: Performing sleep operation 'suspend'..."
	resumeLine2  = "2026-09-14T09:45:00Z cachy-brew systemd-sleep[29601]: System returned from sleep operation 'suspend'."
)

func fixtureSleepProxy(lines []string) *SleepProxy {
	return &SleepProxy{Lines: func(context.Context) ([]string, error) { return lines, nil }}
}

func TestSleepProxySyncWritesSamples(t *testing.T) {
	s := openTestStore(t)
	sp := fixtureSleepProxy([]string{suspendLine1, resumeLine1, suspendLine2, resumeLine2})

	res, err := sp.Sync(context.Background(), s)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if res.New != 2 || res.Updated != 0 {
		t.Fatalf("result = %+v, want New:2 Updated:0", res)
	}
	if res.Details != "2 window(s), 0 skipped" {
		t.Errorf("details = %q, want %q", res.Details, "2 window(s), 0 skipped")
	}

	cases := []struct {
		dedup     string
		ts        string
		hours     float64
		suspended string
		resumed   string
	}{
		{
			dedup: "sleep-proxy/2026-09-13T17:30:00Z", ts: "2026-09-14T01:00:00Z", hours: 7.5,
			suspended: "2026-09-13T17:30:00Z", resumed: "2026-09-14T01:00:00Z",
		},
		{
			dedup: "sleep-proxy/2026-09-14T08:30:00Z", ts: "2026-09-14T09:45:00Z", hours: 1.25,
			suspended: "2026-09-14T08:30:00Z", resumed: "2026-09-14T09:45:00Z",
		},
	}
	for _, tc := range cases {
		ev, err := s.EventByDedup(tc.dedup)
		if err != nil {
			t.Fatalf("EventByDedup(%s): %v", tc.dedup, err)
		}
		if ev.Ts != tc.ts {
			t.Errorf("%s ts = %q, want %q", tc.dedup, ev.Ts, tc.ts)
		}
		if ev.Source != "sleep-proxy" || ev.Type != "sample" {
			t.Errorf("%s source/type = %q/%q, want sleep-proxy/sample", tc.dedup, ev.Source, ev.Type)
		}
		if ev.Subject == nil || *ev.Subject != "sleep/proxy" {
			t.Errorf("%s subject = %v, want sleep/proxy", tc.dedup, ev.Subject)
		}
		if ev.ValueNum == nil {
			t.Errorf("%s value_num is nil, want %v", tc.dedup, tc.hours)
		} else if *ev.ValueNum != tc.hours {
			t.Errorf("%s value_num = %v, want %v", tc.dedup, *ev.ValueNum, tc.hours)
		}
		if ev.DedupKey == nil || *ev.DedupKey != tc.dedup {
			t.Errorf("%s dedup_key = %v, want %s", tc.dedup, ev.DedupKey, tc.dedup)
		}
		var p struct {
			SuspendedAt string  `json:"suspended_at"`
			ResumedAt   string  `json:"resumed_at"`
			Hours       float64 `json:"hours"`
		}
		if err := json.Unmarshal(ev.Payload, &p); err != nil {
			t.Fatalf("%s decode payload %s: %v", tc.dedup, ev.Payload, err)
		}
		if p.SuspendedAt != tc.suspended || p.ResumedAt != tc.resumed || p.Hours != tc.hours {
			t.Errorf("%s payload = %+v, want suspended %s resumed %s hours %v",
				tc.dedup, p, tc.suspended, tc.resumed, tc.hours)
		}
	}
}

func TestSleepProxySyncIdempotent(t *testing.T) {
	s := openTestStore(t)
	sp := fixtureSleepProxy([]string{suspendLine1, resumeLine1, suspendLine2, resumeLine2})

	if _, err := sp.Sync(context.Background(), s); err != nil {
		t.Fatalf("first Sync: %v", err)
	}
	res, err := sp.Sync(context.Background(), s)
	if err != nil {
		t.Fatalf("second Sync: %v", err)
	}
	if res.New != 0 || res.Updated != 0 {
		t.Fatalf("second result = %+v, want New:0 Updated:0", res)
	}
	all, err := s.Events(store.EventFilter{Type: "sample"})
	if err != nil {
		t.Fatalf("Events: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("sample events = %d, want 2 (no duplicates)", len(all))
	}
}

func TestSleepProxySyncSkipsFragments(t *testing.T) {
	cases := []struct {
		name        string
		lines       []string
		wantNew     int
		wantSkipped int
	}{
		{
			name:        "unclosed final suspend",
			lines:       []string{suspendLine1, resumeLine1, suspendLine2},
			wantNew:     1,
			wantSkipped: 1,
		},
		{
			name:        "resume without suspend",
			lines:       []string{resumeLine1, suspendLine1, resumeLine1},
			wantNew:     1,
			wantSkipped: 1,
		},
		{
			name:        "no marker lines",
			lines:       []string{"-- Journal begins at Sun 2026-09-13 --", "2026-09-13T21:14:31+0530 cachy-brew kernel: unrelated"},
			wantNew:     0,
			wantSkipped: 0,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := openTestStore(t)
			sp := fixtureSleepProxy(tc.lines)
			res, err := sp.Sync(context.Background(), s)
			if err != nil {
				t.Fatalf("Sync: %v", err)
			}
			if res.New != tc.wantNew {
				t.Errorf("New = %d, want %d", res.New, tc.wantNew)
			}
			wantDetails := fmt.Sprintf("%d window(s), %d skipped", tc.wantNew, tc.wantSkipped)
			if res.Details != wantDetails {
				t.Errorf("details = %q, want %q", res.Details, wantDetails)
			}
			all, err := s.Events(store.EventFilter{Type: "sample"})
			if err != nil {
				t.Fatalf("Events: %v", err)
			}
			if len(all) != tc.wantNew {
				t.Errorf("sample events = %d, want %d", len(all), tc.wantNew)
			}
		})
	}
}

func TestSleepProxySyncSelfHealsThing(t *testing.T) {
	s := openTestStore(t)
	if _, err := s.Thing("sleep/proxy"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("precondition: Thing err = %v, want ErrNotFound", err)
	}
	sp := fixtureSleepProxy(nil)
	if _, err := sp.Sync(context.Background(), s); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	th, err := s.Thing("sleep/proxy")
	if err != nil {
		t.Fatalf("Thing: %v", err)
	}
	if !th.Archived || th.Active {
		t.Errorf("archived/active = %v/%v, want true/false", th.Archived, th.Active)
	}
	if th.Kind != "habit" {
		t.Errorf("kind = %q, want habit", th.Kind)
	}
	if th.DecisionRule != "" || th.GoalJSON != "" {
		t.Errorf("decision_rule/goal = %q/%q, want empty", th.DecisionRule, th.GoalJSON)
	}

	if _, err := sp.Sync(context.Background(), s); err != nil {
		t.Fatalf("second Sync: %v", err)
	}
	again, err := s.Thing("sleep/proxy")
	if err != nil {
		t.Fatalf("Thing after second Sync: %v", err)
	}
	if again.CreatedAt != th.CreatedAt {
		t.Errorf("created_at = %q, want unchanged %q", again.CreatedAt, th.CreatedAt)
	}
}

func TestSleepProxySyncJournalErrorUnavailable(t *testing.T) {
	s := openTestStore(t)
	sp := &SleepProxy{Lines: func(context.Context) ([]string, error) {
		return nil, errors.New("journalctl: not found")
	}}
	_, err := sp.Sync(context.Background(), s)
	if err == nil {
		t.Fatal("Sync: want error")
	}
	var ue *UnavailableError
	if !errors.As(err, &ue) {
		t.Fatalf("err = %v (%T), want UnavailableError", err, err)
	}
}

func TestParseJournal(t *testing.T) {
	cases := []struct {
		name        string
		lines       []string
		wantWindows []string
		wantSkipped int
	}{
		{
			name:        "complete pair",
			lines:       []string{suspendLine1, resumeLine1},
			wantWindows: []string{"2026-09-13T17:30:00Z..2026-09-14T01:00:00Z"},
		},
		{
			name:        "z-form timestamps",
			lines:       []string{suspendLine2, resumeLine2},
			wantWindows: []string{"2026-09-14T08:30:00Z..2026-09-14T09:45:00Z"},
		},
		{
			name:        "unclosed final suspend",
			lines:       []string{suspendLine1},
			wantSkipped: 1,
		},
		{
			name:        "resume without suspend",
			lines:       []string{resumeLine1},
			wantSkipped: 1,
		},
		{
			name:  "no marker lines ignored",
			lines: []string{"-- Journal begins at Sun 2026-09-13 --", "2026-09-13T21:14:31+0530 cachy-brew kernel: unrelated"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			windows, skipped := parseJournal(tc.lines)
			var got []string
			for _, w := range windows {
				got = append(got, w.suspended.UTC().Format(time.RFC3339)+".."+w.resumed.UTC().Format(time.RFC3339))
			}
			if !reflect.DeepEqual(got, tc.wantWindows) {
				t.Errorf("windows = %v, want %v", got, tc.wantWindows)
			}
			if skipped != tc.wantSkipped {
				t.Errorf("skipped = %d, want %d", skipped, tc.wantSkipped)
			}
		})
	}
}

func TestSleepProxyRegistry(t *testing.T) {
	c, ok := Registry()["sleep-proxy"]
	if !ok {
		t.Fatal("registry missing sleep-proxy")
	}
	if c.Name() != "sleep-proxy" {
		t.Errorf("Name() = %q, want sleep-proxy", c.Name())
	}
}
