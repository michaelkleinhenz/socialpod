package handlers

import (
	"reflect"
	"strings"
	"testing"
)

func TestCheckTemplate(t *testing.T) {
	w, h, urls, err := checkTemplate(`{"version":1,"width":1080,"height":1350,"background":"#ffffff","layers":[
		{"kind":"image","src":"/api/uploads/a.png"},
		{"kind":"text","text":"Hi"},
		{"kind":"image","src":"/api/uploads/a.png"},
		{"kind":"image","src":"https://example.com/b.png"},
		{"kind":"image","src":"/api/uploads/c.jpg"},
		{"kind":"shape","shape":"ellipse","fill":"#000000"},
		{"kind":"paint","strokes":[{"points":[1,2,3,4],"size":10,"feather":20,"color":"#000000","opacity":0.4}]}
	]}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if w != 1080 || h != 1350 {
		t.Errorf("size = %dx%d, want 1080x1350", w, h)
	}
	if want := []string{"/api/uploads/a.png", "/api/uploads/c.jpg"}; !reflect.DeepEqual(urls, want) {
		t.Errorf("urls = %v, want %v", urls, want)
	}

	bad := map[string]string{
		"empty":         ``,
		"not json":      `{`,
		"no size":       `{"layers":[]}`,
		"too large":     `{"width":5000,"height":100,"layers":[]}`,
		"unknown kind":  `{"width":100,"height":100,"layers":[{"kind":"video"}]}`,
		"image, no src": `{"width":100,"height":100,"layers":[{"kind":"image"}]}`,
		"huge":          `{"width":100,"height":100,"pad":"` + strings.Repeat("x", maxTemplateJSON) + `"}`,
	}
	for name, raw := range bad {
		if _, _, _, err := checkTemplate(raw); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}
