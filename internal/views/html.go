package views

import (
	"errors"
	"fmt"
	"html/template"
	"io"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/sadisticbrew/meridian/internal/model"
	"github.com/sadisticbrew/meridian/internal/render"
	"github.com/sadisticbrew/meridian/internal/store"
)

// SVG geometry is computed here, never in the template: html/template only
// substitutes precomputed numbers (phase 3 "no compute paths in templates").
const (
	htmlW       = 720
	htmlPadL    = 48
	htmlPadR    = 16
	htmlPadT    = 14
	htmlPadB    = 26
	htmlPlotW   = htmlW - htmlPadL - htmlPadR
	htmlPlotH   = 180
	htmlBarMaxW = 360
	htmlLabelW  = 230
	htmlRowH    = 42
	htmlDSARowH = 30
)

var htmlKinds = []struct {
	kind  string
	class string
}{
	{"project", "line line-project"},
	{"course", "line line-course"},
	{"self-study", "line line-self-study"},
	{"language", "line line-language"},
}

type htmlLine struct{ X1, Y1, X2, Y2 int }

type htmlText struct {
	X, Y   int
	Anchor string
	Text   string
}

type htmlRect struct{ X, Y, W, H int }

type htmlSeries struct {
	Class  string
	Points string
}

type htmlPlot struct {
	W, H   int
	Lines  []htmlLine
	YTicks []htmlText
	XTicks []htmlText
	Series []htmlSeries
}

type htmlWeeklyRow struct {
	Label   htmlText
	ThisBar htmlRect
	LastBar htmlRect
	ThisVal htmlText
	LastVal htmlText
}

type htmlWeekly struct {
	W, H      int
	Empty     bool
	ThisLabel string
	LastLabel string
	Rows      []htmlWeeklyRow
}

type htmlDSARow struct {
	Label htmlText
	Bar   htmlRect
	Count htmlText
}

type htmlDSA struct {
	W, H     int
	Empty    bool
	Rows     []htmlDSARow
	Solved   int
	FirstTry int
}

type htmlMilestone struct {
	Subject string
	Exam    string
	Score   string
	Date    string
}

type htmlData struct {
	Days       int
	FromLabel  string
	ToLabel    string
	Generated  string
	Minutes    htmlPlot
	Weekly     htmlWeekly
	DSA        htmlDSA
	German     htmlPlot
	Milestones []htmlMilestone
}

// HTML writes a single self-contained dark-theme page: no JavaScript, no
// external resource references, hand-rolled inline SVG (phase 3).
func HTML(w io.Writer, now time.Time, st *store.Store, days int) error {
	days = graphDays(days)
	first, from, to := graphWindow(now, days)

	active, err := st.SubjectsActive()
	if err != nil {
		return err
	}
	events, err := st.Events(store.EventFilter{From: from, To: to})
	if err != nil {
		return err
	}
	names, err := displayNames(st)
	if err != nil {
		return err
	}
	loc := now.Location()
	idx := localDayIndex(first, days)

	perKind, err := htmlDailyKinds(active, events, idx, loc, days)
	if err != nil {
		return err
	}
	weekly, err := htmlWeeklyBars(now, st, active)
	if err != nil {
		return err
	}
	german, err := htmlGermanStep(st, events, idx, loc, first, days, from)
	if err != nil {
		return err
	}
	milestones, err := htmlMilestones(events, names, loc)
	if err != nil {
		return err
	}

	data := htmlData{
		Days:       days,
		FromLabel:  first.Format("Jan 02"),
		ToLabel:    localMidnight(now).Format("Jan 02"),
		Generated:  now.Format("Jan 02 15:04"),
		Minutes:    htmlMinutesPlot(perKind, first, days),
		Weekly:     weekly,
		DSA:        htmlSolverBars(active, events),
		German:     german,
		Milestones: milestones,
	}
	t, err := template.New("export").Parse(htmlPage)
	if err != nil {
		return fmt.Errorf("parse html export template: %w", err)
	}
	if err := t.Execute(w, data); err != nil {
		return fmt.Errorf("render html export: %w", err)
	}
	return nil
}

