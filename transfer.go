package fluxsend

import (
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
)

// UploadFileRequest streams one file to personal storage.
type UploadFileRequest struct {
	Contents io.Reader
	FileName string
	Folder   string
}

// UploadWorkspaceFileRequest streams one file to workspace storage.
type UploadWorkspaceFileRequest struct {
	Contents io.Reader
	FileName string
	Folder   string
}

// UploadFile streams a direct personal-file upload.
func (c *Client) UploadFile(ctx context.Context, request UploadFileRequest) (string, error) {
	return c.uploadMultipart[string](ctx, "/api/files/upload", request.FileName, request.Folder, request.Contents)
}

// UploadWorkspaceFile streams a direct workspace-file upload.
func (c *Client) UploadWorkspaceFile(ctx context.Context, workspaceID string, request UploadWorkspaceFileRequest) ([]WorkspaceFileResult, error) {
	path := "/api/workspaces/" + url.PathEscape(workspaceID) + "/files/upload"
	return c.uploadMultipart[[]WorkspaceFileResult](ctx, path, request.FileName, request.Folder, request.Contents)
}

func (c *Client) uploadMultipart[T any](ctx context.Context, path, fileName, folder string, contents io.Reader) (T, error) {
	var zero T
	body, contentType, err := streamMultipart(fileName, folder, contents)
	if err != nil {
		return zero, err
	}
	result, err := c.doReaderJSON[T](ctx, request{
		method:       http.MethodPost,
		path:         path,
		contentType:  contentType,
		requiresAuth: true,
	}, body)
	if err != nil {
		return zero, err
	}
	return result, nil
}

func streamMultipart(fileName, folder string, contents io.Reader) (io.ReadCloser, string, error) {
	if fileName == "" {
		return nil, "", fmt.Errorf("upload file name is required")
	}
	if contents == nil {
		return nil, "", fmt.Errorf("upload contents are required")
	}

	reader, writer := io.Pipe()
	form := multipart.NewWriter(writer)
	contentType := form.FormDataContentType()

	go func() {
		var copyErr error
		part, err := form.CreateFormFile("file", fileName)
		if err == nil {
			_, copyErr = io.Copy(part, contents)
		}
		if err == nil && folder != "" {
			err = form.WriteField("folder", folder)
		}
		closeErr := form.Close()
		switch {
		case err != nil:
			copyErr = err
		case copyErr == nil:
			copyErr = closeErr
		}
		_ = writer.CloseWithError(copyErr)
	}()

	return reader, contentType, nil
}

// DownloadPrivateFile opens a streaming private-file download.
func (c *Client) DownloadPrivateFile(ctx context.Context, token, mode string) (*Download, error) {
	return c.download(ctx, "/api/d/private/"+url.PathEscape(token), mode, true)
}

// DownloadPublicShare opens a streaming public-share download.
func (c *Client) DownloadPublicShare(ctx context.Context, token, mode string) (*Download, error) {
	return c.download(ctx, "/api/d/"+url.PathEscape(token), mode, false)
}

// DownloadWorkspaceFile opens a streaming workspace-file download.
func (c *Client) DownloadWorkspaceFile(ctx context.Context, workspaceID, fileID, mode string) (*Download, error) {
	query := url.Values{"file_id": {fileID}}
	if mode != "" {
		query.Set("mode", mode)
	}
	return c.doStream(ctx, request{
		method:       http.MethodGet,
		path:         "/api/workspaces/" + url.PathEscape(workspaceID) + "/files/download",
		query:        query,
		requiresAuth: true,
		retryable:    true,
	})
}

func (c *Client) download(ctx context.Context, path, mode string, requiresAuth bool) (*Download, error) {
	query := url.Values{}
	if mode != "" {
		query.Set("mode", mode)
	}
	return c.doStream(ctx, request{
		method:       http.MethodGet,
		path:         path,
		query:        query,
		requiresAuth: requiresAuth,
		retryable:    true,
	})
}
