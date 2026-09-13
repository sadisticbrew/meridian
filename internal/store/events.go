package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/sadisticbrew/meridian/internal/model"
)

// EventFilter selects events. From/To are UTC RFC 3339 strings and form the
// half-open window [From, To); empty means unbounded on that side.
type EventFilter struct {
	From    string
	To      string
	Type    string
	Subject string
}

const eventCols = `id, ts, source, type, subject, value_num, value_text, payload_json, dedup_key, raw_text, quantified, created_at`

func scanEvent(sc scanner) (model.Event, error) {
	var e model.Event
	var subject, valueText, payload, dedup, raw sql.NullString
	var valueNum sql.NullFloat64
	if err := sc.Scan(&e.ID, &e.Ts, &e.Source, &e.Type, &subject, &valueNum, &valueText, &payload, &dedup, &raw, &e.Quantified, &e.CreatedAt); err != nil {
		return e, err
	}
	if subject.Valid {
		e.Subject = &subject.String
	}
	if valueNum.Valid {
		e.ValueNum = &valueNum.Float64
	}
	if valueText.Valid {
		e.ValueText = &valueText.String
	}
	if payload.Valid {
		e.Payload = json.RawMessage(payload.String)
	}
	if dedup.Valid {
		e.DedupKey = &dedup.String
	}
	if raw.Valid {
		e.RawText = &raw.String
	}
	return e, nil
}

func (f EventFilter) where() (string, []any) {
	var conds []string
	var args []any
	if f.From != "" {
		conds = append(conds, "ts >= ?")
		args = append(args, f.From)
	}
	if f.To != "" {
		conds = append(conds, "ts < ?")
		args = append(args, f.To)
	}
	if f.Type != "" {
		conds = append(conds, "type = ?")
		args = append(args, f.Type)
	}
	if f.Subject != "" {
		conds = append(conds, "subject = ?")
		args = append(args, f.Subject)
	}
	if len(conds) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(conds, " AND "), args
}

func nullString(p *string) any {
	if p == nil {
		return nil
	}
	return *p
}

func nullFloat(p *float64) any {
	if p == nil {
		return nil
	}
	return *p
}

func nullPayload(p json.RawMessage) any {
	if len(p) == 0 {
		return nil
	}
	return string(p)
}

// AddEvent inserts e verbatim; created_at comes from the caller-supplied
// e.CreatedAt and the assigned id is returned.
func (s *Store) AddEvent(e model.Event) (int64, error) {
	res, err := s.db.Exec(`INSERT INTO events
		(ts, source, type, subject, value_num, value_text, payload_json, dedup_key, raw_text, quantified, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.Ts, e.Source, e.Type, nullString(e.Subject), nullFloat(e.ValueNum), nullString(e.ValueText),
		nullPayload(e.Payload), nullString(e.DedupKey), nullString(e.RawText), e.Quantified, e.CreatedAt)
	if err != nil {
		return 0, fmt.Errorf("add event: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("add event: %w", err)
	}
	return id, nil
}

// UpsertByDedup inserts e when its dedup key is unseen. When a row with the
// same key exists, only payload_json is rewritten and only if the new payload
// has a higher "attempts" count; an existing solve is enriched, never
// duplicated or re-announced (spec 02, occurrence upsert semantics).
func (s *Store) UpsertByDedup(e model.Event) (added bool, updated bool, err error) {
	if e.DedupKey == nil {
		return false, false, fmt.Errorf("upsert by dedup: dedup_key required")
	}
	row := s.db.QueryRow(`SELECT `+eventCols+` FROM events WHERE dedup_key = ?`, *e.DedupKey)
	existing, err := scanEvent(row)
	if errors.Is(err, sql.ErrNoRows) {
		if _, err := s.AddEvent(e); err != nil {
			return false, false, err
		}
		return true, false, nil
	}
	if err != nil {
		return false, false, fmt.Errorf("upsert by dedup: %w", err)
	}
	if attemptsOf(e.Payload) > attemptsOf(existing.Payload) {
		if _, err := s.db.Exec(`UPDATE events SET payload_json = ? WHERE dedup_key = ?`, nullPayload(e.Payload), *e.DedupKey); err != nil {
			return false, false, fmt.Errorf("upsert by dedup: %w", err)
		}
		return false, true, nil
	}
	return false, false, nil
}

// EventByDedup returns the event carrying key, or ErrNotFound when absent.
func (s *Store) EventByDedup(key string) (*model.Event, error) {
	row := s.db.QueryRow(`SELECT `+eventCols+` FROM events WHERE dedup_key = ?`, key)
	e, err := scanEvent(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("event by dedup: %w", err)
	}
	return &e, nil
}

// attemptsOf treats an absent or unparseable payload as zero attempts.
func attemptsOf(payload json.RawMessage) float64 {
	var p struct {
		Attempts *float64 `json:"attempts"`
	}
	if len(payload) == 0 || json.Unmarshal(payload, &p) != nil || p.Attempts == nil {
		return 0
	}
	return *p.Attempts
}

func (s *Store) Events(f EventFilter) ([]model.Event, error) {
	where, args := f.where()
	rows, err := s.db.Query(`SELECT `+eventCols+` FROM events`+where+` ORDER BY ts, id`, args...)
	if err != nil {
		return nil, fmt.Errorf("list events: %w", err)
	}
	defer rows.Close()
	var out []model.Event
	for rows.Next() {
		e, err := scanEvent(rows)
		if err != nil {
			return nil, fmt.Errorf("list events: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *Store) EventsForWindow(subject, from, to string) ([]model.Event, error) {
	return s.Events(EventFilter{From: from, To: to, Subject: subject})
}

func (s *Store) SessionsForWindow(subject, from, to string) ([]model.Event, error) {
	return s.Events(EventFilter{From: from, To: to, Type: "session", Subject: subject})
}

func (s *Store) OccurrencesForWindow(subject, from, to string) ([]model.Event, error) {
	return s.Events(EventFilter{From: from, To: to, Type: "occurrence", Subject: subject})
}

func (s *Store) MilestonesForSubject(subject string) ([]model.Event, error) {
	return s.Events(EventFilter{Type: "milestone", Subject: subject})
}

func (s *Store) MinutesForWindow(subject, from, to string) (int, error) {
	sessions, err := s.SessionsForWindow(subject, from, to)
	if err != nil {
		return 0, fmt.Errorf("minutes for window: %w", err)
	}
	total := 0
	for _, e := range sessions {
		if len(e.Payload) == 0 {
			continue
		}
		var p struct {
			Minutes *float64 `json:"minutes"`
		}
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return 0, fmt.Errorf("minutes for window: event %d payload: %w", e.ID, err)
		}
		if p.Minutes != nil {
			total += int(*p.Minutes)
		}
	}
	return total, nil
}

func (s *Store) latestEvent(where string, args ...any) (*model.Event, error) {
	row := s.db.QueryRow(`SELECT `+eventCols+` FROM events WHERE `+where+` ORDER BY ts DESC, id DESC LIMIT 1`, args...)
	e, err := scanEvent(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("latest event: %w", err)
	}
	return &e, nil
}

func (s *Store) LatestOccurrence(subject string) (*model.Event, error) {
	return s.latestEvent(`type = 'occurrence' AND subject = ?`, subject)
}

func (s *Store) LatestOccurrenceBefore(subject, to string) (*model.Event, error) {
	return s.latestEvent(`type = 'occurrence' AND subject = ? AND ts < ?`, subject, to)
}
