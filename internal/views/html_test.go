package views

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/sadisticbrew/meridian/internal/model"
	"github.com/sadisticbrew/meridian/internal/store"
)

var htmlSectionIDs = []string{
	`id="chart-minutes"`,
	`id="chart-weekly"`,
	`id="chart-dsa"`,
	`id="chart-german"`,
	`id="table-milestones"`,
}

var htmlForbidden = []string{
	`src="http`, `href="http`, "url(http", "url(//", "<script", "@import", "http://", "https://",
}

func seedHTMLStress(t *testing.T) *store.Store {
	t.Helper()
	st := openViewDB(t)
	subjects := []struct {
		id, kind, name string
	}{
		{"project/meridian", "project", "Meridian"},
		{"course/ddco", "course", "DDCO"},
		{"self-study/ostep", "self-study", "OSTEP"},
		{"language/german", "language", "German"},
	}
	for _, s := range subjects {
		seedThing(t, st, model.Thing{ID: s.id, Kind: s.kind, DisplayName: s.name, Active: true})
	}
	seedThing(t, st, model.Thing{ID: "pattern/monotonic-stack", Kind: "pattern", DisplayName: "Monotonic Stack", Active: true})
	seedThing(t, st, model.Thing{ID: "pattern/stack", Kind: "pattern", DisplayName: "Stack", Active: true})

	first, _, _ := graphWindow(testNow, 30)
	for day := 0; day < 30; day++ {
		dayStart := first.AddDate(0, 0, day)
		for i, s := range subjects {
			start := dayStart.Add(time.Duration(8+i) * time.Hour)
			ts := start.UTC().Format(time.RFC3339)
			mins := 20 + i*15
			subject := s.id
			seedEvent(t, st, model.Event{Ts: ts, Source: "manual", Type: "session",
				Subject: &subject, Payload: jsonRaw(sessionPayload(ts, ts, mins))})
			seedEvent(t, st, model.Event{Ts: ts, Source: "manual", Type: "session",
				Subject: &subject, Payload: jsonRaw(sessionPayload(ts, ts, mins/2))})
		}
		slugTS := dayStart.Add(9 * time.Hour).UTC().Format(time.RFC3339)
		seedPassiveSolve(t, st, "pattern/monotonic-stack", fmt.Sprintf("problem-%02d", day), 1+day%3, slugTS)
	}
	for i := 0; i < 10; i++ {
		sub := "course/ddco"
		score := float64(15 + i)
		ts := first.AddDate(0, 0, i*3).Add(12 * time.Hour).UTC().Format(time.RFC3339)
		seedEvent(t, st, model.Event{Ts: ts, Source: "manual", Type: "milestone",
			Subject: &sub, ValueNum: &score, Payload: jsonRaw(`{"exam":"t1","max":25}`)})
	}
	return st
}

func TestHTMLStructure(t *testing.T) {
	empty := func(t *testing.T) *store.Store { return openViewDB(t) }
	cases := []struct {
		name string
		seed func(*testing.T) *store.Store
		days int
	}{
		{"empty", empty, 30},
		{"single day", empty, 1},
		{"basic", seedBasic, 14},
		{"stress", seedHTMLStress, 30},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := tc.seed(t)
			var buf bytes.Buffer
			if err := HTML(&buf, testNow, st, tc.days); err != nil {
				t.Fatalf("HTML: %v", err)
			}
			out := buf.String()
			prev := -1
			for _, id := range htmlSectionIDs {
				i := strings.Index(out, id)
				if i < 0 {
					t.Fatalf("missing section %s", id)
				}
				if i < prev {
					t.Errorf("section %s out of mandated order", id)
				}
				prev = i
			}
			for _, bad := range htmlForbidden {
				if strings.Contains(out, bad) {
					t.Errorf("output contains forbidden %q", bad)
				}
			}
			if !strings.Contains(out, "first-try: ") {
				t.Error("missing first-try rate line")
			}
			if len(out) >= 300*1024 {
				t.Errorf("output size = %d bytes, want < %d", len(out), 300*1024)
			}
		})
	}
}
