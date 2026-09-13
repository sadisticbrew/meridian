package views

import (
	"bytes"
	"io"
	"testing"
	"time"

	"github.com/sadisticbrew/meridian/internal/model"
	"github.com/sadisticbrew/meridian/internal/store"
)

func TestGraphGolden(t *testing.T) {
	empty := func(t *testing.T) *store.Store { return openViewDB(t) }
	wide := func(t *testing.T) *store.Store {
		st := openViewDB(t)
		seedThing(t, st, model.Thing{ID: "course/ddco", Kind: "course", DisplayName: "DDCO", Active: true})
		ddco := "course/ddco"
		seedEvent(t, st, model.Event{Ts: "2026-09-13T07:00:00Z", Source: "manual", Type: "session",
			Subject: &ddco, Payload: jsonRaw(sessionPayload("2026-09-13T07:00:00Z", "2026-09-13T17:00:00Z", 600))})
		return st
	}

	cases := []struct {
		name   string
		seed   func(*testing.T) *store.Store
		days   int
		render func(io.Writer, time.Time, *store.Store, int) error
		golden string
	}{
		{"minutes basic", seedBasic, 14, GraphMinutes, "graph_minutes.txt"},
		{"minutes empty window", empty, 14, GraphMinutes, "graph_minutes_empty.txt"},
		{"minutes wide value", wide, 14, GraphMinutes, "graph_minutes_wide.txt"},
		{"dsa basic", seedDSA, 14, GraphDSA, "graph_dsa.txt"},
		{"dsa empty window", empty, 14, GraphDSA, "graph_dsa_empty.txt"},
		{"german basic", seedGermanDelta, 14, GraphGerman, "graph_german.txt"},
		{"german seeded before window", seedGermanDelta, 7, GraphGerman, "graph_german_prior.txt"},
		{"german empty window", empty, 14, GraphGerman, "graph_german_empty.txt"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := tc.seed(t)
			var buf bytes.Buffer
			if err := tc.render(&buf, testNow, st, tc.days); err != nil {
				t.Fatalf("render: %v", err)
			}
			goldenCompare(t, tc.golden, buf.String())
		})
	}
}
