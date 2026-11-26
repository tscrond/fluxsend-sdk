package reference

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDoAddsAuthenticationAndDecodesEnvelope(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-API-Key"); got != "test-key" {
			t.Errorf("X-API-Key = %q, want test-key", got)
		}
		if got := r.URL.Path; got != "/api/user/data" {
			t.Errorf("path = %q, want /api/user/data", got)
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"status":200,"msg":"ok","response":{"user_id":"user-1"}}`)
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-key", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	result, err := GetCurrentUserData(context.Background(), client)
	if err != nil {
		t.Fatal(err)
	}
	if result.Envelope == nil || string(result.Envelope.Response) != `{"user_id":"user-1"}` {
		t.Fatalf("unexpected envelope: %+v", result.Envelope)
	}
}

func TestGeneratedCallBuildsQueryAndEscapesPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.EscapedPath(); got != "/api/files/a%2Fb/note" {
			t.Errorf("path = %q, want escaped checksum", got)
		}
		if got := r.URL.Query().Get("file_name"); got != "report 1.pdf" {
			t.Errorf("file_name = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"status":200,"response":{"content":"ok"}}`)
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := GetFileNote(context.Background(), client, "a/b", "report 1.pdf"); err != nil {
		t.Fatal(err)
	}
}

func TestUploadFileUsesMultipartForm(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatal(err)
		}
		if got := r.FormValue("folder"); got != "docs" {
			t.Errorf("folder = %q, want docs", got)
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		contents, err := io.ReadAll(file)
		if err != nil {
			t.Fatal(err)
		}
		if header.Filename != "hello.txt" || string(contents) != "hello" {
			t.Errorf("file = %q %q", header.Filename, contents)
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"status":200,"response":"uploaded"}`)
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "key", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	_, err = UploadFile(context.Background(), client, strings.NewReader("hello"), "hello.txt", "docs")
	if err != nil {
		t.Fatal(err)
	}
}

func TestDoReturnsTypedAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		io.WriteString(w, `{"status":403,"msg":"missing scope","response":null}`)
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "key", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	_, err = ListWorkspaces(context.Background(), client)
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error = %T %v, want *APIError", err, err)
	}
	if apiErr.StatusCode != http.StatusForbidden || apiErr.Message != "missing scope" {
		t.Fatalf("unexpected API error: %+v", apiErr)
	}
}

func TestDecodeTypedResponsePayload(t *testing.T) {
	result := &Result{Envelope: &Envelope{Response: json.RawMessage(`{"api_key":"secret"}`)}}
	payload, err := Decode[struct {
		APIKey string `json:"api_key"`
	}](result)
	if err != nil {
		t.Fatal(err)
	}
	if payload.APIKey != "secret" {
		t.Fatalf("api_key = %q", payload.APIKey)
	}
}
