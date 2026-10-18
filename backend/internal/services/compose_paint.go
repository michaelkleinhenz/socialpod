package services

import (
	"image"
	"image/color"
	"image/draw"
	"math"

	"golang.org/x/image/vector"
)

// Shapes and brush strokes of editor templates, drawn like the editor's
// shapeLayerParts and paintStrokeAttrs (editorModel.ts): a shape's border
// lies on its outline, half inside and half outside, with round joins; a
// stroke is a round-capped line whose edge is blurred by its feather, which
// the editor draws as a canvas shadow (blur b is a Gaussian of sigma b/2).

// TemplateStroke is one brush stroke of a paint layer, its points relative
// to the layer.
type TemplateStroke struct {
	Points  []float64 `json:"points"`
	Size    float64   `json:"size"`
	Feather float64   `json:"feather"`
	Color   string    `json:"color"`
	Opacity *float64  `json:"opacity,omitempty"`
}

// shapeOutline is the outline of a shape layer w × h at (ox, oy), as a
// closed polygon (x0, y0, x1, y1, ...).
func shapeOutline(shape string, ox, oy, w, h, cornerRadius float64) []float64 {
	var pts []float64
	switch shape {
	case "ellipse":
		const n = 120
		for i := 0; i < n; i++ {
			a := 2 * math.Pi * float64(i) / n
			pts = append(pts, ox+w/2+math.Cos(a)*w/2, oy+h/2+math.Sin(a)*h/2)
		}
	case "triangle":
		pts = []float64{ox + w/2, oy, ox + w, oy + h, ox, oy + h}
	default:
		r := math.Max(0, math.Min(cornerRadius, 50)) / 100 * math.Min(w, h)
		if r <= 0 {
			return []float64{ox, oy, ox + w, oy, ox + w, oy + h, ox, oy + h}
		}
		// Each corner an arc about its centre, clockwise from the top left.
		corners := [4][3]float64{
			{ox + r, oy + r, math.Pi}, {ox + w - r, oy + r, 1.5 * math.Pi},
			{ox + w - r, oy + h - r, 0}, {ox + r, oy + h - r, 0.5 * math.Pi},
		}
		const steps = 16
		for _, c := range corners {
			for i := 0; i <= steps; i++ {
				a := c[2] + math.Pi/2*float64(i)/steps
				pts = append(pts, c[0]+math.Cos(a)*r, c[1]+math.Sin(a)*r)
			}
		}
	}
	return pts
}

// An equalizer's bars at rest, as the editor draws them in a still design
// (restLevels, equalizerBars and barCount in equalizer.ts): keep in step.
const (
	equalizerDefaultBars = 24
	equalizerGap         = 0.6
)

func equalizerBarCount(n int) int {
	if n <= 0 {
		n = equalizerDefaultBars
	}
	return max(4, min(64, n))
}

// equalizerRest is each bar's level (0 to 1) at rest.
func equalizerRest(n int) []float64 {
	out := make([]float64, n)
	for i := range out {
		p := 0.0
		if n > 1 {
			p = float64(i) / float64(n-1)
		}
		out[i] = 0.25 + 0.6*(0.5+0.5*math.Sin(float64(i)*1.9+0.6))*(1-0.45*p) + 0.15*(1-p)
	}
	return out
}

// equalizerBars lays the bars out in a w × h box: x, y, width, height each,
// standing on the bottom edge or, mirrored, about the middle, at least as
// high as wide.
func equalizerBars(w, h float64, levels []float64, mirror bool) [][4]float64 {
	n := float64(len(levels))
	bw := w / (n + (n-1)*equalizerGap)
	out := make([][4]float64, len(levels))
	for i, v := range levels {
		bh := math.Min(h, math.Max(bw, h*math.Max(0, math.Min(1, v))))
		y := h - bh
		if mirror {
			y = (h - bh) / 2
		}
		out[i] = [4]float64{float64(i) * bw * (1 + equalizerGap), y, bw, bh}
	}
	return out
}

