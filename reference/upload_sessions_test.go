package reference

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMultipartUploadSessionOperations(t *testing.T) {
	tests := []struct {
		name        string
		method      string
		path        string
		contentType string
		body        string
		bodyLength  int64
		invoke      func(context.Context, *Client) (*Result, error)
	}{
		{
			name:        "create private upload",
			method:      http.MethodPost,
			path:        "/api/files/uploads",
			contentType: "application/json",
			body:        `{"filename":"report.pdf","size":12,"content_type":"application/pdf","folder":"docs"}`,
			invoke: func(ctx context.Context, c *Client) (*Result, error) {
				return InitializePersonalFileMultipartUpload(ctx, c, CreateUploadRequest{
					Filename: "report.pdf", Size: 12, ContentType: "application/pdf", Folder: "docs",
				})
			},
		},
		{
			name:   "abort private upload",
			method: http.MethodDelete,
			path:   "/api/files/uploads/upload-1",
			invoke: func(ctx context.Context, c *Client) (*Result, error) {
				return AbortPersonalFileMultipartUpload(ctx, c, "upload-1")
			},
		},
		{
			name:        "upload private part",
			method:      http.MethodPost,
			path:        "/api/files/uploads/upload-1/parts/2",
			contentType: "application/octet-stream",
			body:        "chunk",
			bodyLength:  5,
			invoke: func(ctx context.Context, c *Client) (*Result, error) {
				return UploadPersonalFileMultipartPart(ctx, c, "upload-1", 2, strings.NewReader("chunk"), 5)
			},
		},
		{
			name:   "complete private upload",
			method: http.MethodPost,
			path:   "/api/files/uploads/upload-1/complete",
			invoke: func(ctx context.Context, c *Client) (*Result, error) {
				return CompletePersonalFileMultipartUpload(ctx, c, "upload-1")
			},
		},
		{
			name:        "create workspace upload",
			method:      http.MethodPost,
			path:        "/api/workspaces/workspace-1/files/uploads",
			contentType: "application/json",
			body:        `{"filename":"report.pdf","size":12,"content_type":"application/pdf","folder":"docs"}`,
			invoke: func(ctx context.Context, c *Client) (*Result, error) {
				return InitializeWorkspaceFileMultipartUpload(ctx, c, "workspace-1", CreateUploadRequest{
					Filename: "report.pdf", Size: 12, ContentType: "application/pdf", Folder: "docs",
				})
			},
		},
		{
			name:   "abort workspace upload",
			method: http.MethodDelete,
			path:   "/api/workspaces/workspace-1/files/uploads/upload-1",
			invoke: func(ctx context.Context, c *Client) (*Result, error) {
				return AbortWorkspaceFileMultipartUpload(ctx, c, "workspace-1", "upload-1")
			},
		},
		{
			name:        "upload workspace part",
			method:      http.MethodPost,
			path:        "/api/workspaces/workspace-1/files/uploads/upload-1/parts/2",
			contentType: "application/octet-stream",
			body:        "chunk",
			bodyLength:  5,
			invoke: func(ctx context.Context, c *Client) (*Result, error) {
				return UploadWorkspaceFileMultipartPart(ctx, c, "workspace-1", "upload-1", 2, strings.NewReader("chunk"), 5)
			},
		},
		{
			name:   "complete workspace upload",
			method: http.MethodPost,
			path:   "/api/workspaces/workspace-1/files/uploads/upload-1/complete",
			invoke: func(ctx context.Context, c *Client) (*Result, error) {
				return CompleteWorkspaceFileMultipartUpload(ctx, c, "workspace-1", "upload-1")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != tt.method || r.URL.Path != tt.path {
					t.Errorf("request = %s %s, want %s %s", r.Method, r.URL.Path, tt.method, tt.path)
				}
				if tt.contentType != "" && r.Header.Get("Content-Type") != tt.contentType {
					t.Errorf("Content-Type = %q, want %q", r.Header.Get("Content-Type"), tt.contentType)
				}
				if tt.body != "" {
					if tt.bodyLength > 0 && r.ContentLength != tt.bodyLength {
						t.Errorf("Content-Length = %d, want %d", r.ContentLength, tt.bodyLength)
					}
					body, err := io.ReadAll(r.Body)
					if err != nil {
						t.Fatal(err)
					}
					if strings.Contains(tt.contentType, "json") {
						var got, want any
						if err := json.Unmarshal(body, &got); err != nil {
							t.Fatal(err)
						}
						if err := json.Unmarshal([]byte(tt.body), &want); err != nil {
							t.Fatal(err)
						}
						gotJSON, _ := json.Marshal(got)
						wantJSON, _ := json.Marshal(want)
						if string(gotJSON) != string(wantJSON) {
							t.Errorf("body = %s, want %s", gotJSON, wantJSON)
						}
					} else if string(body) != tt.body {
						t.Errorf("body = %q, want %q", body, tt.body)
					}
				}
				w.Header().Set("Content-Type", "application/json")
				io.WriteString(w, `{"status":200,"response":null}`)
			}))
			defer server.Close()

			client, err := NewClient(server.URL, "key", server.Client())
			if err != nil {
				t.Fatal(err)
			}
			if _, err := tt.invoke(context.Background(), client); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestUploadPartRejectsEmptyContentLength(t *testing.T) {
	client, err := NewClient("https://example.com", "key", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := UploadPersonalFileMultipartPart(context.Background(), client, "upload-1", 1, strings.NewReader(""), 0); err == nil {
		t.Fatal("expected empty content length error")
	}
}
