package services

import (
	"net/url"
	"testing"
)

func TestReelCoverParams(t *testing.T) {
	offset := 2500
	cases := []struct {
		name   string
		extras MediaExtras
		cover  string
		thumb  string
	}{
		{"nothing", MediaExtras{}, "", ""},
		{"frame", MediaExtras{ThumbOffset: &offset}, "", "2500"},
		{"own image made absolute", MediaExtras{CoverURL: "/api/uploads/c.jpg", ThumbOffset: &offset}, "https://pod.example/api/uploads/c.jpg", ""},
		{"absolute image kept", MediaExtras{CoverURL: "https://cdn.example/c.jpg"}, "https://cdn.example/c.jpg", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := url.Values{}
			reelCoverParams(p, tc.extras, "https://pod.example")
			if p.Get("cover_url") != tc.cover || p.Get("thumb_offset") != tc.thumb {
				t.Errorf("cover_url=%q thumb_offset=%q, want %q %q", p.Get("cover_url"), p.Get("thumb_offset"), tc.cover, tc.thumb)
			}
		})
	}
}
