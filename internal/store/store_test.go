package store

import (
	"errors"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/sadisticbrew/meridian/internal/model"
)

func openTest(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func mkThing(id, kind, name string) model.Thing {
	return model.Thing{
		ID:           id,
		Kind:         kind,
		DisplayName:  name,
		Active:       true,
		DecisionRule: "rule for " + id,
		CreatedAt:    "2026-09-13T10:00:00Z",
	}
}

func ids(things []model.Thing) []string {
	out := make([]string, 0, len(things))
	for _, t := range things {
		out = append(out, t.ID)
	}
	return out
}

func TestWALMode(t *testing.T) {
	s := openTest(t)
	var mode string
	if err := s.db.QueryRow(`PRAGMA journal_mode`).Scan(&mode); err != nil {
		t.Fatalf("PRAGMA journal_mode: %v", err)
	}
	if mode != "wal" {
		t.Fatalf("journal_mode = %q, want wal", mode)
	}
}

func TestMigrationsIdempotent(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "m.db")

	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("first Open: %v", err)
	}
	var v1 int
	if err := s.db.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_version`).Scan(&v1); err != nil {
		t.Fatalf("read schema_version: %v", err)
	}
	if v1 != 1 {
		t.Fatalf("version = %d, want 1", v1)
	}
	for _, table := range []string{"tracked_things", "events", "state"} {
		var name string
		err := s.db.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&name)
		if err != nil {
			t.Fatalf("table %s missing: %v", table, err)
		}
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	s2, err := Open(dbPath)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer s2.Close()
	var v2 int
	if err := s2.db.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_version`).Scan(&v2); err != nil {
		t.Fatalf("read schema_version after reopen: %v", err)
	}
	if v2 != v1 {
		t.Fatalf("version changed on reopen: %d -> %d", v1, v2)
	}
}

func TestThingRoundTrip(t *testing.T) {
	cases := map[string]model.Thing{
		"full":    {ID: "course/ddco", Kind: "course", DisplayName: "DDCO", Active: true, DecisionRule: "book a block", GoalJSON: `{"weekly_minutes":90}`, CreatedAt: "2026-09-13T10:00:00Z"},
		"no-goal": {ID: "pattern/stack", Kind: "pattern", DisplayName: "Stack", Active: true, DecisionRule: "re-drill", CreatedAt: "2026-09-13T10:00:00Z"},
		"archived": {ID: "habit/instagram", Kind: "habit", DisplayName: "Instagram cycle", Active: false, Archived: true,
			DecisionRule: "note the trigger", CreatedAt: "2026-09-13T10:00:00Z"},
	}
	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			s := openTest(t)
			if err := s.UpsertThing(want); err != nil {
				t.Fatalf("UpsertThing: %v", err)
			}
			got, err := s.Thing(want.ID)
			if err != nil {
				t.Fatalf("Thing: %v", err)
			}
			if !reflect.DeepEqual(*got, want) {
				t.Errorf("round trip mismatch:\n got %+v\nwant %+v", *got, want)
			}
		})
	}
}

