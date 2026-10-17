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

	// A reel: its clips are image layers, its music track a file too.
	_, _, urls, err = checkTemplate(`{"version":1,"width":1080,"height":1920,"background":"#000000","duration":10,"layers":[
		{"kind":"image","src":"/api/uploads/clip.mp4","clip":{"trimStart":1,"volume":1,"duration":8},"anim":{"start":0,"end":5}},
		{"kind":"text","text":"Hi","anim":{"start":2}}
	],"audio":{"src":"/api/uploads/music.mp3","start":0,"trimStart":0,"volume":0.8,"fadeIn":1,"fadeOut":2}}`)
	if err != nil {
		t.Fatalf("reel: unexpected error: %v", err)
	}
	if want := []string{"/api/uploads/clip.mp4", "/api/uploads/music.mp3"}; !reflect.DeepEqual(urls, want) {
		t.Errorf("reel urls = %v, want %v", urls, want)
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

func TestCheckImageDraft(t *testing.T) {
	tpl := `{"version":1,"width":6000,"height":4000,"background":"#ffffff","layers":[
		{"kind":"image","src":"/api/uploads/photo.jpg"},
		{"kind":"image","src":"/api/uploads/logo.png"},
		{"kind":"text","text":"Hi"}
	]}`
	w, h, urls, err := checkImageDraft(`{"template":` + tpl + `,"photo":{"src":"/api/uploads/photo.jpg","crop":{"x":0,"y":0,"width":6000,"height":4000}}}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if w != 6000 || h != 4000 {
		t.Errorf("size = %dx%d, want 6000x4000", w, h)
	}
	if want := []string{"/api/uploads/photo.jpg", "/api/uploads/logo.png"}; !reflect.DeepEqual(urls, want) {
		t.Errorf("urls = %v, want %v", urls, want)
	}
	if _, _, urls, err := checkImageDraft(`{"template":{"width":1080,"height":1080,"layers":[]}}`); err != nil || len(urls) != 0 {
		t.Errorf("canvas draft: urls %v, err %v", urls, err)
	}

	bad := map[string]string{
		"empty":       ``,
		"not json":    `{`,
		"no template": `{"photo":{"src":"/api/uploads/a.jpg"}}`,
		"blob photo":  `{"template":{"width":10,"height":10,"layers":[]},"photo":{"src":"blob:http://x/1"}}`,
		"no src":      `{"template":{"width":10,"height":10,"layers":[]},"photo":{}}`,
		"too large":   `{"template":{"width":30000,"height":10,"layers":[]}}`,
	}
	for name, raw := range bad {
		if _, _, _, err := checkImageDraft(raw); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}
