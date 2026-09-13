package views

import (
	"fmt"
	"io"
	"sort"
	"time"

	"github.com/sadisticbrew/meridian/internal/store"
)

type patternRow struct {
	name      string
	solved    int
	firstTry  int
	reviewed  int
	struggled int
}

// DSA renders one row per active pattern with activity in the window: solved,
// first-try, reviewed and struggled counts (no minutes — patterns carry
// occurrences only), sorted by solved desc then name.
func DSA(w io.Writer, now time.Time, st *store.Store, days int) error {
	if days <= 0 {
		days = 14
	}
	today := localMidnight(now)
	first := today.AddDate(0, 0, -(days - 1))
	from, to := utcRange(first, today.AddDate(0, 0, 1))

	header(w, fmt.Sprintf("DSA — last %d days", days))

	active, err := st.SubjectsActive()
	if err != nil {
		return err
	}
	events, err := st.Events(store.EventFilter{From: from, To: to})
	if err != nil {
		return err
	}
	perSubject := groupBySubject(events)

	var rows []patternRow
	for _, t := range active {
		if t.Kind != "pattern" {
			continue
		}
		evs := perSubject[t.ID]
		if len(evs) == 0 {
			continue
		}
		rows = append(rows, patternRow{
			name:      t.DisplayName,
			solved:    solvedCount(evs),
			firstTry:  firstTryCount(evs),
			reviewed:  countOutcome(eventsOfType(evs, "occurrence"), "reviewed"),
			struggled: countOutcome(eventsOfType(evs, "occurrence"), "struggled"),
		})
	}
	if len(rows) == 0 {
		return nil
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].solved != rows[j].solved {
			return rows[i].solved > rows[j].solved
		}
		return rows[i].name < rows[j].name
	})

	fmt.Fprintf(w, "  %-24s %6s %9s %8s %10s\n", "pattern", "solved", "first-try", "reviewed", "struggled")
	for _, r := range rows {
		fmt.Fprintf(w, "  %-24s %6d %9d %8d %10d\n", r.name, r.solved, r.firstTry, r.reviewed, r.struggled)
	}
	return nil
}
