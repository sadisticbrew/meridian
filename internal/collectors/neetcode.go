package collectors

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/sadisticbrew/meridian/data"
	"github.com/sadisticbrew/meridian/internal/config"
	"github.com/sadisticbrew/meridian/internal/model"
	"github.com/sadisticbrew/meridian/internal/store"
)

const neetcodeTopicFolder = "Data Structures & Algorithms"

var (
	submissionFileRe = regexp.MustCompile(`^submission-([0-9]+)\.([A-Za-z0-9]+)$`)
	logHeaderRe      = regexp.MustCompile(`^[0-9a-f]{40} ([0-9]+)$`)
)

var loadPatterns = data.Patterns

type NeetCode struct {
	RepoURL   string
	MirrorDir string
	Patterns  map[string]string
}

func NewNeetCode() *NeetCode { return &NeetCode{} }

func (n *NeetCode) Name() string { return "neetcode" }

type neetcodePayload struct {
	Slug        string `json:"slug"`
	Attempts    int    `json:"attempts"`
	Language    string `json:"language"`
	TopicFolder string `json:"topic_folder"`
}

type solveInfo struct {
	language string
	ts       string
	at       int64
	path     string
}

// marshalNoEscape keeps stored payload bytes raw ("Data Structures & Algorithms",
// not \u0026) — dump output re-emits payload_json verbatim (spec 02's sample line).
func marshalNoEscape(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}

func (n *NeetCode) Sync(ctx context.Context, st *store.Store) (Result, error) {
	var res Result
	repoURL := n.RepoURL
	if repoURL == "" {
		repoURL = config.DefaultNeetCodeURL
	}
	mirrorDir := n.MirrorDir
	if mirrorDir == "" {
		var err error
		mirrorDir, err = defaultMirrorDir()
		if err != nil {
			return res, err
		}
	}
	if err := syncMirror(ctx, repoURL, mirrorDir); err != nil {
		return res, err
	}
	head, err := hasHead(ctx, mirrorDir)
	if err != nil {
		return res, &UnavailableError{Err: err}
	}
	if !head {
		return res, nil
	}
	patterns := n.Patterns
	if patterns == nil {
		patterns, err = loadPatterns()
		if err != nil {
			return res, fmt.Errorf("neetcode: load patterns: %w", err)
		}
	}
	addTimes, err := firstAddTimes(ctx, mirrorDir)
	if err != nil {
		return res, err
	}
	maxN, err := headSubmissionIndexes(ctx, mirrorDir)
	if err != nil {
		return res, err
	}
	solves := firstSolves(addTimes)

	slugs := make([]string, 0, len(maxN))
	for slug := range maxN {
		if _, ok := solves[slug]; ok {
			slugs = append(slugs, slug)
		}
	}
	sort.Strings(slugs)

	createdAt := time.Now().UTC().Format(time.RFC3339)
	var details []string
	for _, slug := range slugs {
		attempts := maxN[slug] + 1
		patternID := patternSubject(patterns, slug)
		payload, err := marshalNoEscape(neetcodePayload{
			Slug:        slug,
			Attempts:    attempts,
			Language:    solves[slug].language,
			TopicFolder: neetcodeTopicFolder,
		})
		if err != nil {
			return res, fmt.Errorf("neetcode: marshal payload for %s: %w", slug, err)
		}
		dedupKey := "neetcode/" + slug
		old, err := st.EventByDedup(dedupKey)
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			return res, fmt.Errorf("neetcode: lookup %s: %w", dedupKey, err)
		}
		ev := model.Event{
			Ts:        solves[slug].ts,
			Source:    "neetcode",
			Type:      "occurrence",
			Subject:   &patternID,
			Payload:   payload,
			DedupKey:  &dedupKey,
			CreatedAt: createdAt,
		}
		added, updated, err := st.UpsertByDedup(ev)
		if err != nil {
			return res, fmt.Errorf("neetcode: upsert %s: %w", dedupKey, err)
		}
		switch {
		case added:
			res.New++
		case updated:
			res.Updated++
			oldAttempts := 0
			if old != nil {
				oldAttempts = payloadAttempts(old.Payload)
			}
			details = append(details, fmt.Sprintf("attempts +%d on %s", attempts-oldAttempts, slug))
		}
	}
	res.Details = strings.Join(details, "; ")
	return res, nil
}

