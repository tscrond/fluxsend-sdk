package fluxsend_test

import (
	"context"

	fluxsend "github.com/tscrond/fluxsend-sdk"
)

func ExampleClient_CreateWorkspace() {
	client, err := fluxsend.NewClient(fluxsend.Config{
		BaseURL: "https://fluxsend.app",
		APIKey:  "your-api-key",
	})
	if err != nil {
		return
	}
	_, _ = client.CreateWorkspace(context.Background(), fluxsend.WorkspaceCreateRequest{
		Name: "Platform Team",
		Slug: "platform-team",
	})
}

func ExampleClient_UploadFileSession() {
	client, err := fluxsend.NewClient(fluxsend.Config{
		BaseURL: "https://fluxsend.app",
		APIKey:  "your-api-key",
	})
	if err != nil || client == nil {
		return
	}

	// Pass an *os.File or another io.ReaderAt implementation in real code.
	_ = fluxsend.UploadSessionRequest{
		FileName: "report.pdf",
		Folder:   "documents",
	}
}
