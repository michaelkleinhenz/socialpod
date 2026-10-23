package handlers

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"testing"
)

func solidJPEG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: 200, G: 40, B: 40, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatalf("encode: %v", err)
	}
	return buf.Bytes()
}

func TestImageAspect(t *testing.T) {
	if got := imageAspect(solidJPEG(t, 300, 400)); got != 0.75 {
		t.Fatalf("aspect = %v, want 0.75", got)
	}
	if got := imageAspect([]byte("junk")); got != 0 {
		t.Fatalf("aspect of junk = %v, want 0", got)
	}
}

// A square page behind a 3:4 first page is fitted into 3:4, not stretched:
// same width, taller frame.
func TestPrepareQueueImage_FitsCarouselShape(t *testing.T) {
	out, ct, ok := prepareQueueImage(solidJPEG(t, 120, 120), 0.75, nil)
	if !ok || ct != "image/jpeg" {
		t.Fatalf("expected a fitted JPEG, got ok=%v ct=%q", ok, ct)
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if cfg.Width != 120 || cfg.Height != 160 {
		t.Fatalf("fitted size = %dx%d, want 120x160", cfg.Width, cfg.Height)
	}
}

func TestPrepareQueueImage_KeepsMatchingShape(t *testing.T) {
	data := solidJPEG(t, 300, 400)
	out, _, ok := prepareQueueImage(data, 0.75, nil)
	if ok || !bytes.Equal(out, data) {
		t.Fatal("an image already in the carousel's shape should go out as it is")
	}
	if _, _, ok := prepareQueueImage(data, 0, nil); ok {
		t.Fatal("no aspect and no overlay should leave the image alone")
	}
}

func TestFitToAspect_CapsSize(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 3000, 3000))
	fitted, ok := fitToAspect(img, 0.8)
	if !ok {
		t.Fatal("expected the image to be fitted")
	}
	b := fitted.Bounds()
	if b.Dy() != maxFittedSide || b.Dx() != 1728 {
		t.Fatalf("fitted size = %dx%d, want 1728x%d", b.Dx(), b.Dy(), maxFittedSide)
	}
}
