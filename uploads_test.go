package fluxsend

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestUploadSessionPersonalAndWorkspace(t *testing.T) {
	tests := []struct {
		name  string
		base  string
		start func(context.Context, *Client, UploadSessionRequest) (CompletedUpload, error)
	}{
		{
			name: "personal",
			base: "/api/files/uploads",
			start: func(ctx context.Context, client *Client, request UploadSessionRequest) (CompletedUpload, error) {
				return client.UploadFileSession(ctx, request)
			},
		},
		{
			name: "workspace",
			base: "/api/workspaces/workspace-1/files/uploads",
			start: func(ctx context.Context, client *Client, request UploadSessionRequest) (CompletedUpload, error) {
				return client.UploadWorkspaceFileSession(ctx, "workspace-1", request)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var active atomic.Int32
			var maxActive atomic.Int32
			parts := make(map[int]string)
			var partsMu sync.Mutex

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == http.MethodPost && r.URL.Path == tt.base:
					var request createUploadSessionRequest
					if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
						t.Error(err)
					}
					if request.Filename != "report.txt" || request.Size != 10 || request.Folder != "docs" {
						t.Errorf("init request = %+v", request)
					}
					io.WriteString(w, `{"status":200,"response":{"upload_id":"upload-1","chunk_size":4}}`)
				case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/parts/"):
					current := active.Add(1)
					defer active.Add(-1)
					for {
						observed := maxActive.Load()
						if current <= observed || maxActive.CompareAndSwap(observed, current) {
							break
						}
					}
					partID, err := strconv.Atoi(r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:])
					if err != nil {
						t.Error(err)
					}
					body, err := io.ReadAll(r.Body)
					if err != nil {
						t.Error(err)
					}
					partsMu.Lock()
					parts[partID] = string(body)
					partsMu.Unlock()
					time.Sleep(20 * time.Millisecond)
					io.WriteString(w, `{"status":200,"response":{"part_number":`+strconv.Itoa(partID)+`,"size":`+strconv.Itoa(len(body))+`}}`)
				case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/complete"):
					io.WriteString(w, `{"status":200,"response":{"upload_id":"upload-1","file_name":"report.txt","md5_checksum":"abc","size":10}}`)
				case r.Method == http.MethodDelete:
					io.WriteString(w, `{"status":200,"response":{"upload_id":"upload-1","status":"aborted","uploaded_size":0}}`)
				default:
					t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer server.Close()

			client, err := NewClient(Config{BaseURL: server.URL, APIKey: "key", HTTPClient: server.Client(), AllowInsecureHTTP: true})
			if err != nil {
				t.Fatal(err)
			}
			var progressMu sync.Mutex
			var progress []UploadProgress
			result, err := tt.start(context.Background(), client, UploadSessionRequest{
				File:        bytes.NewReader([]byte("abcdefghij")),
				FileName:    "report.txt",
				Size:        10,
				ContentType: "text/plain",
				Folder:      "docs",
				Concurrency: 2,
				Progress: func(value UploadProgress) {
					progressMu.Lock()
					progress = append(progress, value)
					progressMu.Unlock()
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			if result.FileName != "report.txt" || result.Size != 10 {
				t.Fatalf("result = %+v", result)
			}
			if maxActive.Load() > 2 {
				t.Fatalf("max concurrency = %d", maxActive.Load())
			}
			partsMu.Lock()
			defer partsMu.Unlock()
			if parts[1] != "abcd" || parts[2] != "efgh" || parts[3] != "ij" {
				t.Fatalf("parts = %#v", parts)
			}
			progressMu.Lock()
			defer progressMu.Unlock()
			if len(progress) == 0 || progress[len(progress)-1].Fraction != 1 {
				t.Fatalf("progress = %+v", progress)
			}
		})
	}
}

func TestUploadSessionFailureAbortsWithoutCompleting(t *testing.T) {
	var aborted, completed atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/files/uploads":
			io.WriteString(w, `{"status":200,"response":{"upload_id":"upload-1","chunk_size":2}}`)
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/parts/"):
			w.WriteHeader(http.StatusServiceUnavailable)
			io.WriteString(w, `{"status":503,"msg":"unavailable","response":null}`)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/complete"):
			completed.Add(1)
		case r.Method == http.MethodDelete:
			aborted.Add(1)
			io.WriteString(w, `{"status":200,"response":{"upload_id":"upload-1","status":"aborted","uploaded_size":0}}`)
		}
	}))
	defer server.Close()

	client, err := NewClient(Config{BaseURL: server.URL, APIKey: "key", HTTPClient: server.Client(), AllowInsecureHTTP: true})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.UploadFileSession(context.Background(), UploadSessionRequest{
		File:     bytes.NewReader([]byte("abcd")),
		FileName: "report.txt",
		Size:     4,
	})
	if !errors.Is(err, ErrServer) {
		t.Fatalf("error = %v, want ErrServer", err)
	}
	if completed.Load() != 0 || aborted.Load() != 1 {
		t.Fatalf("completed = %d, aborted = %d", completed.Load(), aborted.Load())
	}
}

func TestUploadSessionCancellationAborts(t *testing.T) {
	var aborted atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/files/uploads":
			io.WriteString(w, `{"status":200,"response":{"upload_id":"upload-1","chunk_size":2}}`)
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/parts/"):
			io.WriteString(w, `{"status":200,"response":{"part_number":1,"size":2}}`)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/complete"):
			t.Error("complete must not run after cancellation")
		case r.Method == http.MethodDelete:
			aborted.Add(1)
			io.WriteString(w, `{"status":200,"response":{"upload_id":"upload-1","status":"aborted","uploaded_size":2}}`)
		}
	}))
	defer server.Close()

	client, err := NewClient(Config{BaseURL: server.URL, APIKey: "key", HTTPClient: server.Client(), AllowInsecureHTTP: true})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.UploadFileSession(ctx, UploadSessionRequest{
		File:        bytes.NewReader([]byte("abcd")),
		FileName:    "report.txt",
		Size:        4,
		Concurrency: 1,
		Progress: func(value UploadProgress) {
			if value.UploadedBytes >= 2 {
				cancel()
			}
		},
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if aborted.Load() != 1 {
		t.Fatalf("aborted = %d", aborted.Load())
	}
}
