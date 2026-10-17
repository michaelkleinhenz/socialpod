package services

import (
	"image"
	"math"
)

// Blend modes of template layers, as the image editor sets them
// (BlendMode in editorModel.ts) and the browser's canvas draws them: the W3C
// Compositing and Blending formulas on sRGB values, the layer composited
// source-over with its colour replaced by the blend where it covers the
// picture below.

// blendFuncs are the separable modes, per channel: b is the picture below,
// s the layer, both unpremultiplied in 0..1.
var blendFuncs = map[string]func(b, s float64) float64{
	"multiply": func(b, s float64) float64 { return b * s },
	"screen":   blendScreen,
	"overlay":  func(b, s float64) float64 { return blendHardLight(s, b) },
	"darken":   math.Min,
	"lighten":  math.Max,
	"color-dodge": func(b, s float64) float64 {
		switch {
		case b == 0:
			return 0
		case s >= 1:
			return 1
		}
		return math.Min(1, b/(1-s))
	},
	"color-burn": func(b, s float64) float64 {
		switch {
		case b >= 1:
			return 1
		case s == 0:
			return 0
		}
		return 1 - math.Min(1, (1-b)/s)
	},
	"hard-light": blendHardLight,
	"soft-light": func(b, s float64) float64 {
		if s <= 0.5 {
			return b - (1-2*s)*b*(1-b)
		}
		d := math.Sqrt(b)
		if b <= 0.25 {
			d = ((16*b-12)*b + 4) * b
		}
		return b + (2*s-1)*(d-b)
	},
	"difference": func(b, s float64) float64 { return math.Abs(b - s) },
	"exclusion":  func(b, s float64) float64 { return b + s - 2*b*s },
}

func blendScreen(b, s float64) float64 { return b + s - b*s }

func blendHardLight(b, s float64) float64 {
	if s <= 0.5 {
		return b * 2 * s
	}
	return blendScreen(b, 2*s-1)
}

type rgb [3]float64

// blendNonSeparable are the modes that mix hue, saturation and luminosity.
var blendNonSeparable = map[string]func(b, s rgb) rgb{
	"hue":        func(b, s rgb) rgb { return setLum(setSat(s, sat(b)), lum(b)) },
	"saturation": func(b, s rgb) rgb { return setLum(setSat(b, sat(s)), lum(b)) },
	"color":      func(b, s rgb) rgb { return setLum(s, lum(b)) },
	"luminosity": func(b, s rgb) rgb { return setLum(b, lum(s)) },
}

func lum(c rgb) float64 { return 0.3*c[0] + 0.59*c[1] + 0.11*c[2] }

func clipColor(c rgb) rgb {
	l := lum(c)
	n := math.Min(c[0], math.Min(c[1], c[2]))
	x := math.Max(c[0], math.Max(c[1], c[2]))
	for i := range c {
		if n < 0 {
			c[i] = l + (c[i]-l)*l/(l-n)
		}
		if x > 1 {
			c[i] = l + (c[i]-l)*(1-l)/(x-l)
		}
	}
	return c
}

func setLum(c rgb, l float64) rgb {
	d := l - lum(c)
	return clipColor(rgb{c[0] + d, c[1] + d, c[2] + d})
}

func sat(c rgb) float64 {
	return math.Max(c[0], math.Max(c[1], c[2])) - math.Min(c[0], math.Min(c[1], c[2]))
}

func setSat(c rgb, s float64) rgb {
	// The largest channel becomes s, the smallest 0, the middle one keeps its
	// place between them.
	idx := [3]int{0, 1, 2}
	if c[idx[0]] > c[idx[1]] {
		idx[0], idx[1] = idx[1], idx[0]
	}
	if c[idx[1]] > c[idx[2]] {
		idx[1], idx[2] = idx[2], idx[1]
	}
	if c[idx[0]] > c[idx[1]] {
		idx[0], idx[1] = idx[1], idx[0]
	}
	lo, mid, hi := idx[0], idx[1], idx[2]
	var out rgb
	if c[hi] > c[lo] {
		out[mid] = (c[mid] - c[lo]) * s / (c[hi] - c[lo])
		out[hi] = s
	}
	return out
}

// isBlendMode reports whether mode is one blendOnto draws differently from
// normal.
func isBlendMode(mode string) bool {
	_, sep := blendFuncs[mode]
	_, non := blendNonSeparable[mode]
	return sep || non
}

// blendOnto composites src over dst in the given blend mode. Both are
// alpha-premultiplied and the same size.
func blendOnto(dst, src *image.RGBA, mode string) {
	sepFn := blendFuncs[mode]
	nonFn := blendNonSeparable[mode]
	if sepFn == nil && nonFn == nil {
		return
	}
	r := dst.Bounds().Intersect(src.Bounds())
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			si := src.PixOffset(x, y)
			sp := src.Pix[si : si+4 : si+4]
			if sp[3] == 0 {
				continue
			}
			di := dst.PixOffset(x, y)
			dp := dst.Pix[di : di+4 : di+4]
			as, ab := float64(sp[3])/255, float64(dp[3])/255
			var cs, cb rgb
			for i := 0; i < 3; i++ {
				cs[i] = float64(sp[i]) / 255 / as
				if ab > 0 {
					cb[i] = float64(dp[i]) / 255 / ab
				}
			}
			var mixed rgb
			if nonFn != nil {
				mixed = nonFn(cb, cs)
			} else {
				for i := range mixed {
					mixed[i] = sepFn(cb[i], cs[i])
				}
			}
			ao := as + ab*(1-as)
			for i := 0; i < 3; i++ {
				// Premultiplied: the layer where the picture below is
				// uncovered, the picture where the layer is, the blend where
				// both are.
				co := as*(1-ab)*cs[i] + ab*(1-as)*cb[i] + as*ab*math.Max(0, math.Min(1, mixed[i]))
				dp[i] = uint8(math.Round(math.Min(co, ao) * 255))
			}
			dp[3] = uint8(math.Round(ao * 255))
		}
	}
}