// drawTemplateShape draws a shape layer: its fill, then its border, turned
// about its centre (x, y) at the layer's opacity.
func drawTemplateShape(dst *image.RGBA, l TemplateLayer, s float64) {
	w, h := l.Width*s, l.Height*s
	if w < 1 || h < 1 {
		return
	}
	sw := math.Max(0, l.StrokeWidth*s)
	stroke, hasStroke := ParseColor(l.Stroke)
	hasStroke = hasStroke && sw > 0
	fill, hasFill := ParseColor(l.Fill)
	if !hasStroke && !hasFill {
		return
	}
	pad := math.Ceil(sw/2) + 2
	local := image.NewRGBA(image.Rect(0, 0, int(math.Ceil(w+2*pad)), int(math.Ceil(h+2*pad))))
	var outlines [][]float64
	if l.Shape == "equalizer" {
		for _, b := range equalizerBars(w, h, equalizerRest(equalizerBarCount(l.Bars)), l.Mirror) {
			// The corners are relative to a bar, like a box's to the box.
			outlines = append(outlines, shapeOutline("rect", pad+b[0], pad+b[1], b[2], b[3], l.CornerRadius))
		}
	} else {
		outlines = [][]float64{shapeOutline(l.Shape, pad, pad, w, h, l.CornerRadius)}
	}

	for _, pts := range outlines {
		if hasFill {
			b := local.Bounds()
			ras := vector.NewRasterizer(b.Dx(), b.Dy())
			ras.MoveTo(float32(pts[0]), float32(pts[1]))
			for i := 2; i < len(pts); i += 2 {
				ras.LineTo(float32(pts[i]), float32(pts[i+1]))
			}
			ras.ClosePath()
			ras.Draw(local, b, &image.Uniform{fill}, image.Point{})
		}
		if hasStroke {
			m := image.NewAlpha(local.Bounds())
			strokeMask(m, pts, sw/2, true)
			draw.DrawMask(local, local.Bounds(), &image.Uniform{stroke}, image.Point{}, m, image.Point{}, draw.Over)
		}
	}
	compositeRotated(dst, local, pad+w/2, pad+h/2, l.X*s, l.Y*s, l.Rotation, templateOpacity(l))
}

// drawTemplatePaint draws a paint layer's strokes in order, each over the
// last.
func drawTemplatePaint(dst *image.RGBA, l TemplateLayer, s float64) {
	for _, st := range l.Strokes {
		if len(st.Points) < 2 || st.Size <= 0 {
			continue
		}
		c, ok := ParseColor(st.Color)
		if !ok {
			continue
		}
		opacity := 1.0
		if st.Opacity != nil {
			opacity = math.Max(0, math.Min(1, *st.Opacity))
		}
		pts := make([]float64, 0, len(st.Points))
		for i := 0; i+1 < len(st.Points); i += 2 {
			pts = append(pts, (st.Points[i]+l.X)*s, (st.Points[i+1]+l.Y)*s)
		}
		paintStroke(dst, pts, st.Size*s/2, math.Max(0, st.Feather*s)/2, c, opacity)
	}
}

// paintStroke paints a round-capped line r wide on each side along pts,
// blurred with a Gaussian of sigma, in c at the given opacity.
func paintStroke(dst *image.RGBA, pts []float64, r, sigma float64, c color.RGBA, opacity float64) {
	reach := r + 3*sigma + 2
	minX, minY, maxX, maxY := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	for i := 0; i < len(pts); i += 2 {
		minX, maxX = math.Min(minX, pts[i]), math.Max(maxX, pts[i])
		minY, maxY = math.Min(minY, pts[i+1]), math.Max(maxY, pts[i+1])
	}
	area := image.Rect(int(math.Floor(minX-reach)), int(math.Floor(minY-reach)), int(math.Ceil(maxX+reach)), int(math.Ceil(maxY+reach)))
	// Paint off the canvas only matters as far as its blur reaches in.
	area = area.Intersect(dst.Bounds().Inset(-int(math.Ceil(3*sigma + 2))))
	if area.Empty() {
		return
	}
	m := image.NewAlpha(area)
	strokeMask(m, pts, r, false)

	w, h := area.Dx(), area.Dy()
	buf := make([]float32, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			buf[y*w+x] = float32(m.Pix[y*m.Stride+x]) / 255
		}
	}
	if sigma > 0.3 {
		buf = gaussBlur(buf, w, h, sigma)
	}

	on := area.Intersect(dst.Bounds())
	for y := on.Min.Y; y < on.Max.Y; y++ {
		for x := on.Min.X; x < on.Max.X; x++ {
			a := float64(buf[(y-area.Min.Y)*w+(x-area.Min.X)]) * opacity
			if a <= 0.001 {
				continue
			}
			a = math.Min(a, 1)
			i := dst.PixOffset(x, y)
			p := dst.Pix[i : i+4 : i+4]
			// c is opaque: c × a over the pixel.
			p[0] = uint8(float64(c.R)*a + float64(p[0])*(1-a) + 0.5)
			p[1] = uint8(float64(c.G)*a + float64(p[1])*(1-a) + 0.5)
			p[2] = uint8(float64(c.B)*a + float64(p[2])*(1-a) + 0.5)
			p[3] = uint8(255*a + float64(p[3])*(1-a) + 0.5)
		}
	}
}

