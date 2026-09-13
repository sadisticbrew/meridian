package cli

import (
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sadisticbrew/meridian/internal/model"
	"github.com/sadisticbrew/meridian/internal/store"
)

func runQuiet(args ...string) int {
	return executeIO(io.Discard, io.Discard, args)
}

func runCapture(args ...string) (string, string, int) {
	var out, errW strings.Builder
	code := executeIO(&out, &errW, args)
	return out.String(), errW.String(), code
}

type wantRow struct {
	id, kind, name, goal, rule string
	active, archived           bool
}

var seedWant = []wantRow{
	{"course/ddco", "course", "Digital Design & Computer Organization", `{"weekly_minutes":90}`, "0 study hours this week → book one evening block before the next test", true, false},
	{"course/dav", "course", "Data Analytics & Visualization", `{"weekly_minutes":60}`, "0 study hours this week → book one evening block before the next test", true, false},
	{"course/oop-java", "course", "Object Oriented Programming with Java", `{"weekly_minutes":60}`, "0 study hours this week → book one evening block before the next test", true, false},
	{"course/cdd", "course", "Collaborative Development & DevOps", `{"weekly_minutes":30}`, "home-turf subject — don't let it cannibalize DDCO evenings", true, false},
	{"self-study/ostep", "self-study", "OSTEP (audio study)", `{"weekly_minutes":90}`, "book one audio-study block this week", true, false},
	{"self-study/rust", "self-study", "Rust", `{"weekly_minutes":60}`, "pair practice with a -rs port; if it stalls two weeks, cut it", true, false},
	{"language/german", "language", "German (Nicos Weg)", `{"weekly_minutes":90,"occurrences_per_week":3}`, "0 min this week → 15 min Nicos Weg tomorrow; B1 cuts PR 27→21 months", true, false},
	{"pattern/arrays-hashing", "pattern", "Arrays & Hashing", "", "reviews ≥ solves this week → re-drill that pattern's trigger cards", true, false},
	{"pattern/two-pointer", "pattern", "Two Pointer", "", "reviews ≥ solves this week → re-drill that pattern's trigger cards", true, false},
	{"pattern/sliding-window", "pattern", "Sliding Window", "", "reviews ≥ solves this week → re-drill that pattern's trigger cards", true, false},
	{"pattern/stack", "pattern", "Stack", "", "reviews ≥ solves this week → re-drill that pattern's trigger cards", true, false},
	{"pattern/monotonic-stack", "pattern", "Monotonic Stack", "", "reviews ≥ solves this week → re-drill that pattern's trigger cards", true, false},
	{"pattern/binary-search", "pattern", "Binary Search", "", "reviews ≥ solves this week → re-drill that pattern's trigger cards", true, false},
	{"pattern/linked-list", "pattern", "Linked List", "", "reviews ≥ solves this week → re-drill that pattern's trigger cards", true, false},
	{"pattern/trees", "pattern", "Trees", "", "reviews ≥ solves this week → re-drill that pattern's trigger cards", true, false},
	{"pattern/trie", "pattern", "Trie", "", "reviews ≥ solves this week → re-drill that pattern's trigger cards", true, false},
	{"pattern/heap", "pattern", "Heap", "", "reviews ≥ solves this week → re-drill that pattern's trigger cards", true, false},
	{"pattern/backtracking", "pattern", "Backtracking", "", "reviews ≥ solves this week → re-drill that pattern's trigger cards", true, false},
	{"pattern/intervals", "pattern", "Intervals", "", "reviews ≥ solves this week → re-drill that pattern's trigger cards", true, false},
	{"pattern/greedy", "pattern", "Greedy", "", "reviews ≥ solves this week → re-drill that pattern's trigger cards", true, false},
	{"pattern/dynamic-programming", "pattern", "Dynamic Programming", "", "reviews ≥ solves this week → re-drill that pattern's trigger cards", true, false},
	{"pattern/unclassified", "pattern", "Unclassified", "", "reviews ≥ solves this week → re-drill that pattern's trigger cards", true, false},
	{"habit/instagram", "habit", "Instagram cycle", "", "reinstall logged → note the trigger, don't spiral", false, true},
}

