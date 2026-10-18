package handlers

import (
	"context"
	"testing"
	"testing/fstest"
)

func TestReadImageBytesTemplateAsset(t *testing.T) {
	old := FrontendFiles
	defer func() { FrontendFiles = old }()
	FrontendFiles = fstest.MapFS{
		"template-assets/photo-warm.jpg": {Data: []byte("jpeg")},
		"index.html":                     {Data: []byte("<!doctype html>")},
	}
	data, err := readImageBytes(context.Background(), nil, "", "/template-assets/photo-warm.jpg")
	if err != nil || string(data) != "jpeg" {
		t.Fatalf("got %q, %v", data, err)
	}
	for _, url := range []string{"/template-assets/missing.jpg", "/template-assets/../index.html"} {
		if _, err := readImageBytes(context.Background(), nil, "", url); err == nil {
			t.Errorf("%s: expected an error", url)
		}
	}
}