func htmlX(i, days int) float64 {
	denom := days - 1
	if denom < 1 {
		denom = 1
	}
	return float64(htmlPadL) + float64(i)*float64(htmlPlotW)/float64(denom)
}

func htmlY(v, max float64) float64 {
	return float64(htmlPadT) + float64(htmlPlotH) - v/max*float64(htmlPlotH)
}

func svgPoints(xs, ys []float64) string {
	parts := make([]string, len(xs))
	for i := range xs {
		parts[i] = fmt.Sprintf("%.1f,%.1f", xs[i], ys[i])
	}
	return strings.Join(parts, " ")
}

// stepPoints emits horizontal-then-vertical segments so the polyline reads as
// a step function (SVG has no step renderer).
func stepPoints(xs, ys []float64) string {
	parts := make([]string, 0, 2*len(xs))
	for i := range xs {
		if i > 0 {
			parts = append(parts, fmt.Sprintf("%.1f,%.1f", xs[i], ys[i-1]))
		}
		parts = append(parts, fmt.Sprintf("%.1f,%.1f", xs[i], ys[i]))
	}
	return strings.Join(parts, " ")
}

func htmlAxes(maxV float64, first time.Time, days int) ([]htmlLine, []htmlText, []htmlText) {
	var lines []htmlLine
	for _, frac := range []float64{0, 0.5, 1} {
		y := htmlPadT + int(math.Round((1-frac)*float64(htmlPlotH)))
		lines = append(lines, htmlLine{X1: htmlPadL, Y1: y, X2: htmlPadL + htmlPlotW, Y2: y})
	}
	yTicks := []htmlText{
		{X: htmlPadL - 6, Y: htmlPadT + 4, Anchor: "end", Text: fmt.Sprintf("%d", int(maxV))},
		{X: htmlPadL - 6, Y: htmlPadT + htmlPlotH + 4, Anchor: "end", Text: "0"},
	}
	xTicks := []htmlText{{X: htmlPadL, Y: htmlPadT + htmlPlotH + 18, Anchor: "start", Text: first.Format("Jan 02")}}
	if days > 1 {
		xTicks = append(xTicks, htmlText{
			X: htmlPadL + htmlPlotW, Y: htmlPadT + htmlPlotH + 18, Anchor: "end",
			Text: first.AddDate(0, 0, days-1).Format("Jan 02"),
		})
	}
	return lines, yTicks, xTicks
}

func htmlMinutesPlot(perKind [][]float64, first time.Time, days int) htmlPlot {
	maxV := 0.0
	for _, series := range perKind {
		for _, v := range series {
			if v > maxV {
				maxV = v
			}
		}
	}
	plot := htmlPlot{W: htmlW, H: htmlPadT + htmlPlotH + htmlPadB}
	plot.Lines, plot.YTicks, plot.XTicks = htmlAxes(maxV, first, days)
	scale := maxV
	if scale <= 0 {
		scale = 1
	}
	xs := make([]float64, days)
	for i := range xs {
		xs[i] = htmlX(i, days)
	}
	for i, series := range perKind {
		ys := make([]float64, days)
		for d, v := range series {
			ys[d] = htmlY(v, scale)
		}
		plot.Series = append(plot.Series, htmlSeries{Class: htmlKinds[i].class, Points: svgPoints(xs, ys)})
	}
	return plot
}

func htmlDailyKinds(active []model.Thing, events []model.Event, idx map[string]int, loc *time.Location, days int) ([][]float64, error) {
	kindIndex := map[string]int{}
	for i, k := range htmlKinds {
		kindIndex[k.kind] = i
	}
	subjectKind := map[string]int{}
	for _, t := range active {
		if i, ok := kindIndex[t.Kind]; ok {
			subjectKind[t.ID] = i
		}
	}
	perDay := make([][]float64, len(htmlKinds))
	for i := range perDay {
		perDay[i] = make([]float64, days)
	}
	for _, e := range events {
		if e.Type != "session" || e.Subject == nil {
			continue
		}
		ki, ok := subjectKind[*e.Subject]
		if !ok {
			continue
		}
		di, ok, err := eventDay(e, idx, loc)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		p, err := decode[struct {
			Minutes *float64 `json:"minutes"`
		}](e)
		if err != nil || p.Minutes == nil {
			continue
		}
		perDay[ki][di] += *p.Minutes
	}
	return perDay, nil
}

