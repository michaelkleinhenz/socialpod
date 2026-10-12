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
  img: HTMLImageElement;
  x: number;
  y: number;
  width: number;
  height: number;
  rotation: number;
  opacity: number;
}

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
  x: number;
  y: number;
  rotation: number;
}

export type EditorLayer = ImageLayer | TextLayer;

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

export function imageLayerAttrs(l: ImageLayer): Konva.ImageConfig {
  return {
    image: l.img,
    x: l.x,
    y: l.y,
    width: l.width,
    height: l.height,
    offsetX: l.width / 2,
    offsetY: l.height / 2,
    rotation: l.rotation,
    opacity: l.opacity,
  };
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
    align: 'center',
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
  source: HTMLImageElement;
  crop: CropBox;
  adjustments: Adjustments;
  layers: EditorLayer[];
  // Longest side of the output; larger crops are scaled down. Phones refuse
  // canvases much beyond 4096 px a side.
  maxSide?: number;
  quality?: number;
}

// Renders the edit at full resolution, independent of the on-screen stage.
export async function renderEdit({ source, crop, adjustments, layers, maxSide = 4096, quality = 0.92 }: RenderInput): Promise<Blob> {
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

    const base = new Konva.Image({ image: source, width: source.naturalWidth, height: source.naturalHeight });
    group.add(base);
    applyAdjustments(base, adjustments, scale);

    for (const l of layers) {
      group.add(l.kind === 'image' ? new Konva.Image(imageLayerAttrs(l)) : new Konva.Text(textLayerAttrs(l)));
    }

    const canvas = group.toCanvas({ x: 0, y: 0, width, height, pixelRatio: 1 });
    return await new Promise<Blob>((resolve, reject) => {
      canvas.toBlob(b => (b ? resolve(b) : reject(new Error('Failed to encode image'))), 'image/jpeg', quality);
    });
  } finally {
    stage.destroy();
  }
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
