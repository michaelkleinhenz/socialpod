package handlers

import (
	"strings"
	"testing"

	"socialmedia/internal/models"
)

func TestContentLimitError(t *testing.T) {
	long := strings.Repeat("a", 1000)
	short := strings.Repeat("b", 200)
	both := []models.Platform{models.PlatformBluesky, models.PlatformInstagram}

	cases := []struct {
		name      string
		content   string
		overrides map[string]string
		platforms []models.Platform
		wantErr   bool
	}{
		{"short shared text", short, nil, both, false},
		{"long shared text for Bluesky", long, nil, both, true},
		{"long shared text, Instagram only", long, nil, []models.Platform{models.PlatformInstagram}, false},
		{"long shared text, Bluesky has its own", long, map[string]string{"bluesky": short}, both, false},
		{"every platform has its own", long, map[string]string{"bluesky": short, "instagram": long}, both, false},
		{"Bluesky override too long", short, map[string]string{"bluesky": long}, both, true},
		{"empty override falls back to shared text", long, map[string]string{"bluesky": ""}, both, true},
		{"umlauts count as one character", strings.Repeat("ä", 300), nil, both, false},
	}
	for _, tc := range cases {
		got := contentLimitError(tc.content, tc.overrides, tc.platforms)
		if (got != "") != tc.wantErr {
			t.Errorf("%s: got %q, want error %v", tc.name, got, tc.wantErr)
		}
	}
}
