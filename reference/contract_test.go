package reference

import (
	"context"
	"io"
	"testing"
)

var (
	_ func(context.Context, *Client, PrivateAPIKeyCreateRequest) (*Result, error)    = CreatePrivateAPIKey
	_ func(context.Context, *Client, CreateUploadRequest) (*Result, error)           = InitializePersonalFileMultipartUpload
	_ func(context.Context, *Client, string, int, io.Reader, int64) (*Result, error) = UploadPersonalFileMultipartPart
)

func TestGeneratedRequestTypesAreUsable(t *testing.T) {
	request := WorkspaceCreateRequest{Name: "Platform", Slug: "platform"}
	if request.Name == "" || request.Slug == "" {
		t.Fatal("generated request type lost required fields")
	}
}
