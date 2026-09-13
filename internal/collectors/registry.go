package collectors

import (
	"context"

	"github.com/sadisticbrew/meridian/internal/store"
)

type Collector interface {
	Name() string
	Sync(ctx context.Context, st *store.Store) (Result, error)
}

type Result struct {
	New     int
	Updated int
	Details string
}

// UnavailableError marks environmental (git/network) collector failures;
// sync skips these and keeps going, unlike DB errors.
type UnavailableError struct{ Err error }

func (e *UnavailableError) Error() string { return "unavailable (offline?): " + e.Err.Error() }

func (e *UnavailableError) Unwrap() error { return e.Err }

func Registry() map[string]Collector {
	return map[string]Collector{"neetcode": NewNeetCode()}
}
