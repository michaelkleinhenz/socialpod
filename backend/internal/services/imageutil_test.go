package services

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"testing"
)

func TestReadExifOrientation_NoExif(t *testing.T) {
	// Plain JPEG with no EXIF — should return 1
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	var buf bytes.Buffer
	jpeg.Encode(&buf, img, nil)

	got := readExifOrientation(buf.Bytes())
	if got != 1 {
		t.Errorf("expected orientation 1, got %d", got)
	}
}

func TestReadExifOrientation_NotJPEG(t *testing.T) {
	got := readExifOrientation([]byte("not a jpeg"))
	if got != 1 {
		t.Errorf("expected orientation 1, got %d", got)
	}
}

func TestReadExifOrientation_WithOrientation(t *testing.T) {
	for _, orient := range []int{1, 2, 3, 4, 5, 6, 7, 8} {
		data := buildJPEGWithExifOrientation(orient)
		got := readExifOrientation(data)
		if got != orient {
			t.Errorf("orientation %d: expected %d, got %d", orient, orient, got)
		}
	}
}

func TestApplyOrientation_Rotate90(t *testing.T) {
	// 4x2 image, top-left pixel is red
	img := image.NewRGBA(image.Rect(0, 0, 4, 2))
	img.Set(0, 0, color.RGBA{255, 0, 0, 255})

	rotated := rotate90(img)
	b := rotated.Bounds()
	if b.Dx() != 2 || b.Dy() != 4 {
		t.Fatalf("expected 2x4, got %dx%d", b.Dx(), b.Dy())
	}
	// After 90° CW rotation, top-left pixel (0,0) of original goes to (1,0)
	r, _, _, _ := rotated.At(1, 0).RGBA()
	if r == 0 {
		t.Error("expected red pixel at (1,0) after 90° rotation")
	}
}

func TestApplyOrientation_Rotate180(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 4, 2))
	img.Set(0, 0, color.RGBA{255, 0, 0, 255})

	rotated := rotate180(img)
	b := rotated.Bounds()
	if b.Dx() != 4 || b.Dy() != 2 {
		t.Fatalf("expected 4x2, got %dx%d", b.Dx(), b.Dy())
	}
	r, _, _, _ := rotated.At(3, 1).RGBA()
	if r == 0 {
		t.Error("expected red pixel at (3,1) after 180° rotation")
	}
}

func TestResizeImageIfNeeded_AppliesOrientation(t *testing.T) {
	data := buildJPEGWithExifOrientation(6) // 90° CW
	result, ct := ResizeImageIfNeeded(data, "test.jpg")
	if ct != "image/jpeg" {
		t.Errorf("expected image/jpeg, got %s", ct)
	}
	// The result should be re-encoded (orientation applied)
	decoded, _, err := image.Decode(bytes.NewReader(result))
	if err != nil {
		t.Fatal(err)
	}
	// Original is 4x2, after 90° CW should be 2x4
	b := decoded.Bounds()
	if b.Dx() != 2 || b.Dy() != 4 {
		t.Errorf("expected 2x4 after orientation, got %dx%d", b.Dx(), b.Dy())
	}
}

func TestResizeImageIfNeeded_NoOrientation_SmallFile(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	var buf bytes.Buffer
	jpeg.Encode(&buf, img, nil)

	result, _ := ResizeImageIfNeeded(buf.Bytes(), "test.jpg")
	// Should return unchanged since no orientation and small file
	if !bytes.Equal(result, buf.Bytes()) {
		t.Error("expected unchanged data for small file with no EXIF orientation")
	}
}

// buildJPEGWithExifOrientation creates a minimal JPEG with an EXIF APP1 segment
// containing the given orientation value. The image is 4x2 pixels.
func buildJPEGWithExifOrientation(orientation int) []byte {
	// Build EXIF APP1 segment
	var exif bytes.Buffer

	// EXIF header
	exif.WriteString("Exif")
	exif.Write([]byte{0, 0})

	// TIFF header (little-endian)
	tiffStart := exif.Len()
	exif.WriteString("II")                                    // little-endian
	binary.Write(&exif, binary.LittleEndian, uint16(42))      // magic
	binary.Write(&exif, binary.LittleEndian, uint32(8))       // offset to IFD0

	// IFD0: 1 entry
	binary.Write(&exif, binary.LittleEndian, uint16(1))       // number of entries
	// Entry: orientation tag
	binary.Write(&exif, binary.LittleEndian, uint16(0x0112))  // tag
	binary.Write(&exif, binary.LittleEndian, uint16(3))       // type: SHORT
	binary.Write(&exif, binary.LittleEndian, uint32(1))       // count
	binary.Write(&exif, binary.LittleEndian, uint16(orientation))
	binary.Write(&exif, binary.LittleEndian, uint16(0))       // padding
	// Next IFD offset
	binary.Write(&exif, binary.LittleEndian, uint32(0))
	_ = tiffStart

	exifData := exif.Bytes()

	// Build a JPEG with APP1 segment before the image data
	img := image.NewRGBA(image.Rect(0, 0, 4, 2))
	img.Set(0, 0, color.RGBA{255, 0, 0, 255}) // red top-left

	var jpegBuf bytes.Buffer
	jpeg.Encode(&jpegBuf, img, &jpeg.Options{Quality: 95})
	jpegData := jpegBuf.Bytes()

	// Insert APP1 after SOI marker (FF D8)
	var result bytes.Buffer
	result.Write(jpegData[:2]) // SOI

	// APP1 marker
	result.Write([]byte{0xFF, 0xE1})
	segLen := uint16(len(exifData) + 2)
	binary.Write(&result, binary.BigEndian, segLen)
	result.Write(exifData)

	// Rest of JPEG (skip SOI)
	result.Write(jpegData[2:])

	return result.Bytes()
}
