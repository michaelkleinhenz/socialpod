package handlers

import (
	"fmt"
	"image"
	"math"
	"testing"

	"socialmedia/internal/services"
)

const testTemplate = `{"version":1,"width":1000,"height":2000,"background":"#ffffff","layers":[
	{"kind":"image","origin":"media","src":"/api/uploads/bg.png","x":500,"y":1000,"width":1000,"height":2000,"rotation":0,"opacity":1},
	{"kind":"text","text":"TITEL","fontFamily":"SocialPod Media 6ace3400bf05ccc83f413580","fontSize":80,"fill":"#ffffff","bold":true,"outline":false,"shadow":false,"align":"center","x":300,"y":100,"rotation":0},
	{"kind":"text","text":"links","fontFamily":"Arial","fontSize":40,"fill":"#000000","bold":false,"outline":false,"shadow":false,"align":"left","x":50,"y":1800,"rotation":0}]}`

func TestTemplateLayerInfos(t *testing.T) {
	infos := templateLayerInfos(testTemplate)
	if len(infos) != 3 {
		t.Fatalf("got %d layers", len(infos))
	}
	if i := infos[0]; i.Kind != "image" || i.Src != "/api/uploads/bg.png" || i.X != 0 || i.Y != 0 || i.Width != 1 || i.Height != 1 {
		t.Errorf("image layer %+v", i)
	}
	if i := infos[1]; i.Index != 1 || i.Text != "TITEL" || i.Font != "media font 6ace3400bf05ccc83f413580" || i.FontSize != 0.08 || i.X != 0.3 || i.Y != 0.05 {
		t.Errorf("text layer %+v", i)
	}
	if infos[2].Font != "Arial" {
		t.Errorf("system font %q", infos[2].Font)
	}
	if templateLayerInfos("not json") != nil {
		t.Error("no layers for a broken template")
	}
}

func TestCoverCrop(t *testing.T) {
	// A wide image in a square frame keeps its full height.
	c := coverCrop(400, 200, 100, 100, 0, 0)
	if c.Width != 200 || c.Height != 200 || c.X != 100 || c.Y != 0 {
		t.Errorf("centred crop %+v", c)
	}
	// Focus on the left edge.
	if c := coverCrop(400, 200, 100, 100, 0.0001, 0.5); c.X > 1 {
		t.Errorf("left focus %+v", c)
	}
	// A tall image in a wide frame keeps its full width.
	if c := coverCrop(100, 400, 200, 100, 0.5, 1); c.Width != 100 || c.Height != 50 || c.Y != 350 {
		t.Errorf("bottom crop %+v", c)
	}
}

func TestApplyTemplateChanges(t *testing.T) {
	design, err := services.ParseTemplateDesign(testTemplate)
	if err != nil {
		t.Fatal(err)
	}
	title := design.Layers[1]
	left := design.Layers[2]
	var loaded []map[string]any
	load := func(ref map[string]any) (image.Image, error) {
		loaded = append(loaded, ref)
		if ref["url"] == "/broken" {
			return nil, fmt.Errorf("not found")
		}
		return image.NewRGBA(image.Rect(0, 0, 400, 200)), nil
	}
	changes := []any{
		map[string]any{"index": float64(1), "text": "EIN VIEL LÄNGERER TITEL", "color": "#a31b22"},
		map[string]any{"index": float64(2), "text": "links, aber länger", "fontScale": 0.5},
		map[string]any{"index": float64(0), "mediaId": "abc", "focusX": 0.0001},
	}
	if err := applyTemplateChanges(&design, changes, load); err != nil {
		t.Fatal(err)
	}
	got := design.Layers[1]
	if got.Text != "EIN VIEL LÄNGERER TITEL" || got.Fill != "#a31b22" {
		t.Errorf("title %+v", got)
	}
	// A centred text keeps its centre.
	oldCentre := title.X + services.TemplateTextWidth(title)/2
	newCentre := got.X + services.TemplateTextWidth(got)/2
	if math.Abs(oldCentre-newCentre) > 0.5 || got.Y != title.Y {
		t.Errorf("centre moved from %v to %v (y %v to %v)", oldCentre, newCentre, title.Y, got.Y)
	}
	// A left-aligned text keeps its left edge; a smaller size keeps the
	// vertical centre.
	l := design.Layers[2]
	if l.X != left.X || l.FontSize != 20 || math.Abs((l.Y+1.15*20/2)-(left.Y+1.15*40/2)) > 1e-9 {
		t.Errorf("left text %+v", l)
	}
	// The image is swapped and cropped to fill the frame (1:2).
	img := design.Layers[0]
	if img.Image == nil || img.Src != "" || img.SrcCrop == nil || img.SrcCrop.Width != 100 || img.SrcCrop.Height != 200 || img.SrcCrop.X > 1 {
		t.Errorf("image %+v", img)
	}
	if len(loaded) != 1 || loaded[0]["mediaId"] != "abc" {
		t.Errorf("loaded %v", loaded)
	}

	// hide leaves a layer out.
	design, _ = services.ParseTemplateDesign(testTemplate)
	if err := applyTemplateChanges(&design, []any{map[string]any{"index": float64(0), "hide": true}}, load); err != nil {
		t.Fatal(err)
	}
	if len(design.Layers) != 2 || design.Layers[0].Text != "TITEL" {
		t.Errorf("after hide %+v", design.Layers)
	}

	for _, bad := range [][]any{
		{map[string]any{"index": float64(3), "text": "x"}},
		{map[string]any{"index": 1.5, "text": "x"}},
		{map[string]any{"text": "x"}},
		{"x"},
		{map[string]any{"index": float64(1), "color": "rot"}},
		{map[string]any{"index": float64(0), "url": "/broken"}},
	} {
		design, _ = services.ParseTemplateDesign(testTemplate)
		if err := applyTemplateChanges(&design, bad, load); err == nil {
			t.Errorf("changes %v should fail", bad)
		}
	}
}
