package cli

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sadisticbrew/meridian/internal/store"
)

func dbFlag(t *testing.T) []string {
	t.Helper()
	return []string{"--db", filepath.Join(t.TempDir(), "m.db")}
}

func listJSON(t *testing.T, db string, extra ...string) []string {
	t.Helper()
	args := append([]string{"track", "list", "--json", "--db", db}, extra...)
	out, errW, code := runCapture(args...)
	if code != 0 {
		t.Fatalf("track list exit %d, stderr: %s", code, errW)
	}
	var ids []string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		if i := strings.Index(line, `"id":"`); i >= 0 {
			rest := line[i+6:]
			ids = append(ids, rest[:strings.Index(rest, `"`)])
		}
	}
	return ids
}

func containsID(ids []string, id string) bool {
	for _, x := range ids {
		if x == id {
			return true
		}
	}
	return false
}

func TestTrackAddListArchiveRestore(t *testing.T) {
	dbArgs := dbFlag(t)
	out, errW, code := runCapture(append(dbArgs, "track", "add", "project/test", "--name", "T", "--rule", "r")...)
	if code != 0 {
		t.Fatalf("add exit %d, stderr: %s", code, errW)
	}
	if !strings.Contains(out, "added project/test") {
		t.Errorf("add output %q", out)
	}
	if ids := listJSON(t, dbArgs[1]); !containsID(ids, "project/test") {
		t.Errorf("list missing project/test: %v", ids)
	}

	if _, _, code := runCapture(append(dbArgs, "track", "archive", "project/test")...); code != 0 {
		t.Fatalf("archive exit %d", code)
	}
	if ids := listJSON(t, dbArgs[1]); containsID(ids, "project/test") {
		t.Errorf("archived thing still in default list: %v", ids)
	}
	if ids := listJSON(t, dbArgs[1], "--archived"); !containsID(ids, "project/test") {
		t.Errorf("archived thing missing from --archived list: %v", ids)
	}

	if _, _, code := runCapture(append(dbArgs, "track", "restore", "test")...); code != 0 {
		t.Fatalf("restore exit %d", code)
	}
	if ids := listJSON(t, dbArgs[1]); !containsID(ids, "project/test") {
		t.Errorf("restored thing missing from default list: %v", ids)
	}
}

func TestTrackRuleUpdate(t *testing.T) {
	dbArgs := dbFlag(t)
	if _, _, code := runCapture(append(dbArgs, "track", "add", "project/test", "--name", "T", "--rule", "old")...); code != 0 {
		t.Fatal("add failed")
	}
	if _, _, code := runCapture(append(dbArgs, "track", "rule", "test", "new rule text")...); code != 0 {
		t.Fatal("rule failed")
	}
	st, err := store.Open(dbArgs[1])
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer st.Close()
	got, err := st.Thing("project/test")
	if err != nil {
		t.Fatalf("thing: %v", err)
	}
	if got.DecisionRule != "new rule text" {
		t.Errorf("rule = %q", got.DecisionRule)
	}
	if got.CreatedAt == "" {
		t.Error("created_at lost on read-modify-write")
	}
}

func TestTrackGoalSetAndClear(t *testing.T) {
	dbArgs := dbFlag(t)
	if _, _, code := runCapture(append(dbArgs, "track", "add", "project/test", "--name", "T", "--rule", "r")...); code != 0 {
		t.Fatal("add failed")
	}
	if _, _, code := runCapture(append(dbArgs, "track", "goal", "test", "--per-week", "2")...); code != 0 {
		t.Fatal("goal set failed")
	}
	st, err := store.Open(dbArgs[1])
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	got, err := st.Thing("project/test")
	if err != nil {
		t.Fatalf("thing: %v", err)
	}
	if got.GoalJSON != `{"occurrences_per_week":2}` {
		t.Errorf("goal = %q", got.GoalJSON)
	}
	st.Close()
	if _, _, code := runCapture(append(dbArgs, "track", "goal", "test", "--clear")...); code != 0 {
		t.Fatal("goal clear failed")
	}
	st2, err := store.Open(dbArgs[1])
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer st2.Close()
	got2, err := st2.Thing("project/test")
	if err != nil {
		t.Fatalf("thing: %v", err)
	}
	if got2.GoalJSON != "" {
		t.Errorf("goal after clear = %q", got2.GoalJSON)
	}
}

func TestTrackGoalRequiresExactlyOneAction(t *testing.T) {
	dbArgs := dbFlag(t)
	if _, _, code := runCapture(append(dbArgs, "track", "add", "project/test", "--name", "T", "--rule", "r")...); code != 0 {
		t.Fatal("add failed")
	}
	cases := []struct {
		name string
		args []string
	}{
		{"no action", []string{"track", "goal", "test"}},
		{"two actions", []string{"track", "goal", "test", "--per-week", "2", "--clear"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, errW, code := runCapture(append(dbArgs, tc.args...)...)
			if code != 2 {
				t.Errorf("exit = %d, want 2; stderr: %s", code, errW)
			}
		})
	}
}

func TestTrackAddAntiVanity(t *testing.T) {
	dbArgs := dbFlag(t)
	_, errW, code := runCapture(append(dbArgs, "track", "add", "habit/whatever", "--name", "w")...)
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if !strings.Contains(errW, "decision rule required unless --archived") {
		t.Errorf("stderr %q missing anti-vanity message", errW)
	}
}

