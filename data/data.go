package data

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

//go:embed neetcode150-patterns.json
var patternsJSON []byte

func Patterns() (map[string]string, error) {
	m := make(map[string]string)
	if err := json.Unmarshal(patternsJSON, &m); err != nil {
		return nil, fmt.Errorf("data: parse neetcode150-patterns.json: %w", err)
	}
	return m, nil
}
