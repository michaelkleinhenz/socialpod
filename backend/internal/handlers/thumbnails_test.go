package handlers

import (
	"bytes"
	"container/list"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestThumbnailDims(t *testing.T) {
	cases := []struct{ w, h, size, ww, wh int }{
		{4000, 3000, 480, 640, 480},
		{3000, 4000, 480, 480, 640},
		{2000, 2000, 240, 240, 240},
		{10000, 1000, 240, 960, 96},
		{1000, 10000, 240, 96, 960},
	}
	for _, tc := range cases {
		w, h := thumbnailDims(tc.w, tc.h, tc.size)
		if w != tc.ww || h != tc.wh {
			t.Errorf("thumbnailDims(%d, %d, %d) = %dx%d, want %dx%d", tc.w, tc.h, tc.size, w, h, tc.ww, tc.wh)
		}
	}
}

func TestMakeThumbnail(t *testing.T) {
	opaque := image.NewRGBA(image.Rect(0, 0, 1200, 900))
	for i := range opaque.Pix {
		opaque.Pix[i] = 0xff
	}
	var jpg bytes.Buffer
	if err := jpeg.Encode(&jpg, opaque, nil); err != nil {
		t.Fatal(err)
	}
	th, ok := makeThumbnail(jpg.Bytes(), 240)
	if !ok || th.contentType != "image/jpeg" {
		t.Fatalf("jpeg thumbnail: ok=%v type=%q", ok, th.contentType)
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(th.data))
	if err != nil || cfg.Width != 320 || cfg.Height != 240 {
		t.Fatalf("jpeg thumbnail is %dx%d (%v), want 320x240", cfg.Width, cfg.Height, err)
	}

	alpha := image.NewNRGBA(image.Rect(0, 0, 600, 600))
	alpha.Set(1, 1, color.NRGBA{255, 0, 0, 128})
	var pngBuf bytes.Buffer
	if err := png.Encode(&pngBuf, alpha); err != nil {
		t.Fatal(err)
	}
	if th, ok := makeThumbnail(pngBuf.Bytes(), 240); !ok || th.contentType != "image/png" {
		t.Fatalf("transparent thumbnail: ok=%v type=%q, want a PNG", ok, th.contentType)
	}

	// Small enough already: the original is served.
	if _, ok := makeThumbnail(pngBuf.Bytes(), 960); ok {
		t.Fatal("made a thumbnail larger than the image")
	}
}

func TestThumbnailCacheEvicts(t *testing.T) {
	tc := &thumbnailCache{order: list.New(), items: map[string]*list.Element{}}
	big := make([]byte, thumbnailCacheBytes/5)
	for _, k := range []string{"a", "b", "c", "d", "e", "f"} {
		tc.put(k, thumbnail{data: big})
	}
	if _, ok := tc.get("a"); ok {
		t.Fatal("oldest entry still cached")
	}
	if _, ok := tc.get("f"); !ok {
		t.Fatal("newest entry not cached")
	}
	if tc.size > thumbnailCacheBytes {
		t.Fatalf("cache holds %d bytes", tc.size)
	}
}

func TestServeImageThumbnail(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	img := image.NewRGBA(image.Rect(0, 0, 2000, 1500))
	for i := range img.Pix {
		img.Pix[i] = 0xff
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "thumbtest.jpg"), buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	h := &PostHandler{UploadDir: dir}
	r := gin.New()
	r.GET("/api/uploads/:filename", h.ServeImage)

	get := func(url string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, url, nil))
		return w
	}
	for url, want := range map[string]int{
		"/api/uploads/thumbtest.jpg?w=480": 640,
		"/api/uploads/thumbtest.jpg?w=123": 2000, // not a size made: the original
		"/api/uploads/thumbtest.jpg":       2000,
	} {
		w := get(url)
		if w.Code != http.StatusOK {
			t.Fatalf("%s: status %d", url, w.Code)
		}
		cfg, _, err := image.DecodeConfig(w.Body)
		if err != nil || cfg.Width != want {
			t.Errorf("%s: width %d (%v), want %d", url, cfg.Width, err, want)
		}
	}
}
