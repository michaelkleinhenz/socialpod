// A carousel goes out square: a page of another shape is fitted into the square
// over a blurred fill of itself. The gallery's tiles and the carousel pages crop
// to a square, so a photo that is not square already gets a warning.
export const NOT_SQUARE_HINT = 'Not cropped square: in a carousel it is fitted into a square with blurred edges. Crop it to pick the framing.';

export function isSquare(img: HTMLImageElement) {
  const { naturalWidth: w, naturalHeight: h } = img;
  return w > 0 && h > 0 && Math.abs(w - h) / Math.max(w, h) < 0.01;
}
