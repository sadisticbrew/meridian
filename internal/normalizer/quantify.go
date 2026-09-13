// Package normalizer turns note events into validated annotations and derived
// events (spec 04 "Normalizer").
package normalizer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/sadisticbrew/meridian/internal/model"
	"github.com/sadisticbrew/meridian/internal/store"
)

// systemPrompt is copied verbatim from spec 04 "Prompt contract"; <VOCAB> is
// replaced with the registry-derived vocabularies before every call.
const systemPrompt = `You are a data-extraction engine. Convert the user's note into JSON.
Rules:
- Output ONLY valid JSON. No prose, no markdown fences.
- "tags": object; allowed keys "subject", "pattern"; each value MUST come from
  the provided vocabularies. If nothing fits, omit the key.
- "fields": object; allowed keys "minutes" (number), "lesson" (number),
  "problem" (string), "outcome" (solved|reviewed|struggled),
  "difficulty" (easy|medium|hard), "exam" (t1|t2|quiz1|quiz2|see),
  "score" (number).
- If nothing is extractable: {"tags": {},"fields": {}}
Vocabularies: <VOCAB>`

var (
	outcomes     = map[string]bool{"solved": true, "reviewed": true, "struggled": true}
	difficulties = map[string]bool{"easy": true, "medium": true, "hard": true}
	exams        = map[string]bool{"t1": true, "t2": true, "quiz1": true, "quiz2": true, "see": true}
)

// Options controls one quantify run.
type Options struct {
	OnlyUnquantified bool // process quantified=0 notes only
	Redo             bool // drop source='quantify' events and reset all notes first
	DryRun           bool // annotate and print, write nothing
}

// Run processes notes through prov, stores validated annotations and derives
// events (spec 04 "Derivation"). Per-note failures are warnings, never fatal;
// only store errors are returned.
func Run(ctx context.Context, out io.Writer, st *store.Store, prov Provider, opts Options) error {
	things, err := st.Things(store.ListFilter{Archived: store.ArchivedAll})
	if err != nil {
		return err
	}
	prompt, subjects, patterns := buildPrompt(things)

	if opts.Redo && !opts.DryRun {
		if _, err := st.DeleteEventsBySource("quantify"); err != nil {
			return err
		}
		if err := st.ResetNotesQuantified(); err != nil {
			return err
		}
	}
	notes, err := st.Notes(opts.OnlyUnquantified && !opts.Redo)
	if err != nil {
		return err
	}
	if len(notes) == 0 {
		fmt.Fprintln(out, "no pending notes.")
		return nil
	}
	for _, note := range notes {
		if err := quantifyNote(ctx, out, st, prov, prompt, subjects, patterns, note, opts.DryRun); err != nil {
			return err
		}
	}
	return nil
}

// buildPrompt injects the tracked-thing vocabularies into the system prompt.
func buildPrompt(things []model.Thing) (string, map[string]bool, map[string]bool) {
	subjects := make(map[string]bool, len(things))
	patterns := make(map[string]bool)
	var subjectIDs, patternIDs []string
	for _, t := range things {
		subjects[t.ID] = true
		subjectIDs = append(subjectIDs, t.ID)
		if t.Kind == "pattern" {
			patterns[t.ID] = true
			patternIDs = append(patternIDs, t.ID)
		}
	}
	vocab := "subject: " + strings.Join(subjectIDs, ", ") + "\npattern: " + strings.Join(patternIDs, ", ")
	return strings.Replace(systemPrompt, "<VOCAB>", vocab, 1), subjects, patterns
}

func quantifyNote(ctx context.Context, out io.Writer, st *store.Store, prov Provider, prompt string, subjects, patterns map[string]bool, note model.Event, dryRun bool) error {
	raw := ""
	if note.RawText != nil {
		raw = *note.RawText
	}
	reply, err := prov.Complete(ctx, prompt+"\n\n"+raw)
	if err != nil {
		fmt.Fprintf(out, "note #%d rejected: %v\n", note.ID, err)
		return nil
	}
	ann, err := parseAnnotation(reply, subjects, patterns)
	if err != nil {
		fmt.Fprintf(out, "note #%d rejected: %v\n", note.ID, err)
		return nil
	}
	if !dryRun {
		payload, err := json.Marshal(ann.payload())
		if err != nil {
			return fmt.Errorf("quantify note %d: %w", note.ID, err)
		}
		// Derive before marking so a mid-way failure is retried on the next
		// run (existing derived events are dedup-skipped).
		if err := derive(st, note, ann); err != nil {
			return err
		}
		if err := st.MarkQuantified(note.ID, string(payload)); err != nil {
			return err
		}
	}
	fmt.Fprintln(out, summaryLine(note.ID, ann))
	return nil
}

