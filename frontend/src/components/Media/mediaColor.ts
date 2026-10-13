/** A colour as the media library takes it: #rgb, #rrggbb or #rrggbbaa, the # optional. */
export const HEX_COLOR = /^#?([0-9a-f]{3}|[0-9a-f]{6}|[0-9a-f]{8})$/i;

/** #rgb / #rrggbb(aa) as the #rrggbb a colour input can show. */
export function pickerValue(hex: string): string {
  const h = hex.trim().replace(/^#/, '');
  if (h.length === 3) return '#' + h.split('').map(c => c + c).join('').toLowerCase();
  return HEX_COLOR.test(h) ? '#' + h.slice(0, 6).toLowerCase() : '#000000';
}
