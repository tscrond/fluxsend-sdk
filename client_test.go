package fluxsend

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewClientValidatesConfiguration(t *testing.T) {
	tests := []string{"", "example.com", "ftp://example.com", "https://example.com?x=1"}
	for _, baseURL := range tests {
		t.Run(baseURL, func(t *testing.T) {
			if _, err := NewClient(Config{BaseURL: baseURL}); err == nil {
				t.Fatal("expected invalid base URL error")
			}
		})
	}
}

func TestNewClientRequiresExplicitInsecureHTTP(t *testing.T) {
	if _, err := NewClient(Config{BaseURL: "http://example.com"}); err == nil {
		t.Fatal("expected HTTP rejection")
	}
	if _, err := NewClient(Config{BaseURL: "http://example.com", AllowInsecureHTTP: true}); err != nil {
		t.Fatal(err)
	}
}

func TestDoJSONSendsAPIKeyAndDecodesResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-API-Key"); got != "test-key" {
			t.Errorf("X-API-Key = %q", got)
		}
		if got := r.Header.Get("User-Agent"); got != defaultUserAgent {
			t.Errorf("User-Agent = %q", got)
		}
		io.WriteString(w, `{"status":200,"msg":"ok","response":{"name":"workspace"}}`)
	}))
	defer server.Close()

	client, err := NewClient(Config{BaseURL: server.URL, APIKey: "test-key", HTTPClient: server.Client(), AllowInsecureHTTP: true})
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.doJSON[struct {
		Name string `json:"name"`
	}](context.Background(), request{
		method:       http.MethodGet,
		path:         "/api/workspaces/list",
		query:        nil,
		requiresAuth: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Name != "workspace" {
		t.Fatalf("name = %q", result.Name)
	}
}

func TestDoJSONSendsJSONContentType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q", got)
		}
		io.WriteString(w, `{"status":200,"response":{"ok":true}}`)
	}))
	defer server.Close()

	client, err := NewClient(Config{BaseURL: server.URL, APIKey: "key", HTTPClient: server.Client(), AllowInsecureHTTP: true})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.doJSON[any](context.Background(), request{
		method:       http.MethodPost,
		path:         "/api/files/move",
		body:         map[string]string{"source": "a", "destination": "b"},
		requiresAuth: true,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestDoJSONRequiresAPIKeyLocally(t *testing.T) {
	client, err := NewClient(Config{BaseURL: "https://example.com"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.doJSON[any](context.Background(), request{
		method:       http.MethodGet,
		path:         "/api/user/data",
		requiresAuth: true,
	})
	if !errors.Is(err, ErrAuthentication) {
		t.Fatalf("error = %v, want ErrAuthentication", err)
	}
}

func TestAPIErrorClassificationAndSafeMessage(t *testing.T) {
	tests := []struct {
		status  int
		message string
		kind    error
	}{
		{http.StatusBadRequest, "invalid_json", ErrValidation},
		{http.StatusUnauthorized, "unauthorized", ErrAuthentication},
		{http.StatusForbidden, "missing_api_key", ErrAuthentication},
		{http.StatusForbidden, "forbidden", ErrAuthorization},
		{http.StatusNotFound, "not_found", ErrNotFound},
		{http.StatusConflict, "file_already_exists", ErrConflict},
		{http.StatusGone, "invite_expired", ErrGone},
		{http.StatusTooManyRequests, "exceeded_plan_limits", ErrRateLimit},
		{http.StatusInternalServerError, "internal_error", ErrServer},
	}

	for _, tt := range tests {
		t.Run(tt.message, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				io.WriteString(w, fmt.Sprintf(`{"status":%d,"msg":%q,"response":{"detail":"safe"}}`, tt.status, tt.message))
			}))
			defer server.Close()

			client, err := NewClient(Config{BaseURL: server.URL, APIKey: "secret-key", HTTPClient: server.Client(), AllowInsecureHTTP: true})
			if err != nil {
				t.Fatal(err)
			}
			_, err = client.doJSON[any](context.Background(), request{
				method:       http.MethodGet,
				path:         "/api/user/data",
				requiresAuth: true,
			})
			if !errors.Is(err, tt.kind) {
				t.Fatalf("error = %v, want %v", err, tt.kind)
			}
			if strings.Contains(err.Error(), "secret-key") {
				t.Fatal("error leaked API key")
			}
		})
	}
}

func TestRetryableRequestHonorsRetryAfter(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if attempts.Add(1) == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			io.WriteString(w, `{"status":429,"msg":"rate_limited","response":null}`)
			return
		}
		io.WriteString(w, `{"status":200,"response":{"ok":true}}`)
	}))
	defer server.Close()

	client, err := NewClient(Config{BaseURL: server.URL, APIKey: "key", HTTPClient: server.Client(), AllowInsecureHTTP: true})
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.doJSON[struct {
		OK bool `json:"ok"`
	}](context.Background(), request{
		method:       http.MethodGet,
		path:         "/api/user/data",
		requiresAuth: true,
		retryable:    true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.OK || attempts.Load() != 2 {
		t.Fatalf("result = %+v, attempts = %d", result, attempts.Load())
	}
}

func TestUnsafeRequestDoesNotRetry(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
		io.WriteString(w, `{"status":503,"msg":"unavailable","response":null}`)
	}))
	defer server.Close()

	client, err := NewClient(Config{BaseURL: server.URL, APIKey: "key", HTTPClient: server.Client(), AllowInsecureHTTP: true})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.doJSON[any](context.Background(), request{
		method:       http.MethodPost,
		path:         "/api/files/move",
		requiresAuth: true,
		retryable:    false,
	})
	if !errors.Is(err, ErrServer) {
		t.Fatalf("error = %v, want ErrServer", err)
	}
	if attempts.Load() != 1 {
		t.Fatalf("attempts = %d, want 1", attempts.Load())
	}
}

func TestContextCancellationIsPreserved(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
	}))
	defer server.Close()

	client, err := NewClient(Config{BaseURL: server.URL, APIKey: "key", HTTPClient: server.Client(), AllowInsecureHTTP: true})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, err = client.doJSON[any](ctx, request{
		method:       http.MethodGet,
		path:         "/api/user/data",
		requiresAuth: true,
		retryable:    true,
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want context deadline", err)
	}
}

func TestMalformedSuccessEnvelopeReturnsAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `not-json`)
	}))
	defer server.Close()

	client, err := NewClient(Config{BaseURL: server.URL, APIKey: "key", HTTPClient: server.Client(), AllowInsecureHTTP: true})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.doJSON[any](context.Background(), request{
		method:       http.MethodGet,
		path:         "/api/user/data",
		requiresAuth: true,
	})
	if !errors.Is(err, ErrAPI) {
		t.Fatalf("error = %v, want ErrAPI", err)
	}
}

func TestReadBoundedRejectsOversizedBody(t *testing.T) {
	if _, err := readBounded(strings.NewReader("12345"), 4); err == nil {
		t.Fatal("expected oversized body error")
	}
}
