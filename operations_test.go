package fluxsend

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

var (
	_ func(*Client, context.Context, WorkspaceCreateRequest) (WorkspaceCreatedResponseData, error)           = (*Client).CreateWorkspace
	_ func(*Client, context.Context, string) (ShareInfo, error)                                              = (*Client).GetPublicShareInfo
	_ func(*Client, context.Context, string, int, io.Reader, int64) (MultipartUploadPartResponseData, error) = (*Client).UploadPersonalFileMultipartPart
)

func TestGeneratedProtectedOperation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/workspaces/create" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("X-API-Key"); got != "key" {
			t.Errorf("X-API-Key = %q", got)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q", got)
		}
		io.WriteString(w, `{"status":200,"response":{"workspace":{"slug":"platform","name":"Platform"}}}`)
	}))
	defer server.Close()

	client := mustTestClient(t, server, "key")
	result, err := client.CreateWorkspace(context.Background(), WorkspaceCreateRequest{Name: "Platform", Slug: "platform"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Workspace.Slug != "platform" {
		t.Fatalf("workspace = %+v", result.Workspace)
	}
}

func TestGeneratedPublicOperationDoesNotSendAPIKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-API-Key"); got != "" {
			t.Errorf("public operation sent API key %q", got)
		}
		if r.URL.Path != "/api/share/info/token" {
			t.Errorf("path = %q", r.URL.Path)
		}
		io.WriteString(w, `{"status":200,"response":{"file_name":"report.pdf","password_protected":false}}`)
	}))
	defer server.Close()

	client := mustTestClient(t, server, "key")
	result, err := client.GetPublicShareInfo(context.Background(), "token")
	if err != nil {
		t.Fatal(err)
	}
	if result.FileName != "report.pdf" {
		t.Fatalf("share info = %+v", result)
	}
}

func TestGeneratedBinaryPartOperation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/files/uploads/upload-1/parts/2" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if got := r.Header.Get("Content-Type"); got != "application/octet-stream" {
			t.Errorf("Content-Type = %q", got)
		}
		if r.ContentLength != 5 {
			t.Errorf("Content-Length = %d", r.ContentLength)
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != "chunk" {
			t.Errorf("body = %q", body)
		}
		io.WriteString(w, `{"status":200,"response":{"part_number":2,"size":5}}`)
	}))
	defer server.Close()

	client := mustTestClient(t, server, "key")
	result, err := client.UploadPersonalFileMultipartPart(context.Background(), "upload-1", 2, strings.NewReader("chunk"), 5)
	if err != nil {
		t.Fatal(err)
	}
	if result.PartNumber != 2 || result.Size != 5 {
		t.Fatalf("part = %+v", result)
	}
}

func TestGeneratedPathAndQueryOperation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/workspaces/workspace-1/files/tree" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if got := r.URL.Query().Get("path"); got != "docs" {
			t.Errorf("path query = %q", got)
		}
		io.WriteString(w, `{"status":200,"response":{"path":"docs","files":[],"folders":[]}}`)
	}))
	defer server.Close()

	client := mustTestClient(t, server, "key")
	if _, err := client.GetWorkspaceFileTree(context.Background(), "workspace-1", "docs"); err != nil {
		t.Fatal(err)
	}
}

func mustTestClient(t *testing.T, server *httptest.Server, apiKey string) *Client {
	t.Helper()
	client, err := NewClient(Config{
		BaseURL:           server.URL,
		APIKey:            apiKey,
		HTTPClient:        server.Client(),
		AllowInsecureHTTP: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	return client
}
