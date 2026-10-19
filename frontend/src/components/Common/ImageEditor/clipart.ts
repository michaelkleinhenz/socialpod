// Clipart from Iconify (handlers/clipart.go): each search result is an SVG
// body with its view box. Recolourable icons draw in currentColor, which is
// set here; emoji keep their own colours. On the picture a piece of clipart
// is an ordinary image layer: rendered to a PNG and uploaded, so templates,
// drafts, the video export and compose_image need nothing of their own.
import type { ClipartIcon } from '../../../types';

// The longest side of the PNG put on the picture: sharp at full size on a
// 1080 px canvas.
export const CLIPART_PX = 1024;

// Searches offered before anything is typed.
export const CLIPART_SUGGESTIONS = ['arrow', 'star', 'heart', 'check', 'speech bubble', 'fire', 'crown', 'dice', 'sparkles', 'party'];

const escapeAttr = (s: string) => s.replace(/&/g, '&amp;').replace(/"/g, '&quot;').replace(/</g, '&lt;');

// The icon as a standalone SVG document, its longer side `px` pixels.
export function clipartSvg(icon: ClipartIcon, color: string, px: number): string {
  const scale = px / Math.max(icon.width, icon.height);
  const w = Math.max(1, Math.round(icon.width * scale));
  const h = Math.max(1, Math.round(icon.height * scale));
  return `<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" width="${w}" height="${h}" `
    + `viewBox="${icon.left} ${icon.top} ${icon.width} ${icon.height}" color="${escapeAttr(color)}">${icon.body}</svg>`;
}

// A data URL for the icon's tile in the grid.
export const clipartPreviewUrl = (icon: ClipartIcon, color: string) =>
  'data:image/svg+xml;charset=utf-8,' + encodeURIComponent(clipartSvg(icon, color, 96));

// The icon rendered as a transparent PNG file, ready to upload.
export async function clipartPng(icon: ClipartIcon, color: string, px = CLIPART_PX): Promise<File> {
  const svg = clipartSvg(icon, color, px);
  const url = URL.createObjectURL(new Blob([svg], { type: 'image/svg+xml' }));
  try {
    const img = new Image();
    img.decoding = 'async';
    img.src = url;
    await img.decode();
    const scale = px / Math.max(icon.width, icon.height);
    const canvas = document.createElement('canvas');
    canvas.width = Math.max(1, Math.round(icon.width * scale));
    canvas.height = Math.max(1, Math.round(icon.height * scale));
    const ctx = canvas.getContext('2d');
    if (!ctx) throw new Error('No canvas');
    ctx.drawImage(img, 0, 0, canvas.width, canvas.height);
    const blob = await new Promise<Blob | null>(resolve => canvas.toBlob(resolve, 'image/png'));
    if (!blob) throw new Error('Could not render the clipart');
    return new File([blob], `${icon.set}-${icon.name}.png`, { type: 'image/png' });
  } finally {
    URL.revokeObjectURL(url);
  }
}
