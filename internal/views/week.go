package views

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/sadisticbrew/meridian/internal/model"
	"github.com/sadisticbrew/meridian/internal/render"
	"github.com/sadisticbrew/meridian/internal/store"
)

type aggregate struct {
	total int
	parts []string // "<Name> <minutes short>" per contributing thing
}

func addMinutes(a *aggregate, name string, minutes int) {
	if minutes <= 0 {
		return
	}
	a.total += minutes
	a.parts = append(a.parts, fmt.Sprintf("%s %s", name, render.MinutesShort(minutes)))
}

// aggregateLine renders "  <Label>        <4h 05m>   (parts)" per spec 03.
func aggregateLine(label string, a aggregate) (string, bool) {
	if a.total <= 0 {
		return "", false
	}
	line := fmt.Sprintf("  %-15s %s", label, render.Minutes(a.total))
	if len(a.parts) > 1 {
		line += "   (" + strings.Join(a.parts, ", ") + ")"
	}
	return line, true
}

// Week renders the Monday-start weekly review: wins first, then goal
// shortfalls, then the worst-pattern focus line (spec 02/03).
func Week(w io.Writer, now time.Time, st *store.Store, last bool) error {
	fromT, toT := mondayStart(now)
	if last {
		fromT = fromT.AddDate(0, 0, -7)
		toT = toT.AddDate(0, 0, -7)
	}
	from, to := utcRange(fromT, toT)

	header(w, "Week — "+fromT.Format("Jan 02")+" → "+fromT.AddDate(0, 0, 6).Format("Jan 02"))

	active, err := st.SubjectsActive()
	if err != nil {
		return err
	}
	byKind := map[string][]model.Thing{}
	for _, t := range active {
		byKind[t.Kind] = append(byKind[t.Kind], t)
	}

	events, err := st.Events(store.EventFilter{From: from, To: to})
	if err != nil {
		return err
	}
	names, err := displayNames(st)
	if err != nil {
		return err
	}
	perSubject := map[string][]model.Event{}
	for _, e := range events {
		if e.Subject != nil {
			perSubject[*e.Subject] = append(perSubject[*e.Subject], e)
		}
	}

	// Done section — aggregate totals per kind group, per-thing breakdowns.
	project, courses := aggregate{}, aggregate{}
	for _, t := range active {
		minutes := sessionMinutes(perSubject[t.ID])
		switch t.Kind {
		case "project":
			addMinutes(&project, t.DisplayName, minutes)
		case "course", "self-study":
			addMinutes(&courses, t.DisplayName, minutes)
		}
	}
	var done []string
	if line, ok := aggregateLine("Project", project); ok {
		done = append(done, line)
	}
	if line, ok := aggregateLine("Courses", courses); ok {
		done = append(done, line)
	}
	for _, t := range byKind["language"] {
		if line := languageLine(t, perSubject[t.ID]); line != "" {
			done = append(done, line)
		}
	}
	solved := 0
	for _, t := range byKind["pattern"] {
		solved += countOutcome(perSubject[t.ID], "solved")
	}
	if solved > 0 {
		done = append(done, fmt.Sprintf("  %-15s %d solved", "DSA", solved))
	}
	if parts := scoreParts(events, names); len(parts) > 0 {
		done = append(done, fmt.Sprintf("  %-15s %s", "Scores", strings.Join(parts, ", ")))
	}
	if len(done) > 0 {
		fmt.Fprintln(w, "Done")
		for _, line := range done {
			fmt.Fprintln(w, line)
		}
	}

	// Gaps — goal shortfalls for active, non-archived subjects (spec 02).
	var gaps []string
	for _, t := range active {
		if t.GoalJSON == "" {
			continue
		}
		goal, err := decodeGoal(t.GoalJSON)
		if err != nil {
			return err
		}
		if goal.WeeklyMinutes != nil {
			minutes := sessionMinutes(perSubject[t.ID])
			if minutes < *goal.WeeklyMinutes {
				gaps = append(gaps, "  "+gapLine(t.DisplayName,
					render.Minutes(minutes), fmt.Sprintf("%dm", *goal.WeeklyMinutes), t.DecisionRule))
			}
		}
		if goal.OccurrencesPerWeek != nil {
			n := countOccurrences(perSubject[t.ID])
			if n < *goal.OccurrencesPerWeek {
				gaps = append(gaps, "  "+gapLine(t.DisplayName,
					fmt.Sprintf("%d", n), fmt.Sprintf("%d", *goal.OccurrencesPerWeek), t.DecisionRule))
			}
		}
	}
	if len(gaps) > 0 {
		if len(done) > 0 {
			fmt.Fprintln(w)
		}
		fmt.Fprintln(w, "Gaps")
		for _, line := range gaps {
			fmt.Fprintln(w, line)
		}
	}

	// Focus next week — worst (reviews + struggles) vs solves balance.
	if line := focusLine(byKind["pattern"], perSubject); line != "" {
		if len(done) > 0 || len(gaps) > 0 {
			fmt.Fprintln(w)
		}
		fmt.Fprintln(w, "Focus next week")
		fmt.Fprintln(w, line)
	}
	return nil
}

