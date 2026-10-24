package handlers

import (
	"bytes"
	"container/list"
	"context"
	"image"
	"image/jpeg"
	"image/png"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp"

	"socialmedia/internal/services"
	"socialmedia/internal/uploadstore"
)

// Thumbnails: /api/uploads/<file>?w=<size> answers an image scaled down so its
// shorter side is <size> pixels, for grids of many photos (the convention
// gallery) that would otherwise load every camera original in full. Only a few
// sizes are made, so the public endpoint cannot be asked for endless variants,
// and they are kept in a bounded in-memory cache rather than UPLOAD_DIR, which
// holds only the store's own files.

var thumbnailSizes = map[int]bool{240: true, 480: true, 960: true}

const (
	thumbnailCacheBytes = 64 << 20
	// The longer side is capped at this many times the shorter, so a panorama
	// does not come back nearly full size.
	thumbnailMaxAspect = 4
)

// Decoding a camera original takes tens of megabytes; a grid asks for dozens
// at once, so only a couple are made at a time.
var thumbnailSlots = make(chan struct{}, 2)

type thumbnail struct {
	data        []byte
	contentType string
}

type thumbnailCache struct {
	mu    sync.Mutex
	size  int
	order *list.List // front = most recently used
	items map[string]*list.Element
}

type thumbnailEntry struct {
	key string
	t   thumbnail
}

var thumbnails = &thumbnailCache{order: list.New(), items: map[string]*list.Element{}}

func (tc *thumbnailCache) get(key string) (thumbnail, bool) {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	el, ok := tc.items[key]
	if !ok {
		return thumbnail{}, false
	}
	tc.order.MoveToFront(el)
	return el.Value.(*thumbnailEntry).t, true
}

func (tc *thumbnailCache) put(key string, t thumbnail) {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	if _, ok := tc.items[key]; ok || len(t.data) > thumbnailCacheBytes/4 {
		return
	}
	tc.items[key] = tc.order.PushFront(&thumbnailEntry{key: key, t: t})
	tc.size += len(t.data)
	for tc.size > thumbnailCacheBytes {
		el := tc.order.Back()
		e := el.Value.(*thumbnailEntry)
		tc.order.Remove(el)
		delete(tc.items, e.key)
		tc.size -= len(e.t.data)
	}
}

// thumbnailSize reads the w query parameter; false when it is absent or not
// one of the sizes made.
func thumbnailSize(c *gin.Context) (int, bool) {
	w, err := strconv.Atoi(c.Query("w"))
	if err != nil || !thumbnailSizes[w] {
		return 0, false
	}
	return w, true
}

// serveThumbnail answers the thumbnail of an image upload. It returns false,
// having written nothing, when there is none to make (not a still image, too
// small already, unreadable), and the caller serves the file itself.
func (h *PostHandler) serveThumbnail(c *gin.Context, filename string, size int) bool {
	key := filename + "@" + strconv.Itoa(size)
	if t, ok := thumbnails.get(key); ok {
		c.Data(200, t.contentType, t.data)
		return true
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), time.Minute)
	defer cancel()
	data, ct, err := uploadstore.Read(ctx, h.DB, h.UploadDir, filename)
	if err != nil {
		return false
	}
	switch ct {
	case "image/jpeg", "image/png", "image/webp":
	default:
		return false
	}

	select {
	case thumbnailSlots <- struct{}{}:
		defer func() { <-thumbnailSlots }()
	case <-ctx.Done():
		return false
	}
	t, ok := makeThumbnail(data, size)
	if !ok {
		return false
	}
	thumbnails.put(key, t)
	c.Data(200, t.contentType, t.data)
	return true
}

// makeThumbnail scales an image down so its shorter side is size pixels,
// upright by its EXIF orientation (the browser shows the original that way,
// and re-encoding drops the tag). An opaque image becomes a JPEG, one with
// transparency a PNG. False when the image is no bigger than that already.
func makeThumbnail(data []byte, size int) (thumbnail, bool) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || min(cfg.Width, cfg.Height) <= size {
		return thumbnail{}, false
	}
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return thumbnail{}, false
	}
	src = services.ApplyOrientation(src, services.ReadExifOrientation(data))

	b := src.Bounds()
	w, h := thumbnailDims(b.Dx(), b.Dy(), size)
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	xdraw.BiLinear.Scale(dst, dst.Bounds(), src, b, xdraw.Src, nil)

	var buf bytes.Buffer
	if opaque, ok := src.(interface{ Opaque() bool }); ok && opaque.Opaque() {
		if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 82}); err != nil {
			return thumbnail{}, false
		}
		return thumbnail{data: buf.Bytes(), contentType: "image/jpeg"}, true
	}
	if err := png.Encode(&buf, dst); err != nil {
		return thumbnail{}, false
	}
	return thumbnail{data: buf.Bytes(), contentType: "image/png"}, true
}

// thumbnailDims scales w×h so the shorter side is size, keeping the shape
// (the gallery tells square photos by their thumbnail), with the longer side
// capped at thumbnailMaxAspect times size.
func thumbnailDims(w, h, size int) (int, int) {
	short, long := min(w, h), max(w, h)
	scaledLong := (long*size + short/2) / short
	if scaledLong > size*thumbnailMaxAspect {
		// Keep the shape: shrink the short side with the long one.
		scaledLong = size * thumbnailMaxAspect
		size = max(1, (short*scaledLong+long/2)/long)
	}
	if w <= h {
		return size, scaledLong
	}
	return scaledLong, size
}