func TestThingNotFound(t *testing.T) {
	s := openTest(t)
	_, err := s.Thing("course/nope")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestUpsertOverwrites(t *testing.T) {
	s := openTest(t)
	first := mkThing("course/dav", "course", "DAV")
	if err := s.UpsertThing(first); err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	second := mkThing("course/dav", "course", "Data Analytics & Visualization")
	second.GoalJSON = `{"weekly_minutes":60}`
	if err := s.UpsertThing(second); err != nil {
		t.Fatalf("second upsert: %v", err)
	}
	got, err := s.Thing("course/dav")
	if err != nil {
		t.Fatalf("Thing: %v", err)
	}
	if got.DisplayName != "Data Analytics & Visualization" || got.GoalJSON != `{"weekly_minutes":60}` {
		t.Errorf("REPLACE did not overwrite: %+v", got)
	}
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM tracked_things`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Errorf("rows = %d, want 1 (REPLACE, not duplicate)", n)
	}
}

func TestThingsFilters(t *testing.T) {
	seed := []model.Thing{
		mkThing("course/ddco", "course", "DDCO"),
		mkThing("course/dav", "course", "DAV"),
		mkThing("language/german", "language", "German"),
		{ID: "habit/instagram", Kind: "habit", DisplayName: "Instagram cycle", Active: false, Archived: true,
			DecisionRule: "note the trigger", CreatedAt: "2026-09-13T10:00:00Z"},
	}
	s := openTest(t)
	for _, th := range seed {
		if err := s.UpsertThing(th); err != nil {
			t.Fatalf("seed %s: %v", th.ID, err)
		}
	}

	cases := []struct {
		name   string
		filter ListFilter
		want   []string
	}{
		{"default hides archived", ListFilter{}, []string{"course/dav", "course/ddco", "language/german"}},
		{"archived only", ListFilter{Archived: ArchivedOnly}, []string{"habit/instagram"}},
		{"all", ListFilter{Archived: ArchivedAll}, []string{"course/dav", "course/ddco", "habit/instagram", "language/german"}},
		{"kind filter", ListFilter{Kind: "course"}, []string{"course/dav", "course/ddco"}},
		{"kind + archived only", ListFilter{Kind: "habit", Archived: ArchivedOnly}, []string{"habit/instagram"}},
		{"kind with no match", ListFilter{Kind: "project"}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := s.Things(tc.filter)
			if err != nil {
				t.Fatalf("Things: %v", err)
			}
			if !slices.Equal(ids(got), tc.want) {
				t.Errorf("got %v, want %v", ids(got), tc.want)
			}
		})
	}
}

func TestSubjectsActive(t *testing.T) {
	s := openTest(t)
	seed := []model.Thing{
		mkThing("course/ddco", "course", "DDCO"),
		{ID: "course/dav", Kind: "course", DisplayName: "DAV", Active: false, DecisionRule: "r", CreatedAt: "2026-09-13T10:00:00Z"},
		{ID: "habit/instagram", Kind: "habit", DisplayName: "Instagram cycle", Active: false, Archived: true,
			DecisionRule: "r", CreatedAt: "2026-09-13T10:00:00Z"},
	}
	for _, th := range seed {
		if err := s.UpsertThing(th); err != nil {
			t.Fatalf("seed %s: %v", th.ID, err)
		}
	}
	got, err := s.SubjectsActive()
	if err != nil {
		t.Fatalf("SubjectsActive: %v", err)
	}
	want := []string{"course/ddco"}
	if !reflect.DeepEqual(ids(got), want) {
		t.Errorf("got %v, want %v", ids(got), want)
	}
}

func TestArchiveThing(t *testing.T) {
	cases := []struct {
		name             string
		archived         bool
		wantActive       bool
		wantArchived     bool
		inDefaultListing bool
	}{
		{"archive", true, false, true, false},
		{"restore", false, true, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := openTest(t)
			if err := s.UpsertThing(mkThing("course/ddco", "course", "DDCO")); err != nil {
				t.Fatalf("seed: %v", err)
			}
			if err := s.ArchiveThing("course/ddco", tc.archived); err != nil {
				t.Fatalf("ArchiveThing: %v", err)
			}
			got, err := s.Thing("course/ddco")
			if err != nil {
				t.Fatalf("Thing: %v", err)
			}
			if got.Active != tc.wantActive || got.Archived != tc.wantArchived {
				t.Errorf("active=%v archived=%v, want active=%v archived=%v", got.Active, got.Archived, tc.wantActive, tc.wantArchived)
			}
			list, err := s.Things(ListFilter{})
			if err != nil {
				t.Fatalf("Things: %v", err)
			}
			gotInList := len(list) == 1 && list[0].ID == "course/ddco"
			if gotInList != tc.inDefaultListing {
				t.Errorf("in default listing = %v, want %v", gotInList, tc.inDefaultListing)
			}
		})
	}
}

func TestArchiveThingNotFound(t *testing.T) {
	s := openTest(t)
	err := s.ArchiveThing("course/nope", true)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestUpsertPreservesCreatedAt(t *testing.T) {
	s := openTest(t)
	if err := s.UpsertThing(mkThing("course/ddco", "course", "DDCO")); err != nil {
		t.Fatalf("seed: %v", err)
	}
	existing, err := s.Thing("course/ddco")
	if err != nil {
		t.Fatalf("Thing: %v", err)
	}
	existing.DisplayName = "Digital Design & Computer Organization"
	if err := s.UpsertThing(*existing); err != nil {
		t.Fatalf("read-modify-write upsert: %v", err)
	}
	got, err := s.Thing("course/ddco")
	if err != nil {
		t.Fatalf("Thing: %v", err)
	}
	if got.CreatedAt != "2026-09-13T10:00:00Z" {
		t.Errorf("created_at = %q, want original", got.CreatedAt)
	}
	if got.DisplayName != "Digital Design & Computer Organization" {
		t.Errorf("display_name = %q, want updated", got.DisplayName)
	}
}
