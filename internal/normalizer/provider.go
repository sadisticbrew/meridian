package normalizer

import "context"

// Provider is an LLM completion backend (spec 04 "Providers"). OpenCodeProvider
// and HTTPProvider implement it; the normalizer only ever receives one.
type Provider interface {
	Name() string
	// Complete sends prompt and returns the model's raw text output.
	Complete(ctx context.Context, prompt string) (string, error)
}