func seedSnapshot(t *testing.T, db string) string {
	t.Helper()
	st, err := store.Open(db)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer st.Close()
	things, err := st.Things(store.ListFilter{Archived: store.ArchivedAll})
	if err != nil {
		t.Fatalf("things: %v", err)
	}
	var b strings.Builder
	for _, th := range things {
		b.WriteString(th.ID + "\x00" + th.Kind + "\x00" + th.DisplayName + "\x00" +
			th.GoalJSON + "\x00" + th.DecisionRule + "\x00" + th.CreatedAt + "\x00" +
			boolStr(th.Active) + boolStr(th.Archived) + "\n")
	}
	return b.String()
}

func boolStr(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

func TestInitSeedsCatalog(t *testing.T) {
	db := filepath.Join(t.TempDir(), "m.db")
	out, errW, code := runCapture("init", "--db", db)
	if code != 0 {
		t.Fatalf("init exit %d, stderr: %s", code, errW)
	}
	if !strings.Contains(out, "23") {
		t.Errorf("init output should mention created count 23, got %q", out)
	}
	st, err := store.Open(db)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer st.Close()
	things, err := st.Things(store.ListFilter{Archived: store.ArchivedAll})
	if err != nil {
		t.Fatalf("things: %v", err)
	}
	if len(things) != 23 {
		t.Fatalf("want 23 seed rows, got %d", len(things))
	}
	byID := map[string]model.Thing{}
	for _, th := range things {
		byID[th.ID] = th
		if _, err := time.Parse(time.RFC3339, th.CreatedAt); err != nil {
			t.Errorf("%s: created_at %q not RFC 3339 UTC: %v", th.ID, th.CreatedAt, err)
		}
	}
	for _, w := range seedWant {
		t.Run(w.id, func(t *testing.T) {
			got, ok := byID[w.id]
			if !ok {
				t.Fatal("missing")
			}
			if got.Kind != w.kind || got.DisplayName != w.name ||
				got.GoalJSON != w.goal || got.DecisionRule != w.rule ||
				got.Active != w.active || got.Archived != w.archived {
				t.Errorf("got %+v, want kind=%s name=%s goal=%s rule=%s active=%v archived=%v",
					got, w.kind, w.name, w.goal, w.rule, w.active, w.archived)
			}
		})
	}
}

func TestInitIdempotent(t *testing.T) {
	db := filepath.Join(t.TempDir(), "m.db")
	if code := runQuiet("init", "--db", db); code != 0 {
		t.Fatalf("first init exit %d", code)
	}
	before := seedSnapshot(t, db)
	if code := runQuiet("init", "--db", db); code != 0 {
		t.Fatalf("second init exit %d", code)
	}
	after := seedSnapshot(t, db)
	if before != after {
		t.Error("second init changed the row set")
	}
}

func TestInitPreservesEditedRule(t *testing.T) {
	db := filepath.Join(t.TempDir(), "m.db")
	if code := runQuiet("init", "--db", db); code != 0 {
		t.Fatalf("first init exit %d", code)
	}
	st, err := store.Open(db)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	orig, err := st.Thing("course/ddco")
	if err != nil {
		t.Fatalf("thing: %v", err)
	}
	orig.DecisionRule = "user-edited rule"
	if err := st.UpsertThing(*orig); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	st.Close()
	if code := runQuiet("init", "--db", db); code != 0 {
		t.Fatalf("second init exit %d", code)
	}
	st2, err := store.Open(db)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer st2.Close()
	got, err := st2.Thing("course/ddco")
	if err != nil {
		t.Fatalf("thing: %v", err)
	}
	if got.DecisionRule != "user-edited rule" {
		t.Errorf("edited rule overwritten: %q", got.DecisionRule)
	}
}

func TestInitNoSeed(t *testing.T) {
	db := filepath.Join(t.TempDir(), "m.db")
	if code := runQuiet("init", "--seed=false", "--db", db); code != 0 {
		t.Fatalf("init exit %d", code)
	}
	st, err := store.Open(db)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer st.Close()
	things, err := st.Things(store.ListFilter{Archived: store.ArchivedAll})
	if err != nil {
		t.Fatalf("things: %v", err)
	}
	if len(things) != 0 {
		t.Errorf("--seed=false seeded %d rows", len(things))
	}
}
