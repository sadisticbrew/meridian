package model

import "encoding/json"

// Event mirrors the events table columns plus dump-format JSON tags
// (spec 02 "Dump format"). Payload is the decoded payload_json value.
// SubjectName is resolved display name, filled for dump output only.
type Event struct {
	ID          int64           `json:"id"`
	Ts          string          `json:"ts"`
	Source      string          `json:"source"`
	Type        string          `json:"type"`
	Subject     *string         `json:"subject"`
	ValueNum    *float64        `json:"value_num"`
	ValueText   *string         `json:"value_text"`
	Payload     json.RawMessage `json:"payload"`
	DedupKey    *string         `json:"dedup_key"`
	RawText     *string         `json:"raw_text"`
	Quantified  int             `json:"quantified"`
	SubjectName *string         `json:"subject_name"`
	CreatedAt   string          `json:"created_at"`
}