type htmlWeeklyRaw struct {
	label      string
	this, last int
}

func htmlWeeklyBars(now time.Time, st *store.Store, active []model.Thing) (htmlWeekly, error) {
	thisFrom, thisTo := mondayStart(now)
	lastFrom := thisFrom.AddDate(0, 0, -7)
	lastTo := thisTo.AddDate(0, 0, -7)
	tf, tt := utcRange(thisFrom, thisTo)
	lf, lt := utcRange(lastFrom, lastTo)
	thisEvents, err := st.Events(store.EventFilter{From: tf, To: tt})
	if err != nil {
		return htmlWeekly{}, err
	}
	lastEvents, err := st.Events(store.EventFilter{From: lf, To: lt})
	if err != nil {
		return htmlWeekly{}, err
	}
	thisBy := groupBySubject(thisEvents)
	lastBy := groupBySubject(lastEvents)

	var raw []htmlWeeklyRaw
	for _, t := range active {
		if !minuteKinds[t.Kind] {
			continue
		}
		raw = append(raw, htmlWeeklyRaw{
			label: t.DisplayName,
			this:  sessionMinutes(thisBy[t.ID]),
			last:  sessionMinutes(lastBy[t.ID]),
		})
	}
	// Wins-first: any activity outranks an empty row, then total desc.
	sort.SliceStable(raw, func(i, j int) bool {
		ti, tj := raw[i].this+raw[i].last, raw[j].this+raw[j].last
		if ti != tj {
			return ti > tj
		}
		if raw[i].this != raw[j].this {
			return raw[i].this > raw[j].this
		}
		return raw[i].label < raw[j].label
	})

	out := htmlWeekly{
		W:         htmlW,
		Empty:     len(raw) == 0,
		ThisLabel: weekLabel(thisFrom),
		LastLabel: weekLabel(lastFrom),
	}
	maxV := 0
	for _, r := range raw {
		if r.this > maxV {
			maxV = r.this
		}
		if r.last > maxV {
			maxV = r.last
		}
	}
	out.H = htmlPadT + len(raw)*htmlRowH + htmlPadB
	if len(raw) == 0 {
		out.H = 64
	}
	for i, r := range raw {
		y := htmlPadT + i*htmlRowH
		thisW := htmlBarWidth(r.this, maxV)
		lastW := htmlBarWidth(r.last, maxV)
		out.Rows = append(out.Rows, htmlWeeklyRow{
			Label:   htmlText{X: 0, Y: y + 24, Anchor: "start", Text: r.label},
			ThisBar: htmlRect{X: htmlLabelW, Y: y + 6, W: thisW, H: 12},
			LastBar: htmlRect{X: htmlLabelW, Y: y + 22, W: lastW, H: 12},
			ThisVal: htmlText{X: htmlLabelW + thisW + 6, Y: y + 16, Anchor: "start", Text: render.MinutesShort(r.this)},
			LastVal: htmlText{X: htmlLabelW + lastW + 6, Y: y + 32, Anchor: "start", Text: render.MinutesShort(r.last)},
		})
	}
	return out, nil
}

func weekLabel(from time.Time) string {
	return from.Format("Jan 02") + "–" + from.AddDate(0, 0, 6).Format("Jan 02")
}

func htmlBarWidth(v, max int) int {
	if v <= 0 || max <= 0 {
		return 0
	}
	w := int(math.Round(float64(v) / float64(max) * float64(htmlBarMaxW)))
	if w < 1 {
		w = 1
	}
	return w
}

