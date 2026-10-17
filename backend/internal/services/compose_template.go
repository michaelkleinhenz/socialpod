package services

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"math"
	"strings"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/f64"
)

// Templates from the media library, drawn on the server: the image editor's
// designs (components/Common/ImageEditor/editorModel.ts) rendered without a
// browser, so compose_image can use one as its base. The geometry follows the
// editor's Konva stage: image and shape layers sit by their centre and rotate
// about it, paint layers are brush strokes (compose_paint.go),
// text layers sit by their top-left corner and rotate about that. Fonts the
// server does not have (the editor's system fonts) fall back to the built-in
// Go fonts; dashed and dotted borders are drawn solid and a glow as a soft
// ring, which is close enough for a post image.

// TemplateFontPrefix is how the editor names a media library font: the
// family "SocialPod Media <media id>".
const TemplateFontPrefix = "SocialPod Media "

// TemplateDesign is an editor template, as stored in a media entry.
type TemplateDesign struct {
	Width      float64         `json:"width"`
	Height     float64         `json:"height"`
	Background string          `json:"background"`
	Layers     []TemplateLayer `json:"layers"`
}

// TemplateLayer is one editor layer, in template pixels. Image and Font are
// not part of the JSON: the caller resolves them before rendering.
type TemplateLayer struct {
	Kind     string  `json:"kind"`
	X        float64 `json:"x"`
	Y        float64 `json:"y"`
	Rotation float64 `json:"rotation"`

	// image
	Src          string          `json:"src,omitempty"`
	Width        float64         `json:"width,omitempty"`
	Height       float64         `json:"height,omitempty"`
	Opacity      *float64        `json:"opacity,omitempty"`
	CornerRadius float64         `json:"cornerRadius,omitempty"`
	Border       *TemplateBorder `json:"border,omitempty"`
	SrcCrop      *TemplateCrop   `json:"srcCrop,omitempty"`
	// A video clip (a template made for a reel): only the editor plays it,
	// so a still leaves it out unless an image takes its place.
	Clip json.RawMessage `json:"clip,omitempty"`

	// text
	Text       string  `json:"text,omitempty"`
	FontFamily string  `json:"fontFamily,omitempty"`
	FontSize   float64 `json:"fontSize,omitempty"`
	Fill       string  `json:"fill,omitempty"`
	Bold       bool    `json:"bold,omitempty"`
	Outline    bool    `json:"outline,omitempty"`
	Shadow     bool    `json:"shadow,omitempty"`
	Align      string  `json:"align,omitempty"`

	// shape (also Width, Height, Opacity, CornerRadius and Fill; an empty
	// Fill is none)
	Shape       string  `json:"shape,omitempty"`
	Stroke      string  `json:"stroke,omitempty"`
	StrokeWidth float64 `json:"strokeWidth,omitempty"`

	// paint: brush strokes, moved by (x, y)
	Strokes []TemplateStroke `json:"strokes,omitempty"`

	Image image.Image    `json:"-"`
	Font  *opentype.Font `json:"-"`
}

// TemplateBorder is a frame outside an image layer; Width is in pixels.
type TemplateBorder struct {
	Style string  `json:"style"`
	Width float64 `json:"width"`
	Color string  `json:"color"`
}

// TemplateCrop is the part of a layer's image that is shown, in its pixels.
type TemplateCrop struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// ParseTemplateDesign reads a template's JSON.
func ParseTemplateDesign(raw string) (TemplateDesign, error) {
	var t TemplateDesign
	if err := json.Unmarshal([]byte(raw), &t); err != nil {
		return t, fmt.Errorf("the template is not valid JSON: %v", err)
	}
	if t.Width < 1 || t.Height < 1 {
		return t, fmt.Errorf("the template has no canvas size")
	}
	for i, l := range t.Layers {
		switch l.Kind {
		case "image", "text", "shape", "paint":
		default:
			return t, fmt.Errorf("template layer %d has an unknown kind %q", i, l.Kind)
		}
	}
	return t, nil
}

// MediaFontID returns the media id in a "SocialPod Media <id>" family.
func MediaFontID(family string) string {
	if !strings.HasPrefix(family, TemplateFontPrefix) {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(family, TemplateFontPrefix))
}

// Konva's text metrics as the editor sets them (textLayerAttrs).
const templateLineHeight = 1.15

// templateFont is the font a text layer is drawn in: its resolved font, else
// the built-in bold or regular.
func templateFont(l TemplateLayer, fonts Fonts) *opentype.Font {
	switch {
	case l.Font != nil:
		return l.Font
	case l.Bold:
		return fonts.Bold
	}
	if f, err := DefaultRegularFont(); err == nil {
		return f
	}
	return fonts.Bold
}

// TemplateTextWidth is the width of a text layer's widest line in template
// pixels, as it will be drawn. Replacing a text uses it to keep the layer's
// alignment point.
func TemplateTextWidth(l TemplateLayer) float64 {
	fonts, err := Fonts{}.resolve()
	if err != nil || l.FontSize <= 0 {
		return 0
	}
	f := face(templateFont(l, fonts), l.FontSize)
	defer f.Close()
	widest := 0
	for _, s := range strings.Split(l.Text, "\n") {
		widest = max(widest, textWidth(f, s))
	}
	return float64(widest)
}

