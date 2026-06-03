package services

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"math"
	"strings"
)

const maxBlobSize = 976_000 // ~976KB, safely under Bluesky's 1MB limit

// ResizeImageIfNeeded checks if image data exceeds maxSize and resizes it.
// It also applies EXIF orientation so images appear correctly on all platforms.
// Returns the (possibly re-encoded) data and the content type to use.
func ResizeImageIfNeeded(data []byte, filename string) ([]byte, string) {
	contentType := detectContentType(filename)

	orientation := readExifOrientation(data)
	needsOrient := orientation >= 2 && orientation <= 8
	needsResize := len(data) > maxBlobSize

	if !needsOrient && !needsResize {
		return data, contentType
	}

	img, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return data, contentType
	}

	if needsOrient {
		img = applyOrientation(img, orientation)
	}

	if !needsResize {
		// Only needed orientation fix — re-encode at high quality
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 95}); err != nil {
			return data, contentType
		}
		return buf.Bytes(), "image/jpeg"
	}

	// Try progressively lower quality JPEG encoding
	for _, quality := range []int{85, 70, 55, 40} {
		resized := img

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

	// Last resort: scale down aggressively
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

	_ = format
	return data, contentType
}

// readExifOrientation parses JPEG EXIF data to extract the orientation tag.
// Returns 1 (normal) if orientation cannot be determined.
func readExifOrientation(data []byte) int {
	if len(data) < 4 || data[0] != 0xFF || data[1] != 0xD8 {
		return 1 // not a JPEG
	}

	offset := 2
	for offset+4 < len(data) {
		if data[offset] != 0xFF {
			return 1
		}
		marker := data[offset+1]
		if marker == 0xE1 { // APP1 — EXIF
			if offset+4 > len(data) {
				return 1
			}
			segLen := int(binary.BigEndian.Uint16(data[offset+2 : offset+4]))
			segData := data[offset+4:]
			if len(segData) < segLen-2 {
				return 1
			}
			segData = segData[:segLen-2]
			return parseExifOrientation(segData)
		}
		// Skip non-APP1 segments
		if offset+4 > len(data) {
			return 1
		}
		segLen := int(binary.BigEndian.Uint16(data[offset+2 : offset+4]))
		offset += 2 + segLen
	}
	return 1
}

func parseExifOrientation(data []byte) int {
	// EXIF header: "Exif\0\0"
	if len(data) < 8 || string(data[:4]) != "Exif" {
		return 1
	}
	tiff := data[6:] // skip "Exif\0\0"

	if len(tiff) < 8 {
		return 1
	}

	var bo binary.ByteOrder
	switch string(tiff[:2]) {
	case "II":
		bo = binary.LittleEndian
	case "MM":
		bo = binary.BigEndian
	default:
		return 1
	}

	magic := bo.Uint16(tiff[2:4])
	if magic != 42 {
		return 1
	}

	ifdOffset := bo.Uint32(tiff[4:8])
	if int(ifdOffset)+2 > len(tiff) {
		return 1
	}

	numEntries := bo.Uint16(tiff[ifdOffset : ifdOffset+2])
	entryStart := int(ifdOffset) + 2

	for i := 0; i < int(numEntries); i++ {
		eOffset := entryStart + i*12
		if eOffset+12 > len(tiff) {
			break
		}
		tag := bo.Uint16(tiff[eOffset : eOffset+2])
		if tag == 0x0112 { // Orientation tag
			val := bo.Uint16(tiff[eOffset+8 : eOffset+10])
			if val >= 1 && val <= 8 {
				return int(val)
			}
			return 1
		}
	}
	return 1
}

// applyOrientation transforms pixel data according to the EXIF orientation value.
//
//	1: normal
//	2: flip horizontal
//	3: rotate 180
//	4: flip vertical
//	5: transpose (flip horizontal + rotate 270 CW)
//	6: rotate 90 CW
//	7: transverse (flip horizontal + rotate 90 CW)
//	8: rotate 270 CW
func applyOrientation(img image.Image, orientation int) image.Image {
	switch orientation {
	case 2:
		return flipH(img)
	case 3:
		return rotate180(img)
	case 4:
		return flipV(img)
	case 5:
		return transpose(img)
	case 6:
		return rotate90(img)
	case 7:
		return transverse(img)
	case 8:
		return rotate270(img)
	default:
		return img
	}
}

func rotate90(img image.Image) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, h, w))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dst.Set(h-1-y, x, img.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return dst
}

func rotate270(img image.Image) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, h, w))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dst.Set(y, w-1-x, img.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return dst
}

func rotate180(img image.Image) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dst.Set(w-1-x, h-1-y, img.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return dst
}

func flipH(img image.Image) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dst.Set(w-1-x, y, img.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return dst
}

func flipV(img image.Image) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dst.Set(x, h-1-y, img.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return dst
}

func transpose(img image.Image) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, h, w))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dst.Set(y, x, img.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return dst
}

func transverse(img image.Image) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, h, w))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dst.Set(h-1-y, w-1-x, img.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return dst
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
	_ = png.Decode
	_ = color.Black // ensure image/color is available
}
