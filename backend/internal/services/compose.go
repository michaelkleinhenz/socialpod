package services

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"math"
	"strconv"
	"strings"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// Server-side image composition for the MCP compose_image tool: a base image
// (or a plain canvas) fitted to a format, a filter, and layers on top —
// watermarks and other images, text, rectangles and gradients. It is the
// server's counterpart of the browser's ImageEditor, for agents that cannot
// drive the editor. All geometry is given in fractions of the canvas so the
// caller does not have to know pixel sizes.

// MaxComposeSide caps either side of a composed image.
const MaxComposeSide = 2160

// ComposeSpec describes one image to compose.
type ComposeSpec struct {
	Width, Height int
	Background    color.RGBA
	// Base is drawn first, fitted by BaseFit: "cover" (default, crops),
	// "contain" (letterboxed on Background) or "blur" (letterboxed on a
	// blurred, enlarged copy of itself).
	Base    image.Image
	BaseFit string
	// FocusX/FocusY (0..1) choose which part of Base a "cover" crop keeps;
	// 0.5/0.5 is the centre.
	FocusX, FocusY float64
	// Filter is a preset applied to the base: none, grayscale, sepia, warm,
	// cool, vivid, fade, dramatic. Brightness, Contrast and Saturation run
	// from -1 to 1 and are applied after it.
	Filter                           string
	Brightness, Contrast, Saturation float64
	Layers                           []ComposeLayer
}

// ComposeLayer is one layer above the base. Kind is "image", "text", "rect"
// or "gradient".
//
// Placement: Anchor is one of top-left, top, top-right, left, center, right,
// bottom-left, bottom, bottom-right, or full (stretch over the whole canvas,
// for frame overlays). Margin keeps the layer that far from the anchored
// edges (fraction of the canvas width). X and Y, when HasXY is set, place the
// layer's top-left corner instead (fractions of width and height).
// Width and Height are fractions of the canvas width and height; an image
// layer given only one keeps its aspect ratio.
type ComposeLayer struct {
	Kind    string
	Anchor  string
	Margin  float64
	HasXY   bool
	X, Y    float64
	Width   float64
	Height  float64
	Opacity float64 // 0..1, 0 means 1

	// image
	Image image.Image

	// text
	Text         string
	Font         *opentype.Font
	FontSize     float64 // fraction of the canvas width, default 0.06
	Color        color.RGBA
	Align        string // left, center, right; default follows the anchor
	MaxLines     int
	LineSpacing  float64 // multiple of the font size, default 1.2
	OutlineColor color.RGBA
	OutlineWidth float64 // fraction of the font size
	Shadow       bool
	BoxColor     color.RGBA // background box behind the text, when alpha > 0
	BoxPadding   float64    // fraction of the font size, default 0.4
	Radius       float64    // corner radius of box/rect, fraction of the canvas width

	// gradient: from transparent to Color, towards the anchored edge
	// (top or bottom, left or right).
}

// ParseColor reads "#rgb", "#rrggbb", "#rrggbbaa" or a few names. ok is
// false for anything else.
func ParseColor(s string) (color.RGBA, bool) {
	s = strings.TrimSpace(strings.ToLower(s))
	switch s {
	case "white":
		return color.RGBA{255, 255, 255, 255}, true
	case "black":
		return color.RGBA{0, 0, 0, 255}, true
	case "transparent":
		return color.RGBA{}, true
	}
	if !strings.HasPrefix(s, "#") {
		return color.RGBA{}, false
	}
	h := s[1:]
	if len(h) == 3 {
		h = string([]byte{h[0], h[0], h[1], h[1], h[2], h[2]})
	}
	if len(h) == 6 {
		h += "ff"
	}
	if len(h) != 8 {
		return color.RGBA{}, false
	}
	v, err := strconv.ParseUint(h, 16, 32)
	if err != nil {
		return color.RGBA{}, false
	}
	r, g, b, a := uint8(v>>24), uint8(v>>16), uint8(v>>8), uint8(v)
	// color.RGBA is premultiplied.
	return color.RGBA{
		R: uint8(uint16(r) * uint16(a) / 255),
		G: uint8(uint16(g) * uint16(a) / 255),
		B: uint8(uint16(b) * uint16(a) / 255),
		A: a,
	}, true
}

