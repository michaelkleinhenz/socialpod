package services

import (
	"bytes"
	"image"
	"image/jpeg"
	"image/png"
	"math"
	"strings"
)

const maxBlobSize = 976_000 // ~976KB, safely under Bluesky's 1MB limit

// ResizeImageIfNeeded checks if image data exceeds maxSize and resizes it.
// Returns the (possibly resized) data and the content type to use.
func ResizeImageIfNeeded(data []byte, filename string) ([]byte, string) {
	contentType := detectContentType(filename)

	if len(data) <= maxBlobSize {
		return data, contentType
	}

	// Decode the image
	img, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		// Can't decode — try re-encoding as JPEG at lower quality
		return data, contentType
	}

	// Try progressively lower quality JPEG encoding
	// Convert everything to JPEG for size control
	for _, quality := range []int{85, 70, 55, 40} {
		resized := img

		// If still large at lower quality, also scale down
		if quality <= 55 {
			bounds := img.Bounds()
			w, h := bounds.Dx(), bounds.Dy()
			scale := math.Sqrt(float64(maxBlobSize) / float64(len(data)))
			if quality <= 40 {
				scale *= 0.8
			}
			newW := int(float64(w) * scale)
			newH := int(float64(h) * scale)
			if newW > 0 && newH > 0 {
				resized = resizeNearestNeighbor(img, newW, newH)
			}
		}

		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, resized, &jpeg.Options{Quality: quality}); err != nil {
			continue
		}

		if buf.Len() <= maxBlobSize {
			return buf.Bytes(), "image/jpeg"
		}
	}

	// Last resort: scale down aggressively and encode at minimum quality
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	for scale := 0.5; scale >= 0.1; scale -= 0.1 {
		newW := int(float64(w) * scale)
		newH := int(float64(h) * scale)
		if newW < 100 || newH < 100 {
			break
		}
		resized := resizeNearestNeighbor(img, newW, newH)
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, resized, &jpeg.Options{Quality: 60}); err != nil {
			continue
		}
		if buf.Len() <= maxBlobSize {
			return buf.Bytes(), "image/jpeg"
		}
	}

	// Give up, return original and let the API reject it
	_ = format
	return data, contentType
}

// resizeNearestNeighbor does a simple nearest-neighbor resize without external deps.
func resizeNearestNeighbor(src image.Image, newW, newH int) image.Image {
	bounds := src.Bounds()
	srcW := bounds.Dx()
	srcH := bounds.Dy()

	dst := image.NewRGBA(image.Rect(0, 0, newW, newH))

	for y := 0; y < newH; y++ {
		srcY := bounds.Min.Y + y*srcH/newH
		for x := 0; x < newW; x++ {
			srcX := bounds.Min.X + x*srcW/newW
			dst.Set(x, y, src.At(srcX, srcY))
		}
	}

	return dst
}

func detectContentType(filename string) string {
	lower := strings.ToLower(filename)
	switch {
	case strings.HasSuffix(lower, ".png"):
		return "image/png"
	case strings.HasSuffix(lower, ".webp"):
		return "image/webp"
	case strings.HasSuffix(lower, ".gif"):
		return "image/gif"
	default:
		return "image/jpeg"
	}
}

// Register PNG decoder so image.Decode can handle PNGs
func init() {
	// png and jpeg are registered by importing their packages
	_ = png.Decode
}