func syncMirror(ctx context.Context, repoURL, mirrorDir string) error {
	if _, err := os.Stat(filepath.Join(mirrorDir, ".git")); err == nil {
		if _, err := gitOutput(ctx, "-C", mirrorDir, "fetch", "origin"); err != nil {
			return &UnavailableError{Err: err}
		}
		if _, err := gitOutput(ctx, "-C", mirrorDir, "reset", "--hard", "origin/main"); err != nil {
			return &UnavailableError{Err: err}
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(mirrorDir), 0o755); err != nil {
		return fmt.Errorf("neetcode: create mirror parent: %w", err)
	}
	if _, err := gitOutput(ctx, "clone", repoURL, mirrorDir); err != nil {
		return &UnavailableError{Err: err}
	}
	return nil
}

func gitOutput(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("git %s: %s: %w", strings.Join(args, " "), msg, err)
	}
	return stdout.String(), nil
}

func hasHead(ctx context.Context, mirrorDir string) (bool, error) {
	_, err := gitOutput(ctx, "-C", mirrorDir, "rev-parse", "--verify", "--quiet", "HEAD")
	if err == nil {
		return true, nil
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) && ee.ExitCode() == 1 {
		return false, nil
	}
	return false, err
}

func firstAddTimes(ctx context.Context, mirrorDir string) (map[string]int64, error) {
	out, err := gitOutput(ctx, "-C", mirrorDir, "log", "--diff-filter=A", "--name-only", "--format=%H %at")
	if err != nil {
		return nil, &UnavailableError{Err: err}
	}
	times := make(map[string]int64)
	authorTime := int64(-1)
	for _, line := range strings.Split(out, "\n") {
		if line == "" {
			continue
		}
		if m := logHeaderRe.FindStringSubmatch(line); m != nil {
			if at, err := strconv.ParseInt(m[1], 10, 64); err == nil {
				authorTime = at
			}
			continue
		}
		if authorTime < 0 || !strings.HasPrefix(line, neetcodeTopicFolder+"/") {
			continue
		}
		if prev, ok := times[line]; !ok || authorTime < prev {
			times[line] = authorTime
		}
	}
	return times, nil
}

func headSubmissionIndexes(ctx context.Context, mirrorDir string) (map[string]int, error) {
	out, err := gitOutput(ctx, "-C", mirrorDir, "ls-tree", "-r", "--name-only", "HEAD")
	if err != nil {
		return nil, &UnavailableError{Err: err}
	}
	maxN := make(map[string]int)
	for _, line := range strings.Split(out, "\n") {
		if line == "" {
			continue
		}
		slug, base, ok := splitTopicPath(line)
		if !ok {
			continue
		}
		m := submissionFileRe.FindStringSubmatch(base)
		if m == nil {
			continue
		}
		n, err := strconv.Atoi(m[1])
		if err != nil {
			continue
		}
		if prev, ok := maxN[slug]; !ok || n > prev {
			maxN[slug] = n
		}
	}
	return maxN, nil
}

// firstSolves picks, per slug, the submission-0.* file with the earliest
// first-add author time; a same-time tie goes to the lexicographically
// smallest full path (spec 04 edge cases).
func firstSolves(addTimes map[string]int64) map[string]solveInfo {
	solves := make(map[string]solveInfo)
	for p, at := range addTimes {
		slug, base, ok := splitTopicPath(p)
		if !ok {
			continue
		}
		m := submissionFileRe.FindStringSubmatch(base)
		if m == nil || m[1] != "0" {
			continue
		}
		cur, seen := solves[slug]
		if seen && (at > cur.at || (at == cur.at && p > cur.path)) {
			continue
		}
		solves[slug] = solveInfo{
			language: m[2],
			ts:       time.Unix(at, 0).UTC().Format(time.RFC3339),
			at:       at,
			path:     p,
		}
	}
	return solves
}

func splitTopicPath(p string) (slug, base string, ok bool) {
	prefix := neetcodeTopicFolder + "/"
	if !strings.HasPrefix(p, prefix) {
		return "", "", false
	}
	rest := strings.TrimPrefix(p, prefix)
	i := strings.Index(rest, "/")
	if i <= 0 || i == len(rest)-1 {
		return "", "", false
	}
	return rest[:i], path.Base(rest[i+1:]), true
}

// patternSubject returns the mapped pattern id verbatim; the generated map is
// validated to hold pattern/* ids, so no prefix tolerance is needed here.
func patternSubject(patterns map[string]string, slug string) string {
	if id := patterns[slug]; id != "" {
		return id
	}
	return "pattern/unclassified"
}

func payloadAttempts(payload json.RawMessage) int {
	var p struct {
		Attempts int `json:"attempts"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return 0
	}
	return p.Attempts
}

func defaultMirrorDir() (string, error) {
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("neetcode: resolve data home: %w", err)
		}
		base = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(base, "meridian", "mirror", "neetcode"), nil
}
