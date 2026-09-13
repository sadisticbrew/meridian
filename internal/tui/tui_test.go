package tui

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sadisticbrew/meridian/internal/model"
	"github.com/sadisticbrew/meridian/internal/store"
)

var (
	testZone = time.FixedZone("FIX", 2*3600)
	testNow  = time.Date(2026, 9, 13, 18, 0, 0, 0, testZone)
	fixTS    = "2026-09-13T15:00:00Z"
)

func openTUIDB(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "tui.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func seedSession(t *testing.T, st *store.Store, subject string, minutes int, ts string) {
	t.Helper()
	payload := fmt.Sprintf(`{"started_at":%q,"ended_at":%q,"minutes":%d,"tags":[]}`, ts, ts, minutes)
	e := model.Event{Ts: ts, Source: "manual", Type: "session",
		Subject: &subject, Payload: json.RawMessage(payload), CreatedAt: fixTS}
	if _, err := st.AddEvent(e); err != nil {
		t.Fatalf("seed session: %v", err)
	}
}

func seedTUI(t *testing.T) *store.Store {
	t.Helper()
	st := openTUIDB(t)
	ddco := "course/ddco"
	mono := "pattern/monotonic-stack"
	for _, th := range []model.Thing{
		{ID: ddco, Kind: "course", DisplayName: "DDCO", Active: true},
		{ID: mono, Kind: "pattern", DisplayName: "Monotonic Stack", Active: true},
	} {
		th.CreatedAt = fixTS
		if err := st.UpsertThing(th); err != nil {
			t.Fatalf("seed thing %s: %v", th.ID, err)
		}
	}
	seedSession(t, st, ddco, 65, "2026-09-13T07:00:00Z")
	e := model.Event{Ts: "2026-09-13T08:00:00Z", Source: "manual", Type: "occurrence",
		Subject: &mono, Payload: json.RawMessage(`{"problem":"car-fleet","outcome":"solved"}`), CreatedAt: fixTS}
	if _, err := st.AddEvent(e); err != nil {
		t.Fatalf("seed occurrence: %v", err)
	}
	return st
}

func runeKey(r rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
}

func TestModelTabsAndKeys(t *testing.T) {
	addSession := func(minutes int) func(*testing.T, *store.Store, *Model) {
		return func(t *testing.T, st *store.Store, _ *Model) {
			seedSession(t, st, "course/ddco", minutes, "2026-09-13T09:00:00Z")
		}
	}
	cases := []struct {
		name        string
		before      func(*testing.T, *store.Store, *Model)
		msg         tea.Msg
		wantTab     int
		wantContent string
		wantQuit    bool
	}{
		{"initial-tab-is-today", nil, nil, 0, "Today —", false},
		{"key-2-switches-to-week", nil, runeKey('2'), 1, "Week —", false},
		{"key-3-switches-to-subject", nil, runeKey('3'), 2, "course/ddco — DDCO", false},
		{"key-4-switches-to-dsa", nil, runeKey('4'), 3, "DSA — last 14 days", false},
		{"l-cycles-forward-with-wrap",
			func(_ *testing.T, _ *store.Store, m *Model) { m.Tab = 3 }, runeKey('l'), 0, "Today —", false},
		{"h-cycles-back-with-wrap",
			func(_ *testing.T, _ *store.Store, m *Model) { m.Tab = 0 }, runeKey('h'), 3, "DSA — last 14 days", false},
		{"r-re-renders-from-store", addSession(30), runeKey('r'), 0, "95m", false},
		{"tick-re-renders-from-store", addSession(30), tickMsg(testNow), 0, "95m", false},
		{"q-quits", nil, runeKey('q'), 0, "Today —", true},
		{"ctrl-c-quits", nil, tea.KeyMsg{Type: tea.KeyCtrlC}, 0, "Today —", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := seedTUI(t)
			m := NewModel(st, func() time.Time { return testNow })
			if tc.before != nil {
				tc.before(t, st, m)
			}
			if tc.msg != nil {
				model, cmd := m.Update(tc.msg)
				if model != m {
					t.Fatal("Update returned a different model")
				}
				if tc.wantQuit && cmd == nil {
					t.Error("quit key returned nil cmd, want tea.Quit")
				}
			}
			if m.Tab != tc.wantTab {
				t.Errorf("Tab = %d, want %d", m.Tab, tc.wantTab)
			}
			if got := m.content(); !strings.Contains(got, tc.wantContent) {
				t.Errorf("content missing %q:\n%s", tc.wantContent, got)
			}
			if m.quitting != tc.wantQuit {
				t.Errorf("quitting = %v, want %v", m.quitting, tc.wantQuit)
			}
		})
	}
}

func TestViewportClamp(t *testing.T) {
	st := seedTUI(t)
	m := NewModel(st, func() time.Time { return testNow })
	m.height = 2
	m.scroll(100)
	if want := len(m.lines) - m.height; m.offset != want {
		t.Errorf("offset = %d, want clamped %d", m.offset, want)
	}
	m.scroll(-100)
	if m.offset != 0 {
		t.Errorf("offset = %d, want 0", m.offset)
	}
}

func TestRenderErrorShownAsContent(t *testing.T) {
	st := seedTUI(t)
	m := NewModel(st, func() time.Time { return testNow })
	if err := st.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	m.Update(runeKey('r'))
	if m.err == nil {
		t.Fatal("want render error after closing the store")
	}
	if got := m.content(); !strings.Contains(got, m.err.Error()) {
		t.Errorf("content missing error %q:\n%s", m.err.Error(), got)
	}
}
