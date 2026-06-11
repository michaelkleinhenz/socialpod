package handlers

import (
	"image"
	"image/color"
	"testing"
)

func TestOverlayAutoOffset_BottomRight(t *testing.T) {
	// Simulate an overlay with opaque pixels concentrated in the bottom-right.
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	for y := 70; y < 100; y++ {
		for x := 70; x < 100; x++ {
			img.Set(x, y, color.RGBA{255, 255, 255, 255})
		}
	}
	dx, dy := overlayAutoOffset(img, 1080)
	if dx >= 0 {
		t.Errorf("expected negative dx (push left), got %d", dx)
	}
	if dy >= 0 {
		t.Errorf("expected negative dy (push up), got %d", dy)
	}
}

func TestOverlayAutoOffset_TopLeft(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	for y := 0; y < 30; y++ {
		for x := 0; x < 30; x++ {
			img.Set(x, y, color.RGBA{255, 255, 255, 255})
		}
	}
	dx, dy := overlayAutoOffset(img, 1080)
	if dx <= 0 {
		t.Errorf("expected positive dx (push right), got %d", dx)
	}
	if dy <= 0 {
		t.Errorf("expected positive dy (push down), got %d", dy)
	}
}

func TestOverlayAutoOffset_Centered(t *testing.T) {
	// Symmetric overlay should produce near-zero offset.
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			img.Set(x, y, color.RGBA{255, 255, 255, 255})
		}
	}
	dx, dy := overlayAutoOffset(img, 1080)
	if abs(dx) > 1 || abs(dy) > 1 {
		t.Errorf("expected near-zero offset for centered overlay, got (%d, %d)", dx, dy)
	}
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func TestOverlayAutoOffset_Empty(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	dx, dy := overlayAutoOffset(img, 1080)
	if dx != 0 || dy != 0 {
		t.Errorf("expected (0, 0) for empty overlay, got (%d, %d)", dx, dy)
	}
}