func TestTrackAddArchivedNoRule(t *testing.T) {
	dbArgs := dbFlag(t)
	if _, _, code := runCapture(append(dbArgs, "track", "add", "habit/diagnostic", "--name", "D", "--archived")...); code != 0 {
		t.Fatal("archived add without rule failed")
	}
	st, err := store.Open(dbArgs[1])
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer st.Close()
	got, err := st.Thing("habit/diagnostic")
	if err != nil {
		t.Fatalf("thing: %v", err)
	}
	if !got.Archived || got.Active {
		t.Errorf("archived add flags wrong: %+v", got)
	}
}

func TestTrackAddExistingID(t *testing.T) {
	dbArgs := dbFlag(t)
	if _, _, code := runCapture(append(dbArgs, "track", "add", "project/x", "--name", "X", "--rule", "r")...); code != 0 {
		t.Fatal("first add failed")
	}
	_, errW, code := runCapture(append(dbArgs, "track", "add", "project/x", "--name", "X2", "--rule", "r2")...)
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if !strings.Contains(errW, "already exists") {
		t.Errorf("stderr %q", errW)
	}
}

func TestTrackAddBudgetWarning(t *testing.T) {
	dbArgs := dbFlag(t)
	if code := runQuiet("init", "--db", dbArgs[1]); code != 0 {
		t.Fatal("init failed")
	}
	// Seeded db already has 7 active non-pattern things; one more trips >5.
	_, errW, code := runCapture(append(dbArgs, "track", "add", "project/sonar", "--name", "Sonar", "--rule", "weekends only")...)
	if code != 0 {
		t.Fatalf("add exit %d, stderr: %s", code, errW)
	}
	want := "metric budget exceeded — retire something or accept this is a vanity metric."
	if !strings.Contains(errW, want) {
		t.Errorf("stderr %q missing budget warning", errW)
	}
}

func TestTrackAddPatternNoBudgetWarning(t *testing.T) {
	dbArgs := dbFlag(t)
	if code := runQuiet("init", "--db", dbArgs[1]); code != 0 {
		t.Fatal("init failed")
	}
	_, errW, code := runCapture(append(dbArgs, "track", "add", "pattern/extra", "--name", "Extra", "--rule", "r")...)
	if code != 0 {
		t.Fatalf("add exit %d", code)
	}
	if strings.Contains(errW, "metric budget exceeded") {
		t.Errorf("pattern add must not warn, stderr: %q", errW)
	}
}

func TestTrackAddInvalidKindAndID(t *testing.T) {
	dbArgs := dbFlag(t)
	cases := []struct{ name, id string }{
		{"bad kind", "widget/x"},
		{"no slash", "justname"},
		{"empty name part", "course/"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, errW, code := runCapture(append(dbArgs, "track", "add", tc.id, "--name", "N", "--rule", "r")...)
			if code != 2 {
				t.Errorf("exit = %d, want 2; stderr: %s", code, errW)
			}
		})
	}
}

func TestTrackArchiveUnknownID(t *testing.T) {
	dbArgs := dbFlag(t)
	_, errW, code := runCapture(append(dbArgs, "track", "archive", "nope")...)
	if code != 1 {
		t.Errorf("exit = %d, want 1; stderr: %s", code, errW)
	}
}

func TestTrackArchiveAmbiguousSuffix(t *testing.T) {
	dbArgs := dbFlag(t)
	if code := runQuiet("init", "--db", dbArgs[1]); code != 0 {
		t.Fatal("init failed")
	}
	_, errW, code := runCapture(append(dbArgs, "track", "archive", "stack")...)
	if code != 4 {
		t.Errorf("exit = %d, want 4; stderr: %s", code, errW)
	}
	for _, cand := range []string{"pattern/stack", "pattern/monotonic-stack"} {
		if !strings.Contains(errW, cand) {
			t.Errorf("stderr %q missing candidate %s", errW, cand)
		}
	}
}

func TestExitCodeClassification(t *testing.T) {
	cases := []struct {
		name string
		err  error
		ran  bool
		want int
	}{
		{"ambiguous", &AmbiguousError{Input: "x", Candidates: []string{"a/x", "b/x"}}, true, 4},
		{"usage wrapped", usagef("bad flag"), true, 2},
		{"cobra pre-run", errors.New("unknown command"), false, 2},
		{"runtime", errors.New("db failure"), true, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := classify(tc.err, tc.ran); got != tc.want {
				t.Errorf("classify = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestTrackListHumanAndKindFilter(t *testing.T) {
	dbArgs := dbFlag(t)
	if code := runQuiet("init", "--db", dbArgs[1]); code != 0 {
		t.Fatal("init failed")
	}
	out, errW, code := runCapture(append(dbArgs, "track", "list", "--kind", "course")...)
	if code != 0 {
		t.Fatalf("list exit %d, stderr: %s", code, errW)
	}
	if !strings.Contains(out, "course/ddco") || strings.Contains(out, "pattern/stack") {
		t.Errorf("kind filter output wrong:\n%s", out)
	}
	if !strings.HasPrefix(out, "id") || !strings.Contains(out, "name") {
		t.Errorf("expected header row, got:\n%s", out)
	}
}

func TestTrackAddWithWeeklyMin(t *testing.T) {
	dbArgs := dbFlag(t)
	if _, _, code := runCapture(append(dbArgs, "track", "add", "project/g", "--name", "G", "--rule", "r", "--weekly-min", "5")...); code != 0 {
		t.Fatal("add failed")
	}
	st, err := store.Open(dbArgs[1])
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer st.Close()
	got, err := st.Thing("project/g")
	if err != nil {
		t.Fatalf("thing: %v", err)
	}
	if got.GoalJSON != `{"weekly_minutes":5}` {
		t.Errorf("goal = %q", got.GoalJSON)
	}
}
