package fluxsend_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	fluxsend "github.com/tscrond/fluxsend-sdk"
)

// ExampleNewClient shows the client setup used by the docs.fluxsend.app Go SDK
// quickstart.
func ExampleNewClient() {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		exampleJSON(w, `[{"workspace_id":"ws-1","name":"Platform Team","slug":"platform-team"}]`)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := fluxsend.NewClient(fluxsend.Config{
		BaseURL:    server.URL,
		APIKey:     "fake-api-key",
		HTTPClient: server.Client(),
	})
	if err != nil {
		fmt.Println(err)
		return
	}

	workspaces, err := client.ListWorkspaces(ctx)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(workspaces[0].Name)

	// Output: Platform Team
}

// ExampleClient_CreateWorkspace shows a typical typed workspace operation for
// the docs.fluxsend.app workspace guide.
func ExampleClient_CreateWorkspace() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/workspaces/create" {
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}
		exampleJSON(w, `{"workspace":{"workspace_id":"ws-1","name":"Platform Team","slug":"platform-team"}}`)
	}))
	defer server.Close()

	client := newExampleClient(server)
	created, err := client.CreateWorkspace(context.Background(), fluxsend.WorkspaceCreateRequest{
		Name: "Platform Team",
		Slug: "platform-team",
	})
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(created.Workspace.Slug)

	// Output: platform-team
}

// ExampleClient_authenticationErrors demonstrates error classification for the
// docs.fluxsend.app authentication and reliability guides.
func ExampleClient_authenticationErrors() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		exampleJSON(w, `null`)
	}))
	defer server.Close()

	noKey, err := fluxsend.NewClient(fluxsend.Config{
		BaseURL:           server.URL,
		HTTPClient:        server.Client(),
		AllowInsecureHTTP: true,
	})
	if err != nil {
		fmt.Println(err)
		return
	}
	_, err = noKey.ListWorkspaces(context.Background())
	fmt.Println(errors.Is(err, fluxsend.ErrAuthentication))

	rejected := newExampleClient(server)
	_, err = rejected.ListWorkspaces(context.Background())
	var apiErr *fluxsend.APIError
	fmt.Println(errors.As(err, &apiErr), apiErr.StatusCode)

	// Output:
	// true
	// true 401
}

// ExampleClient_UploadFile shows a streaming direct personal upload for the
// docs.fluxsend.app files and transfers guide.
func ExampleClient_UploadFile() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		exampleJSON(w, `"report.txt"`)
	}))
	defer server.Close()

	client := newExampleClient(server)
	uploadedName, err := client.UploadFile(context.Background(), fluxsend.UploadFileRequest{
		Contents: strings.NewReader("example contents"),
		FileName: "report.txt",
		Folder:   "documents",
	})
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(uploadedName)

	// Output: report.txt
}

// ExampleClient_UploadWorkspaceFile shows a workspace-bound direct upload for
// the docs.fluxsend.app files and transfers guide.
func ExampleClient_UploadWorkspaceFile() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		exampleJSON(w, `[{"id":"file-1","file_name":"report.txt","size":16}]`)
	}))
	defer server.Close()

	client := newExampleClient(server)
	files, err := client.UploadWorkspaceFile(context.Background(), "ws-1", fluxsend.UploadWorkspaceFileRequest{
		Contents: strings.NewReader("example contents"),
		FileName: "report.txt",
	})
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(files[0].ID)

	// Output: file-1
}

// ExampleClient_UploadFileSession shows a chunked personal upload with
// progress for the docs.fluxsend.app files and transfers guide. Cancel ctx to
// stop work; the SDK attempts to abort the server-side session.
func ExampleClient_UploadFileSession() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/files/uploads":
			exampleJSON(w, `{"upload_id":"upload-1","chunk_size":2}`)
		case strings.Contains(r.URL.Path, "/parts/"):
			_, _ = io.Copy(io.Discard, r.Body)
			exampleJSON(w, `{"part_number":1,"size":2}`)
		case strings.HasSuffix(r.URL.Path, "/complete"):
			exampleJSON(w, `{"upload_id":"upload-1","file_name":"report.txt","md5_checksum":"checksum","size":4}`)
		}
	}))
	defer server.Close()

	client := newExampleClient(server)
	var last fluxsend.UploadProgress
	completed, err := client.UploadFileSession(context.Background(), fluxsend.UploadSessionRequest{
		File:        bytes.NewReader([]byte("data")),
		FileName:    "report.txt",
		Size:        4,
		Concurrency: 1,
		Progress: func(progress fluxsend.UploadProgress) {
			last = progress
		},
	})
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Printf("%s %.0f%%\n", completed.FileName, last.Fraction*100)

	// Output: report.txt 100%
}

// ExampleClient_DownloadPublicShare shows caller-owned streaming download
// behavior for the docs.fluxsend.app files and transfers guide.
func ExampleClient_DownloadPublicShare() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "hello from FluxSend")
	}))
	defer server.Close()

	client := newExampleClient(server)
	download, err := client.DownloadPublicShare(context.Background(), "share-token", "download")
	if err != nil {
		fmt.Println(err)
		return
	}
	defer download.Body.Close()

	if _, err := io.Copy(io.Discard, download.Body); err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(download.StatusCode)

	// Output: 200
}

// ExampleClient_DownloadPrivateFile shows an authenticated streaming download
// for the docs.fluxsend.app files and transfers guide.
func ExampleClient_DownloadPrivateFile() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "private FluxSend data")
	}))
	defer server.Close()

	client := newExampleClient(server)
	download, err := client.DownloadPrivateFile(context.Background(), "download-token", "download")
	if err != nil {
		fmt.Println(err)
		return
	}
	defer download.Body.Close()

	if _, err := io.Copy(io.Discard, download.Body); err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(download.StatusCode)

	// Output: 200
}

func newExampleClient(server *httptest.Server) *fluxsend.Client {
	client, err := fluxsend.NewClient(fluxsend.Config{
		BaseURL:           server.URL,
		APIKey:            "fake-api-key",
		HTTPClient:        server.Client(),
		AllowInsecureHTTP: true,
	})
	if err != nil {
		panic(err)
	}
	return client
}

func exampleJSON(w http.ResponseWriter, response string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(w, `{"status":200,"response":`+response+`}`)
}
