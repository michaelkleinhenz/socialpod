// Uploads come in a few thumbnail sizes (/api/uploads/<file>?w=<size>, the
// shorter side in pixels), so a grid of photos need not load every original.
const THUMB_SIZES = [240, 480, 960];

// The URL of an upload's thumbnail big enough for a tile of the given CSS
// width on this screen; other URLs are returned unchanged.
export function thumbnailUrl(url: string, tileWidth: number): string {
  if (!url.includes('/api/uploads/') || url.includes('?')) return url;
  const need = tileWidth * (window.devicePixelRatio || 1);
  const size = THUMB_SIZES.find(s => s >= need) ?? THUMB_SIZES[THUMB_SIZES.length - 1];
  return `${url}?w=${size}`;
}
