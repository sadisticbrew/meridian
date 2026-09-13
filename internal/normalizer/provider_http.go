package normalizer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// HTTPProvider talks to an OpenAI-compatible chat completions endpoint
// (spec 04 "Providers"). The caller resolves the api key from the environment;
// this type never logs it and never puts it in an error.
type HTTPProvider struct {
	url     string
	apiKey  string
	model   string
	timeout time.Duration
}

func NewHTTPProvider(url, apiKey, model string, timeout time.Duration) *HTTPProvider {
	return &HTTPProvider{url: url, apiKey: apiKey, model: model, timeout: timeout}
}

func (p *HTTPProvider) Name() string { return "http" }

type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// Complete sends the prompt as the sole user message and returns the first
// choice's content. A non-200 response reports only the status code: the body
// is deliberately discarded because it may echo credentials.
func (p *HTTPProvider) Complete(ctx context.Context, prompt string) (string, error) {
	body, err := json.Marshal(chatRequest{
		Model:    p.model,
		Messages: []chatMessage{{Role: "user", Content: prompt}},
	})
	if err != nil {
		return "", fmt.Errorf("http provider: encode request: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("http provider: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return "", fmt.Errorf("http provider: timeout after %s: %w", p.timeout, ctx.Err())
		}
		return "", fmt.Errorf("http provider: request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("http provider: unexpected status %d", resp.StatusCode)
	}

	var parsed chatResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&parsed); err != nil {
		return "", fmt.Errorf("http provider: decode response: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return "", errors.New("http provider: response contains no choices")
	}
	return parsed.Choices[0].Message.Content, nil
}