// strokeMask raises m to the coverage of a line r wide on each side of the
// polyline pts, with round ends and joins, anti-aliased. closed joins the
// last point back to the first.
func strokeMask(m *image.Alpha, pts []float64, r float64, closed bool) {
	seg := func(ax, ay, bx, by float64) {
		b := image.Rect(
			int(math.Floor(math.Min(ax, bx)-r-1)), int(math.Floor(math.Min(ay, by)-r-1)),
			int(math.Ceil(math.Max(ax, bx)+r+1)), int(math.Ceil(math.Max(ay, by)+r+1)),
		).Intersect(m.Rect)
		dx, dy := bx-ax, by-ay
		l2 := dx*dx + dy*dy
		for y := b.Min.Y; y < b.Max.Y; y++ {
			py := float64(y) + 0.5
			for x := b.Min.X; x < b.Max.X; x++ {
				px := float64(x) + 0.5
				t := 0.0
				if l2 > 0 {
					t = math.Max(0, math.Min(1, ((px-ax)*dx+(py-ay)*dy)/l2))
				}
				cov := r + 0.5 - math.Hypot(px-ax-t*dx, py-ay-t*dy)
				if cov <= 0 {
					continue
				}
				a := uint8(math.Min(cov, 1)*255 + 0.5)
				i := m.PixOffset(x, y)
				if a > m.Pix[i] {
					m.Pix[i] = a
				}
			}
		}
	}
	n := len(pts) / 2
	if n == 1 {
		seg(pts[0], pts[1], pts[0], pts[1])
		return
	}
	for i := 0; i < n-1; i++ {
		seg(pts[2*i], pts[2*i+1], pts[2*i+2], pts[2*i+3])
	}
	if closed && n > 2 {
		seg(pts[2*n-2], pts[2*n-1], pts[0], pts[1])
	}
}

// gaussBlur approximates a Gaussian blur of sigma with three box blurs, each
// pass along rows then columns; outside the buffer counts as empty.
func gaussBlur(buf []float32, w, h int, sigma float64) []float32 {
	tmp := make([]float32, len(buf))
	for _, size := range gaussBoxes(sigma, 3) {
		r := (size - 1) / 2
		boxBlur(buf, tmp, w, h, r, 1, w) // rows
		boxBlur(tmp, buf, h, w, r, w, 1) // columns
	}
	return buf
}

// gaussBoxes returns n odd box sizes whose successive blurs approximate a
// Gaussian of sigma.
func gaussBoxes(sigma float64, n int) []int {
	ideal := math.Sqrt(12*sigma*sigma/float64(n) + 1)
	wl := int(math.Floor(ideal))
	if wl%2 == 0 {
		wl--
	}
	wu := wl + 2
	m := int(math.Round((12*sigma*sigma - float64(n*wl*wl) - 4*float64(n*wl) - 3*float64(n)) / (-4*float64(wl) - 4)))
	sizes := make([]int, n)
	for i := range sizes {
		if i < m {
			sizes[i] = wl
		} else {
			sizes[i] = wu
		}
	}
	return sizes
}

// boxBlur averages src into dst over 2r+1 samples along lines: lines lines
// of length samples each, a sample step apart, the lines lineStep apart.
func boxBlur(src, dst []float32, samples, lines, r, step, lineStep int) {
	div := float32(2*r + 1)
	for ln := 0; ln < lines; ln++ {
		base := ln * lineStep
		var sum float32
		for i := 0; i <= r && i < samples; i++ {
			sum += src[base+i*step]
		}
		for i := 0; i < samples; i++ {
			dst[base+i*step] = sum / div
			if j := i + r + 1; j < samples {
				sum += src[base+j*step]
			}
			if j := i - r; j >= 0 {
				sum -= src[base+j*step]
			}
		}
	}
}
