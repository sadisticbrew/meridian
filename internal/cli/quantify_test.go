package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/sadisticbrew/meridian/internal/model"
	"github.com/sadisticbrew/meridian/internal/store"
)

func TestQuantifyNoProviderConfigured(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	db := filepath.Join(t.TempDir(), "meridian.db")

	st, err := store.Open(db)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	raw := "studied DDCO for 45 minutes"
	if _, err := st.AddEvent(model.Event{
		Ts:        "2026-09-10T10:00:00Z",
		Source:    "manual",
		Type:      "note",
		RawText:   &raw,
		CreatedAt: "2026-09-10T10:00:01Z",
	}); err != nil {
		t.Fatalf("add note: %v", err)
	}
	if err := st.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	_, errW, code := runCapture("quantify", "--db", db)
	if code != 1 {
		t.Fatalf("exit = %d, want 1 (stderr: %s)", code, errW)
	}
	if !strings.Contains(errW, "provider") || !strings.Contains(errW, "[quantify]") {
		t.Errorf("stderr = %q, want a message about configuring a provider", errW)
	}

	st, err = store.Open(db)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer st.Close()
	notes, err := st.Notes(false)
	if err != nil {
		t.Fatalf("Notes: %v", err)
	}
	if len(notes) != 1 {
		t.Fatalf("notes = %d, want 1 intact note", len(notes))
	}
	if notes[0].Quantified != 0 || len(notes[0].Payload) != 0 {
		t.Errorf("note mutated: quantified=%d payload=%s", notes[0].Quantified, notes[0].Payload)
	}
}
