package fluxsend

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"time"
)

const (
	defaultUploadConcurrency = 4
	cleanupTimeout           = 10 * time.Second
)

// UploadSessionRequest describes a large file backed by random-access storage.
type UploadSessionRequest struct {
	File        io.ReaderAt
	FileName    string
	Size        int64
	ContentType string
	Folder      string
	Concurrency int
	Progress    func(UploadProgress)
}

// UploadProgress reports bytes accepted by completed upload parts.
type UploadProgress struct {
	UploadedBytes int64
	TotalBytes    int64
	Fraction      float64
}

// CompletedUpload describes a successfully completed chunked upload.
type CompletedUpload struct {
	UploadID    string `json:"upload_id"`
	FileName    string `json:"file_name"`
	MD5Checksum string `json:"md5_checksum"`
	Size        int64  `json:"size"`
}

type uploadSessionInit struct {
	UploadID  string `json:"upload_id"`
	ChunkSize int64  `json:"chunk_size"`
}

type uploadPartResponse struct {
	PartNumber      int            `json:"part_number"`
	Size            int64          `json:"size"`
	StorageMetadata map[string]any `json:"storage_metadata"`
}

type uploadAbortResponse struct {
	UploadID     string `json:"upload_id"`
	Status       string `json:"status"`
	UploadedSize int64  `json:"uploaded_size"`
}

type createUploadSessionRequest struct {
	Filename    string `json:"filename"`
	Size        int64  `json:"size"`
	ContentType string `json:"content_type,omitempty"`
	Folder      string `json:"folder,omitempty"`
}

type uploadPaths struct {
	init     string
	part     func(uploadID string, partID int) string
	complete func(uploadID string) string
	abort    func(uploadID string) string
}

// UploadFileSession uploads a large personal file using the multipart API.
func (c *Client) UploadFileSession(ctx context.Context, request UploadSessionRequest) (CompletedUpload, error) {
	return c.uploadSession(ctx, request, uploadPaths{
		init: "/api/files/uploads",
		part: func(uploadID string, partID int) string {
			return "/api/files/uploads/" + url.PathEscape(uploadID) + fmt.Sprintf("/parts/%d", partID)
		},
		complete: func(uploadID string) string {
			return "/api/files/uploads/" + url.PathEscape(uploadID) + "/complete"
		},
		abort: func(uploadID string) string {
			return "/api/files/uploads/" + url.PathEscape(uploadID)
		},
	})
}

// UploadWorkspaceFileSession uploads a large workspace file using the multipart API.
func (c *Client) UploadWorkspaceFileSession(ctx context.Context, workspaceID string, request UploadSessionRequest) (CompletedUpload, error) {
	base := "/api/workspaces/" + url.PathEscape(workspaceID) + "/files/uploads"
	return c.uploadSession(ctx, request, uploadPaths{
		init: base,
		part: func(uploadID string, partID int) string {
			return base + "/" + url.PathEscape(uploadID) + fmt.Sprintf("/parts/%d", partID)
		},
		complete: func(uploadID string) string {
			return base + "/" + url.PathEscape(uploadID) + "/complete"
		},
		abort: func(uploadID string) string {
			return base + "/" + url.PathEscape(uploadID)
		},
	})
}

func (c *Client) uploadSession(parent context.Context, upload UploadSessionRequest, paths uploadPaths) (CompletedUpload, error) {
	var zero CompletedUpload
	if err := validateUploadSession(upload); err != nil {
		return zero, err
	}

	ctx, cancel := context.WithCancel(parent)
	defer cancel()

	init, err := c.doJSON[uploadSessionInit](ctx, request{
		method: http.MethodPost,
		path:   paths.init,
		body: createUploadSessionRequest{
			Filename:    upload.FileName,
			Size:        upload.Size,
			ContentType: upload.ContentType,
			Folder:      upload.Folder,
		},
		requiresAuth: true,
	})
	if err != nil {
		return zero, err
	}
	if init.UploadID == "" || init.ChunkSize <= 0 {
		return zero, fmt.Errorf("%w: invalid upload session response", ErrAPI)
	}

	partCount := int((upload.Size + init.ChunkSize - 1) / init.ChunkSize)
	workerCount := normalizeUploadConcurrency(upload.Concurrency, partCount)
	var nextPart atomic.Int64
	var uploadedBytes atomic.Int64
	var firstErr error
	var errOnce sync.Once
	var progressMu sync.Mutex

	reportProgress := func(final bool) {
		if upload.Progress == nil {
			return
		}
		progressMu.Lock()
		defer progressMu.Unlock()
		done := uploadedBytes.Load()
		if final {
			done = upload.Size
		}
		fraction := float64(done) / float64(upload.Size)
		if fraction > 1 {
			fraction = 1
		}
		upload.Progress(UploadProgress{
			UploadedBytes: done,
			TotalBytes:    upload.Size,
			Fraction:      fraction,
		})
	}
	reportProgress(false)

	var workers sync.WaitGroup
	for range workerCount {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for {
				partID := int(nextPart.Add(1))
				if partID > partCount || ctx.Err() != nil {
					return
				}
				offset := int64(partID-1) * init.ChunkSize
				size := min(init.ChunkSize, upload.Size-offset)
				if _, err := c.doReaderJSON[uploadPartResponse](ctx, request{
					method:        http.MethodPost,
					path:          paths.part(init.UploadID, partID),
					contentType:   "application/octet-stream",
					contentLength: size,
					requiresAuth:  true,
				}, io.NewSectionReader(upload.File, offset, size)); err != nil {
					errOnce.Do(func() {
						firstErr = err
						cancel()
					})
					return
				}
				uploadedBytes.Add(size)
				reportProgress(false)
			}
		}()
	}
	workers.Wait()

	if firstErr == nil && ctx.Err() != nil {
		firstErr = ctx.Err()
	}
	if firstErr == nil {
		completed, completeErr := c.doJSON[CompletedUpload](ctx, request{
			method:       http.MethodPost,
			path:         paths.complete(init.UploadID),
			requiresAuth: true,
		})
		if completeErr == nil {
			reportProgress(true)
			return completed, nil
		}
		firstErr = completeErr
	}

	abortErr := c.abortUpload(parent, paths.abort(init.UploadID))
	if abortErr != nil {
		return zero, errors.Join(firstErr, fmt.Errorf("abort upload: %w", abortErr))
	}
	return zero, firstErr
}

func validateUploadSession(request UploadSessionRequest) error {
	if request.File == nil {
		return fmt.Errorf("upload source is required")
	}
	if request.FileName == "" {
		return fmt.Errorf("upload file name is required")
	}
	if request.Size <= 0 {
		return fmt.Errorf("upload size must be positive")
	}
	return nil
}

func normalizeUploadConcurrency(configured, partCount int) int {
	if configured <= 0 {
		configured = defaultUploadConcurrency
	}
	if configured > partCount {
		configured = partCount
	}
	if configured < 1 {
		return 1
	}
	return configured
}

func (c *Client) abortUpload(parent context.Context, path string) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), cleanupTimeout)
	defer cancel()
	_, err := c.doJSON[uploadAbortResponse](ctx, request{
		method:       http.MethodDelete,
		path:         path,
		requiresAuth: true,
	})
	return err
}
