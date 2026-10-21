package handlers

import (
	"strings"
	"testing"
)

func TestPlaceUploadedImages(t *testing.T) {
	up := []string{"/u/a", "/u/b"}
	cases := []struct {
		name string
		urls []string
		want []string
		err  bool
	}{
		{"no placeholders appends", []string{"/k"}, []string{"/k", "/u/a", "/u/b"}, false},
		{"placeholders place files", []string{"upload:1", "/k", "upload:0"}, []string{"/u/b", "/k", "/u/a"}, false},
		{"unplaced files go last", []string{"upload:1", "/k"}, []string{"/u/b", "/k", "/u/a"}, false},
		{"nil keeps nil", nil, []string{"/u/a", "/u/b"}, false},
		{"out of range", []string{"upload:2"}, nil, true},
		{"twice", []string{"upload:0", "upload:0"}, nil, true},
		{"not a number", []string{"upload:x"}, nil, true},
	}
	for _, tc := range cases {
		got, err := placeUploadedImages(tc.urls, up)
		if (err != nil) != tc.err {
			t.Fatalf("%s: err = %v", tc.name, err)
		}
		if !tc.err && strings.Join(got, ",") != strings.Join(tc.want, ",") {
			t.Fatalf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
	if got, _ := placeUploadedImages(nil, nil); got != nil {
		t.Fatalf("nothing given: got %v, want nil", got)
	}
	if got, _ := placeUploadedImages([]string{}, nil); got == nil || len(got) != 0 {
		t.Fatalf("empty list: got %v, want an empty, non-nil list", got)
	}
}
