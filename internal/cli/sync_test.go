package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sadisticbrew/meridian/internal/store"
)

func syncGit(t *testing.T, dir string, env []string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func syncInitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	syncGit(t, dir, nil, "init", "-b", "main")
	syncGit(t, dir, nil, "config", "user.email", "test@example.com")
	syncGit(t, dir, nil, "config", "user.name", "Test")
	return dir
}

func syncCommitFile(t *testing.T, repo, rel, date string) {
	t.Helper()
	p := filepath.Join(repo, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(p, []byte("// "+rel+"\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	env := []string{"GIT_AUTHOR_DATE=" + date, "GIT_COMMITTER_DATE=" + date}
	syncGit(t, repo, env, "add", "--", rel)
	syncGit(t, repo, env, "commit", "-m", "add "+rel)
}

func syncConfig(t *testing.T, url string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "meridian.toml")
	body := fmt.Sprintf("[collectors.neetcode]\nurl = %q\n", url)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

// syncIsolate keeps the collector mirror inside the test's temp space
// instead of the user's real XDG data home.
func syncIsolate(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", t.TempDir())
}

func syncFixtureRepo(t *testing.T) string {
	t.Helper()
	repo := syncInitRepo(t)
	syncCommitFile(t, repo, "Data Structures & Algorithms/car-fleet/submission-0.py", "2026-09-01T10:00:00 +0000")
	return repo
}

func TestSyncCollectsAndIsIdempotent(t *testing.T) {
	syncIsolate(t)
	cfgPath := syncConfig(t, syncFixtureRepo(t))
	db := filepath.Join(t.TempDir(), "m.db")

	out, errW, code := runCapture("sync", "--db", db, "--config", cfgPath)
	if code != 0 {
		t.Fatalf("first sync exit %d, stderr: %s", code, errW)
	}
	if !strings.Contains(out, "neetcode: 1 new, 0 updated") {
		t.Errorf("first sync output = %q, want line %q", out, "neetcode: 1 new, 0 updated")
	}

	out, errW, code = runCapture("sync", "--db", db, "--config", cfgPath)
	if code != 0 {
		t.Fatalf("second sync exit %d, stderr: %s", code, errW)
	}
	if !strings.Contains(out, "neetcode: 0 new, 0 updated") {
		t.Errorf("second sync output = %q, want line %q", out, "neetcode: 0 new, 0 updated")
	}

	st, err := store.Open(db)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer st.Close()
	ev, err := st.EventByDedup("neetcode/car-fleet")
	if err != nil {
		t.Fatalf("EventByDedup: %v", err)
	}
	if ev.Source != "neetcode" {
		t.Errorf("source = %q, want neetcode", ev.Source)
	}
	all, err := st.Events(store.EventFilter{})
	if err != nil {
		t.Fatalf("Events: %v", err)
	}
	if len(all) != 1 {
		t.Errorf("events = %d, want exactly 1", len(all))
	}
}

func TestSyncUnavailableSkipsWithExitZero(t *testing.T) {
	syncIsolate(t)
	cfgPath := syncConfig(t, filepath.Join(t.TempDir(), "no-such-repo"))
	db := filepath.Join(t.TempDir(), "m.db")

	out, errW, code := runCapture("sync", "--db", db, "--config", cfgPath)
	if code != 0 {
		t.Fatalf("sync exit %d, stderr: %s", code, errW)
	}
	if !strings.Contains(out, "neetcode: unavailable (offline?) — skipped") {
		t.Errorf("output = %q, want skip line", out)
	}
}

func TestSyncUnknownOnly(t *testing.T) {
	cfgPath := syncConfig(t, filepath.Join(t.TempDir(), "no-such-repo"))
	db := filepath.Join(t.TempDir(), "m.db")

	_, _, code := runCapture("sync", "--db", db, "--config", cfgPath, "--only", "nope")
	if code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
}

func TestSyncOnlyNeetcode(t *testing.T) {
	syncIsolate(t)
	cfgPath := syncConfig(t, syncFixtureRepo(t))
	db := filepath.Join(t.TempDir(), "m.db")

	out, errW, code := runCapture("sync", "--db", db, "--config", cfgPath, "--only", "neetcode")
	if code != 0 {
		t.Fatalf("sync exit %d, stderr: %s", code, errW)
	}
	if !strings.Contains(out, "neetcode: 1 new, 0 updated") {
		t.Errorf("output = %q, want line %q", out, "neetcode: 1 new, 0 updated")
	}
}

func TestSyncBadTimeout(t *testing.T) {
	_, _, code := runCapture("sync", "--timeout", "nonsense")
	if code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
}

func TestSyncDBFailure(t *testing.T) {
	db := filepath.Join(t.TempDir(), "nodir", "m.db")

	_, errW, code := runCapture("sync", "--db", db)
	if code != 1 {
		t.Errorf("exit = %d, want 1 (stderr: %s)", code, errW)
	}
}
