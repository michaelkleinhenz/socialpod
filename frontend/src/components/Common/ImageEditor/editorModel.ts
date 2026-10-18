import Konva from 'konva';
import type { Filter } from 'konva/lib/Node';
import type { LayerAnim } from './animation';
import { barCount, equalizerBars, restLevels } from './equalizer';

// Everything the editor holds is in source-image pixels: the crop is a window
// onto the original image, and watermarks and texts sit on the original too,
// so cropping again later never moves them relative to the photo.

export interface CropBox {
  x: number;
  y: number;
  width: number;
  height: number;
}

// Image layers are positioned by their centre (offsetX/offsetY = half the
// size), so they rotate and scale about it. Texts are positioned by their
// top-left corner, as Konva measures them.
export interface ImageLayer {
  id: string;
  kind: 'image';
  // The tab it was added from, which selecting it later reopens.
  origin: 'watermark' | 'media';
  // Where the image came from (a watermark's or library entry's URL), so a
  // template can load it again.
  src: string;
  // What is drawn: the picture, or for a clip its video (an export swaps in
  // the decoded frame, a canvas).
  img: LayerSource;
  x: number;
  y: number;
  width: number;
  height: number;
  rotation: number;
  opacity: number;
  // Corner rounding in percent of the shorter side: 0 square, 50 a circle
  // (or a pill). Relative, so a resize keeps the look.
  cornerRadius?: number;
  border?: ImageBorder;
  // The part of the image shown, in its own pixels; the whole image when
  // absent. Set when an image takes a placeholder's place, so it fills the
  // placeholder's frame instead of changing its shape.
  srcCrop?: CropBox;
  // Came with a template: a pasted image takes its place. Images the user
  // added are left alone by a paste. Not stored in a template.
  placeholder?: boolean;
  // In a video: when it shows and how it moves.
  anim?: LayerAnim;
  // A video clip instead of a picture: src is the video, and it plays (with
  // its sound) while the layer shows.
  clip?: ClipInfo;
  blend?: BlendMode;
}

export type LayerSource = HTMLImageElement | HTMLVideoElement | HTMLCanvasElement | OffscreenCanvas;

// A clip's playback: it starts trimStart seconds into the video, and its
// sound plays at volume (0 to 1). duration is the video's own length, so a
// layer knows where its clip ends without loading it.
export interface ClipInfo {
  trimStart: number;
  volume: number;
  duration: number;
}

// A video's music: a sound file, from start seconds into the video,
// trimStart seconds into the file, faded in and out over fadeIn and fadeOut
// seconds. duration is the file's own length.
export interface AudioTrack {
  src: string;
  name: string;
  start: number;
  trimStart: number;
  volume: number;
  fadeIn: number;
  fadeOut: number;
  duration: number;
}

// The pixel size of what a layer draws.
export function sourceSize(img: LayerSource): { width: number; height: number } {
  if (img instanceof HTMLImageElement) return { width: img.naturalWidth || img.width, height: img.naturalHeight || img.height };
  if (img instanceof HTMLVideoElement) return { width: img.videoWidth || 1, height: img.videoHeight || 1 };
  return { width: img.width, height: img.height };
}

// How a layer mixes with what lies under it, as the canvas (and CSS) blend
// modes do; normal (drawn over it) when absent. The server's template
// renderer (services/compose_blend.go) knows the same modes.
export type BlendMode =
  | 'normal' | 'multiply' | 'screen' | 'overlay' | 'darken' | 'lighten' | 'color-dodge' | 'color-burn'
  | 'hard-light' | 'soft-light' | 'difference' | 'exclusion' | 'hue' | 'saturation' | 'color' | 'luminosity';

export const BLEND_MODES: { id: BlendMode; label: string }[] = [
  { id: 'normal', label: 'Normal' },
  { id: 'multiply', label: 'Multiply' },
  { id: 'screen', label: 'Screen' },
  { id: 'overlay', label: 'Overlay' },
  { id: 'darken', label: 'Darken' },
  { id: 'lighten', label: 'Lighten' },
  { id: 'color-dodge', label: 'Colour dodge' },
  { id: 'color-burn', label: 'Colour burn' },
  { id: 'hard-light', label: 'Hard light' },
  { id: 'soft-light', label: 'Soft light' },
  { id: 'difference', label: 'Difference' },
  { id: 'exclusion', label: 'Exclusion' },
  { id: 'hue', label: 'Hue' },
  { id: 'saturation', label: 'Saturation' },
  { id: 'color', label: 'Colour' },
  { id: 'luminosity', label: 'Luminosity' },
];

// The attribute that blends a layer's node. Set on the layer's own node (the
// group of an image, shape or paint layer, a text itself), whose shapes then
// draw with it onto the picture below; normal is spelled out so a layer
// switched back really stops blending.
export function blendAttrs(l: { blend?: BlendMode }): { globalCompositeOperation: Konva.NodeConfig['globalCompositeOperation'] } {
  const b = l.blend && l.blend !== 'normal' && BLEND_MODES.some(m => m.id === l.blend) ? l.blend : 'source-over';
  return { globalCompositeOperation: b };
}

