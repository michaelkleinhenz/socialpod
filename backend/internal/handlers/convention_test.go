package handlers

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"testing"
	"time"
)

func TestEffectivePostsPerDay_DelayTakesPrecedence(t *testing.T) {
	// 5 posts/day configured but a 10h minimum delay -> at most floor(24/10)=2.
	if got := effectivePostsPerDay(5, 10); got != 2 {
		t.Fatalf("expected 2 posts/day with 10h delay, got %d", got)
	}
	// No delay -> honour the configured count.
	if got := effectivePostsPerDay(5, 0); got != 5 {
		t.Fatalf("expected 5 posts/day with no delay, got %d", got)
	}
	// A delay larger than a day collapses to a single post per day.
	if got := effectivePostsPerDay(3, 30); got != 1 {
		t.Fatalf("expected 1 post/day with 30h delay, got %d", got)
	}
	// The configured count wins when it is below the delay cap.
	if got := effectivePostsPerDay(1, 4); got != 1 {
		t.Fatalf("expected 1 post/day, got %d", got)
	}
	// Safety ceiling.
	if got := effectivePostsPerDay(1000, 0); got != maxConventionPostsPerDay {
		t.Fatalf("expected clamp to %d, got %d", maxConventionPostsPerDay, got)
	}
}

func TestGenerateSlots_DelayPrecedenceCountsPerDay(t *testing.T) {
	h := &ConventionHandler{}
	start := time.Now().Add(24 * time.Hour)
	end := start.AddDate(0, 0, 2) // spans 3 calendar days

	slots := h.generateSlots(start, end, 5, 10)

	// 3 days, capped at 2 posts/day by the 10h minimum delay.
	if len(slots) != 6 {
		t.Fatalf("expected 6 slots (3 days x 2), got %d", len(slots))
	}

	// Slots must be chronological and respect roughly the minimum gap
	// (minimum delay minus the 60 minute jitter) between consecutive posts
	// on the same day.
	for i := 1; i < len(slots); i++ {
		if slots[i].Before(slots[i-1]) {
			t.Fatalf("slots not chronological at index %d", i)
		}
		if slots[i].YearDay() == slots[i-1].YearDay() {
			gap := slots[i].Sub(slots[i-1])
			if gap < 9*time.Hour {
				t.Fatalf("same-day gap %v below minimum delay minus jitter", gap)
			}
		}
	}
}

func TestApplyWatermarkOverlay(t *testing.T) {
	// Base: a 4x4 solid red JPEG.
	base := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			base.Set(x, y, color.RGBA{R: 255, A: 255})
		}
	}
	var baseBuf bytes.Buffer
	if err := jpeg.Encode(&baseBuf, base, nil); err != nil {
		t.Fatalf("encode base: %v", err)
	}

	// Overlay: a 2x2 opaque blue PNG that gets stretched to cover the base.
	overlay := image.NewRGBA(image.Rect(0, 0, 2, 2))
	for y := 0; y < 2; y++ {
		for x := 0; x < 2; x++ {
			overlay.Set(x, y, color.RGBA{B: 255, A: 255})
		}
	}

	out, ct := applyWatermarkOverlay(baseBuf.Bytes(), overlay)
	if ct != "image/jpeg" {
		t.Fatalf("expected image/jpeg, got %s", ct)
	}

	result, _, err := image.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("decode result: %v", err)
	}
	b := result.Bounds()
	if b.Dx() != 4 || b.Dy() != 4 {
		t.Fatalf("expected 4x4 result, got %dx%d", b.Dx(), b.Dy())
	}
	// The opaque overlay should have fully replaced the red base with blue.
	r, g, bl, _ := result.At(2, 2).RGBA()
	if bl>>8 < 200 || r>>8 > 60 || g>>8 > 60 {
		t.Fatalf("expected blue-dominant pixel after overlay, got r=%d g=%d b=%d", r>>8, g>>8, bl>>8)
	}
}

func TestApplyWatermarkOverlay_InvalidBaseReturnsOriginal(t *testing.T) {
	overlay := image.NewRGBA(image.Rect(0, 0, 2, 2))
	junk := []byte("not an image")
	out, _ := applyWatermarkOverlay(junk, overlay)
	if !bytes.Equal(out, junk) {
		t.Fatalf("expected original bytes returned for undecodable base")
	}
}

func TestNextPostDelay_RespectsMinimumAndSpread(t *testing.T) {
	// With no minimum delay, 4 posts/day spreads to ~6h ± up to 60min jitter,
	// so every draw stays inside [5h, 7h].
	for i := 0; i < 200; i++ {
		d := nextPostDelay(4, 0)
		if d < 5*time.Hour || d > 7*time.Hour {
			t.Fatalf("no-delay gap out of expected range: %v", d)
		}
	}

	// A 10h minimum delay caps 5 posts/day at 2/day. The base gap becomes 12h,
	// and the minimum floor guarantees nothing shorter than 10h.
	for i := 0; i < 200; i++ {
		d := nextPostDelay(5, 10)
		if d < 10*time.Hour {
			t.Fatalf("delayed gap %v fell below the 10h minimum", d)
		}
		if d > 13*time.Hour {
			t.Fatalf("delayed gap %v exceeded base 12h plus jitter", d)
		}
	}
}

func TestGenerateSlots_NoDelaySpreadsAcrossDay(t *testing.T) {
	h := &ConventionHandler{}
	start := time.Now().Add(24 * time.Hour)
	end := start.AddDate(0, 0, 0) // single day

	slots := h.generateSlots(start, end, 4, 0)
	if len(slots) != 4 {
		t.Fatalf("expected 4 slots for a single day, got %d", len(slots))
	}
	for _, s := range slots {
		if s.YearDay() != start.YearDay() {
			t.Fatalf("slot %v fell outside the target day", s)
		}
	}
}
