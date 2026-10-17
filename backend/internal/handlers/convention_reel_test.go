package handlers

import (
	"strings"
	"testing"
	"time"

	"socialmedia/internal/models"
)

func TestQuickReelTime(t *testing.T) {
	now := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name     string
		uploaded time.Time
		want     time.Time
	}{
		{"just uploaded", now, now.Add(30 * time.Minute)},
		{"uploaded ten minutes ago", now.Add(-10 * time.Minute), now.Add(20 * time.Minute)},
		{"uploaded an hour ago", now.Add(-time.Hour), now},
		{"no upload time", time.Time{}, now.Add(30 * time.Minute)},
		{"upload time in the future", now.Add(time.Hour), now.Add(30 * time.Minute)},
	}
	for _, tc := range cases {
		if got := quickReelTime(tc.uploaded, now); !got.Equal(tc.want) {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestQuickReelCaptionInput(t *testing.T) {
	q := models.ConventionQueue{Name: "SPIEL 2026", Hashtags: []string{"#spiel26", "#essen"}}
	got := quickReelCaptionInput(q, "Wingspan Asia, Stonemaier booth")
	for _, want := range []string{"SPIEL 2026", "Wingspan Asia, Stonemaier booth", "#spiel26 #essen"} {
		if !strings.Contains(got, want) {
			t.Errorf("input lacks %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "website") {
		t.Errorf("input mentions a website the queue does not have:\n%s", got)
	}
}
