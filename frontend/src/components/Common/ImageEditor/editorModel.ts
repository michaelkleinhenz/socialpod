import Konva from 'konva';
import type { Filter } from 'konva/lib/Node';

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
  img: HTMLImageElement;
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
  x: number;
  y: number;
  rotation: number;
}

export type EditorLayer = ImageLayer | TextLayer;

// A blank canvas the editor draws on instead of a photo.
export interface EditorCanvas {
  width: number;
  height: number;
  background: string;
}

// A saved design: a canvas and its layers, images by URL. The media library
// stores these as templates (JSON); the server reads only the outline and the
// image layers' src.
export type TemplateLayer = Omit<ImageLayer, 'id' | 'img'> | Omit<TextLayer, 'id'>;

export interface EditorTemplate {
  version: 1;
  width: number;
  height: number;
  background: string;
  layers: TemplateLayer[];
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
  | { kind: 'rect'; attrs: Konva.RectConfig };

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
    group: { x: l.x, y: l.y, width: w, height: h, offsetX: w / 2, offsetY: h / 2, rotation: l.rotation, opacity: l.opacity },
    parts,
  };
}

export function buildImageLayerNode(l: ImageLayer): Konva.Group {
  const { group, parts } = imageLayerParts(l);
  const g = new Konva.Group(group);
  for (const p of parts) g.add(p.kind === 'image' ? new Konva.Image(p.attrs) : new Konva.Rect(p.attrs));
  return g;
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
  };
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

    for (const l of layers) {
      group.add(l.kind === 'image' ? buildImageLayerNode(l) : new Konva.Text(textLayerAttrs(l)));
    }

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
export function toTemplate(crop: CropBox, layers: EditorLayer[], background: string): EditorTemplate {
  return {
    version: 1,
    width: Math.round(crop.width),
    height: Math.round(crop.height),
    background,
    // Everything but the id and the loaded image.
    layers: layers.map((l): TemplateLayer => {
      const x = l.x - crop.x;
      const y = l.y - crop.y;
      if (l.kind === 'image') {
        return {
          kind: 'image', origin: l.origin, src: l.src, x, y, width: l.width, height: l.height, rotation: l.rotation, opacity: l.opacity,
          ...(l.cornerRadius ? { cornerRadius: l.cornerRadius } : {}),
          ...(l.border ? { border: l.border } : {}),
          ...(l.srcCrop ? { srcCrop: l.srcCrop } : {}),
        };
      }
      return {
        kind: 'text', text: l.text, fontFamily: l.fontFamily, fontSize: l.fontSize, fill: l.fill,
        bold: l.bold, outline: l.outline, shadow: l.shadow, align: l.align ?? 'center', x, y, rotation: l.rotation,
      };
    }),
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

// Turns a template's layers into editor layers on the box target, loading its
// images. Images that fail to load are left out and counted.
export async function instantiateTemplate(
  t: EditorTemplate, target: CropBox, resolve: (url: string) => string,
): Promise<{ layers: EditorLayer[]; missing: number }> {
  const fitted = fitLayers(t.layers, { x: 0, y: 0, width: t.width, height: t.height }, target);
  let missing = 0;
  const loaded = await Promise.all(fitted.map(async (l): Promise<EditorLayer | null> => {
    if (l.kind === 'text') return { ...(l as Omit<TextLayer, 'id'>), id: newLayerId() };
    const il = l as Omit<ImageLayer, 'id' | 'img'>;
    try {
      const img = await loadImage(resolve(il.src));
      return { ...il, id: newLayerId(), img };
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
