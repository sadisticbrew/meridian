package views

import (
	"errors"
	"fmt"
	"io"
	"sort"
	"time"

	"github.com/guptarohit/asciigraph"
	"github.com/sadisticbrew/meridian/internal/model"
	"github.com/sadisticbrew/meridian/internal/render"
	"github.com/sadisticbrew/meridian/internal/store"
)

// Fixed plot geometry keeps golden output stable across runs.
const (
	graphWidth    = 50
	graphHeight   = 6
	graphBarWidth = 20
	germanThingID = "language/german"
)

var minuteKinds = map[string]bool{
	"project":    true,
	"course":     true,
	"self-study": true,
	"language":   true,
}

func graphDays(days int) int {
	if days < 1 {
		return 30
	}
	return days
}

// graphWindow returns the window start plus its UTC half-open range, matching
// Subject: [today-(days-1) local midnight, today+1d).
func graphWindow(now time.Time, days int) (time.Time, string, string) {
	today := localMidnight(now)
	first := today.AddDate(0, 0, -(days - 1))
	from, to := utcRange(first, today.AddDate(0, 0, 1))
	return first, from, to
}

func groupBySubject(events []model.Event) map[string][]model.Event {
	m := map[string][]model.Event{}
	for _, e := range events {
		if e.Subject != nil {
			m[*e.Subject] = append(m[*e.Subject], e)
		}
	}
	return m
}

// localDayIndex maps each local calendar day of the window to its offset.
func localDayIndex(first time.Time, days int) map[string]int {
	m := make(map[string]int, days)
	for i := 0; i < days; i++ {
		m[first.AddDate(0, 0, i).Format("2006-01-02")] = i
	}
	return m
}

func eventDay(e model.Event, idx map[string]int, loc *time.Location) (int, bool, error) {
	ts, err := time.Parse(time.RFC3339, e.Ts)
	if err != nil {
		return 0, false, fmt.Errorf("event %d ts %q: %w", e.ID, e.Ts, err)
	}
	i, ok := idx[ts.In(loc).Format("2006-01-02")]
	return i, ok, nil
}

func plotLine(w io.Writer, series []float64) {
	fmt.Fprintln(w, asciigraph.Plot(series,
		asciigraph.Width(graphWidth),
		asciigraph.Height(graphHeight)))
}

type graphRow struct {
	name  string
	count int
}

// GraphMinutes renders the daily minutes line for project/course/self-study/
// language sessions plus each active contributor's window total (phase 3).
func GraphMinutes(w io.Writer, now time.Time, st *store.Store, days int) error {
	days = graphDays(days)
	first, from, to := graphWindow(now, days)
	header(w, fmt.Sprintf("Minutes — last %d days", days))

	active, err := st.SubjectsActive()
	if err != nil {
		return err
	}
	events, err := st.Events(store.EventFilter{From: from, To: to})
	if err != nil {
		return err
	}
	loc := now.Location()
	idx := localDayIndex(first, days)
	perSubject := groupBySubject(events)

	daily := make([]float64, days)
	var rows []graphRow
	maxRow := 0
	for _, t := range active {
		if !minuteKinds[t.Kind] {
			continue
		}
		minutes := sessionMinutes(perSubject[t.ID])
		if minutes <= 0 {
			continue
		}
		if minutes > maxRow {
			maxRow = minutes
		}
		rows = append(rows, graphRow{name: t.DisplayName, count: minutes})

		perDay := make([][]model.Event, days)
		for _, e := range perSubject[t.ID] {
			if e.Type != "session" {
				continue
			}
			i, ok, err := eventDay(e, idx, loc)
			if err != nil {
				return err
			}
			if ok {
				perDay[i] = append(perDay[i], e)
			}
		}
		for i, evs := range perDay {
			daily[i] += float64(sessionMinutes(evs))
		}
	}

	plotLine(w, daily)
	if len(rows) > 0 {
		fmt.Fprintln(w)
		for _, r := range rows {
			fmt.Fprintf(w, "  %-15s %-9s %s\n", r.name, render.MinutesShort(r.count),
				render.Bar(r.count, maxRow, graphBarWidth))
		}
	}
	return nil
}

// GraphDSA renders the cumulative solves line, per-pattern solved counts and
// the window's first-try rate (phase 3).
func GraphDSA(w io.Writer, now time.Time, st *store.Store, days int) error {
	days = graphDays(days)
	first, from, to := graphWindow(now, days)
	header(w, fmt.Sprintf("DSA — last %d days", days))

	active, err := st.SubjectsActive()
	if err != nil {
		return err
	}
	events, err := st.Events(store.EventFilter{From: from, To: to})
	if err != nil {
		return err
	}
	loc := now.Location()
	idx := localDayIndex(first, days)
	perSubject := groupBySubject(events)

	byDay := make([][]model.Event, days)
	var rows []graphRow
	maxRow, solved, firstTry := 0, 0, 0
	for _, t := range active {
		if t.Kind != "pattern" {
			continue
		}
		evs := perSubject[t.ID]
		n := solvedCount(evs)
		solved += n
		firstTry += firstTryCount(evs)
		if n > 0 {
			rows = append(rows, graphRow{name: t.DisplayName, count: n})
			if n > maxRow {
				maxRow = n
			}
		}
		for _, e := range evs {
			if e.Type != "occurrence" {
				continue
			}
			i, ok, err := eventDay(e, idx, loc)
			if err != nil {
				return err
			}
			if ok {
				byDay[i] = append(byDay[i], e)
			}
		}
	}

	cumulative := make([]float64, days)
	total := 0
	for i, evs := range byDay {
		total += solvedCount(evs)
		cumulative[i] = float64(total)
	}
	plotLine(w, cumulative)

	if len(rows) > 0 {
		sort.Slice(rows, func(i, j int) bool {
			if rows[i].count != rows[j].count {
				return rows[i].count > rows[j].count
			}
			return rows[i].name < rows[j].name
		})
		fmt.Fprintln(w)
		for _, r := range rows {
			fmt.Fprintf(w, "  %-24s %-3d %s\n", r.name, r.count, render.Bar(r.count, maxRow, graphBarWidth))
		}
	}
	if solved > 0 {
		fmt.Fprintf(w, "  first-try: %d/%d\n", firstTry, solved)
	}
	return nil
}

// GraphGerman renders the running latest lesson number over the window. The
// latest lesson before the window seeds the line; asciigraph has no step
// renderer, so this is the sanctioned plain-line simplification (phase 3).
func GraphGerman(w io.Writer, now time.Time, st *store.Store, days int) error {
	days = graphDays(days)
	first, from, to := graphWindow(now, days)
	header(w, fmt.Sprintf("German — last %d days", days))

	current := 0
	prev, err := st.LatestOccurrenceBefore(germanThingID, from)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return err
	}
	if prev != nil && prev.ValueNum != nil {
		current = int(*prev.ValueNum)
	}

	events, err := st.Events(store.EventFilter{From: from, To: to, Subject: germanThingID})
	if err != nil {
		return err
	}
	loc := now.Location()
	idx := localDayIndex(first, days)
	perDay := make([]int, days)
	has := make([]bool, days)
	for _, e := range events {
		if e.Type != "occurrence" || e.ValueNum == nil {
			continue
		}
		i, ok, err := eventDay(e, idx, loc)
		if err != nil {
			return err
		}
		if ok {
			perDay[i] = int(*e.ValueNum)
			has[i] = true
		}
	}

	series := make([]float64, days)
	for i := range series {
		if has[i] {
			current = perDay[i]
		}
		series[i] = float64(current)
	}
	plotLine(w, series)
	return nil
}