export type BorderStyle = 'solid' | 'dashed' | 'dotted' | 'double' | 'glow' | 'offset';

export const BORDER_STYLES: { id: BorderStyle; label: string }[] = [
  { id: 'solid', label: 'Solid' },
  { id: 'dashed', label: 'Dashed' },
  { id: 'dotted', label: 'Dotted' },
  { id: 'double', label: 'Double' },
  { id: 'glow', label: 'Glow' },
  { id: 'offset', label: 'Offset' },
];

// A frame around an image layer. The width is in source-image pixels and
// sits outside the image, so the picture itself is never covered.
export interface ImageBorder {
  style: BorderStyle;
  width: number;
  color: string;
}

// The fonts every device has (or falls back from), listed after the media
// library's.
export interface FontOption {
  label: string;
  family: string;
}

export const BUILTIN_FONTS: FontOption[] = [
  { label: 'Arial', family: 'Arial, Helvetica, sans-serif' },
  { label: 'Helvetica', family: 'Helvetica, Arial, sans-serif' },
  { label: 'Verdana', family: 'Verdana, sans-serif' },
  { label: 'Trebuchet MS', family: 'Trebuchet MS, sans-serif' },
  { label: 'Impact', family: 'Impact, Haettenschweiler, Arial Black, sans-serif' },
  { label: 'Georgia', family: 'Georgia, serif' },
  { label: 'Times New Roman', family: 'Times New Roman, Times, serif' },
  { label: 'Rockwell', family: 'Rockwell, Rockwell Nova, Roboto Slab, DejaVu Serif, Sitka Small, serif' },
  { label: 'Courier New', family: 'Courier New, monospace' },
  { label: 'Comic Sans MS', family: 'Comic Sans MS, cursive' },
];