// Compose renders the spec and returns it as a JPEG.
func Compose(spec ComposeSpec) ([]byte, error) {
	img, err := ComposeImage(spec)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ComposeImage renders the spec.
func ComposeImage(spec ComposeSpec) (*image.RGBA, error) {
	if spec.Width <= 0 || spec.Height <= 0 {
		return nil, fmt.Errorf("width and height must be positive")
	}
	if spec.Width > MaxComposeSide || spec.Height > MaxComposeSide {
		return nil, fmt.Errorf("width and height are limited to %d px", MaxComposeSide)
	}
	fonts, err := RadarFonts{}.resolve()
	if err != nil {
		return nil, err
	}
	bg := spec.Background
	if bg.A == 0 {
		bg = color.RGBA{0, 0, 0, 255}
	}
	dst := image.NewRGBA(image.Rect(0, 0, spec.Width, spec.Height))
	xdraw.Draw(dst, dst.Bounds(), &image.Uniform{bg}, image.Point{}, xdraw.Src)

	if spec.Base != nil {
		drawBase(dst, spec)
		applyFilter(dst, spec.Filter, spec.Brightness, spec.Contrast, spec.Saturation)
	}

	for i, l := range spec.Layers {
		if l.Opacity <= 0 || l.Opacity > 1 {
			l.Opacity = 1
		}
		switch l.Kind {
		case "image":
			drawImageLayer(dst, l)
		case "text":
			if l.Font == nil {
				l.Font = fonts.Bold
			}
			drawTextLayer(dst, l)
		case "rect":
			drawRectLayer(dst, l)
		case "gradient":
			drawGradientLayer(dst, l)
		default:
			return nil, fmt.Errorf("layer %d: unknown kind %q", i, l.Kind)
		}
	}
	return dst, nil
}

func drawBase(dst *image.RGBA, spec ComposeSpec) {
	src := spec.Base
	db := dst.Bounds()
	switch spec.BaseFit {
	case "contain":
		fitInto(dst, db, src)
	case "blur":
		blurred := blurCover(src, db.Dx(), db.Dy())
		xdraw.Draw(dst, db, blurred, image.Point{}, xdraw.Src)
		fitInto(dst, db, src)
	default:
		coverFocus(dst, src, spec.FocusX, spec.FocusY)
	}
}

// coverFocus scales src to cover dst, keeping the focus point in view.
func coverFocus(dst *image.RGBA, src image.Image, fx, fy float64) {
	sb := src.Bounds()
	if sb.Dx() == 0 || sb.Dy() == 0 {
		return
	}
	if fx <= 0 || fx > 1 {
		fx = 0.5
	}
	if fy <= 0 || fy > 1 {
		fy = 0.5
	}
	db := dst.Bounds()
	scale := math.Max(float64(db.Dx())/float64(sb.Dx()), float64(db.Dy())/float64(sb.Dy()))
	cropW, cropH := int(float64(db.Dx())/scale), int(float64(db.Dy())/scale)
	x0 := sb.Min.X + int(float64(sb.Dx()-cropW)*fx)
	y0 := sb.Min.Y + int(float64(sb.Dy()-cropH)*fy)
	xdraw.CatmullRom.Scale(dst, db, src, image.Rect(x0, y0, x0+cropW, y0+cropH), xdraw.Src, nil)
}

// blurCover is src enlarged to cover w x h and blurred, by scaling it down
// hard and back up, then darkened a little so a letterboxed image stands out.
func blurCover(src image.Image, w, h int) *image.RGBA {
	full := image.NewRGBA(image.Rect(0, 0, w, h))
	coverFocus(full, src, 0.5, 0.5)
	sw, sh := max(w/24, 1), max(h/24, 1)
	small := image.NewRGBA(image.Rect(0, 0, sw, sh))
	xdraw.ApproxBiLinear.Scale(small, small.Bounds(), full, full.Bounds(), xdraw.Src, nil)
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	xdraw.BiLinear.Scale(out, out.Bounds(), small, small.Bounds(), xdraw.Src, nil)
	shade := &image.Uniform{color.RGBA{0, 0, 0, 70}}
	xdraw.Draw(out, out.Bounds(), shade, image.Point{}, xdraw.Over)
	return out
}

func clamp8(v float64) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v + 0.5)
}

