package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExportHTMLUsageErrors(t *testing.T) {
	db := filepath.Join(t.TempDir(), "m.db")
	cases := []struct {
		name string
		args []string
	}{
		{"days zero", []string{"export", "html", "--days", "0", "--db", db}},
		{"days negative", []string{"export", "html", "--days", "-1", "--db", db}},
		{"extra args", []string{"export", "html", "out.html", "--db", db}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, errW, code := runCapture(tc.args...)
			if code != 2 {
				t.Errorf("exit = %d, want 2; stderr: %s", code, errW)
			}
		})
	}
}

func TestExportHTMLWritesFile(t *testing.T) {
	dir := t.TempDir()
	db := filepath.Join(dir, "m.db")
	out := filepath.Join(dir, "out.html")
	stdout, errW, code := runCapture("export", "html", "--out", out, "--db", db)
	if code != 0 {
		t.Fatalf("exit = %d, stderr: %s", code, errW)
	}
	if !strings.Contains(stdout, out) {
		t.Errorf("stdout missing %q:\n%s", out, stdout)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read export: %v", err)
	}
	for _, id := range []string{`id="chart-minutes"`, `id="chart-weekly"`, `id="chart-dsa"`, `id="chart-german"`, `id="table-milestones"`} {
		if !strings.Contains(string(data), id) {
			t.Errorf("export missing %s", id)
		}
	}
}

func TestExportHTMLDefaultPath(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdg)
	db := filepath.Join(t.TempDir(), "m.db")
	stdout, errW, code := runCapture("export", "html", "--db", db)
	if code != 0 {
		t.Fatalf("exit = %d, stderr: %s", code, errW)
	}
	want := filepath.Join(xdg, "meridian", "export.html")
	if !strings.Contains(stdout, want) {
		t.Errorf("stdout = %q, want path %q", stdout, want)
	}
	if _, err := os.Stat(want); err != nil {
		t.Errorf("export not written: %v", err)
	}
}
