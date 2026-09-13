package data

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/sadisticbrew/meridian/internal/model"
	"github.com/sadisticbrew/meridian/internal/seed"
	"github.com/sadisticbrew/meridian/internal/store"
)

func TestPatternsHas150Keys(t *testing.T) {
	m, err := Patterns()
	if err != nil {
		t.Fatalf("Patterns: %v", err)
	}
	if len(m) != 150 {
		t.Fatalf("len(Patterns()) = %d, want 150", len(m))
	}
}

func TestEmbeddedJSONHasNoDuplicateSlugs(t *testing.T) {
	dec := json.NewDecoder(bytes.NewReader(patternsJSON))
	open, err := dec.Token()
	if err != nil {
		t.Fatalf("first token: %v", err)
	}
	if open != json.Delim('{') {
		t.Fatalf("first token = %v, want {", open)
	}
	var slugs []string
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			t.Fatalf("key token: %v", err)
		}
		key, ok := keyTok.(string)
		if !ok {
			t.Fatalf("key token = %T, want string", keyTok)
		}
		slugs = append(slugs, key)
		var value string
		if err := dec.Decode(&value); err != nil {
			t.Fatalf("value for %q: %v", key, err)
		}
	}
	closeTok, err := dec.Token()
	if err != nil {
		t.Fatalf("closing token: %v", err)
	}
	if closeTok != json.Delim('}') {
		t.Fatalf("closing token = %v, want }", closeTok)
	}
	if len(slugs) != 150 {
		t.Fatalf("embedded object has %d keys, want 150", len(slugs))
	}
	seen := make(map[string]bool, len(slugs))
	for _, s := range slugs {
		if seen[s] {
			t.Errorf("duplicate slug %q in embedded JSON", s)
		}
		seen[s] = true
	}
}

func TestValidatePatternMapAgainstSeedRegistry(t *testing.T) {
	m, err := Patterns()
	if err != nil {
		t.Fatalf("Patterns: %v", err)
	}
	st := openSeedStore(t)
	if err := validatePatternMap(m, st); err != nil {
		t.Fatalf("validatePatternMap: %v", err)
	}
}

func TestValidatePatternMapRejectsBadValues(t *testing.T) {
	cases := []struct {
		name string
		m    map[string]string
	}{
		{"missing pattern id", map[string]string{"some-slug": "pattern/does-not-exist"}},
		{"wrong kind", map[string]string{"some-slug": "course/ddco"}},
		{"empty value", map[string]string{"some-slug": ""}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := openSeedStore(t)
			if err := validatePatternMap(tc.m, st); err == nil {
				t.Fatal("validatePatternMap accepted a bad value")
			}
		})
	}
}

func openSeedStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	created := time.Now().UTC().Format(time.RFC3339)
	rows := seed.Rows(created)
	// Owner-added patterns kept in sync with the map; deliberately not in the phase-0 seed.
	const patternRule = "reviews ≥ solves this week → re-drill that pattern's trigger cards"
	rows = append(rows,
		model.Thing{ID: "pattern/graphs", Kind: "pattern", DisplayName: "Graphs", Active: true, DecisionRule: patternRule, CreatedAt: created},
		model.Thing{ID: "pattern/advanced-graphs", Kind: "pattern", DisplayName: "Advanced Graphs", Active: true, DecisionRule: patternRule, CreatedAt: created},
		model.Thing{ID: "pattern/math-geometry", Kind: "pattern", DisplayName: "Math & Geometry", Active: true, DecisionRule: patternRule, CreatedAt: created},
		model.Thing{ID: "pattern/bit-manipulation", Kind: "pattern", DisplayName: "Bit Manipulation", Active: true, DecisionRule: patternRule, CreatedAt: created},
	)
	for _, th := range rows {
		if err := st.UpsertThing(th); err != nil {
			t.Fatalf("UpsertThing %s: %v", th.ID, err)
		}
	}
	return st
}

func validatePatternMap(m map[string]string, st *store.Store) error {
	slugs := make([]string, 0, len(m))
	for slug := range m {
		slugs = append(slugs, slug)
	}
	sort.Strings(slugs)
	var errs []error
	for _, slug := range slugs {
		id := m[slug]
		thing, err := st.Thing(id)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %q: %w", slug, id, err))
			continue
		}
		if thing.Kind != "pattern" {
			errs = append(errs, fmt.Errorf("%s: %q has kind %q, want pattern", slug, id, thing.Kind))
		}
	}
	return errors.Join(errs...)
}
