// Clipart (handlers/clipart.go). From Iconify each search result is an SVG
// body with its view box: recolourable icons draw in currentColor, which is
// set here; emoji keep their own colours. From OpenClipart a result is a
// preview and a file, imported through the backend: a PNG arrives stored, an
// SVG as markup rendered here. On the picture a piece of clipart is an
// ordinary image layer, rendered to a PNG and uploaded, so templates,
// drafts, the video export and compose_image need nothing of their own.
import type { ClipartIcon } from '../../../types';

// The longest side of the PNG put on the picture: sharp at full size on a
// 1080 px canvas.
export const CLIPART_PX = 1024;

// Searches offered before anything is typed.
export const CLIPART_SUGGESTIONS = ['arrow', 'star', 'heart', 'check', 'speech bubble', 'fire', 'crown', 'dice', 'sparkles', 'party'];

const escapeAttr = (s: string) => s.replace(/&/g, '&amp;').replace(/"/g, '&quot;').replace(/</g, '&lt;');

const iconBox = (icon: ClipartIcon) => ({ left: icon.left ?? 0, top: icon.top ?? 0, width: icon.width || 16, height: icon.height || 16 });

// An Iconify icon as a standalone SVG document, its longer side `px` pixels.
export function clipartSvg(icon: ClipartIcon, color: string, px: number): string {
  const box = iconBox(icon);
  const scale = px / Math.max(box.width, box.height);
  const w = Math.max(1, Math.round(box.width * scale));
  const h = Math.max(1, Math.round(box.height * scale));
  return `<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" width="${w}" height="${h}" `
    + `viewBox="${box.left} ${box.top} ${box.width} ${box.height}" color="${escapeAttr(color)}">${icon.body ?? ''}</svg>`;
}

// The image for a result's tile in the grid.
export const clipartPreviewUrl = (icon: ClipartIcon, color: string) =>
  icon.source === 'openclipart'
    ? icon.previewUrl ?? ''
    : 'data:image/svg+xml;charset=utf-8,' + encodeURIComponent(clipartSvg(icon, color, 96));

// Draws an SVG document of the given size onto a canvas and returns it as a
// transparent PNG file.
async function svgToPng(svg: string, width: number, height: number, name: string): Promise<File> {
  const url = URL.createObjectURL(new Blob([svg], { type: 'image/svg+xml' }));
  try {
    const img = new Image();
    img.decoding = 'async';
    img.src = url;
    await img.decode();
    const canvas = document.createElement('canvas');
    canvas.width = width;
    canvas.height = height;
    const ctx = canvas.getContext('2d');
    if (!ctx) throw new Error('No canvas');
    ctx.drawImage(img, 0, 0, width, height);
    const blob = await new Promise<Blob | null>(resolve => canvas.toBlob(resolve, 'image/png'));
    if (!blob) throw new Error('Could not render the clipart');
    return new File([blob], name, { type: 'image/png' });
  } finally {
    URL.revokeObjectURL(url);
  }
}

// An Iconify icon rendered as a transparent PNG file, ready to upload.
export function clipartPng(icon: ClipartIcon, color: string, px = CLIPART_PX): Promise<File> {
  const box = iconBox(icon);
  const scale = px / Math.max(box.width, box.height);
  return svgToPng(
    clipartSvg(icon, color, px),
    Math.max(1, Math.round(box.width * scale)),
    Math.max(1, Math.round(box.height * scale)),
    `${icon.set}-${icon.name}.png`,
  );
}

// A whole SVG document (OpenClipart's) rendered as a PNG, its longer side
// `px` pixels: its size comes from its view box, else its width and height.
export function svgDocumentPng(svgText: string, name: string, px = CLIPART_PX): Promise<File> {
  const doc = new DOMParser().parseFromString(svgText, 'image/svg+xml');
  const root = doc.documentElement;
  if (root.nodeName.toLowerCase() !== 'svg' || doc.querySelector('parsererror')) {
    return Promise.reject(new Error('That clipart is not a valid SVG'));
  }
  const vb = (root.getAttribute('viewBox') ?? '').split(/[\s,]+/).map(Number);
  let w = parseFloat(root.getAttribute('width') ?? '');
  let h = parseFloat(root.getAttribute('height') ?? '');
  if (vb.length === 4 && vb[2] > 0 && vb[3] > 0) {
    w = vb[2];
    h = vb[3];
  } else if (w > 0 && h > 0) {
    root.setAttribute('viewBox', `0 0 ${w} ${h}`);
  } else {
    w = h = 100;
  }
  const scale = px / Math.max(w, h);
  const width = Math.max(1, Math.round(w * scale));
  const height = Math.max(1, Math.round(h * scale));
  root.setAttribute('width', String(width));
  root.setAttribute('height', String(height));
  return svgToPng(new XMLSerializer().serializeToString(doc), width, height, name);
}
