package handlers

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func onceContext(t *testing.T, body *bytes.Buffer, contentType string) *gin.Context {
	t.Helper()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPut, uploadOncePath+"x", body)
	if contentType != "" {
		c.Request.Header.Set("Content-Type", contentType)
	}
	return c
}

func TestReadOnceUpload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	file := bytes.Repeat([]byte("a"), 100)

	// The whole body (curl -T).
	data, err := readOnceUpload(onceContext(t, bytes.NewBuffer(file), "application/octet-stream"), 1000)
	if err != nil || !bytes.Equal(data, file) {
		t.Fatalf("raw body: %v, %d bytes", err, len(data))
	}

	// A multipart file field (curl -F file=@…).
	var form bytes.Buffer
	w := multipart.NewWriter(&form)
	part, _ := w.CreateFormFile("file", "clip.mp4")
	part.Write(file)
	w.Close()
	data, err = readOnceUpload(onceContext(t, &form, w.FormDataContentType()), 1000)
	if err != nil || !bytes.Equal(data, file) {
		t.Fatalf("multipart: %v, %d bytes", err, len(data))
	}

	if _, err := readOnceUpload(onceContext(t, bytes.NewBuffer(file), ""), 50); err == nil {
		t.Error("a file over the limit was accepted")
	}
	if _, err := readOnceUpload(onceContext(t, &bytes.Buffer{}, ""), 50); err == nil {
		t.Error("an empty file was accepted")
	}
}

func TestOnceUploadTypes(t *testing.T) {
	for ext, limit := range map[string]int64{".jpg": maxOnceImageSize, ".mp4": maxClipSize, ".webm": maxClipSize, ".mp3": maxAudioSize, ".ttf": maxFontSize} {
		ct, ok := onceUploadTypes[ext]
		if !ok {
			t.Errorf("%s is not accepted", ext)
			continue
		}
		if got := onceUploadLimit(ct); got != limit {
			t.Errorf("limit for %s = %d, want %d", ext, got, limit)
		}
	}
	if _, ok := onceUploadTypes[".exe"]; ok {
		t.Error(".exe is accepted")
	}
}
