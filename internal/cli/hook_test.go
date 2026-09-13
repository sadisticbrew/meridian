package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sadisticbrew/meridian/internal/model"
)

func hookConfig(t *testing.T, repos map[string]string) string {
	t.Helper()
	var b strings.Builder
	b.WriteString("[hooks.repos]\n")
	for repo, subject := range repos {
		fmt.Fprintf(&b, "%q = %q\n", repo, subject)
	}
	path := filepath.Join(t.TempDir(), "meridian.toml")
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func seedProjectThing(t *testing.T, db, id string) {
	t.Helper()
	st := openTestStore(t, db)
	if err := st.UpsertThing(model.Thing{
		ID: id, Kind: "project", DisplayName: id,
		Active: true, Archived: true, CreatedAt: nowRFC3339(),
	}); err != nil {
		t.Fatalf("seed %s: %v", id, err)
	}
}

func TestHookPostCommitRecordsMappedRepo(t *testing.T) {
	repo := syncInitRepo(t)
	root, err := gitToplevel(repo)
	if err != nil {
		t.Fatalf("git toplevel: %v", err)
	}
	db := filepath.Join(t.TempDir(), "m.db")
	seedProjectThing(t, db, "project/meridian")
	cfg := hookConfig(t, map[string]string{root: "project/meridian"})

	t.Chdir(repo)
	out, errW, code := runCapture("hook", "post-commit", "--db", db, "--config", cfg)
	if code != 0 {
		t.Fatalf("exit = %d, stderr: %s", code, errW)
	}
	if out != "" || errW != "" {
		t.Errorf("hook output = %q / %q, want silence", out, errW)
	}

	evs := logEvents(t, db)
	if len(evs) != 1 {
		t.Fatalf("events = %d, want 1", len(evs))
	}
	e := evs[0]
	if e.Source != "hook" || e.Type != "occurrence" {
		t.Errorf("source/type = %q/%q, want hook/occurrence", e.Source, e.Type)
	}
	if e.Subject == nil || *e.Subject != "project/meridian" {
		t.Errorf("subject = %v, want project/meridian", e.Subject)
	}
	if string(e.Payload) != `{"source":"hook"}` {
		t.Errorf("payload = %s, want {\"source\":\"hook\"}", e.Payload)
	}
	if _, err := time.Parse(time.RFC3339, e.Ts); err != nil {
		t.Errorf("ts %q not RFC 3339: %v", e.Ts, err)
	}
}

func TestHookPostCommitDropsSilently(t *testing.T) {
	cases := []struct {
		name      string
		subject   string
		seedThing bool
		mapped    bool
	}{
		{"unmapped repo", "", false, false},
		{"mapped subject missing", "project/nope", false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := syncInitRepo(t)
			root, err := gitToplevel(repo)
			if err != nil {
				t.Fatalf("git toplevel: %v", err)
			}
			db := filepath.Join(t.TempDir(), "m.db")
			if tc.seedThing {
				seedProjectThing(t, db, tc.subject)
			}
			repos := map[string]string{}
			if tc.mapped {
				repos[root] = tc.subject
			} else {
				repos[filepath.Join(root, "elsewhere")] = "project/meridian"
			}
			cfg := hookConfig(t, repos)

			state := &rt{dbPath: db, configPath: cfg}
			if err := hookPostCommit(state, repo, gitToplevel); err != nil {
				t.Fatalf("hookPostCommit = %v, want silent drop", err)
			}
			if evs := logEvents(t, db); len(evs) != 0 {
				t.Errorf("events = %d, want 0", len(evs))
			}
		})
	}
}

func TestHookPostCommitNotARepoDropsSilently(t *testing.T) {
	db := filepath.Join(t.TempDir(), "m.db")
	cfg := hookConfig(t, map[string]string{"/some/repo": "project/meridian"})
	state := &rt{dbPath: db, configPath: cfg}

	if err := hookPostCommit(state, "", func(string) (string, error) {
		return "", fmt.Errorf("not a git repository")
	}); err != nil {
		t.Fatalf("hookPostCommit = %v, want silent drop", err)
	}
	if evs := logEvents(t, db); len(evs) != 0 {
		t.Errorf("events = %d, want 0", len(evs))
	}
}

func TestHookUsage(t *testing.T) {
	if _, _, code := runCapture("hook", "post-commit", "extra"); code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
}

func TestContribPostCommit(t *testing.T) {
	path := filepath.Join("..", "..", "contrib", "post-commit")
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	want := "#!/bin/sh\nexec meridian hook post-commit\n"
	if string(got) != want {
		t.Errorf("contents = %q, want %q", got, want)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if info.Mode()&0o111 == 0 {
		t.Errorf("mode = %v, want executable", info.Mode())
	}
}