// ExtractJSON returns the substring from the first '{' to the last '}'; ok is
// false when no such pair exists. Providers reuse it on raw model output.
func ExtractJSON(s string) (string, bool) {
	first := strings.IndexByte(s, '{')
	last := strings.LastIndexByte(s, '}')
	if first < 0 || last < first {
		return "", false
	}
	return s[first : last+1], true
}

type annotation struct {
	subject    string
	pattern    string
	minutes    *float64
	lesson     *float64
	problem    string
	outcome    string
	difficulty string
	exam       string
	score      *float64
}

type annotationTags struct {
	Subject string `json:"subject,omitempty"`
	Pattern string `json:"pattern,omitempty"`
}

type annotationFields struct {
	Minutes    *float64 `json:"minutes,omitempty"`
	Lesson     *float64 `json:"lesson,omitempty"`
	Problem    string   `json:"problem,omitempty"`
	Outcome    string   `json:"outcome,omitempty"`
	Difficulty string   `json:"difficulty,omitempty"`
	Exam       string   `json:"exam,omitempty"`
	Score      *float64 `json:"score,omitempty"`
}

type annotationPayload struct {
	Tags   annotationTags   `json:"tags"`
	Fields annotationFields `json:"fields"`
}

// payload renders the canonical stored form {"tags":...,"fields":...}.
func (a annotation) payload() annotationPayload {
	return annotationPayload{
		Tags: annotationTags{Subject: a.subject, Pattern: a.pattern},
		Fields: annotationFields{
			Minutes:    a.minutes,
			Lesson:     a.lesson,
			Problem:    a.problem,
			Outcome:    a.outcome,
			Difficulty: a.difficulty,
			Exam:       a.exam,
			Score:      a.score,
		},
	}
}

