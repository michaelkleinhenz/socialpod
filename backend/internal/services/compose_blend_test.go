package services

import (
	"image"
	"image/color"
	"testing"
)

func TestBlendOnto(t *testing.T) {
	below := color.RGBA{200, 100, 50, 255}
	layer := color.RGBA{100, 200, 255, 255}
	for _, c := range []struct {
		mode string
		want color.RGBA
	}{
		{"multiply", color.RGBA{78, 78, 50, 255}},
		{"screen", color.RGBA{222, 222, 255, 255}},
		{"darken", color.RGBA{100, 100, 50, 255}},
		{"lighten", color.RGBA{200, 200, 255, 255}},
		{"difference", color.RGBA{100, 100, 205, 255}},
		{"overlay", color.RGBA{188, 157, 100, 255}},
		// Luminosity keeps the hue below and takes the layer's lightness.
		{"luminosity", color.RGBA{255, 155, 105, 255}},
	} {
		dst := solid(4, 4, below)
		blendOnto(dst, solid(4, 4, layer), c.mode)
		near(t, dst, 1, 1, c.want)
	}

	// Where the layer is see-through, the picture below stays as it was;
	// at half cover, the blend is mixed in by half.
	dst := solid(2, 1, below)
	src := image.NewRGBA(image.Rect(0, 0, 2, 1))
	src.SetRGBA(1, 0, color.RGBA{0, 0, 0, 128})
	blendOnto(dst, src, "multiply")
	near(t, dst, 0, 0, below)
	near(t, dst, 1, 0, color.RGBA{100, 50, 25, 255})

	// Over nothing, a blended layer is drawn as it is.
	empty := image.NewRGBA(image.Rect(0, 0, 1, 1))
	blendOnto(empty, solid(1, 1, layer), "multiply")
	near(t, empty, 0, 0, layer)

	if isBlendMode("") || isBlendMode("normal") || isBlendMode("xor") || !isBlendMode("soft-light") || !isBlendMode("hue") {
		t.Error("isBlendMode knows the wrong modes")
	}
}

func TestComposeTemplateBlend(t *testing.T) {
	d, err := ParseTemplateDesign(`{"width":100,"height":100,"background":"#c86432","layers":[
		{"kind":"shape","shape":"rect","x":25,"y":50,"width":50,"height":100,"rotation":0,"opacity":1,"fill":"#808080","stroke":"","strokeWidth":0,"blend":"multiply"},
		{"kind":"shape","shape":"rect","x":75,"y":50,"width":50,"height":100,"rotation":0,"opacity":1,"fill":"#808080","stroke":"","strokeWidth":0}]}`)
	if err != nil {
		t.Fatal(err)
	}
	if d.Layers[0].Blend != "multiply" {
		t.Fatalf("blend not read: %+v", d.Layers[0])
	}
	img, err := ComposeImage(ComposeSpec{Width: 100, Height: 100, Template: &d, TemplateScale: 1})
	if err != nil {
		t.Fatal(err)
	}
	near(t, img, 20, 50, color.RGBA{100, 50, 25, 255})   // multiplied with the background
	near(t, img, 80, 50, color.RGBA{128, 128, 128, 255}) // normal: drawn over it
}
