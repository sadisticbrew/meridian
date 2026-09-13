package views

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/sadisticbrew/meridian/internal/model"
	"github.com/sadisticbrew/meridian/internal/render"
	"github.com/sadisticbrew/meridian/internal/store"
)

// Today renders the daily glance: running session first, then what was
// banked today, then the 7-day-zero gaps (spec 03).
func Today(w io.Writer, now time.Time, st *store.Store) error {
	dayFrom := localMidnight(now)
	from, to := utcRange(dayFrom, dayFrom.AddDate(0, 0, 1))
	gFrom, gTo := utcRange(now.Add(-7*24*time.Hour), now)

	header(w, "Today — "+now.Format("Mon Jan 2"))

	rs, err := st.RunningSession()
	if err != nil {
		return err
	}
	if rs != nil {
		started, err := time.Parse(time.RFC3339, rs.StartedAt)
		if err != nil {
			return fmt.Errorf("running session: invalid started_at %q: %w", rs.StartedAt, err)
		}
		fmt.Fprintf(w, "▶ %s — %s (running)\n\n",
			rs.Subject, render.Compact(int(now.UTC().Sub(started.UTC()).Seconds())))
	}

	active, err := st.SubjectsActive()
	if err != nil {
		return err
	}

	events, err := st.Events(store.EventFilter{From: from, To: to})
	if err != nil {
		return err
	}
	perSubject := map[string][]model.Event{}
	for _, e := range events {
		if e.Subject != nil {
			perSubject[*e.Subject] = append(perSubject[*e.Subject], e)
		}
	}

	var banked []string
	for _, t := range active {
		evs := perSubject[t.ID]
		minutes := sessionMinutes(evs)
		var detail string
		var bank bool
		switch t.Kind {
		case "project":
			detail, bank = "build", minutes > 0
		case "course", "self-study":
			detail, bank = "study", minutes > 0
		case "language":
			var derr error
			detail, bank, derr = languageDetail(st, t.ID, from, to)
			if derr != nil {
				return derr
			}
			bank = bank && (minutes > 0 || detail != "study")
		}
		if !bank {
			continue
		}
		// Today's banked minutes render as plain "Nm" per spec 03 ("65m").
		banked = append(banked, fmt.Sprintf("  %-15s %dm   %s", t.DisplayName, minutes, detail))
	}
	activePatterns := map[string]bool{}
	for _, t := range active {
		if t.Kind == "pattern" {
			activePatterns[t.ID] = true
		}
	}
	var dsaEvents []model.Event
	for _, e := range events {
		if e.Subject != nil && activePatterns[*e.Subject] {
			dsaEvents = append(dsaEvents, e)
		}
	}
	// Pattern rows carry no minutes column (spec 03: "DSA  2 solved (...)").
	if n := solvedCount(dsaEvents); n > 0 {
		banked = append(banked, fmt.Sprintf("  %-15s %d solved, %d/%d first-try (%s)",
			"DSA", n, firstTryCount(dsaEvents), n, strings.Join(solvedSlugs(dsaEvents), ", ")))
	}

	if len(banked) > 0 {
		fmt.Fprintln(w, "Banked")
		for _, line := range banked {
			fmt.Fprintln(w, line)
		}
	}

	gapEvents, err := st.Events(store.EventFilter{From: gFrom, To: gTo})
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, e := range gapEvents {
		if e.Subject != nil {
			seen[*e.Subject] = true
		}
	}
	var gaps []string
	for _, t := range active {
		if t.GoalJSON == "" || seen[t.ID] {
			continue
		}
		gaps = append(gaps, "  ⚠ "+t.DisplayName+": nothing in 7 days")
		if t.DecisionRule != "" {
			gaps[len(gaps)-1] += " — " + t.DecisionRule
		}
	}
	if len(gaps) > 0 {
		if len(banked) > 0 || rs != nil {
			fmt.Fprintln(w)
		}
		fmt.Fprintln(w, "Gaps")
		for _, line := range gaps {
			fmt.Fprintln(w, line)
		}
	}
	return nil
}

// languageDetail renders the lesson delta: previous lesson before today ->
// latest lesson today; "study" when only minutes were banked today.
func languageDetail(st *store.Store, subject, from, to string) (string, bool, error) {
	occs, err := st.OccurrencesForWindow(subject, from, to)
	if err != nil {
		return "", false, err
	}
	values := lessonValues(occs)
	if len(values) == 0 {
		return "study", false, nil
	}
	latest := values[len(values)-1]
	prev, err := st.LatestOccurrenceBefore(subject, from)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return "", false, err
	}
	if prev != nil && prev.ValueNum != nil && int(*prev.ValueNum) != latest {
		return fmt.Sprintf("lesson %d → %d", int(*prev.ValueNum), latest), true, nil
	}
	return fmt.Sprintf("lesson %d", latest), true, nil
}

func sessionMinutes(evs []model.Event) int {
	total := 0
	for _, e := range evs {
		if e.Type != "session" {
			continue
		}
		p, err := decode[struct {
			Minutes *float64 `json:"minutes"`
		}](e)
		if err != nil {
			continue
		}
		if p.Minutes != nil {
			total += int(*p.Minutes)
		}
	}
	return total
}
