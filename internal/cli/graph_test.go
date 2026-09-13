package cli

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestGraphUsageErrors(t *testing.T) {
	db := filepath.Join(t.TempDir(), "m.db")
	seedViewThings(t, db)
	cases := []struct {
		name string
		args []string
	}{
		{"unknown kind", []string{"graph", "hours", "--db", db}},
		{"days zero", []string{"graph", "--days", "0", "--db", db}},
		{"days negative", []string{"graph", "dsa", "--days", "-1", "--db", db}},
		{"too many args", []string{"graph", "minutes", "dsa", "--db", db}},
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

func TestGraphKinds(t *testing.T) {
	db := filepath.Join(t.TempDir(), "m.db")
	seedViewThings(t, db)
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"default kind", []string{"graph", "--db", db}, "Minutes — last 30 days"},
		{"minutes", []string{"graph", "minutes", "--days", "7", "--db", db}, "Minutes — last 7 days"},
		{"dsa", []string{"graph", "dsa", "--db", db}, "DSA — last 30 days"},
		{"german", []string{"graph", "german", "--db", db}, "German — last 30 days"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, errW, code := runCapture(tc.args...)
			if code != 0 {
				t.Fatalf("exit = %d, stderr: %s", code, errW)
			}
			if !strings.Contains(out, tc.want) {
				t.Errorf("output missing %q:\n%s", tc.want, out)
			}
		})
	}
}
