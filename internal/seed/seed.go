package seed

import "github.com/sadisticbrew/meridian/internal/model"

// Rows returns the phase-0 catalog verbatim (spec/phase-0.md "Seed catalog");
// no project/* rows are seeded.
func Rows(created string) []model.Thing {
	const (
		courseRule  = "0 study hours this week → book one evening block before the next test"
		patternRule = "reviews ≥ solves this week → re-drill that pattern's trigger cards"
	)
	rows := []model.Thing{
		{ID: "course/ddco", Kind: "course", DisplayName: "Digital Design & Computer Organization",
			Active: true, GoalJSON: `{"weekly_minutes":90}`, DecisionRule: courseRule, CreatedAt: created},
		{ID: "course/dav", Kind: "course", DisplayName: "Data Analytics & Visualization",
			Active: true, GoalJSON: `{"weekly_minutes":60}`, DecisionRule: courseRule, CreatedAt: created},
		{ID: "course/oop-java", Kind: "course", DisplayName: "Object Oriented Programming with Java",
			Active: true, GoalJSON: `{"weekly_minutes":60}`, DecisionRule: courseRule, CreatedAt: created},
		{ID: "course/cdd", Kind: "course", DisplayName: "Collaborative Development & DevOps",
			Active: true, GoalJSON: `{"weekly_minutes":30}`,
			DecisionRule: "home-turf subject — don't let it cannibalize DDCO evenings", CreatedAt: created},
		{ID: "self-study/ostep", Kind: "self-study", DisplayName: "OSTEP (audio study)",
			Active: true, GoalJSON: `{"weekly_minutes":90}`,
			DecisionRule: "book one audio-study block this week", CreatedAt: created},
		{ID: "self-study/rust", Kind: "self-study", DisplayName: "Rust",
			Active: true, GoalJSON: `{"weekly_minutes":60}`,
			DecisionRule: "pair practice with a -rs port; if it stalls two weeks, cut it", CreatedAt: created},
		{ID: "language/german", Kind: "language", DisplayName: "German (Nicos Weg)",
			Active: true, GoalJSON: `{"weekly_minutes":90,"occurrences_per_week":3}`,
			DecisionRule: "0 min this week → 15 min Nicos Weg tomorrow; B1 cuts PR 27→21 months", CreatedAt: created},
	}
	patterns := []struct{ id, name string }{
		{"arrays-hashing", "Arrays & Hashing"},
		{"two-pointer", "Two Pointer"},
		{"sliding-window", "Sliding Window"},
		{"stack", "Stack"},
		{"monotonic-stack", "Monotonic Stack"},
		{"binary-search", "Binary Search"},
		{"linked-list", "Linked List"},
		{"trees", "Trees"},
		{"trie", "Trie"},
		{"heap", "Heap"},
		{"backtracking", "Backtracking"},
		{"intervals", "Intervals"},
		{"greedy", "Greedy"},
		{"dynamic-programming", "Dynamic Programming"},
		{"unclassified", "Unclassified"},
	}
	for _, p := range patterns {
		rows = append(rows, model.Thing{ID: "pattern/" + p.id, Kind: "pattern",
			DisplayName: p.name, Active: true, DecisionRule: patternRule, CreatedAt: created})
	}
	rows = append(rows, model.Thing{ID: "habit/instagram", Kind: "habit",
		DisplayName: "Instagram cycle", Archived: true,
		DecisionRule: "reinstall logged → note the trigger, don't spiral", CreatedAt: created})
	return rows
}