func htmlSolverBars(active []model.Thing, events []model.Event) htmlDSA {
	bySubject := groupBySubject(events)
	type rawRow struct {
		label string
		count int
	}
	var raw []rawRow
	solved, firstTry := 0, 0
	for _, t := range active {
		if t.Kind != "pattern" {
			continue
		}
		evs := bySubject[t.ID]
		n := solvedCount(evs)
		solved += n
		firstTry += firstTryCount(evs)
		raw = append(raw, rawRow{label: t.DisplayName, count: n})
	}
	sort.SliceStable(raw, func(i, j int) bool {
		if raw[i].count != raw[j].count {
			return raw[i].count > raw[j].count
		}
		return raw[i].label < raw[j].label
	})

	out := htmlDSA{
		W:        htmlW,
		Empty:    len(raw) == 0,
		Solved:   solved,
		FirstTry: firstTry,
	}
	maxV := 0
	for _, r := range raw {
		if r.count > maxV {
			maxV = r.count
		}
	}
	out.H = htmlPadT + len(raw)*htmlDSARowH + htmlPadB
	if len(raw) == 0 {
		out.H = 64
	}
	for i, r := range raw {
		y := htmlPadT + i*htmlDSARowH
		w := htmlBarWidth(r.count, maxV)
		out.Rows = append(out.Rows, htmlDSARow{
			Label: htmlText{X: 0, Y: y + 19, Anchor: "start", Text: r.label},
			Bar:   htmlRect{X: htmlLabelW, Y: y + 6, W: w, H: 14},
			Count: htmlText{X: htmlLabelW + w + 6, Y: y + 18, Anchor: "start", Text: fmt.Sprintf("%d", r.count)},
		})
	}
	return out
}

func htmlGermanStep(st *store.Store, events []model.Event, idx map[string]int, loc *time.Location, first time.Time, days int, from string) (htmlPlot, error) {
	current := 0
	prev, err := st.LatestOccurrenceBefore(germanThingID, from)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return htmlPlot{}, err
	}
	if prev != nil && prev.ValueNum != nil {
		current = int(*prev.ValueNum)
	}
	perDay := make([]int, days)
	has := make([]bool, days)
	for _, e := range events {
		if e.Type != "occurrence" || e.Subject == nil || *e.Subject != germanThingID || e.ValueNum == nil {
			continue
		}
		i, ok, err := eventDay(e, idx, loc)
		if err != nil {
			return htmlPlot{}, err
		}
		if ok {
			perDay[i] = int(*e.ValueNum)
			has[i] = true
		}
	}
	values := make([]float64, days)
	for i := range values {
		if has[i] {
			current = perDay[i]
		}
		values[i] = float64(current)
	}
	maxV := 0.0
	for _, v := range values {
		if v > maxV {
			maxV = v
		}
	}
	plot := htmlPlot{W: htmlW, H: htmlPadT + htmlPlotH + htmlPadB}
	plot.Lines, plot.YTicks, plot.XTicks = htmlAxes(maxV, first, days)
	scale := maxV
	if scale <= 0 {
		scale = 1
	}
	xs := make([]float64, days)
	ys := make([]float64, days)
	for i, v := range values {
		xs[i] = htmlX(i, days)
		ys[i] = htmlY(v, scale)
	}
	plot.Series = []htmlSeries{{Class: "line line-german", Points: stepPoints(xs, ys)}}
	return plot, nil
}

func htmlMilestones(events []model.Event, names map[string]string, loc *time.Location) ([]htmlMilestone, error) {
	type row struct {
		htmlMilestone
		ts time.Time
		id int64
	}
	var rows []row
	for _, e := range events {
		if e.Type != "milestone" || e.Subject == nil || e.ValueNum == nil {
			continue
		}
		p, err := decode[milestonePayload](e)
		if err != nil {
			continue
		}
		ts, err := time.Parse(time.RFC3339, e.Ts)
		if err != nil {
			return nil, fmt.Errorf("event %d ts %q: %w", e.ID, e.Ts, err)
		}
		display := names[*e.Subject]
		if display == "" {
			display = *e.Subject
		}
		score := fmt.Sprintf("%d", int(*e.ValueNum))
		if p.Max != nil {
			score += fmt.Sprintf("/%d", int(*p.Max))
		}
		rows = append(rows, row{
			htmlMilestone: htmlMilestone{Subject: display, Exam: p.Exam, Score: score, Date: ts.In(loc).Format("Jan 02")},
			ts:            ts,
			id:            e.ID,
		})
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if !rows[i].ts.Equal(rows[j].ts) {
			return rows[i].ts.After(rows[j].ts)
		}
		return rows[i].id > rows[j].id
	})
	out := make([]htmlMilestone, len(rows))
	for i, r := range rows {
		out[i] = r.htmlMilestone
	}
	return out, nil
}