// drawTemplate draws t's layers onto dst, scaled by s (canvas px per template
// px).
func drawTemplate(dst *image.RGBA, t TemplateDesign, s float64, fonts Fonts) {
	for _, l := range t.Layers {
		switch l.Kind {
		case "image":
			drawTemplateImage(dst, l, s)
		case "text":
			drawTemplateText(dst, l, s, fonts)
		case "shape":
			drawTemplateShape(dst, l, s)
		case "paint":
			drawTemplatePaint(dst, l, s)
		}
	}
}

func templateOpacity(l TemplateLayer) float64 {
	if l.Opacity == nil {
		return 1
	}
	return math.Max(0, math.Min(1, *l.Opacity))
}

func templateColor(s string, def color.RGBA) color.RGBA {
	if c, ok := ParseColor(s); ok {
		return c
	}
	return def
}

// drawTemplateImage draws an image layer: the image (or its srcCrop)
// stretched to the layer's size with rounded corners, its border outside it,
// rotated about the centre.
func drawTemplateImage(dst *image.RGBA, l TemplateLayer, s float64) {
	if l.Image == nil || l.Width <= 0 || l.Height <= 0 {
		return
	}
	w, h := int(math.Round(l.Width*s)), int(math.Round(l.Height*s))
	if w < 1 || h < 1 {
		return
	}
	radius := int(math.Max(0, math.Min(l.CornerRadius, 50)) / 100 * float64(min(w, h)))
	var bw int
	var bc color.RGBA
	style := ""
	if l.Border != nil && l.Border.Width > 0 {
		bw = max(int(math.Round(l.Border.Width*s)), 1)
		bc = templateColor(l.Border.Color, color.RGBA{0, 0, 0, 255})
		style = l.Border.Style
	}
	pad := bw
	if style == "glow" {
		pad = 2 * bw
	}
	local := image.NewRGBA(image.Rect(0, 0, w+2*pad, h+2*pad))
	inner := image.Rect(pad, pad, pad+w, pad+h)

	if style == "offset" {
		fillRoundedRect(local, inner.Add(image.Pt(bw, bw)), radius, bc)
	}
	if style == "glow" {
		// A soft ring fading outwards, in place of the editor's shadow blur.
		for d := pad; d > 0; d-- {
			a := 1 - float64(d)/float64(pad+1)
			strokeRoundedRect(local, inner.Inset(-d), radiusAround(radius, d), 1, withOpacity(bc, a*a))
		}
	}

	sb := l.Image.Bounds()
	if c := l.SrcCrop; c != nil && c.Width > 0 && c.Height > 0 {
		sb = image.Rect(int(c.X), int(c.Y), int(math.Round(c.X+c.Width)), int(math.Round(c.Y+c.Height))).Add(sb.Min).Intersect(sb)
	}
	if !sb.Empty() {
		pic := image.NewRGBA(image.Rect(0, 0, w, h))
		xdraw.CatmullRom.Scale(pic, pic.Bounds(), l.Image, sb, xdraw.Src, nil)
		var mask image.Image
		if radius > 0 {
			m := image.NewAlpha(pic.Bounds())
			fillRoundedRectAlpha(m, m.Bounds(), radius)
			mask = m
		}
		xdraw.DrawMask(local, inner, pic, image.Point{}, mask, image.Point{}, xdraw.Over)
	}

	switch style {
	case "solid", "dashed", "dotted":
		strokeRoundedRect(local, inner.Inset(-bw), radiusAround(radius, bw), bw, bc)
	case "double":
		third := max(bw/3, 1)
		strokeRoundedRect(local, inner.Inset(-third), radiusAround(radius, third), third, bc)
		strokeRoundedRect(local, inner.Inset(-bw), radiusAround(radius, bw), third, bc)
	}

	// The layer's centre is (x, y); local's centre is the image's centre.
	compositeRotated(dst, local, float64(local.Bounds().Dx())/2, float64(local.Bounds().Dy())/2,
		l.X*s, l.Y*s, l.Rotation, templateOpacity(l))
}

func radiusAround(r, d int) int {
	if r <= 0 {
		return 0
	}
	return r + d
}

// fillRoundedRectAlpha makes r opaque in m with rounded corners.
func fillRoundedRectAlpha(m *image.Alpha, r image.Rectangle, radius int) {
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			if insideRounded(x, y, r, radius) {
				m.SetAlpha(x, y, color.Alpha{0xff})
			}
		}
	}
}

func insideRounded(x, y int, r image.Rectangle, radius int) bool {
	if !image.Pt(x, y).In(r) {
		return false
	}
	if radius <= 0 {
		return true
	}
	radius = min(radius, r.Dx()/2, r.Dy()/2)
	cx, cy := x, y
	switch {
	case x < r.Min.X+radius:
		cx = r.Min.X + radius
	case x >= r.Max.X-radius:
		cx = r.Max.X - radius - 1
	}
	switch {
	case y < r.Min.Y+radius:
		cy = r.Min.Y + radius
	case y >= r.Max.Y-radius:
		cy = r.Max.Y - radius - 1
	}
	dx, dy := x-cx, y-cy
	return dx*dx+dy*dy <= radius*radius
}

