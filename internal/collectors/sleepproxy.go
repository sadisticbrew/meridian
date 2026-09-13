package collectors

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/sadisticbrew/meridian/internal/model"
	"github.com/sadisticbrew/meridian/internal/store"
)

const (
	sleepProxySubject = "sleep/proxy"
	// Verified on CachyOS (Arch, systemd); journalctl --grep matches the
	// MESSAGE field only, so the grep cannot anchor on the syslog identifier.
	sleepSuspendMarker = "Performing sleep operation 'suspend'"
	sleepResumeMarker  = "System returned from sleep operation 'suspend'"
)

var defaultJournalArgs = []string{
	"-o", "short-iso",
	"--grep", "(Performing|returned from) sleep operation",
}

// SleepProxy derives sleep hours from suspend/resume markers in the system
// journal. It is a documented proxy: the window is [suspend, resume], not
// "last user activity before suspend" (spec 04 calls the heuristic
// experimental). Findings live in sleepproxy_NOTES.md.
type SleepProxy struct {
	// JournalArgs are the journalctl arguments; nil means defaultJournalArgs.
	JournalArgs []string
	// Lines replaces the journalctl invocation when non-nil. Tests inject
	// fixture lines here; production leaves it nil.
	Lines func(ctx context.Context) ([]string, error)
}

func NewSleepProxy() *SleepProxy { return &SleepProxy{} }

func (s *SleepProxy) Name() string { return "sleep-proxy" }

type sleepProxyPayload struct {
	SuspendedAt string  `json:"suspended_at"`
	ResumedAt   string  `json:"resumed_at"`
	Hours       float64 `json:"hours"`
}

type sleepWindow struct {
	suspended time.Time
	resumed   time.Time
}

func (s *SleepProxy) Sync(ctx context.Context, st *store.Store) (Result, error) {
	var res Result
	if err := ensureSleepProxyThing(st); err != nil {
		return res, err
	}
	lines, err := s.journalLines(ctx)
	if err != nil {
		return res, &UnavailableError{Err: err}
	}
	windows, skipped := parseJournal(lines)
	createdAt := time.Now().UTC().Format(time.RFC3339)
	for _, w := range windows {
		suspendedAt := w.suspended.UTC()
		resumedAt := w.resumed.UTC()
		hours := w.resumed.Sub(w.suspended).Hours()
		dedupKey := "sleep-proxy/" + suspendedAt.Format(time.RFC3339)
		payload, err := marshalNoEscape(sleepProxyPayload{
			SuspendedAt: suspendedAt.Format(time.RFC3339),
			ResumedAt:   resumedAt.Format(time.RFC3339),
			Hours:       hours,
		})
		if err != nil {
			return res, fmt.Errorf("sleep-proxy: marshal payload: %w", err)
		}
		subject := sleepProxySubject
		value := hours
		added, _, err := st.UpsertByDedup(model.Event{
			Ts:        resumedAt.Format(time.RFC3339),
			Source:    "sleep-proxy",
			Type:      "sample",
			Subject:   &subject,
			ValueNum:  &value,
			Payload:   payload,
			DedupKey:  &dedupKey,
			CreatedAt: createdAt,
		})
		if err != nil {
			return res, fmt.Errorf("sleep-proxy: upsert %s: %w", dedupKey, err)
		}
		if added {
			res.New++
		}
	}
	res.Details = fmt.Sprintf("%d window(s), %d skipped", len(windows), skipped)
	return res, nil
}

// ensureSleepProxyThing self-heals the archived registry row; `meridian track
// add sleep/proxy --archived` can also create it, but sync must not depend on
// the user having done so.
func ensureSleepProxyThing(st *store.Store) error {
	if _, err := st.Thing(sleepProxySubject); err == nil {
		return nil
	} else if !errors.Is(err, store.ErrNotFound) {
		return fmt.Errorf("sleep-proxy: lookup thing: %w", err)
	}
	// kind "habit": the closed kind list is project|course|self-study|language|
	// pattern|habit and sleep is a body habit, not coursework; habit/* is
	// already the archived, goal-less corner of the registry (habit/instagram).
	return st.UpsertThing(model.Thing{
		ID:          sleepProxySubject,
		Kind:        "habit",
		DisplayName: "Sleep (proxy)",
		Active:      false,
		Archived:    true,
		CreatedAt:   time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *SleepProxy) journalLines(ctx context.Context) ([]string, error) {
	if s.Lines != nil {
		return s.Lines(ctx)
	}
	args := s.JournalArgs
	if args == nil {
		args = defaultJournalArgs
	}
	cmd := exec.CommandContext(ctx, "journalctl", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("journalctl %s: %s: %w", strings.Join(args, " "), msg, err)
	}
	out := strings.TrimRight(stdout.String(), "\n")
	if out == "" {
		return nil, nil
	}
	return strings.Split(out, "\n"), nil
}

// parseJournal pairs suspend/resume markers in order; stray resumes and a
// final unclosed suspend are counted as skipped fragments, never emitted.
func parseJournal(lines []string) ([]sleepWindow, int) {
	var windows []sleepWindow
	var pending *time.Time
	skipped := 0
	for _, line := range lines {
		ts, ok := parseJournalTimestamp(line)
		if !ok {
			continue
		}
		switch {
		case strings.Contains(line, sleepSuspendMarker):
			t := ts
			pending = &t
		case strings.Contains(line, sleepResumeMarker):
			if pending == nil {
				skipped++
				continue
			}
			windows = append(windows, sleepWindow{suspended: *pending, resumed: ts})
			pending = nil
		}
	}
	if pending != nil {
		skipped++
	}
	return windows, skipped
}

// parseJournalTimestamp reads the leading short-iso timestamp of a journalctl
// line; "2026-09-13T21:14:31+0530" is the common form, Z is tolerated.
func parseJournalTimestamp(line string) (time.Time, bool) {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return time.Time{}, false
	}
	for _, layout := range []string{"2006-01-02T15:04:05-0700", time.RFC3339} {
		if t, err := time.Parse(layout, fields[0]); err == nil {
			return t.UTC(), true
		}
	}
	return time.Time{}, false
}
