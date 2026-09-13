package cli

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sadisticbrew/meridian/internal/model"
	"github.com/sadisticbrew/meridian/internal/store"
)

func testStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func mustUpsert(t *testing.T, st *store.Store, things ...model.Thing) {
	t.Helper()
	for _, th := range things {
		if err := st.UpsertThing(th); err != nil {
			t.Fatalf("upsert %s: %v", th.ID, err)
		}
	}
}

func TestResolveThing(t *testing.T) {
	st := testStore(t)
	mustUpsert(t, st,
		model.Thing{ID: "course/ddco", Kind: "course", DisplayName: "DDCO", Active: true, CreatedAt: "2026-01-01T00:00:00Z"},
		model.Thing{ID: "pattern/stack", Kind: "pattern", DisplayName: "Stack", Active: true, CreatedAt: "2026-01-01T00:00:00Z"},
		model.Thing{ID: "pattern/monotonic-stack", Kind: "pattern", DisplayName: "Monotonic Stack", Active: true, CreatedAt: "2026-01-01T00:00:00Z"},
		model.Thing{ID: "language/german", Kind: "language", DisplayName: "German", Active: true, CreatedAt: "2026-01-01T00:00:00Z"},
	)

	t.Run("exact id", func(t *testing.T) {
		got, err := resolveThing(st, "course/ddco")
		if err != nil || got.ID != "course/ddco" {
			t.Fatalf("got %v, %v", got, err)
		}
	})
	t.Run("unique suffix", func(t *testing.T) {
		got, err := resolveThing(st, "ddco")
		if err != nil || got.ID != "course/ddco" {
			t.Fatalf("got %v, %v", got, err)
		}
	})
	t.Run("ambiguous suffix", func(t *testing.T) {
		_, err := resolveThing(st, "stack")
		var ae *AmbiguousError
		if !errors.As(err, &ae) {
			t.Fatalf("want AmbiguousError, got %v", err)
		}
		want := map[string]bool{"pattern/stack": true, "pattern/monotonic-stack": true}
		if len(ae.Candidates) != 2 || !want[ae.Candidates[0]] || !want[ae.Candidates[1]] {
			t.Fatalf("candidates = %v", ae.Candidates)
		}
		for _, id := range ae.Candidates {
			if !strings.Contains(ae.Error(), id) {
				t.Errorf("Error() must list candidate %s: %q", id, ae.Error())
			}
		}
	})
	t.Run("unknown", func(t *testing.T) {
		_, err := resolveThing(st, "nope")
		if err == nil {
			t.Fatal("want error")
		}
		var ae *AmbiguousError
		if errors.As(err, &ae) {
			t.Fatalf("plain error expected, got AmbiguousError: %v", err)
		}
	})
}