type goalValues struct {
	WeeklyMinutes      *int `json:"weekly_minutes"`
	OccurrencesPerWeek *int `json:"occurrences_per_week"`
}

func decodeGoal(raw string) (goalValues, error) {
	var g goalValues
	if raw == "" {
		return g, nil
	}
	if err := json.Unmarshal([]byte(raw), &g); err != nil {
		return g, fmt.Errorf("goal_json %q: %w", raw, err)
	}
	return g, nil
}

// languageLine renders one language thing: minutes plus lesson progress.
func languageLine(t model.Thing, evs []model.Event) string {
	minutes := sessionMinutes(evs)
	values := lessonValues(eventsOfType(evs, "occurrence"))
	if minutes == 0 && len(values) == 0 {
		return ""
	}
	var parts []string
	if minutes > 0 {
		parts = append(parts, render.Minutes(minutes))
	}
	if len(values) > 0 {
		lo, hi := minMax(values)
		if lo == hi {
			parts = append(parts, fmt.Sprintf("lesson %d", lo))
		} else {
			parts = append(parts, fmt.Sprintf("lessons %d → %d", lo, hi))
		}
	}
	return fmt.Sprintf("  %-15s %s", t.DisplayName, strings.Join(parts, ", "))
}

func eventsOfType(evs []model.Event, typ string) []model.Event {
	var out []model.Event
	for _, e := range evs {
		if e.Type == typ {
			out = append(out, e)
		}
	}
	return out
}

func countOutcome(evs []model.Event, outcome string) int {
	n := 0
	for _, e := range evs {
		if e.Type != "occurrence" {
			continue
		}
		p, err := decode[occurrencePayload](e)
		if err != nil {
			continue
		}
		if p.Outcome == outcome {
			n++
		}
	}
	return n
}

func countOccurrences(evs []model.Event) int {
	n := 0
	for _, e := range evs {
		if e.Type == "occurrence" {
			n++
		}
	}
	return n
}

// scoreParts renders "<display> <exam>: <score>[/<max>]" per milestone.
func scoreParts(evs []model.Event, names map[string]string) []string {
	var parts []string
	for _, e := range eventsOfType(evs, "milestone") {
		if e.Subject == nil || e.ValueNum == nil {
			continue
		}
		p, err := decode[milestonePayload](e)
		if err != nil {
			continue
		}
		display := names[*e.Subject]
		if display == "" {
			display = *e.Subject
		}
		part := fmt.Sprintf("%s %s: %d", display, p.Exam, int(*e.ValueNum))
		if p.Max != nil {
			part += fmt.Sprintf("/%d", int(*p.Max))
		}
		parts = append(parts, part)
	}
	return parts
}

// focusLine picks the pattern with the worst (reviews + struggles) vs
// solves balance; score > 0 means unhealthy (spec 03).
func focusLine(patterns []model.Thing, perSubject map[string][]model.Event) string {
	type candidate struct {
		thing    model.Thing
		reviews  int
		struggle int
		solves   int
		score    int
	}
	var best *candidate
	for _, t := range patterns {
		evs := eventsOfType(perSubject[t.ID], "occurrence")
		c := candidate{
			thing:    t,
			reviews:  countOutcome(evs, "reviewed"),
			struggle: countOutcome(evs, "struggled"),
			solves:   countOutcome(evs, "solved"),
		}
		c.score = c.reviews + c.struggle - c.solves
		if c.score <= 0 {
			continue
		}
		if best == nil || c.score > best.score ||
			(c.score == best.score && c.thing.ID < best.thing.ID) {
			best = &c
		}
	}
	if best == nil {
		return ""
	}
	rule := best.thing.DecisionRule
	line := fmt.Sprintf("  %s — %s, %s, %s",
		best.thing.DisplayName,
		render.Plural(best.reviews, "review"),
		render.Plural(best.struggle, "struggle"),
		render.Plural(best.solves, "solve"))
	if rule != "" {
		line += " → " + rule
	}
	return line
}
