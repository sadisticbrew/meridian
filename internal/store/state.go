package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

const runningSessionKey = "active-session"

type RunningSession struct {
	Subject   string `json:"subject"`
	StartedAt string `json:"started_at"` // UTC RFC 3339
	Note      string `json:"note"`
}

// RunningSession returns (nil, nil) when no session is running.
func (s *Store) RunningSession() (*RunningSession, error) {
	var raw string
	err := s.db.QueryRow(`SELECT value FROM state WHERE key = ?`, runningSessionKey).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("running session: %w", err)
	}
	var rs RunningSession
	if err := json.Unmarshal([]byte(raw), &rs); err != nil {
		return nil, fmt.Errorf("running session: decode state: %w", err)
	}
	return &rs, nil
}

func (s *Store) SetRunning(rs RunningSession) error {
	raw, err := json.Marshal(rs)
	if err != nil {
		return fmt.Errorf("set running session: %w", err)
	}
	if _, err := s.db.Exec(`INSERT OR REPLACE INTO state (key, value) VALUES (?, ?)`, runningSessionKey, string(raw)); err != nil {
		return fmt.Errorf("set running session: %w", err)
	}
	return nil
}

func (s *Store) ClearRunning() error {
	if _, err := s.db.Exec(`DELETE FROM state WHERE key = ?`, runningSessionKey); err != nil {
		return fmt.Errorf("clear running session: %w", err)
	}
	return nil
}