// applyFilter runs the preset and the adjustments over the opaque canvas.
func applyFilter(img *image.RGBA, preset string, brightness, contrast, saturation float64) {
	switch preset {
	case "vivid":
		saturation += 0.35
		contrast += 0.1
	case "dramatic":
		contrast += 0.35
		saturation -= 0.15
		brightness -= 0.05
	case "fade":
		contrast -= 0.25
		brightness += 0.08
		saturation -= 0.2
	}
	if preset == "" || preset == "none" {
		if brightness == 0 && contrast == 0 && saturation == 0 {
			return
		}
	}
	cf := 1 + contrast
	sf := 1 + saturation
	bo := brightness * 255
	p := img.Pix
	for i := 0; i+3 < len(p); i += 4 {
		r, g, b := float64(p[i]), float64(p[i+1]), float64(p[i+2])
		switch preset {
		case "grayscale":
			l := 0.299*r + 0.587*g + 0.114*b
			r, g, b = l, l, l
		case "sepia":
			r, g, b = 0.393*r+0.769*g+0.189*b, 0.349*r+0.686*g+0.168*b, 0.272*r+0.534*g+0.131*b
		case "warm":
			r, b = r*1.08+6, b*0.9
		case "cool":
			r, b = r*0.92, b*1.08+6
		}
		if sf != 1 {
			l := 0.299*r + 0.587*g + 0.114*b
			r, g, b = l+(r-l)*sf, l+(g-l)*sf, l+(b-l)*sf
		}
		if cf != 1 {
			r, g, b = (r-128)*cf+128, (g-128)*cf+128, (b-128)*cf+128
		}
		p[i], p[i+1], p[i+2] = clamp8(r+bo), clamp8(g+bo), clamp8(b+bo)
	}
}

// place resolves a layer's box of size w x h on the canvas.
func place(cw, ch int, l ComposeLayer, w, h int) image.Rectangle {
	if l.HasXY {
		x, y := int(l.X*float64(cw)), int(l.Y*float64(ch))
		return image.Rect(x, y, x+w, y+h)
	}
	m := int(l.Margin * float64(cw))
	var x, y int
	anchor := l.Anchor
	if anchor == "" {
		anchor = "center"
	}
	switch {
	case strings.HasSuffix(anchor, "left") || anchor == "left":
		x = m
	case strings.HasSuffix(anchor, "right") || anchor == "right":
		x = cw - m - w
	default:
		x = (cw - w) / 2
	}
	switch {
	case strings.HasPrefix(anchor, "top"):
		y = m
	case strings.HasPrefix(anchor, "bottom"):
		y = ch - m - h
	default:
		y = (ch - h) / 2
	}
	return image.Rect(x, y, x+w, y+h)
}

func opacityMask(r image.Rectangle, opacity float64) image.Image {
	if opacity >= 1 {
		return nil
	}
	return &image.Uniform{color.Alpha{uint8(opacity * 255)}}
}

func drawImageLayer(dst *image.RGBA, l ComposeLayer) {
	if l.Image == nil {
		return
	}
	cw, ch := dst.Bounds().Dx(), dst.Bounds().Dy()
	sb := l.Image.Bounds()
	if sb.Dx() == 0 || sb.Dy() == 0 {
		return
	}
	var box image.Rectangle
	if l.Anchor == "full" {
		box = dst.Bounds()
	} else {
		w := int(l.Width * float64(cw))
		h := int(l.Height * float64(ch))
		switch {
		case w <= 0 && h <= 0:
			w = cw / 4
			h = w * sb.Dy() / sb.Dx()
		case w <= 0:
			w = h * sb.Dx() / sb.Dy()
		case h <= 0:
			h = w * sb.Dy() / sb.Dx()
		}
		box = place(cw, ch, l, max(w, 1), max(h, 1))
	}
	scaled := image.NewRGBA(image.Rect(0, 0, box.Dx(), box.Dy()))
	xdraw.CatmullRom.Scale(scaled, scaled.Bounds(), l.Image, sb, xdraw.Src, nil)
	xdraw.DrawMask(dst, box, scaled, image.Point{}, opacityMask(box, l.Opacity), image.Point{}, xdraw.Over)
}

