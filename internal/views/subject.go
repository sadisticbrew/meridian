package views

import (
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/sadisticbrew/meridian/internal/model"
	"github.com/sadisticbrew/meridian/internal/render"
	"github.com/sadisticbrew/meridian/internal/store"
)

type dayRow struct {
	label   string
	minutes int
}

// Subject renders a per-thing review: minute bars per day over the window,
// then occurrence/milestone history (spec 03).
func Subject(w io.Writer, now time.Time, st *store.Store, id string, days int) error {
	t, err := st.Thing(id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return fmt.Errorf("unknown subject %q", id)
		}
		return err
	}
	if days <= 0 {
		days = 14
	}

	header(w, t.ID+" — "+t.DisplayName)

	today := localMidnight(now)
	firstDay := today.AddDate(0, 0, -(days - 1))
	winFrom, winTo := utcRange(firstDay, today.AddDate(0, 0, 1))

	goal, err := decodeGoal(t.GoalJSON)
	if err != nil {
		return err
	}
	minutes, err := st.MinutesForWindow(t.ID, winFrom, winTo)
	if err != nil {
		return err
	}
	occurrences, err := st.OccurrencesForWindow(t.ID, winFrom, winTo)
	if err != nil {
		return err
	}
	switch {
	case goal.WeeklyMinutes != nil:
		fmt.Fprintf(w, "goal %dm/week — last %d days: %s\n\n", *goal.WeeklyMinutes, days, render.Minutes(minutes))
	case goal.OccurrencesPerWeek != nil:
		fmt.Fprintf(w, "goal %d/week — last %d days: %d occurrences\n\n", *goal.OccurrencesPerWeek, days, len(occurrences))
	default:
		fmt.Fprintf(w, "no goal — last %d days\n\n", days)
	}

	var rows []dayRow
	var maxMinutes int
	for i := 0; i < days; i++ {
		day := firstDay.AddDate(0, 0, i)
		from, to := utcRange(day, day.AddDate(0, 0, 1))
		dayMinutes, err := st.MinutesForWindow(t.ID, from, to)
		if err != nil {
			return err
		}
		if dayMinutes > maxMinutes {
			maxMinutes = dayMinutes
		}
		rows = append(rows, dayRow{label: day.Format("Jan 02"), minutes: dayMinutes})
	}
	for _, r := range rows {
		fmt.Fprintf(w, "  %-7s  %-10s %s\n", r.label, render.Bar(r.minutes, maxMinutes, 10), render.MinutesShort(r.minutes))
	}

	evs, err := st.Events(store.EventFilter{From: winFrom, To: winTo, Subject: t.ID})
	if err != nil {
		return err
	}
	if t.Kind == "pattern" {
		return problemsTable(w, now, evs, days)
	}
	var history []string
	for _, e := range evs {
		var detail string
		switch e.Type {
		case "occurrence":
			if e.ValueNum != nil {
				detail = fmt.Sprintf("lesson %d", int(*e.ValueNum))
			} else {
				p, err := decode[occurrencePayload](e)
				if err != nil {
					return err
				}
				detail = p.Problem + " " + p.Outcome
			}
		case "milestone":
			if e.ValueNum == nil {
				continue
			}
			p, err := decode[milestonePayload](e)
			if err != nil {
				return err
			}
			detail = fmt.Sprintf("%s: %d", p.Exam, int(*e.ValueNum))
			if p.Max != nil {
				detail += fmt.Sprintf("/%d", int(*p.Max))
			}
		default:
			continue
		}
		ts, err := time.Parse(time.RFC3339, e.Ts)
		if err != nil {
			return fmt.Errorf("event %d ts %q: %w", e.ID, e.Ts, err)
		}
		history = append(history, fmt.Sprintf("  %-10s %-16s %s",
			e.Type, detail, ts.In(now.Location()).Format("Jan 2 15:04")))
	}
	if len(history) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "History")
		for _, line := range history {
			fmt.Fprintln(w, line)
		}
	}
	return nil
}

// problemsTable renders the per-problem pattern view: passive neetcode solves
// and manual reviews/struggles in ts order.
func problemsTable(w io.Writer, now time.Time, evs []model.Event, days int) error {
	var rows []string
	for _, e := range evs {
		if e.Type != "occurrence" {
			continue
		}
		var slug, attempts, outcome string
		if e.Source == "neetcode" {
			p, err := decode[passivePayload](e)
			if err != nil {
				return err
			}
			slug, outcome = p.Slug, "solved"
			if p.Attempts == nil {
				attempts = "-"
			} else {
				attempts = fmt.Sprintf("%d", int(*p.Attempts))
			}
		} else {
			p, err := decode[occurrencePayload](e)
			if err != nil {
				return err
			}
			slug, attempts, outcome = p.Problem, "-", p.Outcome
		}
		ts, err := time.Parse(time.RFC3339, e.Ts)
		if err != nil {
			return fmt.Errorf("event %d ts %q: %w", e.ID, e.Ts, err)
		}
		rows = append(rows, fmt.Sprintf("  %-24s %-8s %-9s %s",
			slug, attempts, outcome, ts.In(now.Location()).Format("Jan 2")))
	}
	if len(rows) == 0 {
		return nil
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Problems (last %d days)\n", days)
	fmt.Fprintf(w, "  %-24s %-8s %-9s %s\n", "slug", "attempts", "outcome", "date")
	for _, row := range rows {
		fmt.Fprintln(w, row)
	}
	return nil
}
