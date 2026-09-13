package normalizer

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// ansiEscRE matches the CSI and OSC sequences opencode wraps around model
// output (reset codes, colour, window-title chatter).
var ansiEscRE = regexp.MustCompile(`\x1b\[[0-9;?]*[a-zA-Z]|\x1b\][^\x07]*\x07`)

// OpenCodeProvider shells out to the user's existing opencode CLI, so no API
// key is needed (spec 04 "Providers").
type OpenCodeProvider struct {
	command string
	model   string
	timeout time.Duration
}

func NewOpenCodeProvider(command, model string, timeout time.Duration) *OpenCodeProvider {
	return &OpenCodeProvider{command: command, model: model, timeout: timeout}
}

func (p *OpenCodeProvider) Name() string { return "opencode" }

// Complete runs the command with prompt as one argv element — never through a
// shell — and returns stdout with ANSI escapes stripped. JSON extraction stays
// with the normalizer, which is also the real gate: opencode exits 0 even when
// the backend fails, so output validation happens downstream.
func (p *OpenCodeProvider) Complete(ctx context.Context, prompt string) (string, error) {
	argv := strings.Fields(p.command)
	if len(argv) == 0 {
		return "", errors.New("opencode: command is empty")
	}
	if p.model != "" {
		argv = append(argv, "-m", p.model)
	}
	argv = append(argv, prompt)

	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	out, err := exec.CommandContext(ctx, argv[0], argv[1:]...).Output()
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return "", fmt.Errorf("opencode: timeout after %s: %w", p.timeout, ctx.Err())
		}
		return "", fmt.Errorf("opencode: run %q: %w", argv[0], err)
	}
	return stripANSI(string(out)), nil
}

func stripANSI(s string) string { return ansiEscRE.ReplaceAllString(s, "") }