func withOpacity(c color.RGBA, o float64) color.RGBA {
	if o >= 1 {
		return c
	}
	return color.RGBA{uint8(float64(c.R) * o), uint8(float64(c.G) * o), uint8(float64(c.B) * o), uint8(float64(c.A) * o)}
}

func drawRectLayer(dst *image.RGBA, l ComposeLayer) {
	cw, ch := dst.Bounds().Dx(), dst.Bounds().Dy()
	var box image.Rectangle
	if l.Anchor == "full" {
		box = dst.Bounds()
	} else {
		w, h := int(l.Width*float64(cw)), int(l.Height*float64(ch))
		if w <= 0 {
			w = cw
		}
		if h <= 0 {
			h = ch / 5
		}
		box = place(cw, ch, l, w, h)
	}
	c := withOpacity(l.Color, l.Opacity)
	if r := int(l.Radius * float64(cw)); r > 0 {
		fillRoundedRect(dst, box.Intersect(dst.Bounds()), r, c)
		return
	}
	xdraw.Draw(dst, box.Intersect(dst.Bounds()), &image.Uniform{c}, image.Point{}, xdraw.Over)
}

// drawGradientLayer fades from transparent to Color towards the anchored
// edge, the usual way to make text over a photo legible.
func drawGradientLayer(dst *image.RGBA, l ComposeLayer) {
	cw, ch := dst.Bounds().Dx(), dst.Bounds().Dy()
	c := l.Color
	if c.A == 0 {
		c = color.RGBA{0, 0, 0, 255}
	}
	horizontal := l.Anchor == "left" || l.Anchor == "right"
	size := l.Height
	if horizontal {
		size = l.Width
	}
	if size <= 0 {
		size = 0.45
	}
	extent := int(size * float64(ch))
	if horizontal {
		extent = int(size * float64(cw))
	}
	if extent <= 0 {
		return
	}
	for i := 0; i < extent; i++ {
		// t is 0 at the inner end of the gradient and 1 at the edge.
		t := float64(i+1) / float64(extent)
		col := withOpacity(c, t*t*l.Opacity)
		var r image.Rectangle
		switch l.Anchor {
		case "top":
			r = image.Rect(0, extent-1-i, cw, extent-i)
		case "left":
			r = image.Rect(extent-1-i, 0, extent-i, ch)
		case "right":
			r = image.Rect(cw-extent+i, 0, cw-extent+i+1, ch)
		default: // bottom
			r = image.Rect(0, ch-extent+i, cw, ch-extent+i+1)
		}
		xdraw.Draw(dst, r, &image.Uniform{col}, image.Point{}, xdraw.Over)
	}
}

