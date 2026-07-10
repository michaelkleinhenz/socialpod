package handlers

import (
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
