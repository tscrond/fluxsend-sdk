package main

import (
	"context"
	"flag"
	"fmt"
	"mime"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/tscrond/fluxsend-sdk"
)

func main() {
	baseURL := flag.String("base-url", os.Getenv("FLUXSEND_BASE_URL"), "FluxSend instance URL")
	apiKey := flag.String("api-key", os.Getenv("FLUXSEND_API_KEY"), "private API key with private_files:write scope")
	filePath := flag.String("file", "", "file to upload")
	folder := flag.String("folder", "", "optional destination folder")
	contentType := flag.String("content-type", "", "optional content type; inferred from the file extension")
	timeout := flag.Duration("timeout", 10*time.Minute, "overall upload timeout")
	flag.Parse()

	if *baseURL == "" || *apiKey == "" || *filePath == "" {
		flag.Usage()
		fail("-base-url, -api-key, and -file are required")
	}

	file, err := os.Open(*filePath)
	if err != nil {
		fail("open file: %v", err)
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		fail("stat file: %v", err)
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 {
		fail("file must be a non-empty regular file")
	}

	if *contentType == "" {
		*contentType = mime.TypeByExtension(strings.ToLower(filepath.Ext(*filePath)))
		if *contentType == "" {
			*contentType = "application/octet-stream"
		}
	}

	client, err := fluxsend.NewClient(fluxsend.Config{
		BaseURL:           *baseURL,
		APIKey:            *apiKey,
		AllowInsecureHTTP: strings.HasPrefix(strings.ToLower(*baseURL), "http://"),
	})
	if err != nil {
		fail("create client: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()

	lastPercent := -1
	result, err := client.UploadFileSession(ctx, fluxsend.UploadSessionRequest{
		File:        file,
		FileName:    filepath.Base(*filePath),
		Size:        info.Size(),
		ContentType: *contentType,
		Folder:      *folder,
		Progress: func(progress fluxsend.UploadProgress) {
			percent := int(progress.Fraction * 100)
			if percent != lastPercent {
				lastPercent = percent
				fmt.Fprintf(os.Stderr, "\ruploaded: %3d%%", percent)
			}
		},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr)
		fail("upload: %v", err)
	}
	fmt.Fprintln(os.Stderr)
	fmt.Printf("uploaded %s (%d bytes)\n", result.FileName, result.Size)
	fmt.Printf("upload_id: %s\n", result.UploadID)
	fmt.Printf("md5_checksum: %s\n", result.MD5Checksum)
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
