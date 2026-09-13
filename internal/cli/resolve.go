package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/sadisticbrew/meridian/internal/model"
	"github.com/sadisticbrew/meridian/internal/store"
)

// AmbiguousError reports a subject input that matched multiple tracked
// things; Error() lists the candidate ids (spec 01 exit code 4).
type AmbiguousError struct {
	Input      string
	Candidates []string
}

func (e *AmbiguousError) Error() string {
	return fmt.Sprintf("ambiguous subject %q — candidates:\n  %s",
		e.Input, strings.Join(e.Candidates, "\n  "))
}

// resolveThing resolves an id or unique suffix (spec 03): exact id first,
// then unique suffix match over all tracked things (archived included, so
// archive/restore can target them).
func resolveThing(st *store.Store, input string) (*model.Thing, error) {
	t, err := st.Thing(input)
	switch {
	case err == nil:
		return t, nil
	case !errors.Is(err, store.ErrNotFound):
		return nil, err
	}
	all, err := st.Things(store.ListFilter{Archived: store.ArchivedAll})
	if err != nil {
		return nil, err
	}
	var ids []string
	var match model.Thing
	for _, c := range all {
		if strings.HasSuffix(c.ID, input) {
			ids = append(ids, c.ID)
			match = c
		}
	}
	switch len(ids) {
	case 1:
		return &match, nil
	case 0:
		return nil, fmt.Errorf("unknown subject %q", input)
	default:
		return nil, &AmbiguousError{Input: input, Candidates: ids}
	}
}
