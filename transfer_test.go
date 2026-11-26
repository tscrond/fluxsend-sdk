package fluxsend

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestUploadFileStreamsSourceBeforeCompletion(t *testing.T) {
	firstChunkSeen := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reader, err := r.MultipartReader()
		if err != nil {
			t.Error(err)
			return
		}
		part, err := reader.NextPart()
		if err != nil {
			t.Error(err)
			return
		}
		first := make([]byte, 5)
		if _, err := io.ReadFull(part, first); err != nil {
			t.Error(err)
			return
		}
		close(firstChunkSeen)
		rest, err := io.ReadAll(part)
		if err != nil {
			t.Error(err)
			return
		}
		if got := string(first) + string(rest); got != "hello world" {
			t.Errorf("file body = %q", got)
			return
		}
		folder, err := reader.NextPart()
		if err == nil {
			value, _ := io.ReadAll(folder)
			if folder.FormName() != "folder" || string(value) != "docs" {
				t.Errorf("folder = %q %q", folder.FormName(), value)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"status":200,"response":"uploaded"}`)
	}))
	defer server.Close()

	client, err := NewClient(Config{BaseURL: server.URL, APIKey: "key", HTTPClient: server.Client(), AllowInsecureHTTP: true})
	if err != nil {
		t.Fatal(err)
	}
	reader, writer := io.Pipe()
	uploadResult := make(chan error, 1)
	go func() {
		_, err := client.UploadFile(context.Background(), UploadFileRequest{
			Contents: reader,
			FileName: "hello.txt",
			Folder:   "docs",
		})
		uploadResult <- err
	}()
	defer reader.Close()

	if _, err := writer.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}
	select {
	case <-firstChunkSeen:
	case <-time.After(time.Second):
		t.Fatal("upload did not stream before the complete file was available")
	}
	if _, err := writer.Write([]byte(" world")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := <-uploadResult; err != nil {
		t.Fatal(err)
	}
}

func TestDownloadReturnsStreamBeforeCompletion(t *testing.T) {
	firstChunkSent := make(chan struct{})
	continueResponse := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("mode"); got != "download" {
			t.Errorf("mode = %q", got)
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		io.WriteString(w, "hello")
		w.(http.Flusher).Flush()
		close(firstChunkSent)
		<-continueResponse
		io.WriteString(w, " world")
	}))
	defer server.Close()

	client, err := NewClient(Config{BaseURL: server.URL, APIKey: "key", HTTPClient: server.Client(), AllowInsecureHTTP: true})
	if err != nil {
		t.Fatal(err)
	}
	download, err := client.DownloadPrivateFile(context.Background(), "token", "download")
	if err != nil {
		t.Fatal(err)
	}
	defer download.Body.Close()

	select {
	case <-firstChunkSent:
	case <-time.After(time.Second):
		t.Fatal("download did not return after response headers")
	}
	first := make([]byte, 5)
	if _, err := io.ReadFull(download.Body, first); err != nil {
		t.Fatal(err)
	}
	close(continueResponse)
	rest, err := io.ReadAll(download.Body)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(first) + string(rest); got != "hello world" {
		t.Fatalf("download = %q", got)
	}
}

func TestDownloadErrorPreservesAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		io.WriteString(w, `{"status":403,"msg":"missing_scope","response":null}`)
	}))
	defer server.Close()

	client, err := NewClient(Config{BaseURL: server.URL, APIKey: "key", HTTPClient: server.Client(), AllowInsecureHTTP: true})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.DownloadWorkspaceFile(context.Background(), "workspace", "file", "")
	if !errors.Is(err, ErrAuthorization) {
		t.Fatalf("error = %v, want ErrAuthorization", err)
	}
}

func TestCrossHostRedirectStripsAPIKey(t *testing.T) {
	var redirectedKey string
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirectedKey = r.Header.Get("X-API-Key")
		io.WriteString(w, "file")
	}))
	defer target.Close()

	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") != "secret" {
			t.Errorf("origin API key = %q", r.Header.Get("X-API-Key"))
		}
		http.Redirect(w, r, target.URL+"/download", http.StatusFound)
	}))
	defer origin.Close()

	client, err := NewClient(Config{BaseURL: origin.URL, APIKey: "secret", HTTPClient: origin.Client(), AllowInsecureHTTP: true})
	if err != nil {
		t.Fatal(err)
	}
	download, err := client.DownloadPrivateFile(context.Background(), "token", "")
	if err != nil {
		t.Fatal(err)
	}
	defer download.Body.Close()
	if redirectedKey != "" {
		t.Fatalf("cross-host redirect forwarded API key %q", redirectedKey)
	}
}

func TestUploadRejectsMissingInput(t *testing.T) {
	client, err := NewClient(Config{BaseURL: "https://example.com", APIKey: "key"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.UploadFile(context.Background(), UploadFileRequest{Contents: strings.NewReader("data")})
	if err == nil {
		t.Fatal("expected missing filename error")
	}
}
