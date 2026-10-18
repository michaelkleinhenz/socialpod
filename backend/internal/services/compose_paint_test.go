package services

import (
	"image/color"
	"math"
	"testing"
)

func TestParseTemplateShapesAndPaint(t *testing.T) {
	d, err := ParseTemplateDesign(`{"version":1,"width":100,"height":100,"background":"#ffffff","layers":[
		{"kind":"shape","shape":"ellipse","x":50,"y":50,"width":40,"height":20,"rotation":0,"opacity":1,"fill":"","stroke":"#000000","strokeWidth":3},
		{"kind":"paint","x":5,"y":0,"strokes":[{"points":[1,2,3,4],"size":10,"feather":20,"color":"#000000","opacity":0.4}]}]}`)
	if err != nil {
		t.Fatal(err)
	}
	if l := d.Layers[0]; l.Shape != "ellipse" || l.StrokeWidth != 3 || l.Fill != "" {
		t.Errorf("shape = %+v", l)
	}
	if l := d.Layers[1]; l.X != 5 || len(l.Strokes) != 1 || *l.Strokes[0].Opacity != 0.4 || l.Strokes[0].Feather != 20 {
		t.Errorf("paint = %+v", l)
	}
}

func TestComposeTemplateShapes(t *testing.T) {
	white := color.RGBA{255, 255, 255, 255}
	red := color.RGBA{255, 0, 0, 255}
	blue := color.RGBA{0, 0, 255, 255}
	d := TemplateDesign{
		Width: 400, Height: 400, Background: "#ffffff",
		Layers: []TemplateLayer{
			// A red box with a 10 px blue border on its outline, centred at (100, 100).
			{Kind: "shape", Shape: "rect", X: 100, Y: 100, Width: 100, Height: 60, Fill: "#ff0000", Stroke: "#0000ff", StrokeWidth: 10},
			// A ring: an ellipse with no fill.
			{Kind: "shape", Shape: "ellipse", X: 300, Y: 100, Width: 100, Height: 100, Stroke: "#0000ff", StrokeWidth: 8},
			// A triangle pointing up, turned upside down.
			{Kind: "shape", Shape: "triangle", X: 100, Y: 300, Width: 100, Height: 100, Fill: "#ff0000", Rotation: 180},
		},
	}
	img, err := ComposeImage(ComposeSpec{Width: 400, Height: 400, Template: &d, TemplateScale: 1})
	if err != nil {
		t.Fatal(err)
	}
	near(t, img, 100, 100, red)   // inside the box
	near(t, img, 50, 100, blue)   // on its left edge
	near(t, img, 43, 100, white)  // beyond the border's outer half
	near(t, img, 300, 100, white) // the ring has no fill
	near(t, img, 300, 52, blue)   // but has its outline
	near(t, img, 100, 345, red)   // the turned triangle is wide at the top...
	near(t, img, 60, 260, red)
	near(t, img, 60, 340, white) // ...and narrow at the bottom
}

func TestComposeTemplatePaint(t *testing.T) {
	half := 0.5
	hard := TemplateStroke{Points: []float64{50, 100, 350, 100}, Size: 40, Color: "#000000", Opacity: &half}
	soft := TemplateStroke{Points: []float64{50, 300, 350, 300}, Size: 40, Feather: 60, Color: "#000000"}
	d := TemplateDesign{
		Width: 400, Height: 400, Background: "#ffffff",
		// The layer's x moves its strokes.
		Layers: []TemplateLayer{{Kind: "paint", X: 10, Strokes: []TemplateStroke{hard, soft}}},
	}
	img, err := ComposeImage(ComposeSpec{Width: 400, Height: 400, Template: &d, TemplateScale: 1})
	if err != nil {
		t.Fatal(err)
	}
	near(t, img, 200, 100, color.RGBA{128, 128, 128, 255}) // half black over white
	near(t, img, 200, 125, color.RGBA{255, 255, 255, 255}) // a hard edge
	near(t, img, 365, 100, color.RGBA{128, 128, 128, 255}) // moved right by 10
	// The feathered stroke fades out gradually: darkest on its line, still
	// tinted well past its edge, lighter further out.
	g := func(y int) uint8 { return img.RGBAAt(200, y).R }
	if !(g(300) < g(330) && g(330) < g(350) && g(350) < 250) {
		t.Errorf("feathered stroke grey levels %d, %d, %d", g(300), g(330), g(350))
	}
	if g(300) < 40 {
		t.Errorf("a feathered stroke should not be fully dark at its centre, got %d", g(300))
	}
}

func TestGaussBoxes(t *testing.T) {
	// Three boxes of these sizes have about the Gaussian's variance (box
	// sizes are whole, so a small sigma comes out coarser).
	for _, sigma := range []float64{3, 5, 40, 200} {
		v := 0.0
		for _, s := range gaussBoxes(sigma, 3) {
			if s%2 == 0 {
				t.Fatalf("box size %d is even", s)
			}
			v += float64(s*s-1) / 12
		}
		if math.Abs(math.Sqrt(v)-sigma)/sigma > 0.15 {
			t.Errorf("sigma %v: boxes give %v", sigma, math.Sqrt(v))
		}
	}
}

func TestComposeTemplateEqualizer(t *testing.T) {
	white := color.RGBA{255, 255, 255, 255}
	red := color.RGBA{255, 0, 0, 255}
	d := TemplateDesign{
		Width: 400, Height: 400, Background: "#ffffff",
		// Four bars in a 232 × 100 box centred at (200, 200): 40 px wide with
		// 24 px gaps, standing on y = 250.
		Layers: []TemplateLayer{{Kind: "shape", Shape: "equalizer", Bars: 4, X: 200, Y: 200, Width: 232, Height: 100, Fill: "#ff0000"}},
	}
	img, err := ComposeImage(ComposeSpec{Width: 400, Height: 400, Template: &d, TemplateScale: 1})
	if err != nil {
		t.Fatal(err)
	}
	rest := equalizerRest(4)
	for i, v := range rest {
		x := 84 + 64*i + 20
		// Every bar stands on the bottom and reaches its level, but is at
		// least as high as wide.
		top := 250 - int(math.Max(40, 100*v))
		near(t, img, x, 245, red)
		near(t, img, x, top+6, red)
		near(t, img, x, top-6, white)
	}
	near(t, img, 84+52, 245, white) // the gap between the first two bars

	// Mirrored, the bars grow from the middle both ways.
	d.Layers[0].Mirror = true
	img, err = ComposeImage(ComposeSpec{Width: 400, Height: 400, Template: &d, TemplateScale: 1})
	if err != nil {
		t.Fatal(err)
	}
	near(t, img, 104, 200, red)
	near(t, img, 104, 200-int(50*rest[0])+6, red)
	near(t, img, 104, 200+int(50*rest[0])+6, white)
}
