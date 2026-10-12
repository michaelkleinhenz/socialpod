package services

import (
	"image"
	"image/color"
	"os"
	"testing"
)

func solid(w, h int, c color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, c.A
	}
	return img
}

func TestParseColor(t *testing.T) {
	cases := []struct {
		in   string
		want color.RGBA
		ok   bool
	}{
		{"#fff", color.RGBA{255, 255, 255, 255}, true},
		{"#ff0000", color.RGBA{255, 0, 0, 255}, true},
		{"#00000080", color.RGBA{0, 0, 0, 128}, true},
		{"#ffffff80", color.RGBA{128, 128, 128, 128}, true}, // premultiplied
		{"white", color.RGBA{255, 255, 255, 255}, true},
		{"red", color.RGBA{}, false},
		{"#12345", color.RGBA{}, false},
	}
	for _, tc := range cases {
		got, ok := ParseColor(tc.in)
		if ok != tc.ok || got != tc.want {
			t.Errorf("ParseColor(%q) = %v, %v; want %v, %v", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

func TestComposeSizeAndLayers(t *testing.T) {
	base := solid(400, 300, color.RGBA{200, 30, 30, 255})
	// A frame overlay: opaque blue border, transparent centre.
	frame := image.NewRGBA(image.Rect(0, 0, 100, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			if x < 10 || y < 10 || x >= 90 || y >= 90 {
				frame.Set(x, y, color.RGBA{0, 0, 255, 255})
			}
		}
	}
	logo := solid(50, 50, color.RGBA{0, 255, 0, 255})

	img, err := ComposeImage(ComposeSpec{
		Width: 1080, Height: 1350,
		Base: base, BaseFit: "cover",
		Layers: []ComposeLayer{
			{Kind: "gradient", Anchor: "bottom", Height: 0.4},
			{Kind: "image", Image: frame, Anchor: "full"},
			{Kind: "image", Image: logo, Anchor: "top-right", Margin: 0.2, Width: 0.1},
			{Kind: "text", Text: "Neu: \"Wingspan\" Asia", Anchor: "bottom", Margin: 0.12, FontSize: 0.07, Shadow: true},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != 1080 || b.Dy() != 1350 {
		t.Fatalf("size %v", b)
	}
	// Frame border is blue, the centre shows the red base.
	if c := img.RGBAAt(2, 2); c.B < 200 || c.R > 50 {
		t.Errorf("frame corner = %v, want blue", c)
	}
	if c := img.RGBAAt(540, 400); c.R < 150 || c.B > 80 {
		t.Errorf("centre = %v, want the red base", c)
	}
	// Logo: top-right, 108 px wide, 216 px from the top and right edges.
	if c := img.RGBAAt(1080-216-54, 216+54); c.G < 200 || c.R > 50 {
		t.Errorf("logo = %v, want green", c)
	}
	// The gradient darkens the bottom edge.
	if c := img.RGBAAt(540, 1349); c.R > 120 {
		t.Errorf("bottom edge = %v, want darkened", c)
	}

	if dir := os.Getenv("COMPOSE_PREVIEW_DIR"); dir != "" {
		data, _ := EncodeJPEG(img)
		os.WriteFile(dir+"/compose_test.jpg", data, 0o644)
	}
}

func TestComposeContainAndFilter(t *testing.T) {
	base := solid(200, 100, color.RGBA{255, 0, 0, 255})
	img, err := ComposeImage(ComposeSpec{
		Width: 500, Height: 500, Base: base, BaseFit: "contain",
		Background: color.RGBA{255, 255, 255, 255}, Filter: "grayscale",
	})
	if err != nil {
		t.Fatal(err)
	}
	// Letterbox bands keep the background (the filter runs on them too, and
	// white stays white); the image is grey.
	if c := img.RGBAAt(250, 10); c.R < 250 || c.G < 250 {
		t.Errorf("band = %v, want white", c)
	}
	if c := img.RGBAAt(250, 250); c.R != c.G || c.G != c.B {
		t.Errorf("centre = %v, want grey", c)
	}
}

func TestComposeRejectsBadInput(t *testing.T) {
	if _, err := ComposeImage(ComposeSpec{Width: 0, Height: 10}); err == nil {
		t.Error("zero width accepted")
	}
	if _, err := ComposeImage(ComposeSpec{Width: MaxComposeSide + 1, Height: 10}); err == nil {
		t.Error("oversized canvas accepted")
	}
	if _, err := ComposeImage(ComposeSpec{Width: 10, Height: 10, Layers: []ComposeLayer{{Kind: "blob"}}}); err == nil {
		t.Error("unknown layer kind accepted")
	}
}

func TestThumbnail(t *testing.T) {
	data, w, h, err := Thumbnail(solid(2000, 1000, color.RGBA{1, 2, 3, 255}), 500)
	if err != nil || len(data) == 0 {
		t.Fatal(err)
	}
	if w != 500 || h != 250 {
		t.Errorf("thumbnail %dx%d, want 500x250", w, h)
	}
}