func drawTextLayer(dst *image.RGBA, l ComposeLayer) {
	text := strings.TrimSpace(l.Text)
	if text == "" {
		return
	}
	cw, ch := dst.Bounds().Dx(), dst.Bounds().Dy()
	size := l.FontSize
	if size <= 0 {
		size = 0.06
	}
	px := size * float64(cw)
	f := face(l.Font, px)
	defer f.Close()

	maxW := int(l.Width * float64(cw))
	if maxW <= 0 {
		maxW = cw - 2*int(math.Max(l.Margin, 0.05)*float64(cw))
	}
	pad := 0
	if l.BoxColor.A > 0 {
		bp := l.BoxPadding
		if bp <= 0 {
			bp = 0.4
		}
		pad = int(bp * px)
		maxW -= 2 * pad
	}
	maxLines := l.MaxLines
	if maxLines <= 0 {
		maxLines = 6
	}
	var lines []string
	for _, para := range strings.Split(text, "\n") {
		if strings.TrimSpace(para) == "" {
			lines = append(lines, "")
			continue
		}
		lines = append(lines, wrapText(f, para, maxW, maxLines)...)
	}
	if len(lines) > maxLines {
		lines = lines[:maxLines]
	}
	spacing := l.LineSpacing
	if spacing <= 0 {
		spacing = 1.2
	}
	lineH := int(px * spacing)
	widest := 0
	for _, s := range lines {
		widest = max(widest, textWidth(f, s))
	}
	metrics := f.Metrics()
	ascent := metrics.Ascent.Round()
	blockH := lineH*(len(lines)-1) + ascent + metrics.Descent.Round()
	box := place(cw, ch, l, widest+2*pad, blockH+2*pad)

	if l.BoxColor.A > 0 {
		c := withOpacity(l.BoxColor, l.Opacity)
		if r := int(l.Radius * float64(cw)); r > 0 {
			fillRoundedRect(dst, box.Intersect(dst.Bounds()), r, c)
		} else {
			xdraw.Draw(dst, box.Intersect(dst.Bounds()), &image.Uniform{c}, image.Point{}, xdraw.Over)
		}
	}

	align := l.Align
	if align == "" {
		switch {
		case strings.HasSuffix(l.Anchor, "left") || l.Anchor == "left":
			align = "left"
		case strings.HasSuffix(l.Anchor, "right") || l.Anchor == "right":
			align = "right"
		default:
			align = "center"
		}
	}
	textColor := l.Color
	if textColor.A == 0 {
		textColor = color.RGBA{255, 255, 255, 255}
	}
	textColor = withOpacity(textColor, l.Opacity)
	outline := int(l.OutlineWidth * px)
	outlineColor := withOpacity(l.OutlineColor, l.Opacity)
	if outline > 0 && outlineColor.A == 0 {
		outlineColor = withOpacity(color.RGBA{0, 0, 0, 255}, l.Opacity)
	}

	inner := widest
	for i, s := range lines {
		w := textWidth(f, s)
		x := box.Min.X + pad
		switch align {
		case "center":
			x += (inner - w) / 2
		case "right":
			x += inner - w
		}
		baseline := box.Min.Y + pad + ascent + i*lineH
		if l.Shadow {
			off := max(int(px/18), 2)
			drawTextAt(dst, f, withOpacity(color.RGBA{0, 0, 0, 150}, l.Opacity), x+off, baseline+off, s)
		}
		if outline > 0 {
			steps := 16
			for k := 0; k < steps; k++ {
				a := 2 * math.Pi * float64(k) / float64(steps)
				ox := int(math.Round(math.Cos(a) * float64(outline)))
				oy := int(math.Round(math.Sin(a) * float64(outline)))
				drawTextAt(dst, f, outlineColor, x+ox, baseline+oy, s)
			}
		}
		drawTextAt(dst, f, textColor, x, baseline, s)
	}
}

func drawTextAt(dst *image.RGBA, f font.Face, c color.Color, x, baseline int, s string) {
	d := &font.Drawer{Dst: dst, Src: &image.Uniform{c}, Face: f, Dot: fixed.P(x, baseline)}
	d.DrawString(s)
}

// Thumbnail scales img so its longest side is at most maxSide and encodes it
// as a JPEG, for previews returned to an agent.
func Thumbnail(img image.Image, maxSide int) ([]byte, int, int, error) {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w == 0 || h == 0 {
		return nil, 0, 0, fmt.Errorf("empty image")
	}
	if w > maxSide || h > maxSide {
		if w >= h {
			h = h * maxSide / w
			w = maxSide
		} else {
			w = w * maxSide / h
			h = maxSide
		}
	}
	out := image.NewRGBA(image.Rect(0, 0, max(w, 1), max(h, 1)))
	// Transparent areas (watermarks) are shown on a checker-free mid grey.
	xdraw.Draw(out, out.Bounds(), &image.Uniform{color.RGBA{128, 128, 128, 255}}, image.Point{}, xdraw.Src)
	xdraw.CatmullRom.Scale(out, out.Bounds(), img, b, xdraw.Over, nil)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, out, &jpeg.Options{Quality: 80}); err != nil {
		return nil, 0, 0, err
	}
	return buf.Bytes(), out.Bounds().Dx(), out.Bounds().Dy(), nil
}

// DefaultRegularFont is the built-in regular face.
func DefaultRegularFont() (*opentype.Font, error) {
	f, err := RadarFonts{}.resolve()
	return f.Regular, err
}

// EncodeJPEG encodes a composed image at the quality posts are stored with.
func EncodeJPEG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