// parseAnnotation validates the provider reply against the closed vocabularies
// (spec 04 "Normalizer", hard rule 1): anything unknown, mistyped, or outside
// the registry rejects the whole note.
func parseAnnotation(reply string, subjects, patterns map[string]bool) (annotation, error) {
	var ann annotation
	body, ok := ExtractJSON(reply)
	if !ok {
		return ann, errors.New("no JSON object in provider output")
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal([]byte(body), &top); err != nil {
		return ann, fmt.Errorf("invalid JSON: %w", err)
	}
	if rawTags, ok := top["tags"]; ok {
		var tags map[string]any
		if err := json.Unmarshal(rawTags, &tags); err != nil {
			return ann, errors.New("tags must be an object")
		}
		for _, key := range sortedKeys(tags) {
			s, ok := tags[key].(string)
			if !ok {
				return ann, fmt.Errorf("tag %q must be a string", key)
			}
			switch key {
			case "subject":
				if !subjects[s] {
					return ann, fmt.Errorf("unknown subject %q", s)
				}
				ann.subject = s
			case "pattern":
				if !patterns[s] {
					return ann, fmt.Errorf("unknown pattern %q", s)
				}
				ann.pattern = s
			default:
				return ann, fmt.Errorf("unknown tag key %q", key)
			}
		}
	}
	if rawFields, ok := top["fields"]; ok {
		var fields map[string]any
		if err := json.Unmarshal(rawFields, &fields); err != nil {
			return ann, errors.New("fields must be an object")
		}
		for _, key := range sortedKeys(fields) {
			v := fields[key]
			switch key {
			case "minutes", "lesson", "score":
				f, ok := v.(float64)
				if !ok {
					return ann, fmt.Errorf("%s must be a number", key)
				}
				switch key {
				case "minutes":
					ann.minutes = &f
				case "lesson":
					ann.lesson = &f
				default:
					ann.score = &f
				}
			case "problem":
				s, ok := v.(string)
				if !ok {
					return ann, errors.New("problem must be a string")
				}
				ann.problem = s
			case "outcome":
				s, ok := v.(string)
				if !ok || !outcomes[s] {
					return ann, errors.New("outcome must be one of solved|reviewed|struggled")
				}
				ann.outcome = s
			case "difficulty":
				s, ok := v.(string)
				if !ok || !difficulties[s] {
					return ann, errors.New("difficulty must be one of easy|medium|hard")
				}
				ann.difficulty = s
			case "exam":
				s, ok := v.(string)
				if !ok || !exams[s] {
					return ann, errors.New("exam must be one of t1|t2|quiz1|quiz2|see")
				}
				ann.exam = s
			default:
				return ann, fmt.Errorf("unknown field key %q", key)
			}
		}
	}
	return ann, nil
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// derivation is one derived event plus the suffix that keeps its dedup key
// unique when a single note yields several events.
type derivation struct {
	kind  string
	event model.Event
}

type sessionPayload struct {
	StartedAt string   `json:"started_at"`
	EndedAt   string   `json:"ended_at"`
	Minutes   int      `json:"minutes"`
	Tags      []string `json:"tags"`
	Manual    bool     `json:"manual"`
}

type problemPayload struct {
	Problem string `json:"problem"`
	Outcome string `json:"outcome"`
}

type examPayload struct {
	Exam string `json:"exam"`
}

// derive materializes the spec 04 "Derivation" table. Rows are keyed
// quantify/<note-id>; when one note fires several rows the key gains a row
// suffix so the UNIQUE dedup_key constraint never collides. A plain insert if
// absent keeps re-runs duplicate-free (UpsertByDedup's attempts semantics do
// not apply here).
func derive(st *store.Store, note model.Event, ann annotation) error {
	ds, err := derivations(note, ann)
	if err != nil {
		return err
	}
	for _, d := range ds {
		key := fmt.Sprintf("quantify/%d", note.ID)
		if len(ds) > 1 {
			key = fmt.Sprintf("quantify/%d/%s", note.ID, d.kind)
		}
		if _, err := st.EventByDedup(key); err == nil {
			continue
		} else if !errors.Is(err, store.ErrNotFound) {
			return err
		}
		d.event.DedupKey = &key
		if _, err := st.AddEvent(d.event); err != nil {
			return err
		}
	}
	return nil
}

func derivations(note model.Event, ann annotation) ([]derivation, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	var ds []derivation

	if ann.subject != "" && ann.minutes != nil {
		started, err := time.Parse(time.RFC3339, note.Ts)
		if err != nil {
			return nil, fmt.Errorf("quantify note %d: bad ts %q: %w", note.ID, note.Ts, err)
		}
		ended := started.Add(time.Duration(*ann.minutes*60) * time.Second)
		payload, err := json.Marshal(sessionPayload{
			StartedAt: note.Ts,
			EndedAt:   ended.UTC().Format(time.RFC3339),
			Minutes:   int(*ann.minutes),
			Tags:      []string{},
			Manual:    true,
		})
		if err != nil {
			return nil, fmt.Errorf("quantify note %d: session payload: %w", note.ID, err)
		}
		subject := ann.subject
		ds = append(ds, derivation{kind: "session", event: model.Event{
			Ts:        note.Ts,
			Source:    "quantify",
			Type:      "session",
			Subject:   &subject,
			Payload:   payload,
			CreatedAt: now,
		}})
	}
	if ann.lesson != nil {
		subject := "language/german"
		value := *ann.lesson
		ds = append(ds, derivation{kind: "lesson", event: model.Event{
			Ts:        note.Ts,
			Source:    "quantify",
			Type:      "occurrence",
			Subject:   &subject,
			ValueNum:  &value,
			CreatedAt: now,
		}})
	}
	if ann.pattern != "" && ann.problem != "" && ann.outcome != "" {
		payload, err := json.Marshal(problemPayload{Problem: ann.problem, Outcome: ann.outcome})
		if err != nil {
			return nil, fmt.Errorf("quantify note %d: occurrence payload: %w", note.ID, err)
		}
		subject := ann.pattern
		ds = append(ds, derivation{kind: "pattern", event: model.Event{
			Ts:        note.Ts,
			Source:    "quantify",
			Type:      "occurrence",
			Subject:   &subject,
			Payload:   payload,
			CreatedAt: now,
		}})
	}
	if ann.subject != "" && ann.exam != "" && ann.score != nil {
		payload, err := json.Marshal(examPayload{Exam: ann.exam})
		if err != nil {
			return nil, fmt.Errorf("quantify note %d: milestone payload: %w", note.ID, err)
		}
		subject := ann.subject
		value := *ann.score
		ds = append(ds, derivation{kind: "milestone", event: model.Event{
			Ts:        note.Ts,
			Source:    "quantify",
			Type:      "milestone",
			Subject:   &subject,
			ValueNum:  &value,
			Payload:   payload,
			CreatedAt: now,
		}})
	}
	return ds, nil
}

// summaryLine renders "note #12 → subject:course/ddco, minutes:45" with only
// the extracted keys.
func summaryLine(id int64, ann annotation) string {
	var parts []string
	if ann.subject != "" {
		parts = append(parts, "subject:"+ann.subject)
	}
	if ann.pattern != "" {
		parts = append(parts, "pattern:"+ann.pattern)
	}
	if ann.minutes != nil {
		parts = append(parts, "minutes:"+formatNumber(*ann.minutes))
	}
	if ann.lesson != nil {
		parts = append(parts, "lesson:"+formatNumber(*ann.lesson))
	}
	if ann.problem != "" {
		parts = append(parts, "problem:"+ann.problem)
	}
	if ann.outcome != "" {
		parts = append(parts, "outcome:"+ann.outcome)
	}
	if ann.difficulty != "" {
		parts = append(parts, "difficulty:"+ann.difficulty)
	}
	if ann.exam != "" {
		parts = append(parts, "exam:"+ann.exam)
	}
	if ann.score != nil {
		parts = append(parts, "score:"+formatNumber(*ann.score))
	}
	line := fmt.Sprintf("note #%d →", id)
	if len(parts) > 0 {
		line += " " + strings.Join(parts, ", ")
	}
	return line
}

func formatNumber(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }
