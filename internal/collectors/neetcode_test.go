package collectors

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/sadisticbrew/meridian/internal/model"
	"github.com/sadisticbrew/meridian/internal/store"
)

func openTestStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func runGit(t *testing.T, dir string, env []string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func initRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, nil, "init", "-b", "main")
	runGit(t, dir, nil, "config", "user.email", "test@example.com")
	runGit(t, dir, nil, "config", "user.name", "Test")
	return dir
}

func commitFile(t *testing.T, repo, rel, date string) {
	t.Helper()
	p := filepath.Join(repo, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(p, []byte("// "+rel+"\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	env := []string{"GIT_AUTHOR_DATE=" + date, "GIT_COMMITTER_DATE=" + date}
	runGit(t, repo, env, "add", "--", rel)
	runGit(t, repo, env, "commit", "-m", "add "+rel)
}

func newTestCollector(t *testing.T, repo string) *NeetCode {
	t.Helper()
	return &NeetCode{
		RepoURL:   repo,
		MirrorDir: filepath.Join(t.TempDir(), "mirror", "neetcode"),
		Patterns:  map[string]string{"car-fleet": "pattern/monotonic-stack"},
	}
}

func assertPayload(t *testing.T, e *model.Event, wantAttempts int, wantLanguage string) {
	t.Helper()
	var p struct {
		Slug        string `json:"slug"`
		Attempts    int    `json:"attempts"`
		Language    string `json:"language"`
		TopicFolder string `json:"topic_folder"`
	}
	if err := json.Unmarshal(e.Payload, &p); err != nil {
		t.Fatalf("decode payload %s: %v", e.Payload, err)
	}
	if p.Slug != "car-fleet" {
		t.Errorf("slug = %q, want car-fleet", p.Slug)
	}
	if p.Attempts != wantAttempts {
		t.Errorf("attempts = %d, want %d", p.Attempts, wantAttempts)
	}
	if p.Language != wantLanguage {
		t.Errorf("language = %q, want %q", p.Language, wantLanguage)
	}
	if p.TopicFolder != "Data Structures & Algorithms" {
		t.Errorf("topic_folder = %q, want Data Structures & Algorithms", p.TopicFolder)
	}
}

func TestNeetCodeSyncFirstAdd(t *testing.T) {
	repo := initRepo(t)
	commitFile(t, repo, "Data Structures & Algorithms/car-fleet/submission-0.py", "2026-09-01T10:00:00 +0000")
	s := openTestStore(t)
	n := newTestCollector(t, repo)

	res, err := n.Sync(context.Background(), s)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if res.New != 1 || res.Updated != 0 || res.Details != "" {
		t.Fatalf("result = %+v, want New:1 Updated:0 Details:\"\"", res)
	}

	ev, err := s.EventByDedup("neetcode/car-fleet")
	if err != nil {
		t.Fatalf("EventByDedup: %v", err)
	}
	if ev.Ts != "2026-09-01T10:00:00Z" {
		t.Errorf("ts = %q, want 2026-09-01T10:00:00Z", ev.Ts)
	}
	if ev.Subject == nil || *ev.Subject != "pattern/monotonic-stack" {
		t.Errorf("subject = %v, want pattern/monotonic-stack", ev.Subject)
	}
	if ev.Source != "neetcode" || ev.Type != "occurrence" {
		t.Errorf("source/type = %q/%q, want neetcode/occurrence", ev.Source, ev.Type)
	}
	if ev.DedupKey == nil || *ev.DedupKey != "neetcode/car-fleet" {
		t.Errorf("dedup_key = %v, want neetcode/car-fleet", ev.DedupKey)
	}
	if ev.CreatedAt == "" {
		t.Error("created_at is empty")
	}
	assertPayload(t, ev, 1, "py")
	before := *ev

	res, err = n.Sync(context.Background(), s)
	if err != nil {
		t.Fatalf("second Sync: %v", err)
	}
	if res.New != 0 || res.Updated != 0 {
		t.Fatalf("second result = %+v, want New:0 Updated:0", res)
	}
	ev, err = s.EventByDedup("neetcode/car-fleet")
	if err != nil {
		t.Fatalf("EventByDedup after second sync: %v", err)
	}
	if !reflect.DeepEqual(*ev, before) {
		t.Errorf("event changed on no-op sync:\n got %+v\nwant %+v", *ev, before)
	}
	all, err := s.Events(store.EventFilter{})
	if err != nil {
		t.Fatalf("Events: %v", err)
	}
	if len(all) != 1 {
		t.Errorf("events = %d, want 1", len(all))
	}
}

func TestNeetCodeSyncAttemptsUpdate(t *testing.T) {
	repo := initRepo(t)
	commitFile(t, repo, "Data Structures & Algorithms/car-fleet/submission-0.py", "2026-09-01T10:00:00 +0000")
	s := openTestStore(t)
	n := newTestCollector(t, repo)

	if _, err := n.Sync(context.Background(), s); err != nil {
		t.Fatalf("first Sync: %v", err)
	}
	first, err := s.EventByDedup("neetcode/car-fleet")
	if err != nil {
		t.Fatalf("first EventByDedup: %v", err)
	}

	commitFile(t, repo, "Data Structures & Algorithms/car-fleet/submission-1.py", "2026-09-08T10:00:00 +0000")
	res, err := n.Sync(context.Background(), s)
	if err != nil {
		t.Fatalf("second Sync: %v", err)
	}
	if res.New != 0 || res.Updated != 1 {
		t.Fatalf("result = %+v, want New:0 Updated:1", res)
	}
	if res.Details != "attempts +1 on car-fleet" {
		t.Errorf("details = %q, want %q", res.Details, "attempts +1 on car-fleet")
	}

	ev, err := s.EventByDedup("neetcode/car-fleet")
	if err != nil {
		t.Fatalf("EventByDedup after update: %v", err)
	}
	assertPayload(t, ev, 2, "py")
	if ev.ID != first.ID {
		t.Errorf("id = %d, want %d (no duplicate)", ev.ID, first.ID)
	}
	if ev.Ts != first.Ts {
		t.Errorf("ts = %q, want unchanged %q", ev.Ts, first.Ts)
	}
	if ev.CreatedAt != first.CreatedAt {
		t.Errorf("created_at = %q, want unchanged %q", ev.CreatedAt, first.CreatedAt)
	}
}

func TestNeetCodeSyncMultiLanguage(t *testing.T) {
	repo := initRepo(t)
	commitFile(t, repo, "Data Structures & Algorithms/car-fleet/submission-0.py", "2026-09-01T10:00:00 +0000")
	s := openTestStore(t)
	n := newTestCollector(t, repo)

	if _, err := n.Sync(context.Background(), s); err != nil {
		t.Fatalf("first Sync: %v", err)
	}
	first, err := s.EventByDedup("neetcode/car-fleet")
	if err != nil {
		t.Fatalf("first EventByDedup: %v", err)
	}

	commitFile(t, repo, "Data Structures & Algorithms/car-fleet/submission-0.go", "2026-09-08T10:00:00 +0000")
	commitFile(t, repo, "Data Structures & Algorithms/car-fleet/submission-1.go", "2026-09-08T10:00:00 +0000")
	res, err := n.Sync(context.Background(), s)
	if err != nil {
		t.Fatalf("second Sync: %v", err)
	}
	if res.New != 0 || res.Updated != 1 {
		t.Fatalf("result = %+v, want New:0 Updated:1", res)
	}

	ev, err := s.EventByDedup("neetcode/car-fleet")
	if err != nil {
		t.Fatalf("EventByDedup after update: %v", err)
	}
	assertPayload(t, ev, 2, "py")
	if ev.Ts != first.Ts || ev.Ts != "2026-09-01T10:00:00Z" {
		t.Errorf("ts = %q, want original .py date %q", ev.Ts, first.Ts)
	}
	if ev.CreatedAt != first.CreatedAt {
		t.Errorf("created_at = %q, want unchanged %q", ev.CreatedAt, first.CreatedAt)
	}
}

func TestNeetCodeSyncUnavailable(t *testing.T) {
	s := openTestStore(t)
	n := &NeetCode{
		RepoURL:   filepath.Join(t.TempDir(), "does-not-exist"),
		MirrorDir: filepath.Join(t.TempDir(), "mirror", "neetcode"),
		Patterns:  map[string]string{},
	}
	_, err := n.Sync(context.Background(), s)
	if err == nil {
		t.Fatal("Sync: want error")
	}
	var ue *UnavailableError
	if !errors.As(err, &ue) {
		t.Fatalf("err = %v (%T), want UnavailableError", err, err)
	}
	if !strings.Contains(ue.Error(), "unavailable (offline?): ") {
		t.Errorf("message = %q, want unavailable prefix", ue.Error())
	}
}

func TestNeetCodeSyncNoMatchingPaths(t *testing.T) {
	cases := []struct {
		name      string
		commit    bool
		rel, date string
	}{
		{name: "empty repo"},
		{name: "no matching paths", commit: true, rel: "notes/readme.md", date: "2026-09-01T10:00:00 +0000"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := initRepo(t)
			if tc.commit {
				commitFile(t, repo, tc.rel, tc.date)
			}
			s := openTestStore(t)
			n := newTestCollector(t, repo)

			res, err := n.Sync(context.Background(), s)
			if err != nil {
				t.Fatalf("Sync: %v", err)
			}
			if res.New != 0 || res.Updated != 0 {
				t.Fatalf("result = %+v, want zero", res)
			}
			all, err := s.Events(store.EventFilter{})
			if err != nil {
				t.Fatalf("Events: %v", err)
			}
			if len(all) != 0 {
				t.Errorf("events = %d, want 0", len(all))
			}
		})
	}
}

func TestPatternSubject(t *testing.T) {
	cases := []struct {
		name     string
		patterns map[string]string
		slug     string
		want     string
	}{
		{"full id", map[string]string{"car-fleet": "pattern/monotonic-stack"}, "car-fleet", "pattern/monotonic-stack"},
		{"value verbatim", map[string]string{"car-fleet": "pattern/custom"}, "car-fleet", "pattern/custom"},
		{"empty value", map[string]string{"car-fleet": ""}, "car-fleet", "pattern/unclassified"},
		{"missing slug", map[string]string{}, "car-fleet", "pattern/unclassified"},
		{"nil map", nil, "car-fleet", "pattern/unclassified"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := patternSubject(tc.patterns, tc.slug); got != tc.want {
				t.Errorf("patternSubject = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestLoadPatternsEmbedded(t *testing.T) {
	m, err := loadPatterns()
	if err != nil {
		t.Fatalf("loadPatterns: %v", err)
	}
	if len(m) != 150 {
		t.Errorf("len = %d, want 150", len(m))
	}
	if got := m["car-fleet"]; got != "pattern/monotonic-stack" {
		t.Errorf("car-fleet = %q, want pattern/monotonic-stack", got)
	}
}

func TestRegistry(t *testing.T) {
	reg := Registry()
	c, ok := reg["neetcode"]
	if !ok {
		t.Fatal("registry missing neetcode")
	}
	if c.Name() != "neetcode" {
		t.Errorf("Name() = %q, want neetcode", c.Name())
	}
}

func TestDefaultMirrorDir(t *testing.T) {
	cases := []struct {
		name string
		xdg  string
		home string
		want string
	}{
		{name: "xdg data home", xdg: "/xdg/data", want: filepath.Join("/xdg/data", "meridian", "mirror", "neetcode")},
		{name: "home fallback", home: "/home/tester", want: filepath.Join("/home/tester", ".local", "share", "meridian", "mirror", "neetcode")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("XDG_DATA_HOME", tc.xdg)
			if tc.home != "" {
				t.Setenv("HOME", tc.home)
			}
			got, err := defaultMirrorDir()
			if err != nil {
				t.Fatalf("defaultMirrorDir: %v", err)
			}
			if got != tc.want {
				t.Errorf("dir = %q, want %q", got, tc.want)
			}
		})
	}
}
