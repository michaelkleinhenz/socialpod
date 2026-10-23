package handlers

import (
	"bytes"
	"image"
	"image/draw"
	"image/jpeg"
	"math"

	"socialmedia/internal/services"
)

// maxFittedSide caps the longer side of a carousel page fitted into another
// shape: Instagram shows at most 1440 px across, and the blurred fill is
// costly on a full-size phone photo.
const maxFittedSide = 2160

// carouselAspect is the shape every page of a carousel item is posted in:
// square, like the queue's crop and overlays. Instagram frames a whole
// carousel by its first image and squeezes the others into that frame, so
// pages of mixed shapes would go out distorted. 0 for a single image, which
// keeps its own shape.
func carouselAspect(srcURLs []string) float64 {
	if len(srcURLs) < 2 {
		return 0
	}
	return 1
}

// fitToAspect fits img whole into a frame of the given width/height ratio over
// a blurred, frame-filling copy of itself, so nothing is cut off or stretched.
// It returns img and false when the image already has that shape (within 1%)
// or aspect is 0.
func fitToAspect(img image.Image, aspect float64) (image.Image, bool) {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if aspect <= 0 || w == 0 || h == 0 {
		return img, false
	}
	own := float64(w) / float64(h)
	if math.Abs(own-aspect)/aspect < 0.01 {
		return img, false
	}

	// The frame holds the image at its own size: as wide as a wider image,
	// as tall as a taller one.
	fw, fh := float64(w), float64(h)
	if own > aspect {
		fh = fw / aspect
	} else {
		fw = fh * aspect
	}
	if longer := math.Max(fw, fh); longer > maxFittedSide {
		s := maxFittedSide / longer
		fw, fh = fw*s, fh*s
	}
	frameW, frameH := max(1, int(math.Round(fw))), max(1, int(math.Round(fh)))

	scale := math.Min(float64(frameW)/float64(w), float64(frameH)/float64(h))
	fitW, fitH := max(1, int(float64(w)*scale)), max(1, int(float64(h)*scale))

	dst := image.NewRGBA(image.Rect(0, 0, frameW, frameH))
	draw.Draw(dst, dst.Bounds(), blurredBackground(img, frameW, frameH), image.Point{}, draw.Src)
	x, y := (frameW-fitW)/2, (frameH-fitH)/2
	draw.Draw(dst, image.Rect(x, y, x+fitW, y+fitH), resizeImage(img, fitW, fitH), image.Point{}, draw.Src)
	return dst, true
}

// prepareQueueImage turns a queue item's image into the one that is posted:
// fitted into the carousel's shape (aspect, 0 to leave it) and stamped with the
// queue's overlay (nil for none). It returns false when the image goes out as
// it is, unreadable images included.
func prepareQueueImage(data []byte, aspect float64, overlay image.Image) ([]byte, string, bool) {
	if overlay == nil && aspect <= 0 {
		return data, "", false
	}
	base, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return data, "", false
	}
	// image.Decode ignores EXIF orientation, and the JPEG re-encode below
	// strips the tag, so turn the pixels upright first.
	base = services.ApplyOrientation(base, services.ReadExifOrientation(data))
	img, fitted := fitToAspect(base, aspect)
	if overlay == nil && !fitted {
		return data, "", false
	}
	if overlay != nil {
		img = stampOverlay(img, overlay)
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
		return data, "", false
	}
	return buf.Bytes(), "image/jpeg", true
}
