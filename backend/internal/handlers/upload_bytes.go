package handlers

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"socialmedia/internal/database"
	"socialmedia/internal/uploadstore"
)

// readImageBytes reads a file referenced by URL: one of our uploads (through
// uploadstore) or, failing that, a remote http(s) URL.
func readImageBytes(ctx context.Context, db *database.MongoDB, uploadDir, url string) ([]byte, error) {
	if name := uploadFilename(url); name != "" {
		data, _, err := uploadstore.Read(ctx, db, uploadDir, name)
		return data, err
	}
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return nil, fmt.Errorf("unsupported image URL")
	}
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 15*1024*1024))
}

// storeUploadBytes saves bytes like any upload (uploadstore: the bucket, a
// record, the disk cache) and returns its URL.
func storeUploadBytes(ctx context.Context, db *database.MongoDB, uploadDir, filename, contentType string, data []byte) (string, error) {
	// Its own deadline, long enough for a video to reach the bucket: callers'
	// contexts are sized for database calls.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
	defer cancel()
	if err := uploadstore.Save(ctx, db, uploadDir, filename, contentType, data); err != nil {
		return "", err
	}
	return uploadURLPrefix + filename, nil
}