// A font family as CSS takes it: every name with a space quoted, as Konva
// does for the canvas. Unquoted, a library font's family (`SocialPod Media
// <id>`, the id possibly starting with a digit) is invalid CSS and the
// browser falls back to its default font.
export function cssFontFamily(family: string): string {
  return family.split(',').map(f => {
    f = f.trim();
    return f.includes(' ') && !/["']/.test(f) ? `"${f}"` : f;
  }).join(', ');
}

export type TextAlign = 'left' | 'center' | 'right';

export interface TextLayer {
  id: string;
  kind: 'text';
  text: string;
  fontFamily: string;
  fontSize: number;
  fill: string;
  bold: boolean;
  outline: boolean;
  shadow: boolean;
  // How the lines of a multi-line text line up; centred when absent.
  align?: TextAlign;
  // An auto-caption of the video's speech (captions.ts), which the Captions
  // section restyles and removes together.
  caption?: boolean;
  x: number;
  y: number;
  rotation: number;
  anim?: LayerAnim;
  blend?: BlendMode;
}

export type ShapeKind = 'rect' | 'ellipse' | 'triangle' | 'equalizer';

export const SHAPE_KINDS: { id: ShapeKind; label: string }[] = [
  { id: 'rect', label: 'Box' },
  { id: 'ellipse', label: 'Circle' },
  { id: 'triangle', label: 'Triangle' },
  { id: 'equalizer', label: 'Equalizer' },
];

// A box, circle (an ellipse in its box), triangle or equalizer (a row of bars
// that move to the video's music, equalizer.ts), positioned by its centre
// like an image. Its border is drawn on the outline, half inside and half
// outside, with round joins (the server draws it the same way).
export interface ShapeLayer {
  id: string;
  kind: 'shape';
  shape: ShapeKind;
  x: number;
  y: number;
  width: number;
  height: number;
  rotation: number;
  opacity: number;
  // Fill colour; empty for none.
  fill: string;
  // Border colour and width in source-image pixels; no border at width 0.
  stroke: string;
  strokeWidth: number;
  // Boxes and an equalizer's bars: corner rounding in percent of the
  // shorter side (of a bar).
  cornerRadius?: number;
  // An equalizer: how many bars, and whether they grow from the middle up
  // and down instead of from the bottom.
  bars?: number;
  mirror?: boolean;
  anim?: LayerAnim;
  blend?: BlendMode;
}

// Shapes whose corners can be rounded.
export const roundable = (l: ShapeLayer) => l.shape === 'rect' || l.shape === 'equalizer';

// One brush stroke: a path through points (x0, y0, x1, y1, ...) relative to
// its layer, painted size wide and blurred feather pixels at its edges.
export interface PaintStroke {
  points: number[];
  size: number;
  feather: number;
  color: string;
  opacity: number;
}

// Brush strokes painted onto the picture, e.g. a soft dark patch behind a
// text. (x, y) moves them all; they are not scaled or turned.
export interface PaintLayer {
  id: string;
  kind: 'paint';
  x: number;
  y: number;
  strokes: PaintStroke[];
  anim?: LayerAnim;
  blend?: BlendMode;
}

export type EditorLayer = ImageLayer | TextLayer | ShapeLayer | PaintLayer;

// A blank canvas the editor draws on instead of a photo.
export interface EditorCanvas {
  width: number;
  height: number;
  background: string;
}

// A saved design: a canvas and its layers, images by URL. The media library
// stores these as templates (JSON); the server reads only the outline and the
// image layers' src.
export type TemplateLayer = Omit<ImageLayer, 'id' | 'img'> | Omit<TextLayer, 'id'> | Omit<ShapeLayer, 'id'> | Omit<PaintLayer, 'id'>;

export interface EditorTemplate {
  version: 1;
  width: number;
  height: number;
  background: string;
  layers: TemplateLayer[];
  // A video design: its length in seconds. The layers' anim says when each
  // shows; a still design has neither.
  duration?: number;
  // A video design's music.
  audio?: AudioTrack;
}

// Unfinished work saved to come back to (an image draft): the design as a
// template in the picture's own pixels (the photo's, or the canvas's), and for
// an edited photo the photo, its crop, aspect and filters. Unlike a template
// it is restored exactly as it was, not fitted to another picture.
export interface EditorDraft {
  version: 1;
  template: EditorTemplate;
  photo?: { src: string; crop: CropBox; aspectId: string; adjustments: Adjustments };
}

// Reads a stored draft; null if it is not one.
export function parseDraft(raw: string | undefined): EditorDraft | null {
  if (!raw) return null;
  try {
    const d = JSON.parse(raw) as EditorDraft;
    if (!d || typeof d !== 'object' || !d.template || !Array.isArray(d.template.layers)) return null;
    if (!(d.template.width > 0) || !(d.template.height > 0)) return null;
    if (d.photo && (typeof d.photo.src !== 'string' || !d.photo.crop)) return null;
    return d;
  } catch {
    return null;
  }
}

// The uploads an edit uses: its photo, image layers and clips, and music.
export function editUploadUrls(d: EditorDraft): string[] {
  const urls = new Set<string>();
  if (d.photo?.src) urls.add(d.photo.src);
  for (const l of d.template.layers) if (l.kind === 'image' && l.src) urls.add(l.src);
  if (d.template.audio?.src) urls.add(d.template.audio.src);
  return [...urls];
}

// What a blank canvas starts as when nothing else is given.
export const DEFAULT_CANVAS: EditorCanvas = { width: 1080, height: 1350, background: '#ffffff' };

export const CANVAS_FORMATS: { id: string; label: string; width: number; height: number }[] = [
  { id: '1:1', label: 'Square 1:1', width: 1080, height: 1080 },
  { id: '4:5', label: 'Portrait 4:5', width: 1080, height: 1350 },
  { id: '9:16', label: 'Story 9:16', width: 1080, height: 1920 },
  { id: '16:9', label: 'Landscape 16:9', width: 1920, height: 1080 },
];

export type FilterPreset = 'none' | 'vivid' | 'mono' | 'sepia' | 'warm' | 'cool' | 'fade';

// Slider values, -100..100 each, on top of the preset.
export interface Adjustments {
  preset: FilterPreset;
  brightness: number;
  contrast: number;
  saturation: number;
}

export const NO_ADJUSTMENTS: Adjustments = { preset: 'none', brightness: 0, contrast: 0, saturation: 0 };

export const FILTER_PRESETS: { id: FilterPreset; label: string }[] = [
  { id: 'none', label: 'Original' },
  { id: 'vivid', label: 'Vivid' },
  { id: 'warm', label: 'Warm' },
  { id: 'cool', label: 'Cool' },
  { id: 'fade', label: 'Fade' },
  { id: 'mono', label: 'Mono' },
  { id: 'sepia', label: 'Sepia' },
];

// What each preset adds to the sliders, plus the tint or colour filter it uses.
const PRESET_SHIFT: Record<FilterPreset, { brightness: number; contrast: number; saturation: number }> = {
  none: { brightness: 0, contrast: 0, saturation: 0 },
  vivid: { brightness: 0, contrast: 15, saturation: 40 },
  warm: { brightness: 3, contrast: 5, saturation: 10 },
  cool: { brightness: 0, contrast: 5, saturation: -5 },
  fade: { brightness: 8, contrast: -25, saturation: -30 },
  mono: { brightness: 0, contrast: 10, saturation: 0 },
  sepia: { brightness: 0, contrast: 0, saturation: 0 },
};

const TINTS: Partial<Record<FilterPreset, { red: number; green: number; blue: number; alpha: number }>> = {
  warm: { red: 255, green: 170, blue: 80, alpha: 0.14 },
  cool: { red: 80, green: 150, blue: 255, alpha: 0.12 },
};

export function hasAdjustments(a: Adjustments): boolean {
  return a.preset !== 'none' || a.brightness !== 0 || a.contrast !== 0 || a.saturation !== 0;
}

// Applies the adjustments to the base image node. Filters work on the node's
// cache, drawn at pixelRatio times the image's own size: the preview caches a
// reduced copy so a slider stays responsive on a phone, the export the output
// size.
export function applyAdjustments(node: Konva.Image, a: Adjustments, pixelRatio: number) {
  if (!hasAdjustments(a)) {
    node.filters([]);
    node.clearCache();
    return;
  }
  const shift = PRESET_SHIFT[a.preset];
  const brightness = clamp(a.brightness + shift.brightness, -100, 100);
  const contrast = clamp(a.contrast + shift.contrast, -100, 100);
  const saturation = clamp(a.saturation + shift.saturation, -100, 100);

  const filters: Filter[] = [];
  if (brightness) filters.push(Konva.Filters.Brighten);
  if (contrast) filters.push(Konva.Filters.Contrast);
  if (saturation && a.preset !== 'mono' && a.preset !== 'sepia') filters.push(Konva.Filters.HSL);
  if (a.preset === 'mono') filters.push(Konva.Filters.Grayscale);
  if (a.preset === 'sepia') filters.push(Konva.Filters.Sepia);
  const tint = TINTS[a.preset];
  if (tint) filters.push(Konva.Filters.RGBA);

  node.setAttrs({
    // Brighten adds brightness * 255 to each channel; half of that range is
    // plenty for a photo.
    brightness: brightness / 200,
    contrast: contrast * 0.6,
    // HSL scales saturation by 2^saturation: -100 is grey, +100 doubles it.
    saturation: Math.log2(Math.max(0.01, 1 + saturation / 100)),
    hue: 0,
    luminance: 0,
    ...(tint ?? {}),
  });
  node.filters(filters);
  node.cache({ pixelRatio });
}

export type ImagePart =
  | { kind: 'image'; attrs: Konva.ImageConfig }
  | { kind: 'rect'; attrs: Konva.RectConfig }
  | { kind: 'ellipse'; attrs: Konva.EllipseConfig }
  | { kind: 'line'; attrs: Konva.LineConfig };

// An image layer is a group, positioned and turned about its centre, holding
// the image and whatever its border draws. The editor's stage and renderEdit
// both build it from this, so screen and output stay alike.
export function imageLayerParts(l: ImageLayer): { group: Konva.GroupConfig; parts: ImagePart[] } {
  const { width: w, height: h } = l;
  const r = (clamp(l.cornerRadius ?? 0, 0, 50) / 100) * Math.min(w, h);
  const b = l.border && l.border.width > 0 ? l.border : null;
  const bw = b?.width ?? 0;
  const parts: ImagePart[] = [];
  // The radius of a line drawn d outside the image edge, so it follows the
  // image's rounding.
  const around = (d: number) => (r > 0 ? r + d : 0);
  const ring = (d: number, strokeWidth: number, extra: Konva.RectConfig = {}): ImagePart => ({
    kind: 'rect',
    attrs: { x: -d, y: -d, width: w + 2 * d, height: h + 2 * d, cornerRadius: around(d), stroke: b!.color, strokeWidth, ...extra },
  });

  if (b?.style === 'offset') {
    parts.push({ kind: 'rect', attrs: { x: bw, y: bw, width: w, height: h, cornerRadius: r, fill: b.color } });
  }
  const crop = l.srcCrop;
  parts.push({
    kind: 'image',
    attrs: {
      image: l.img,
      width: w,
      height: h,
      cornerRadius: r,
      ...(crop ? { cropX: crop.x, cropY: crop.y, cropWidth: crop.width, cropHeight: crop.height } : {}),
      ...(b?.style === 'glow'
        ? { shadowEnabled: true, shadowColor: b.color, shadowBlur: bw * 2, shadowOpacity: 1, shadowOffsetX: 0, shadowOffsetY: 0 }
        : {}),
    },
  });
  switch (b?.style) {
    case 'solid':
      parts.push(ring(bw / 2, bw));
      break;
    case 'dashed':
      parts.push(ring(bw / 2, bw, { dash: [bw * 3, bw * 2] }));
      break;
    case 'dotted':
      // Zero-length dashes with round caps draw dots.
      parts.push(ring(bw / 2, bw, { dash: [0.001, bw * 2], lineCap: 'round' }));
      break;
    case 'double':
      parts.push(ring(bw / 6, bw / 3), ring((bw * 5) / 6, bw / 3));
      break;
  }

  return {
    group: { x: l.x, y: l.y, width: w, height: h, offsetX: w / 2, offsetY: h / 2, rotation: l.rotation, opacity: l.opacity, ...blendAttrs(l) },
    parts,
  };
}

// A shape layer is a group like an image layer, holding the one shape drawn
// over its box. An equalizer holds its bars at levels (0 to 1 each; at rest
// when absent) over an empty box, which picks and frames the layer whatever
// the bars do.
export function shapeLayerParts(l: ShapeLayer, levels?: number[]): { group: Konva.GroupConfig; parts: ImagePart[] } {
  const { width: w, height: h } = l;
  const sw = Math.max(0, l.strokeWidth);
  const paint = {
    fill: l.fill || undefined,
    fillEnabled: !!l.fill,
    stroke: sw > 0 ? l.stroke : undefined,
    strokeWidth: sw,
    strokeEnabled: sw > 0,
    lineJoin: 'round' as const,
  };
  const group = { x: l.x, y: l.y, width: w, height: h, offsetX: w / 2, offsetY: h / 2, rotation: l.rotation, opacity: l.opacity, ...blendAttrs(l) };
  if (l.shape === 'equalizer') {
    const bars = equalizerBars(w, h, levels ?? restLevels(barCount(l)), !!l.mirror);
    const corner = clamp(l.cornerRadius ?? 0, 0, 50) / 100;
    return {
      group,
      parts: [
        { kind: 'rect', attrs: { x: 0, y: 0, width: w, height: h, fillEnabled: false, strokeEnabled: false } },
        ...bars.map((b): ImagePart => ({ kind: 'rect', attrs: { ...b, cornerRadius: corner * Math.min(b.width, b.height), ...paint } })),
      ],
    };
  }
  let part: ImagePart;
  switch (l.shape) {
    case 'ellipse':
      part = { kind: 'ellipse', attrs: { x: w / 2, y: h / 2, radiusX: w / 2, radiusY: h / 2, ...paint } };
      break;
    case 'triangle':
      part = { kind: 'line', attrs: { points: [w / 2, 0, w, h, 0, h], closed: true, ...paint } };
      break;
    default:
      part = {
        kind: 'rect',
        attrs: { x: 0, y: 0, width: w, height: h, cornerRadius: (clamp(l.cornerRadius ?? 0, 0, 50) / 100) * Math.min(w, h), ...paint },
      };
  }
  return { group, parts: [part] };
}

function partNode(p: ImagePart): Konva.Shape {
  switch (p.kind) {
    case 'image': return new Konva.Image(p.attrs);
    case 'ellipse': return new Konva.Ellipse(p.attrs);
    case 'line': return new Konva.Line(p.attrs);
    default: return new Konva.Rect(p.attrs);
  }
}

// levels: an equalizer's bars at the moment drawn (at rest when absent).
export function buildImageLayerNode(l: ImageLayer | ShapeLayer, levels?: number[]): Konva.Group {
  const { group, parts } = l.kind === 'image' ? imageLayerParts(l) : shapeLayerParts(l, levels);
  const g = new Konva.Group(group);
  for (const p of parts) g.add(partNode(p));
  return g;
}

// A brush stroke as a Konva shape. A feathered edge is a blur, which the
// canvas draws fastest (and at any strength) as a shadow: the stroke itself
// is drawn off the canvas and only its shadow, shifted back, lands on the
// picture. The shape's box is the stroke's, so selecting and snapping see
// where the paint is.
export function paintStrokeAttrs(st: PaintStroke): Konva.ShapeConfig {
  const pts = st.points.length >= 4 ? st.points : [st.points[0] ?? 0, st.points[1] ?? 0, (st.points[0] ?? 0) + 0.01, st.points[1] ?? 0];
  let minX = Infinity;
  let minY = Infinity;
  let maxX = -Infinity;
  let maxY = -Infinity;
  for (let i = 0; i < pts.length; i += 2) {
    minX = Math.min(minX, pts[i]);
    maxX = Math.max(maxX, pts[i]);
    minY = Math.min(minY, pts[i + 1]);
    maxY = Math.max(maxY, pts[i + 1]);
  }
  const pad = st.size / 2 + st.feather / 2;
  const ox = minX - pad;
  const oy = minY - pad;
  const local = pts.map((v, i) => v - (i % 2 === 0 ? ox : oy));
  const trace = (ctx: { moveTo(x: number, y: number): void; lineTo(x: number, y: number): void; quadraticCurveTo(a: number, b: number, c: number, d: number): void }, p: number[]) => {
    ctx.moveTo(p[0], p[1]);
    // Through the midpoints, so the stroke bends smoothly between samples.
    for (let i = 2; i < p.length - 2; i += 2) {
      ctx.quadraticCurveTo(p[i], p[i + 1], (p[i] + p[i + 2]) / 2, (p[i + 1] + p[i + 3]) / 2);
    }
    ctx.lineTo(p[p.length - 2], p[p.length - 1]);
  };
  return {
    x: ox,
    y: oy,
    width: maxX - minX + pad * 2,
    height: maxY - minY + pad * 2,
    // For hit detection only; the scene is drawn by sceneFunc.
    stroke: st.color,
    strokeWidth: Math.max(st.size, 4),
    lineCap: 'round',
    lineJoin: 'round',
    perfectDrawEnabled: false,
    sceneFunc: (context) => {
      const c = context._context as CanvasRenderingContext2D;
      const m = c.getTransform();
      const s = Math.hypot(m.a, m.b);
      // The stroke in device pixels.
      const dev = local.map((v, i) => (i % 2 === 0 ? m.a * v + m.c * local[i + 1] + m.e : m.b * local[i - 1] + m.d * v + m.f));
      const lw = Math.max(0.5, st.size * s);
      let right = -Infinity;
      for (let i = 0; i < dev.length; i += 2) right = Math.max(right, dev[i]);
      // Far enough left that the stroke itself is never on the canvas.
      const shift = Math.max(0, right) + lw + 10;
      c.save();
      c.setTransform(1, 0, 0, 1, 0, 0);
      c.beginPath();
      trace(c, dev.map((v, i) => (i % 2 === 0 ? v - shift : v)));
      c.lineCap = 'round';
      c.lineJoin = 'round';
      c.lineWidth = lw;
      c.strokeStyle = st.color;
      c.globalAlpha *= clamp(st.opacity, 0, 1);
      c.shadowColor = st.color;
      c.shadowBlur = Math.max(0, st.feather * s);
      c.shadowOffsetX = shift;
      c.shadowOffsetY = 0;
      c.stroke();
      c.restore();
    },
    hitFunc: (context, shape) => {
      context.beginPath();
      trace(context, local);
      context.strokeShape(shape);
    },
  };
}

export function buildPaintLayerNode(l: PaintLayer): Konva.Group {
  const g = new Konva.Group({ x: l.x, y: l.y, ...blendAttrs(l) });
  for (const st of l.strokes) g.add(new Konva.Shape(paintStrokeAttrs(st)));
  return g;
}

export function buildLayerNode(l: EditorLayer, levels?: number[]): Konva.Group | Konva.Shape {
  if (l.kind === 'text') return new Konva.Text(textLayerAttrs(l));
  if (l.kind === 'paint') return buildPaintLayerNode(l);
  return buildImageLayerNode(l, levels);
}

export function textLayerAttrs(l: TextLayer): Konva.TextConfig {
  return {
    x: l.x,
    y: l.y,
    rotation: l.rotation,
    text: l.text,
    fontFamily: l.fontFamily,
    fontSize: l.fontSize,
    fontStyle: l.bold ? 'bold' : 'normal',
    fill: l.fill,
    align: l.align ?? 'center',
    lineHeight: 1.15,
    stroke: l.outline ? (isLight(l.fill) ? '#000000' : '#ffffff') : undefined,
    strokeWidth: l.outline ? Math.max(1, l.fontSize * 0.08) : 0,
    strokeEnabled: l.outline,
    // The outline sits behind the letters, so it widens them instead of
    // eating into them.
    fillAfterStrokeEnabled: true,
    lineJoin: 'round',
    shadowEnabled: l.shadow,
    shadowColor: '#000000',
    shadowBlur: l.fontSize * 0.15,
    shadowOpacity: 0.6,
    shadowOffsetX: l.fontSize * 0.03,
    shadowOffsetY: l.fontSize * 0.03,
    ...blendAttrs(l),
  };
}

// What the Objects list and the timeline call a layer.
export function layerName(layer: EditorLayer): string {
  switch (layer.kind) {
    case 'text': return layer.text.trim().split('\n')[0] || 'Empty text';
    case 'shape': return SHAPE_KINDS.find(k => k.id === layer.shape)?.label ?? 'Shape';
    case 'paint': return 'Paint';
    default: return layer.clip ? 'Clip' : layer.origin === 'watermark' ? 'Watermark' : 'Image';
  }
}

// The size Konva gives a text at rotation 0, in source-image pixels.
export function measureText(l: TextLayer): { width: number; height: number } {
  const node = new Konva.Text(textLayerAttrs(l));
  const size = { width: node.width(), height: node.height() };
  node.destroy();
  return size;
}

// The text at a new font size, its centre kept where it was (rotation
// included), so a text grows or shrinks in place.
export function resizeText(l: TextLayer, fontSize: number): Pick<TextLayer, 'fontSize' | 'x' | 'y'> {
  const a = measureText(l);
  const b = measureText({ ...l, fontSize });
  const rad = (l.rotation * Math.PI) / 180;
  const cos = Math.cos(rad);
  const sin = Math.sin(rad);
  // The box's centre is its corner plus half its size, turned with it.
  const dx = (a.width - b.width) / 2;
  const dy = (a.height - b.height) / 2;
  return { fontSize, x: l.x + dx * cos - dy * sin, y: l.y + dx * sin + dy * cos };
}

// The largest box of the given aspect ratio (width / height) centred in the
// image.
export function centredCrop(natW: number, natH: number, ratio: number | null): CropBox {
  if (!ratio) return { x: 0, y: 0, width: natW, height: natH };
  let width = natW;
  let height = width / ratio;
  if (height > natH) {
    height = natH;
    width = height * ratio;
  }
  return { x: (natW - width) / 2, y: (natH - height) / 2, width, height };
}

export interface RenderInput {
  // The photo, or null for a blank canvas filled with background.
  source: HTMLImageElement | null;
  background?: string;
  crop: CropBox;
  adjustments: Adjustments;
  layers: EditorLayer[];
  // Longest side of the output; larger crops are scaled down. Phones refuse
  // canvases much beyond 4096 px a side.
  maxSide?: number;
  quality?: number;
}

// Renders the edit at full resolution, independent of the on-screen stage.
export async function renderEdit({ source, background = '#ffffff', crop, adjustments, layers, maxSide = 4096, quality = 0.92 }: RenderInput): Promise<Blob> {
  const scale = Math.min(1, maxSide / Math.max(crop.width, crop.height));
  const width = Math.max(1, Math.round(crop.width * scale));
  const height = Math.max(1, Math.round(crop.height * scale));

  // A 1×1 stage: the layer canvases it allocates are sized by the stage (and
  // the device pixel ratio), so the output is drawn with toCanvas instead.
  const container = document.createElement('div');
  const stage = new Konva.Stage({ container, width: 1, height: 1 });
  try {
    const layer = new Konva.Layer({ listening: false });
    stage.add(layer);
    const group = new Konva.Group({ x: -crop.x * scale, y: -crop.y * scale, scaleX: scale, scaleY: scale });
    layer.add(group);

    if (source) {
      const base = new Konva.Image({ image: source, width: source.naturalWidth, height: source.naturalHeight });
      group.add(base);
      applyAdjustments(base, adjustments, scale);
    } else {
      group.add(new Konva.Rect({ x: crop.x, y: crop.y, width: crop.width, height: crop.height, fill: background }));
    }

    for (const l of layers) group.add(buildLayerNode(l));

    const canvas = group.toCanvas({ x: 0, y: 0, width, height, pixelRatio: 1 });
    return await new Promise<Blob>((resolve, reject) => {
      canvas.toBlob(b => (b ? resolve(b) : reject(new Error('Failed to encode image'))), 'image/jpeg', quality);
    });
  } finally {
    stage.destroy();
  }
}

// Maps layers laid out on the box from onto the box to: positions follow the
// box on each axis, sizes scale evenly by the smaller factor so nothing is
// distorted. Texts keep their centre, since they are placed by their corner.
export function fitLayers<L extends EditorLayer | TemplateLayer>(layers: L[], from: CropBox, to: CropBox): L[] {
  const sx = to.width / from.width;
  const sy = to.height / from.height;
  const s = Math.min(sx, sy);
  if (sx === 1 && sy === 1 && from.x === to.x && from.y === to.y) return layers;
  return layers.map(l => {
    if (l.kind === 'shape') {
      const sl = l as ShapeLayer;
      return {
        ...l, x: to.x + (sl.x - from.x) * sx, y: to.y + (sl.y - from.y) * sy, width: sl.width * s, height: sl.height * s, strokeWidth: sl.strokeWidth * s,
      };
    }
    if (l.kind === 'paint') {
      // The strokes move into place and the layer's own shift is folded in.
      const pl = l as PaintLayer;
      return {
        ...l, x: 0, y: 0,
        strokes: pl.strokes.map(st => ({
          ...st, size: st.size * s, feather: st.feather * s,
          points: st.points.map((v, i) => (i % 2 === 0 ? to.x + (v + pl.x - from.x) * sx : to.y + (v + pl.y - from.y) * sy)),
        })),
      };
    }
    if (l.kind === 'image') {
      const il = l as ImageLayer;
      return {
        ...l, x: to.x + (l.x - from.x) * sx, y: to.y + (l.y - from.y) * sy, width: l.width * s, height: l.height * s,
        ...(il.border ? { border: { ...il.border, width: il.border.width * s } } : {}),
      };
    }
    const t = l as TextLayer;
    const m = measureText(t);
    const fontSize = t.fontSize * s;
    const m2 = measureText({ ...t, fontSize });
    const cx = to.x + (t.x + m.width / 2 - from.x) * sx;
    const cy = to.y + (t.y + m.height / 2 - from.y) * sy;
    return { ...l, fontSize, x: cx - m2.width / 2, y: cy - m2.height / 2 };
  });
}

// The design as a template: the crop becomes the canvas, layers move with it.
// A video design passes its duration, which keeps the layers' timing.
export function toTemplate(crop: CropBox, layers: EditorLayer[], background: string, duration?: number, audio?: AudioTrack | null): EditorTemplate {
  return {
    version: 1,
    width: Math.round(crop.width),
    height: Math.round(crop.height),
    background,
    ...(duration ? { duration } : {}),
    ...(duration && audio ? { audio } : {}),
    // Everything but the id and the loaded image.
    layers: layers.map((l): TemplateLayer => ({
      ...templateLayer(l, crop),
      ...(l.blend && l.blend !== 'normal' ? { blend: l.blend } : {}),
      ...(duration && l.anim ? { anim: l.anim } : {}),
    })),
  };
}

function templateLayer(l: EditorLayer, crop: CropBox): TemplateLayer {
  const x = l.x - crop.x;
  const y = l.y - crop.y;
  if (l.kind === 'shape') {
    return {
      kind: 'shape', shape: l.shape, x, y, width: l.width, height: l.height, rotation: l.rotation, opacity: l.opacity,
      fill: l.fill, stroke: l.stroke, strokeWidth: l.strokeWidth,
      ...(roundable(l) && l.cornerRadius ? { cornerRadius: l.cornerRadius } : {}),
      ...(l.shape === 'equalizer' ? { bars: barCount(l), ...(l.mirror ? { mirror: true } : {}) } : {}),
    };
  }
  if (l.kind === 'paint') {
    // A tenth of a pixel is plenty, and keeps long strokes small.
    const r = (v: number) => Math.round(v * 10) / 10;
    return {
      kind: 'paint', x: r(x), y: r(y),
      strokes: l.strokes.map(st => ({ ...st, points: st.points.map(r), size: r(st.size), feather: r(st.feather) })),
    };
  }
  if (l.kind === 'image') {
    return {
      kind: 'image', origin: l.origin, src: l.src, x, y, width: l.width, height: l.height, rotation: l.rotation, opacity: l.opacity,
      ...(l.cornerRadius ? { cornerRadius: l.cornerRadius } : {}),
      ...(l.border ? { border: l.border } : {}),
      ...(l.srcCrop ? { srcCrop: l.srcCrop } : {}),
      ...(l.clip ? { clip: l.clip } : {}),
    };
  }
  return {
    kind: 'text', text: l.text, fontFamily: l.fontFamily, fontSize: l.fontSize, fill: l.fill,
    bold: l.bold, outline: l.outline, shadow: l.shadow, align: l.align ?? 'center', x, y, rotation: l.rotation,
    ...(l.caption ? { caption: true } : {}),
  };
}

// Reads a template stored in the media library; null if it is not one.
export function parseTemplate(raw: string | undefined): EditorTemplate | null {
  if (!raw) return null;
  try {
    const t = JSON.parse(raw) as EditorTemplate;
    if (!(t.width > 0 && t.height > 0) || !Array.isArray(t.layers)) return null;
    return { ...t, background: t.background || '#ffffff' };
  } catch {
    return null;
  }
}

// Whether a media library template is a video design (it has a length);
// a video editor offers them, a still one does not.
export function isVideoTemplate(raw: string | undefined): boolean {
  return !!parseTemplate(raw)?.duration;
}

// Turns a template's layers into editor layers on the box target, loading its
// images. Images that fail to load are left out and counted.
export async function instantiateTemplate(
  t: EditorTemplate, target: CropBox, resolve: (url: string) => string, { placeholders = true } = {},
): Promise<{ layers: EditorLayer[]; missing: number }> {
  const fitted = fitLayers(t.layers, { x: 0, y: 0, width: t.width, height: t.height }, target);
  let missing = 0;
  const loaded = await Promise.all(fitted.map(async (l): Promise<EditorLayer | null> => {
    if (l.kind === 'text') return { ...(l as Omit<TextLayer, 'id'>), id: newLayerId() };
    if (l.kind === 'shape') return { ...(l as Omit<ShapeLayer, 'id'>), id: newLayerId() };
    if (l.kind === 'paint') return { ...(l as Omit<PaintLayer, 'id'>), id: newLayerId() };
    const il = l as Omit<ImageLayer, 'id' | 'img'>;
    try {
      const img = il.clip ? await loadVideo(resolve(il.src), il.clip.trimStart) : await loadImage(resolve(il.src));
      return { ...il, id: newLayerId(), img, ...(placeholders ? { placeholder: true } : {}) };
    } catch {
      missing++;
      return null;
    }
  }));
  return { layers: loaded.filter((l): l is EditorLayer => l !== null), missing };
}

export function loadImage(url: string): Promise<HTMLImageElement> {
  return new Promise((resolve, reject) => {
    const img = new window.Image();
    // Server images need CORS to be exported from a canvas; blob and data URLs
    // are same-origin already.
    if (!url.startsWith('blob:') && !url.startsWith('data:')) img.crossOrigin = 'anonymous';
    img.onload = () => resolve(img);
    img.onerror = () => reject(new Error('Failed to load image'));
    img.src = url;
  });
}

// A clip's video, loaded far enough to draw its frame at time at (seconds).
// It is not muted: it plays its sound in the preview.
export function loadVideo(url: string, at = 0): Promise<HTMLVideoElement> {
  return new Promise((resolve, reject) => {
    const v = document.createElement('video');
    if (!url.startsWith('blob:') && !url.startsWith('data:')) v.crossOrigin = 'anonymous';
    v.preload = 'auto';
    v.playsInline = true;
    v.onerror = () => reject(new Error('Failed to load the clip'));
    v.onloadeddata = () => {
      v.onloadeddata = null;
      if (at <= 0) { resolve(v); return; }
      v.onseeked = () => { v.onseeked = null; resolve(v); };
      v.currentTime = Math.min(at, Math.max(0, (Number.isFinite(v.duration) ? v.duration : at) - 0.05));
    };
    v.src = url;
  });
}

export function isLight(hex: string): boolean {
  const m = /^#?([0-9a-f]{2})([0-9a-f]{2})([0-9a-f]{2})$/i.exec(hex);
  if (!m) return true;
  const [r, g, b] = [m[1], m[2], m[3]].map(h => parseInt(h, 16));
  return 0.299 * r + 0.587 * g + 0.114 * b > 150;
}

export function clamp(v: number, min: number, max: number): number {
  return Math.max(min, Math.min(max, v));
}

let idSeq = 0;
export function newLayerId(): string {
  idSeq += 1;
  return `layer-${Date.now().toString(36)}-${idSeq}`;
}