const htmlPage = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>meridian export</title>
<style>
:root{color-scheme:dark}
body{background:#16161e;color:#c0caf5;font:14px/1.5 ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;margin:0;padding:24px}
main{max-width:1000px}
h1{font-size:18px;margin:0 0 2px}
h2{font-size:13px;text-transform:uppercase;letter-spacing:.08em;color:#7aa2f7;margin:0 0 12px}
.meta{color:#565f89;margin:0 0 28px}
section{background:#1a1b26;border:1px solid #292e42;border-radius:6px;padding:16px;margin:0 0 20px;overflow-x:auto}
svg{display:block;max-width:100%;height:auto}
.grid{stroke:#292e42;stroke-width:1}
.tick{fill:#565f89;font-size:11px}
.label{fill:#c0caf5;font-size:12px}
.value{fill:#565f89;font-size:11px}
polyline{fill:none;stroke-width:2;stroke-linejoin:round;stroke-linecap:round}
.line-project{stroke:#7aa2f7}
.line-course{stroke:#9ece6a}
.line-self-study{stroke:#e0af68}
.line-language{stroke:#bb9af7}
.line-german{stroke:#bb9af7}
.bar-this{fill:#7aa2f7}
.bar-last{fill:#3b4261}
.bar-solve{fill:#9ece6a}
.legend{list-style:none;display:flex;flex-wrap:wrap;gap:16px;padding:0;margin:10px 0 0}
.legend li{display:flex;align-items:center;gap:6px;color:#565f89}
.swatch{width:12px;height:12px;border-radius:2px;display:inline-block}
.swatch-project{background:#7aa2f7}
.swatch-course{background:#9ece6a}
.swatch-self-study{background:#e0af68}
.swatch-language{background:#bb9af7}
.swatch-this{background:#7aa2f7}
.swatch-last{background:#3b4261}
.rate{color:#9ece6a;margin:12px 0 0}
table{border-collapse:collapse;width:100%}
th,td{text-align:left;padding:6px 10px;border-bottom:1px solid #292e42}
th{color:#565f89;font-weight:400;text-transform:uppercase;font-size:11px;letter-spacing:.06em}
td.num{color:#e0af68}
.muted{fill:#565f89;color:#565f89;font-size:12px}
</style>
</head>
<body>
<header>
<h1>meridian</h1>
<p class="meta">last {{.Days}} days · {{.FromLabel}} → {{.ToLabel}} · generated {{.Generated}}</p>
</header>
<main>
<section id="chart-minutes">
<h2>Daily minutes</h2>
<svg viewBox="0 0 {{.Minutes.W}} {{.Minutes.H}}" role="img" aria-label="daily minutes by kind">
{{range .Minutes.Lines}}<line class="grid" x1="{{.X1}}" y1="{{.Y1}}" x2="{{.X2}}" y2="{{.Y2}}"/>
{{end}}{{range .Minutes.YTicks}}<text class="tick" x="{{.X}}" y="{{.Y}}" text-anchor="{{.Anchor}}">{{.Text}}</text>
{{end}}{{range .Minutes.Series}}<polyline class="{{.Class}}" points="{{.Points}}"/>
{{end}}{{range .Minutes.XTicks}}<text class="tick" x="{{.X}}" y="{{.Y}}" text-anchor="{{.Anchor}}">{{.Text}}</text>
{{end}}</svg>
<ul class="legend">
<li><span class="swatch swatch-project"></span>project</li>
<li><span class="swatch swatch-course"></span>course</li>
<li><span class="swatch swatch-self-study"></span>self-study</li>
<li><span class="swatch swatch-language"></span>language</li>
</ul>
</section>
<section id="chart-weekly">
<h2>This week vs last week</h2>
<svg viewBox="0 0 {{.Weekly.W}} {{.Weekly.H}}" role="img" aria-label="this week versus last week minutes">
{{range .Weekly.Rows}}<text class="label" x="{{.Label.X}}" y="{{.Label.Y}}" text-anchor="{{.Label.Anchor}}">{{.Label.Text}}</text>
<rect class="bar-this" x="{{.ThisBar.X}}" y="{{.ThisBar.Y}}" width="{{.ThisBar.W}}" height="{{.ThisBar.H}}"/>
<rect class="bar-last" x="{{.LastBar.X}}" y="{{.LastBar.Y}}" width="{{.LastBar.W}}" height="{{.LastBar.H}}"/>
<text class="value" x="{{.ThisVal.X}}" y="{{.ThisVal.Y}}" text-anchor="{{.ThisVal.Anchor}}">{{.ThisVal.Text}}</text>
<text class="value" x="{{.LastVal.X}}" y="{{.LastVal.Y}}" text-anchor="{{.LastVal.Anchor}}">{{.LastVal.Text}}</text>
{{end}}{{if .Weekly.Empty}}<text class="muted" x="0" y="24">no active subjects</text>
{{end}}</svg>
<ul class="legend">
<li><span class="swatch swatch-this"></span>this week {{.Weekly.ThisLabel}}</li>
<li><span class="swatch swatch-last"></span>last week {{.Weekly.LastLabel}}</li>
</ul>
</section>
<section id="chart-dsa">
<h2>DSA solves by pattern</h2>
<svg viewBox="0 0 {{.DSA.W}} {{.DSA.H}}" role="img" aria-label="DSA solves by pattern">
{{range .DSA.Rows}}<text class="label" x="{{.Label.X}}" y="{{.Label.Y}}" text-anchor="{{.Label.Anchor}}">{{.Label.Text}}</text>
<rect class="bar-solve" x="{{.Bar.X}}" y="{{.Bar.Y}}" width="{{.Bar.W}}" height="{{.Bar.H}}"/>
<text class="value" x="{{.Count.X}}" y="{{.Count.Y}}" text-anchor="{{.Count.Anchor}}">{{.Count.Text}}</text>
{{end}}{{if .DSA.Empty}}<text class="muted" x="0" y="24">no active patterns</text>
{{end}}</svg>
<p class="rate">first-try: {{.DSA.FirstTry}}/{{.DSA.Solved}}</p>
</section>
<section id="chart-german">
<h2>German lesson progression</h2>
<svg viewBox="0 0 {{.German.W}} {{.German.H}}" role="img" aria-label="German lesson progression">
{{range .German.Lines}}<line class="grid" x1="{{.X1}}" y1="{{.Y1}}" x2="{{.X2}}" y2="{{.Y2}}"/>
{{end}}{{range .German.YTicks}}<text class="tick" x="{{.X}}" y="{{.Y}}" text-anchor="{{.Anchor}}">{{.Text}}</text>
{{end}}{{range .German.Series}}<polyline class="{{.Class}}" points="{{.Points}}"/>
{{end}}{{range .German.XTicks}}<text class="tick" x="{{.X}}" y="{{.Y}}" text-anchor="{{.Anchor}}">{{.Text}}</text>
{{end}}</svg>
</section>
<section id="table-milestones">
<h2>Milestones</h2>
{{if .Milestones}}<table>
<thead><tr><th>subject</th><th>exam</th><th>score</th><th>date</th></tr></thead>
<tbody>
{{range .Milestones}}<tr><td>{{.Subject}}</td><td>{{.Exam}}</td><td class="num">{{.Score}}</td><td>{{.Date}}</td></tr>
{{end}}</tbody>
</table>
{{else}}<p class="muted">no milestones in this window</p>
{{end}}</section>
</main>
</body>
</html>
`
