package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewClient(t *testing.T) {
	cfg := DefaultConfig()
	client := New(cfg)
	if client == nil {
		t.Fatal("expected client to be created")
	}
	client.Close()
}

func TestNewClientNilConfig(t *testing.T) {
	client := New(nil)
	if client == nil {
		t.Fatal("expected client to be created with default config")
	}
	client.Close()
}

func TestClientGetSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test response"))
	}))
	defer server.Close()

	client := New(DefaultConfig())
	defer client.Close()

	resp, err := client.Get(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	if string(resp.Body) != "test response" {
		t.Errorf("expected 'test response', got %q", string(resp.Body))
	}
}

func TestClientDoWithHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader != "Bearer token123" {
			t.Errorf("expected Authorization header, got %q", authHeader)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))
	defer server.Close()

	client := New(DefaultConfig())
	defer client.Close()

	headers := map[string]string{
		"Authorization": "Bearer token123",
	}

	resp, err := client.Do(context.Background(), http.MethodGet, server.URL, nil, headers)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
}

func TestClientRateLimitRetry(t *testing.T) {
	attempt := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt++
		if attempt == 1 {
			// First attempt: rate limited
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte("rate limited"))
			return
		}
		// Second attempt: success
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	}))
	defer server.Close()

	cfg := DefaultConfig()
	cfg.Retry.InitialBackoff = 10 * time.Millisecond
	cfg.Retry.MaxBackoff = 50 * time.Millisecond

	client := New(cfg)
	defer client.Close()

	resp, err := client.Get(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	if attempt != 2 {
		t.Errorf("expected 2 attempts, got %d", attempt)
	}
}

func TestClientServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("server error"))
	}))
	defer server.Close()

	cfg := DefaultConfig()
	cfg.Retry.MaxRetries = 1
	cfg.Retry.InitialBackoff = 10 * time.Millisecond
	cfg.Retry.MaxBackoff = 10 * time.Millisecond

	client := New(cfg)
	defer client.Close()

	_, err := client.Get(context.Background(), server.URL)
	if err == nil {
		t.Fatal("expected error for server error")
	}

	// Check that error contains mention of server error
	errMsg := err.Error()
	if !strings.Contains(errMsg, "500") {
		t.Logf("error message: %v", err)
	}
}

func TestClientNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("not found"))
	}))
	defer server.Close()

	client := New(DefaultConfig())
	defer client.Close()

	_, err := client.Get(context.Background(), server.URL)
	if err == nil {
		t.Fatal("expected error for 404")
	}

	httpErr, ok := err.(*HTTPError)
	if !ok {
		t.Fatalf("expected HTTPError, got %T", err)
	}

	if httpErr.StatusCode != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", httpErr.StatusCode)
	}
}

func TestParseRetryAfter(t *testing.T) {
	tests := []struct {
		name      string
		header    string
		expectMin time.Duration
		expectMax time.Duration
	}{
		{
			name:      "empty",
			header:    "",
			expectMin: 0,
			expectMax: 0,
		},
		{
			name:      "seconds",
			header:    "60",
			expectMin: 60 * time.Second,
			expectMax: 60 * time.Second,
		},
		{
			name:      "seconds_zero",
			header:    "0",
			expectMin: 0,
			expectMax: 0,
		},
	}

	client := New(nil)
	defer client.Close()

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			header := make(http.Header)
			if tc.header != "" {
				header.Set("Retry-After", tc.header)
			}

			result := client.parseRetryAfter(header)
			if result < tc.expectMin || result > tc.expectMax {
				t.Errorf("expected %v to %v, got %v", tc.expectMin, tc.expectMax, result)
			}
		})
	}
}

func TestClientContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := New(DefaultConfig())
	defer client.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := client.Get(ctx, server.URL)
	if err == nil {
		t.Fatal("expected error for canceled context")
	}

	if !strings.Contains(err.Error(), "context") {
		t.Errorf("expected context error, got: %v", err)
	}
}

func TestRateLimitError(t *testing.T) {
	err := &RateLimitError{
		StatusCode: http.StatusTooManyRequests,
		RetryAfter: 5 * time.Second,
	}

	msg := err.Error()
	if !strings.Contains(msg, "rate limited") {
		t.Errorf("expected 'rate limited' in message, got: %s", msg)
	}
	if !strings.Contains(msg, "429") {
		t.Errorf("expected '429' in message, got: %s", msg)
	}
	if !strings.Contains(msg, "5s") {
		t.Errorf("expected '5s' in message, got: %s", msg)
	}
}

func TestHTTPError(t *testing.T) {
	err := &HTTPError{
		StatusCode: http.StatusNotFound,
		Body:       []byte("not found"),
	}

	msg := err.Error()
	if !strings.Contains(msg, "404") {
		t.Errorf("expected '404' in message, got: %s", msg)
	}
}
