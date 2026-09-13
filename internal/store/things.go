package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/sadisticbrew/meridian/internal/model"
)

var ErrNotFound = errors.New("store: not found")

type ArchivedMode int

const (
	ArchivedExclude ArchivedMode = iota
	ArchivedOnly
	ArchivedAll
)

type ListFilter struct {
	Kind     string
	Archived ArchivedMode
}

const thingCols = `id, kind, display_name, active, archived, decision_rule, goal_json, created_at`

type scanner interface{ Scan(dest ...any) error }

func scanThing(sc scanner) (model.Thing, error) {
	var t model.Thing
	var active, archived int
	var rule, goal sql.NullString
	if err := sc.Scan(&t.ID, &t.Kind, &t.DisplayName, &active, &archived, &rule, &goal, &t.CreatedAt); err != nil {
		return t, err
	}
	t.Active = active == 1
	t.Archived = archived == 1
	t.DecisionRule = rule.String
	t.GoalJSON = goal.String
	return t, nil
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

func (f ListFilter) where() (string, []any) {
	var conds []string
	var args []any
	switch f.Archived {
	case ArchivedOnly:
		conds = append(conds, "archived = 1")
	case ArchivedAll:
	default:
		conds = append(conds, "archived = 0")
	}
	if f.Kind != "" {
		conds = append(conds, "kind = ?")
		args = append(args, f.Kind)
	}
	if len(conds) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(conds, " AND "), args
}

func (s *Store) Thing(id string) (*model.Thing, error) {
	row := s.db.QueryRow(`SELECT `+thingCols+` FROM tracked_things WHERE id = ?`, id)
	t, err := scanThing(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("thing %s: %w", id, err)
	}
	return &t, nil
}

func (s *Store) Things(filter ListFilter) ([]model.Thing, error) {
	where, args := filter.where()
	rows, err := s.db.Query(`SELECT `+thingCols+` FROM tracked_things`+where+` ORDER BY id`, args...)
	if err != nil {
		return nil, fmt.Errorf("list things: %w", err)
	}
	defer rows.Close()
	var out []model.Thing
	for rows.Next() {
		t, err := scanThing(rows)
		if err != nil {
			return nil, fmt.Errorf("list things: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) SubjectsActive() ([]model.Thing, error) {
	rows, err := s.db.Query(`SELECT ` + thingCols + ` FROM tracked_things WHERE active = 1 AND archived = 0 ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list active subjects: %w", err)
	}
	defer rows.Close()
	var out []model.Thing
	for rows.Next() {
		t, err := scanThing(rows)
		if err != nil {
			return nil, fmt.Errorf("list active subjects: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// Full-row INSERT OR REPLACE; callers do read-modify-write to preserve fields
// they don't change (created_at included).
func (s *Store) UpsertThing(t model.Thing) error {
	var rule, goal any
	if t.DecisionRule != "" {
		rule = t.DecisionRule
	}
	if t.GoalJSON != "" {
		goal = t.GoalJSON
	}
	_, err := s.db.Exec(`INSERT OR REPLACE INTO tracked_things (`+thingCols+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.Kind, t.DisplayName, b2i(t.Active), b2i(t.Archived), rule, goal, t.CreatedAt)
	if err != nil {
		return fmt.Errorf("upsert thing %s: %w", t.ID, err)
	}
	return nil
}

func (s *Store) ArchiveThing(id string, archived bool) error {
	res, err := s.db.Exec(`UPDATE tracked_things SET archived = ?, active = ? WHERE id = ?`,
		b2i(archived), b2i(!archived), id)
	if err != nil {
		return fmt.Errorf("archive thing %s: %w", id, err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return ErrNotFound
	}
	return nil
}