// strokeRoundedRect paints a ring width px wide just inside outer.
func strokeRoundedRect(dst *image.RGBA, outer image.Rectangle, radius, width int, c color.RGBA) {
	in := outer.Inset(width)
	innerRadius := max(radius-width, 0)
	mask := image.NewAlpha(outer)
	for y := outer.Min.Y; y < outer.Max.Y; y++ {
		for x := outer.Min.X; x < outer.Max.X; x++ {
			if insideRounded(x, y, outer, radius) && !insideRounded(x, y, in, innerRadius) {
				mask.SetAlpha(x, y, color.Alpha{0xff})
			}
		}
	}
	xdraw.DrawMask(dst, outer, &image.Uniform{c}, image.Point{}, mask, outer.Min, xdraw.Over)
}

// drawTemplateText draws a text layer like Konva does without a set width:
// lines split at \n only, each aligned within the widest one, line height
// 1.15 × the font size, the block's top-left at (x, y) and rotated about it.
func drawTemplateText(dst *image.RGBA, l TemplateLayer, s float64, fonts Fonts) {
	if strings.TrimSpace(l.Text) == "" || l.FontSize <= 0 {
		return
	}
	px := l.FontSize * s
	f := face(templateFont(l, fonts), px)
	defer f.Close()
	lines := strings.Split(l.Text, "\n")
	widest := 0
	for _, line := range lines {
		widest = max(widest, textWidth(f, line))
	}
	lineH := px * templateLineHeight
	m := f.Metrics()
	ascent, descent := float64(m.Ascent.Round()), float64(m.Descent.Round())

	// Room around the text for the outline and the shadow.
	margin := int(math.Ceil(px*0.2)) + 2
	local := image.NewRGBA(image.Rect(0, 0, widest+2*margin, int(math.Ceil(lineH*float64(len(lines))))+2*margin))

	fill := templateColor(l.Fill, color.RGBA{0, 0, 0, 255})
	stroke := 0
	strokeColor := color.RGBA{255, 255, 255, 255}
	if l.Outline {
		// Konva strokes the glyph outline with max(1, 0.08 × size), half of
		// it outside the letters.
		stroke = max(int(math.Round(math.Max(1, px*0.08)/2)), 1)
		if isLightColor(fill) {
			strokeColor = color.RGBA{0, 0, 0, 255}
		}
	}
	for i, line := range lines {
		lw := textWidth(f, line)
		x := margin
		switch l.Align {
		case "left":
		case "right":
			x += widest - lw
		default:
			x += (widest - lw) / 2
		}
		// Konva centres each line in its slot.
		baseline := margin + int(math.Round(float64(i)*lineH+lineH/2+(ascent-descent)/2))
		if l.Shadow {
			off := max(int(math.Round(px*0.03)), 1)
			drawTextAt(local, f, color.RGBA{0, 0, 0, 153}, x+off, baseline+off, line)
		}
		if stroke > 0 {
			for k := 0; k < 16; k++ {
				a := 2 * math.Pi * float64(k) / 16
				drawTextAt(local, f, strokeColor, x+int(math.Round(math.Cos(a)*float64(stroke))), baseline+int(math.Round(math.Sin(a)*float64(stroke))), line)
			}
		}
		drawTextAt(local, f, fill, x, baseline, line)
	}
	compositeRotated(dst, local, float64(margin), float64(margin), l.X*s, l.Y*s, l.Rotation, 1)
}

// isLightColor mirrors the editor's isLight, which picks the outline colour.
func isLightColor(c color.RGBA) bool {
	if c.A == 0 {
		return true
	}
	r, g, b := float64(c.R)*255/float64(c.A), float64(c.G)*255/float64(c.A), float64(c.B)*255/float64(c.A)
	return 0.299*r+0.587*g+0.114*b > 150
}

// compositeRotated draws src over dst so that its point (px, py) lands on
// (dx, dy), rotated by deg degrees (clockwise, as in the editor) about that
// point, at the given opacity.
func compositeRotated(dst *image.RGBA, src *image.RGBA, px, py, dx, dy, deg, opacity float64) {
	if opacity < 1 {
		for i := range src.Pix {
			src.Pix[i] = uint8(float64(src.Pix[i]) * opacity)
		}
	}
	if math.Mod(deg, 360) == 0 {
		at := image.Pt(int(math.Round(dx-px)), int(math.Round(dy-py)))
		xdraw.Draw(dst, src.Bounds().Add(at), src, image.Point{}, xdraw.Over)
		return
	}
	rad := deg * math.Pi / 180
	cos, sin := math.Cos(rad), math.Sin(rad)
	m := f64.Aff3{
		cos, -sin, dx - (cos*px - sin*py),
		sin, cos, dy - (sin*px + cos*py),
	}
	xdraw.BiLinear.Transform(dst, m, src, src.Bounds(), xdraw.Over, nil)
}
