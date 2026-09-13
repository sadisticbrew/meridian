package normalizer

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type capturedHTTP struct {
	method      string
	path        string
	auth        string
	contentType string
	req         chatRequest
}

func TestHTTPProviderComplete(t *testing.T) {
	const (
		apiKey       = "sk-test-secret-key"
		endpointPath = "/v1/chat/completions"
	)

	tests := []struct {
		name        string
		timeout     time.Duration
		handle      func(w http.ResponseWriter, r *http.Request)
		want        string
		wantErr     bool
		errContains []string
	}{
		{
			name:    "request shape and response parsing",
			timeout: 5 * time.Second,
			handle: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				io.WriteString(w, `{"choices":[{"message":{"content":"{\"tags\":{},\"fields\":{\"minutes\":45}}"}}]}`)
			},
			want: `{"tags":{},"fields":{"minutes":45}}`,
		},
		{
			name:    "non-200 error redacts the api key",
			timeout: 5 * time.Second,
			handle: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
				io.WriteString(w, `{"error":{"message":"invalid key `+apiKey+`"}}`)
			},
			wantErr:     true,
			errContains: []string{"401"},
		},
		{
			name:    "timeout",
			timeout: 25 * time.Millisecond,
			handle: func(w http.ResponseWriter, _ *http.Request) {
				time.Sleep(500 * time.Millisecond)
				io.WriteString(w, `{"choices":[]}`)
			},
			wantErr:     true,
			errContains: []string{"timeout", "deadline"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got capturedHTTP
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				got.method = r.Method
				got.path = r.URL.Path
				got.auth = r.Header.Get("Authorization")
				got.contentType = r.Header.Get("Content-Type")
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Errorf("read request body: %v", err)
				}
				if err := json.Unmarshal(body, &got.req); err != nil {
					t.Errorf("decode request body: %v", err)
				}
				tt.handle(w, r)
			}))
			defer srv.Close()

			prov := NewHTTPProvider(srv.URL+endpointPath, apiKey, "test-model", tt.timeout)
			if name := prov.Name(); name != "http" {
				t.Errorf("Name() = %q, want http", name)
			}

			out, err := prov.Complete(context.Background(), "hello world")
			if tt.wantErr {
				if err == nil {
					t.Fatal("Complete returned nil error")
				}
				for _, want := range tt.errContains {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("error %q does not contain %q", err, want)
					}
				}
				if strings.Contains(err.Error(), apiKey) {
					t.Errorf("error %q leaks the api key", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Complete: %v", err)
			}
			if out != tt.want {
				t.Errorf("output = %q, want %q", out, tt.want)
			}
			if got.method != http.MethodPost {
				t.Errorf("method = %q, want POST", got.method)
			}
			if got.path != endpointPath {
				t.Errorf("path = %q, want %q", got.path, endpointPath)
			}
			if got.auth != "Bearer "+apiKey {
				t.Errorf("Authorization = %q, want %q", got.auth, "Bearer "+apiKey)
			}
			if got.contentType != "application/json" {
				t.Errorf("Content-Type = %q, want application/json", got.contentType)
			}
			if got.req.Model != "test-model" {
				t.Errorf("model = %q, want test-model", got.req.Model)
			}
			if len(got.req.Messages) != 1 || got.req.Messages[0].Role != "user" || got.req.Messages[0].Content != "hello world" {
				t.Errorf("messages = %+v, want one user message carrying the prompt", got.req.Messages)
			}
		})
	}
}
