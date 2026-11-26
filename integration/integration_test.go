//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"testing"
	"time"

	fluxsend "github.com/tscrond/fluxsend-sdk"
)

type integrationConfig struct {
	baseURL          string
	privateKey       string
	readOnlyKey      string
	workspaceKey     string
	workspaceID      string
	otherWorkspaceID string
}

func TestPrivateChunkedUploadDownloadAndDelete(t *testing.T) {
	config := loadIntegrationConfig(t)
	client := newClient(t, config.baseURL, config.privateKey)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	folder := "sdk-integration"
	fileName := fmt.Sprintf("private-%d.txt", time.Now().UnixNano())
	filePath := folder + "/" + fileName
	content := bytes.Repeat([]byte("private integration data\n"), 400_000)
	defer client.DeleteFile(context.Background(), filePath) //nolint:errcheck

	result, err := client.UploadFileSession(ctx, fluxsend.UploadSessionRequest{
		File:        bytes.NewReader(content),
		FileName:    fileName,
		Size:        int64(len(content)),
		ContentType: "text/plain",
		Folder:      folder,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.UploadID == "" {
		t.Fatal("upload ID is empty")
	}

	tree, err := client.GetFileTree(ctx, folder)
	if err != nil {
		t.Fatal(err)
	}
	if !containsFile(tree.Files, filePath) {
		t.Fatalf("uploaded file missing from tree: %+v", tree)
	}

	token, err := client.CreatePrivateDownloadToken(ctx, filePath)
	if err != nil {
		t.Fatal(err)
	}
	download, err := client.DownloadPrivateFile(ctx, token.PrivateDownloadToken, "download")
	if err != nil {
		t.Fatal(err)
	}
	downloaded, err := io.ReadAll(download.Body)
	download.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(downloaded, content) {
		t.Fatal("downloaded private content differs from uploaded content")
	}
}

func TestWorkspaceChunkedUploadDownloadAndDelete(t *testing.T) {
	config := loadIntegrationConfig(t)
	client := newClient(t, config.baseURL, config.workspaceKey)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	folder := "sdk-integration"
	fileName := fmt.Sprintf("workspace-%d.txt", time.Now().UnixNano())
	content := bytes.Repeat([]byte("workspace integration data\n"), 400_000)

	result, err := client.UploadWorkspaceFileSession(ctx, config.workspaceID, fluxsend.UploadSessionRequest{
		File:        bytes.NewReader(content),
		FileName:    fileName,
		Size:        int64(len(content)),
		ContentType: "text/plain",
		Folder:      folder,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.UploadID == "" {
		t.Fatal("upload ID is empty")
	}

	tree, err := client.GetWorkspaceFileTree(ctx, config.workspaceID, folder)
	if err != nil {
		t.Fatal(err)
	}
	fileID := workspaceFileID(tree.Files, fileName)
	if fileID == "" {
		t.Fatalf("uploaded workspace file missing from tree: %+v", tree)
	}
	defer client.DeleteWorkspaceFile(context.Background(), config.workspaceID, fileID) //nolint:errcheck

	download, err := client.DownloadWorkspaceFile(ctx, config.workspaceID, fileID, "download")
	if err != nil {
		t.Fatal(err)
	}
	downloaded, err := io.ReadAll(download.Body)
	download.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(downloaded, content) {
		t.Fatal("downloaded workspace content differs from uploaded content")
	}
}

func TestAPIKeyScopeAndWorkspaceBindingErrors(t *testing.T) {
	config := loadIntegrationConfig(t)
	readOnly := newClient(t, config.baseURL, config.readOnlyKey)
	workspace := newClient(t, config.baseURL, config.workspaceKey)
	ctx := context.Background()

	_, err := readOnly.DeleteFile(ctx, "not-owned.txt")
	assertAPIError(t, err, fluxsend.ErrAuthorization, 403)

	_, err = workspace.GetWorkspaceFileTree(ctx, config.otherWorkspaceID, "")
	assertAPIError(t, err, fluxsend.ErrAuthorization, 403)
}

func loadIntegrationConfig(t *testing.T) integrationConfig {
	t.Helper()
	config := integrationConfig{
		baseURL:          os.Getenv("FLUXSEND_INTEGRATION_BASE_URL"),
		privateKey:       os.Getenv("FLUXSEND_INTEGRATION_PRIVATE_KEY"),
		readOnlyKey:      os.Getenv("FLUXSEND_INTEGRATION_READONLY_KEY"),
		workspaceKey:     os.Getenv("FLUXSEND_INTEGRATION_WORKSPACE_KEY"),
		workspaceID:      os.Getenv("FLUXSEND_INTEGRATION_WORKSPACE_ID"),
		otherWorkspaceID: os.Getenv("FLUXSEND_INTEGRATION_OTHER_WORKSPACE_ID"),
	}
	if config.baseURL == "" || config.privateKey == "" || config.readOnlyKey == "" ||
		config.workspaceKey == "" || config.workspaceID == "" || config.otherWorkspaceID == "" {
		t.Skip("FluxSend integration environment variables are not configured")
	}
	return config
}

func newClient(t *testing.T, baseURL, apiKey string) *fluxsend.Client {
	t.Helper()
	client, err := fluxsend.NewClient(fluxsend.Config{
		BaseURL:           baseURL,
		APIKey:            apiKey,
		AllowInsecureHTTP: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func containsFile(files []fluxsend.FileTreeEntry, name string) bool {
	for _, file := range files {
		if file.Name == name {
			return true
		}
	}
	return false
}

func workspaceFileID(files []fluxsend.WorkspaceFileTreeEntry, name string) string {
	for _, file := range files {
		if file.Name == name {
			return file.ID
		}
	}
	return ""
}

func assertAPIError(t *testing.T, err, kind error, status int) {
	t.Helper()
	var apiErr *fluxsend.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error = %v, want *APIError", err)
	}
	if !errors.Is(err, kind) {
		t.Fatalf("error kind = %v, want %v", err, kind)
	}
	if apiErr.StatusCode != status {
		t.Fatalf("status = %d, want %d", apiErr.StatusCode, status)
	}
}
