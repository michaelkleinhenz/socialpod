package services

import (
	"image"
	"image/color"
	"testing"
)

func TestParseTemplateDesign(t *testing.T) {
	d, err := ParseTemplateDesign(`{"version":1,"width":1080,"height":1350,"background":"#eee4b8","layers":[
		{"kind":"image","origin":"media","src":"/api/uploads/a.png","x":540,"y":675,"width":200,"height":100,"rotation":0,"opacity":0.5},
		{"kind":"text","text":"Hallo","fontFamily":"SocialPod Media 6ace3400bf05ccc83f413580","fontSize":56,"fill":"#ffffff","bold":true,"outline":false,"shadow":false,"align":"left","x":10,"y":20,"rotation":0}]}`)
	if err != nil {
		t.Fatal(err)
	}
	if d.Width != 1080 || len(d.Layers) != 2 || *d.Layers[0].Opacity != 0.5 || d.Layers[1].Align != "left" {
		t.Fatalf("unexpected design %+v", d)
	}
	if id := MediaFontID(d.Layers[1].FontFamily); id != "6ace3400bf05ccc83f413580" {
		t.Errorf("MediaFontID = %q", id)
	}
	if MediaFontID("Arial") != "" {
		t.Error("a system font has no media id")
	}
	for _, bad := range []string{`{`, `{"width":0,"height":10,"layers":[]}`, `{"width":10,"height":10,"layers":[{"kind":"video"}]}`} {
		if _, err := ParseTemplateDesign(bad); err == nil {
			t.Errorf("ParseTemplateDesign(%s) should fail", bad)
		}
	}
}

func near(t *testing.T, img *image.RGBA, x, y int, want color.RGBA) {
	t.Helper()
	got := img.RGBAAt(x, y)
	d := func(a, b uint8) int {
		if a > b {
			return int(a - b)
		}
		return int(b - a)
	}
	if d(got.R, want.R) > 12 || d(got.G, want.G) > 12 || d(got.B, want.B) > 12 {
		t.Errorf("pixel (%d,%d) = %v, want about %v", x, y, got, want)
	}
}

func TestComposeTemplate(t *testing.T) {
	red := color.RGBA{200, 0, 0, 255}
	blue := color.RGBA{0, 0, 200, 255}
	bg := color.RGBA{0xee, 0xe4, 0xb8, 255}
	half := 0.5
	d := TemplateDesign{
		Width: 1000, Height: 1000, Background: "#eee4b8",
		Layers: []TemplateLayer{
			// Centred at (250, 250), 200 x 100, with a 10 px solid border.
			{Kind: "image", X: 250, Y: 250, Width: 200, Height: 100, Image: solid(20, 10, red),
				Border: &TemplateBorder{Style: "solid", Width: 10, Color: "#0000c8"}},
			// Rotated 90° about its centre (750, 250): 100 wide, 300 high.
			{Kind: "image", X: 750, Y: 250, Width: 300, Height: 100, Rotation: 90, Image: solid(30, 10, red)},
			// Half transparent.
			{Kind: "image", X: 250, Y: 750, Width: 200, Height: 200, Opacity: &half, Image: solid(10, 10, color.RGBA{0, 0, 0, 255})},
			// Only the right half of a red|blue image is shown.
			{Kind: "image", X: 750, Y: 750, Width: 100, Height: 100, Image: splitImage(red, blue), SrcCrop: &TemplateCrop{X: 10, Y: 0, Width: 10, Height: 10}},
			{Kind: "text", Text: "HALLO\nWELT", X: 400, Y: 450, FontSize: 60, Fill: "#000000", Bold: true},
		},
	}
	// Drawn at half size, as for a template larger than the compose limit.
	img, err := ComposeImage(ComposeSpec{Width: 500, Height: 500, Template: &d, TemplateScale: 0.5})
	if err != nil {
		t.Fatal(err)
	}
	s := func(v int) int { return v / 2 }
	near(t, img, s(250), s(250), red)                           // image centre
	near(t, img, s(250)-s(100)-2, s(250), blue)                 // border left of the image
	near(t, img, s(250)-s(100)-s(10)-3, s(250), bg)             // outside the border
	near(t, img, s(750), s(250)-s(140), red)                    // rotated: tall now
	near(t, img, s(750)-s(140), s(250), bg)                     // and no longer wide
	near(t, img, s(250), s(750), color.RGBA{119, 114, 92, 255}) // 50 % black over the background
	near(t, img, s(750), s(750), blue)                          // srcCrop shows the right half

	// The text darkens its area and nothing far from it.
	dark := 0
	for y := s(450); y < s(450+140); y++ {
		for x := s(400); x < s(400+260); x++ {
			if img.RGBAAt(x, y).R < 80 {
				dark++
			}
		}
	}
	if dark < 200 {
		t.Errorf("text drew only %d dark pixels", dark)
	}

	// TemplateTextWidth measures the widest line.
	one := TemplateLayer{Kind: "text", Text: "WELT", FontSize: 60, Bold: true}
	two := one
	two.Text = "WELT\nWELTWELT"
	if w1, w2 := TemplateTextWidth(one), TemplateTextWidth(two); w1 <= 0 || w2 < 1.8*w1 {
		t.Errorf("TemplateTextWidth = %v, %v", w1, w2)
	}
}

// splitImage is 20 x 10: left half a, right half b.
func splitImage(a, b color.RGBA) *image.RGBA {
	img := solid(20, 10, a)
	for y := 0; y < 10; y++ {
		for x := 10; x < 20; x++ {
			img.SetRGBA(x, y, b)
		}
	}
	return img
}
