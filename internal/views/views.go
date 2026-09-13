package views

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/sadisticbrew/meridian/internal/model"
	"github.com/sadisticbrew/meridian/internal/render"
	"github.com/sadisticbrew/meridian/internal/store"
)

func localMidnight(now time.Time) time.Time {
	y, m, d := now.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, now.Location())
}

// mondayStart returns the local Monday 00:00 that begins now's week and the
// exclusive end of that week window.
func mondayStart(now time.Time) (time.Time, time.Time) {
	from := localMidnight(now).AddDate(0, 0, -(int(now.Weekday())+6)%7)
	return from, from.AddDate(0, 0, 7)
}

func utcRange(from, to time.Time) (string, string) {
	return from.UTC().Format(time.RFC3339), to.UTC().Format(time.RFC3339)
}

func header(w io.Writer, title string) {
	fmt.Fprintf(w, "%s\n%s\n", title, render.Underline(title))
}

// displayNames caches id -> display name for every registered thing,
// archived included (archived things still appear in history-style rows).
func displayNames(st *store.Store) (map[string]string, error) {
	things, err := st.Things(store.ListFilter{Archived: store.ArchivedAll})
	if err != nil {
		return nil, err
	}
	m := make(map[string]string, len(things))
	for _, t := range things {
		m[t.ID] = t.DisplayName
	}
	return m, nil
}

type occurrencePayload struct {
	Problem string `json:"problem"`
	Outcome string `json:"outcome"`
}

type milestonePayload struct {
	Exam string   `json:"exam"`
	Max  *float64 `json:"max"`
}

func decode[T any](e model.Event) (T, error) {
	var p T
	if len(e.Payload) == 0 {
		return p, nil
	}
	if err := json.Unmarshal(e.Payload, &p); err != nil {
		return p, fmt.Errorf("event %d payload: %w", e.ID, err)
	}
	return p, nil
}

// lessonValues returns occurrence value_nums in ts order.
func lessonValues(evs []model.Event) []int {
	var out []int
	for _, e := range evs {
		if e.ValueNum != nil {
			out = append(out, int(*e.ValueNum))
		}
	}
	return out
}

func minMax(values []int) (int, int) {
	lo, hi := values[0], values[0]
	for _, v := range values[1:] {
		if v < lo {
			lo = v
		}
		if v > hi {
			hi = v
		}
	}
	return lo, hi
}

// gapLine builds "⚠ <display>: <actual> / <target> — <rule>" with the rule
// dropped when empty.
func gapLine(display, actual, target, rule string) string {
	line := fmt.Sprintf("⚠ %s: %s / %s", display, actual, target)
	if rule != "" {
		line += " — " + rule
	}
	return line
}
