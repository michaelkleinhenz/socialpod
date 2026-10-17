// Where Instagram lays its own interface over a full-screen 9:16 video, so
// the editor can shade it: text and faces belong outside these areas. The
// boxes are fractions of the frame, measured on a 1080×1920 reel or story.

export type SafeZoneKind = 'reel' | 'story';

export interface SafeZoneBox {
  x: number;
  y: number;
  width: number;
  height: number;
  label: string;
}

const W = 1080;
const H = 1920;
const box = (x: number, y: number, width: number, height: number, label: string): SafeZoneBox =>
  ({ x: x / W, y: y / H, width: width / W, height: height / H, label });

export const SAFE_ZONES: Record<SafeZoneKind, SafeZoneBox[]> = {
  reel: [
    box(0, 0, W, 220, 'Header'),
    box(W - 140, 1000, 140, H - 420 - 1000, 'Icons'),
    box(0, H - 420, W, 420, 'Caption, handle and audio'),
  ],
  story: [
    box(0, 0, W, 250, 'Progress bar and profile'),
    box(0, H - 340, W, 340, 'Reply field'),
  ],
};

// The boxes laid over a frame of the given size, in its own units.
export function safeZoneRects(kind: SafeZoneKind, frame: { x: number; y: number; width: number; height: number }) {
  return SAFE_ZONES[kind].map(b => ({
    x: frame.x + b.x * frame.width,
    y: frame.y + b.y * frame.height,
    width: b.width * frame.width,
    height: b.height * frame.height,
    label: b.label,
  }));
}
