package handlers

import (
	"encoding/json"
	"strings"
	"testing"

	"socialmedia/internal/models"
)

func TestNormalizeDesign(t *testing.T) {
	tmpl := `{"version":1,"width":1080,"height":1920,"background":"#000000","layers":[],"duration":8}`
	// A bare template is wrapped as a draft, a string is read as JSON, a
	// draft is kept.
	for _, raw := range []string{tmpl, mustJSON(tmpl), `{"version":1,"template":` + tmpl + `}`} {
		got, err := normalizeDesign(json.RawMessage(raw))
		if err != nil {
			t.Fatalf("normalizeDesign(%s): %v", raw, err)
		}
		if _, _, _, err := checkImageDraft(got); err != nil {
			t.Errorf("normalizeDesign(%s) = %s, not a draft: %v", raw, got, err)
		}
		if !designIsVideo(got) {
			t.Errorf("normalizeDesign(%s) lost the duration", raw)
		}
	}
	for _, raw := range []string{``, `null`, `[]`, `{"foo":1}`, `"not json"`} {
		if _, err := normalizeDesign(json.RawMessage(raw)); err == nil {
			t.Errorf("normalizeDesign(%q) accepted", raw)
		}
	}
}

func TestBuildPostDesigns(t *testing.T) {
	still := json.RawMessage(`{"version":1,"width":1080,"height":1350,"background":"#fff","layers":[{"kind":"image","src":"/api/uploads/layer.png"}]}`)
	video := json.RawMessage(`{"version":1,"width":1080,"height":1920,"background":"#000","duration":10,"audio":{"src":"/api/uploads/music.mp3"},"layers":[{"kind":"image","src":"/api/uploads/clip.mp4"}]}`)
	one := 0
	images := []string{"/api/uploads/a.jpg", "/api/uploads/b.jpg"}
	designs, urls, err := buildPostDesigns([]postDesignInput{
		{ImageURL: "/api/uploads/a.jpg", Design: still},
		{File: &one, Design: still},
		{PreviewURL: "/api/uploads/preview.jpg", Design: video},
	}, []string{"/api/uploads/b.jpg"}, images)
	if err != nil {
		t.Fatal(err)
	}
	if len(designs) != 3 || designs[1].ImageURL != "/api/uploads/b.jpg" || designs[2].ImageURL != "" {
		t.Fatalf("designs = %+v", designs)
	}
	if designs[0].Video || !designs[2].Video {
		t.Errorf("video flags wrong: %+v", designs)
	}
	if !hasUnexportedDesign(designs) || hasUnexportedDesign(designs[:2]) {
		t.Errorf("hasUnexportedDesign wrong")
	}
	joined := strings.Join(urls, " ")
	for _, u := range []string{"layer.png", "clip.mp4", "music.mp3", "preview.jpg"} {
		if !strings.Contains(joined, u) {
			t.Errorf("design uploads %v lack %s", urls, u)
		}
	}
	if strings.Count(joined, "layer.png") != 1 {
		t.Errorf("design uploads %v repeat a file", urls)
	}

	bad := [][]postDesignInput{
		{{ImageURL: "/api/uploads/other.jpg", Design: still}},
		{{ImageURL: "/api/uploads/a.jpg", Design: still}, {ImageURL: "/api/uploads/a.jpg", Design: still}},
		{{File: &one, Design: still}},
		{{Design: json.RawMessage(`{"layers":[]}`)}},
	}
	for i, in := range bad {
		if _, _, err := buildPostDesigns(in, nil, images); err == nil {
			t.Errorf("bad case %d accepted", i)
		}
	}
}

func TestKeptDesigns(t *testing.T) {
	designs := []models.PostDesign{{ImageURL: "/api/uploads/a.jpg"}, {ImageURL: "/api/uploads/b.jpg"}, {}}
	kept := keptDesigns(designs, []string{"/api/uploads/b.jpg"})
	if len(kept) != 2 || kept[0].ImageURL != "/api/uploads/b.jpg" || kept[1].ImageURL != "" {
		t.Errorf("keptDesigns = %+v", kept)
	}
}
