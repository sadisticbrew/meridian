package views

import (
	"bytes"
	"testing"

	"github.com/sadisticbrew/meridian/internal/store"
)

func TestDSAGolden(t *testing.T) {
	cases := []struct {
		name   string
		seed   func(*testing.T) *store.Store
		golden string
	}{
		{"empty", func(t *testing.T) *store.Store { return openViewDB(t) }, "dsa_empty.txt"},
		{"multi-pattern", seedDSA, "dsa_multi.txt"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := tc.seed(t)
			var buf bytes.Buffer
			if err := DSA(&buf, testNow, st, 14); err != nil {
				t.Fatalf("DSA: %v", err)
			}
			goldenCompare(t, tc.golden, buf.String())
		})
	}
}
