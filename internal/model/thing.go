package model

// Thing is one row of the tracked_things registry. Active/Archived are bool in
// Go; the store converts to/from INTEGER columns. Empty GoalJSON means no goal
// (stored as NULL).
type Thing struct {
	ID           string `json:"id"`
	Kind         string `json:"kind"`
	DisplayName  string `json:"display_name"`
	Active       bool   `json:"active"`
	Archived     bool   `json:"archived"`
	DecisionRule string `json:"decision_rule"`
	GoalJSON     string `json:"goal_json"`
	CreatedAt    string `json:"created_at"`
}
