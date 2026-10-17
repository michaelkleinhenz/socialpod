import Konva from 'konva';
import { buildLayerNode, clamp, measureText, textLayerAttrs, type AudioTrack, type CropBox, type EditorLayer, type ImageLayer, type TextLayer } from './editorModel';

// Reels: a design with a length, whose layers each show for a span of it and
// come in, move and go out with a preset effect. Everything is a pure function
// of the time, so the preview, a scrub and the export draw the same frame.

export type EnterEffect = 'none' | 'fade' | 'slide-up' | 'slide-down' | 'slide-left' | 'slide-right' | 'zoom' | 'pop' | 'wipe-right' | 'wipe-left' | 'typewriter';
export type ExitEffect = 'none' | 'fade' | 'slide-up' | 'slide-down' | 'slide-left' | 'slide-right' | 'zoom' | 'wipe-right' | 'wipe-left';
export type LoopEffect = 'none' | 'pulse' | 'float' | 'wiggle' | 'spin';

// When a layer shows and how it moves, in seconds of the video. Without it a
// layer shows the whole time and does not move.
export interface LayerAnim {
  start: number;
  // Up to the end of the video when absent, so a longer video keeps it.
  end?: number;
  enter: EnterEffect;
  enterDuration: number;
  exit: ExitEffect;
  exitDuration: number;
  loop: LoopEffect;
}

export const ENTER_EFFECTS: { id: EnterEffect; label: string; textOnly?: boolean }[] = [
  { id: 'none', label: 'None' },
  { id: 'fade', label: 'Fade' },
  { id: 'slide-up', label: 'Slide up' },
  { id: 'slide-down', label: 'Slide down' },
  { id: 'slide-left', label: 'Slide left' },
  { id: 'slide-right', label: 'Slide right' },
  { id: 'zoom', label: 'Zoom' },
  { id: 'pop', label: 'Pop' },
  { id: 'wipe-right', label: 'Wipe right' },
  { id: 'wipe-left', label: 'Wipe left' },
  { id: 'typewriter', label: 'Typewriter', textOnly: true },
];

export const EXIT_EFFECTS: { id: ExitEffect; label: string }[] = [
  { id: 'none', label: 'None' },
  { id: 'fade', label: 'Fade' },
  { id: 'slide-up', label: 'Slide up' },
  { id: 'slide-down', label: 'Slide down' },
  { id: 'slide-left', label: 'Slide left' },
  { id: 'slide-right', label: 'Slide right' },
  { id: 'zoom', label: 'Zoom' },
  { id: 'wipe-right', label: 'Wipe right' },
  { id: 'wipe-left', label: 'Wipe left' },
];

export const LOOP_EFFECTS: { id: LoopEffect; label: string }[] = [
  { id: 'none', label: 'None' },
  { id: 'pulse', label: 'Pulse' },
  { id: 'float', label: 'Float' },
  { id: 'wiggle', label: 'Wiggle' },
  { id: 'spin', label: 'Spin' },
];

// Instagram takes reels of 3 seconds and more; 90 is plenty for a design.
export const MIN_DURATION = 3;
export const MAX_DURATION = 90;
export const DEFAULT_DURATION = 10;
export const DURATION_PRESETS = [5, 10, 15, 30];
export const FPS = 30;

export const NO_ANIM: LayerAnim = { start: 0, enter: 'none', enterDuration: 0.6, exit: 'none', exitDuration: 0.6, loop: 'none' };

export function layerAnim(l: EditorLayer): LayerAnim {
  return { ...NO_ANIM, ...l.anim };
}

// The span a layer shows for, within the video.
export function layerSpan(l: EditorLayer, duration: number): { start: number; end: number } {
  const a = layerAnim(l);
  const start = clamp(a.start, 0, duration);
  const end = clamp(a.end ?? duration, start, duration);
  return { start, end };
}

// How a layer is drawn at one moment, on top of its own attributes: shifted,
// scaled and turned about its centre, faded, and for a typing text how many
// characters are out.
export interface AnimState {
  visible: boolean;
  opacity: number;
  dx: number;
  dy: number;
  scale: number;
  rotation: number;
  chars: number | null;
  // A wipe: the share of the layer's box shown, from the left edge (right)
  // or the right edge (left).
  reveal: { from: 'left' | 'right'; share: number } | null;
}

const IDENTITY: AnimState = { visible: true, opacity: 1, dx: 0, dy: 0, scale: 1, rotation: 0, chars: null, reveal: null };

const easeOut = (p: number) => 1 - Math.pow(1 - p, 3);
const easeIn = (p: number) => p * p * p;
// Overshoots a little and settles, for Pop.
const easeOutBack = (p: number) => 1 + 2.70158 * Math.pow(p - 1, 3) + 1.70158 * Math.pow(p - 1, 2);

// The state of layer l at time t. box is the frame, which sets how far a
// slide travels.
export function animStateAt(l: EditorLayer, t: number, duration: number, box: CropBox): AnimState {
  if (!l.anim) return IDENTITY;
  const a = layerAnim(l);
  const { start, end } = layerSpan(l, duration);
  // The last frame of the video still shows what lasts to the end.
  if (t < start || t > end || (t === end && end < duration)) return { ...IDENTITY, visible: false };
  const s: AnimState = { ...IDENTITY };
  const travel = Math.min(box.width, box.height) * 0.25;
  const length = end - start;
  // Enter and exit share the span when it is too short for both.
  const enterLen = a.enter === 'none' ? 0 : Math.min(a.enterDuration, length / (a.exit === 'none' ? 1 : 2));
  const exitLen = a.exit === 'none' ? 0 : Math.min(a.exitDuration, length / (a.enter === 'none' ? 1 : 2));

  const slide = (effect: string, k: number) => {
    // k runs from 1 (away) to 0 (in place).
    if (effect === 'slide-up') s.dy += travel * k;
    else if (effect === 'slide-down') s.dy -= travel * k;
    else if (effect === 'slide-left') s.dx += travel * k;
    else if (effect === 'slide-right') s.dx -= travel * k;
  };

  if (enterLen > 0 && t < start + enterLen) {
    const p = clamp((t - start) / enterLen, 0, 1);
    const e = easeOut(p);
    switch (a.enter) {
      case 'fade': s.opacity *= p; break;
      case 'zoom': s.scale *= 0.4 + 0.6 * e; s.opacity *= e; break;
      case 'pop': s.scale *= Math.max(0, easeOutBack(p)); s.opacity *= clamp(p * 3, 0, 1); break;
      case 'wipe-right': s.reveal = { from: 'left', share: e }; break;
      case 'wipe-left': s.reveal = { from: 'right', share: e }; break;
      case 'typewriter':
        if (l.kind === 'text') s.chars = Math.floor(p * l.text.length);
        else s.opacity *= p;
        break;
      default: slide(a.enter, 1 - e); s.opacity *= e;
    }
  }
  if (exitLen > 0 && t > end - exitLen) {
    const q = clamp((t - (end - exitLen)) / exitLen, 0, 1);
    const e = easeIn(q);
    switch (a.exit) {
      case 'fade': s.opacity *= 1 - q; break;
      case 'zoom': s.scale *= 1 - 0.6 * e; s.opacity *= 1 - e; break;
      // Wipes away towards the side it names.
      case 'wipe-right': s.reveal = { from: 'right', share: 1 - e }; break;
      case 'wipe-left': s.reveal = { from: 'left', share: 1 - e }; break;
      // Slides out the way it would come in.
      default: slide(a.exit, -e); s.opacity *= 1 - e;
    }
  }

  const since = t - start;
  const wave = (period: number) => Math.sin((2 * Math.PI * since) / period);
  switch (a.loop) {
    case 'pulse': s.scale *= 1 + 0.06 * wave(1.2); break;
    case 'float': s.dy += Math.min(box.width, box.height) * 0.015 * wave(2.4); break;
    case 'wiggle': s.rotation += 4 * wave(0.8); break;
    case 'spin': s.rotation += (360 * since) / 4; break;
  }
  return s;
}

// A layer's centre in picture pixels, which effects scale and turn about.
// Cached per layer object, since a text has to be measured.
const centres = new WeakMap<EditorLayer, { x: number; y: number }>();

export function layerCentre(l: EditorLayer): { x: number; y: number } {
  const cached = centres.get(l);
  if (cached) return cached;
  let c: { x: number; y: number };
  if (l.kind === 'image' || l.kind === 'shape') {
    c = { x: l.x, y: l.y };
  } else if (l.kind === 'text') {
    const m = measureText(l);
    const rad = (l.rotation * Math.PI) / 180;
    const hx = m.width / 2;
    const hy = m.height / 2;
    c = { x: l.x + hx * Math.cos(rad) - hy * Math.sin(rad), y: l.y + hx * Math.sin(rad) + hy * Math.cos(rad) };
  } else {
    let minX = Infinity; let minY = Infinity; let maxX = -Infinity; let maxY = -Infinity;
    for (const st of l.strokes) {
      for (let i = 0; i < st.points.length; i += 2) {
        minX = Math.min(minX, st.points[i]); maxX = Math.max(maxX, st.points[i]);
        minY = Math.min(minY, st.points[i + 1]); maxY = Math.max(maxY, st.points[i + 1]);
      }
    }
    c = Number.isFinite(minX) ? { x: l.x + (minX + maxX) / 2, y: l.y + (minY + maxY) / 2 } : { x: l.x, y: l.y };
  }
  centres.set(l, c);
  return c;
}

// The box a layer covers in picture pixels, turned with it, with room for an
// outline, a shadow or a border; what a wipe cuts.
const bounds = new WeakMap<EditorLayer, CropBox>();

export function layerBounds(l: EditorLayer): CropBox {
  const cached = bounds.get(l);
  if (cached) return cached;
  const node = buildLayerNode(l);
  const r = node.getClientRect();
  node.destroy();
  const pad = Math.max(r.width, r.height) * 0.05;
  const b = { x: r.x - pad, y: r.y - pad, width: r.width + pad * 2, height: r.height + pad * 2 };
  bounds.set(l, b);
  return b;
}

// The attributes of the group a layer is wrapped in to animate it: it turns
// and scales about the layer's centre and moves it by the effect's shift.
export function animGroupAttrs(l: EditorLayer, s: AnimState) {
  const c = layerCentre(l);
  let clip = {};
  if (s.reveal) {
    // The group's own coordinates are the picture's, so the clip is the
    // layer's box, cut down to the share shown.
    const b = layerBounds(l);
    const w = b.width * clamp(s.reveal.share, 0, 1);
    clip = { clipX: s.reveal.from === 'left' ? b.x : b.x + b.width - w, clipY: b.y, clipWidth: w, clipHeight: b.height };
  }
  return {
    ...clip,
    x: c.x + s.dx,
    y: c.y + s.dy,
    offsetX: c.x,
    offsetY: c.y,
    scaleX: s.scale,
    scaleY: s.scale,
    rotation: s.rotation,
    opacity: clamp(s.opacity, 0, 1),
    visible: s.visible,
  };
}

// The layers that show at time t, for a still of the video (a draft's
// preview).
export function layersAt(layers: EditorLayer[], t: number, duration: number): EditorLayer[] {
  return layers.filter(l => {
    if (!l.anim) return true;
    const { start, end } = layerSpan(l, duration);
    return t >= start && t <= end;
  });
}

export function formatTime(t: number): string {
  return `${t.toFixed(1)}s`;
}

// A text's attributes at a moment: while typing, the characters out so far
// in a box as wide as the whole text, so its lines stay where they end up.
export function animatedTextAttrs(l: TextLayer, s: AnimState): Konva.TextConfig {
  const attrs = textLayerAttrs(l);
  if (s.chars === null || s.chars >= l.text.length) return attrs;
  return { ...attrs, text: l.text.slice(0, s.chars), width: measureText(l).width + 0.5 };
}

// A layer's node for one frame, wrapped in the group that animates it.
export function buildAnimatedLayerNode(l: EditorLayer, s: AnimState): Konva.Group {
  const g = new Konva.Group(animGroupAttrs(l, s));
  g.add(l.kind === 'text' ? new Konva.Text(animatedTextAttrs(l, s)) : buildLayerNode(l));
  return g;
}

// --- Clips and sound ---------------------------------------------------------

// Where in its video a clip is at time t: from its trim point on, holding the
// last frame once the video has run out.
export function clipTimeAt(l: ImageLayer, t: number, duration: number): number {
  const c = l.clip;
  if (!c) return 0;
  const { start } = layerSpan(l, duration);
  const end = c.duration > 0 ? c.duration - 1 / FPS : Infinity;
  return clamp(c.trimStart + Math.max(0, t - start), 0, Math.max(0, end));
}

// How loud a clip plays at time t: its volume, faded in and out with the
// layer when it comes in or goes out with an effect, so a crossfade between
// clips crossfades their sound too. Silent once its video has run out.
export function clipGainAt(l: ImageLayer, t: number, duration: number): number {
  const c = l.clip;
  if (!c || c.volume <= 0) return 0;
  const { start, end } = layerSpan(l, duration);
  if (t < start || t >= end) return 0;
  if (c.duration > 0 && c.trimStart + (t - start) >= c.duration) return 0;
  return c.volume * edgeFade(t, start, end, clipFadeIn(l, start, end), clipFadeOut(l, start, end));
}

export function clipFadeIn(l: EditorLayer, start: number, end: number): number {
  const a = layerAnim(l);
  return a.enter === 'none' ? 0 : Math.min(a.enterDuration, (end - start) / 2);
}

export function clipFadeOut(l: EditorLayer, start: number, end: number): number {
  const a = layerAnim(l);
  return a.exit === 'none' ? 0 : Math.min(a.exitDuration, (end - start) / 2);
}

// The span the music plays for: from its start until the video or the file
// ends.
export function musicSpan(a: AudioTrack, duration: number): { start: number; end: number } {
  const start = clamp(a.start, 0, duration);
  const left = a.duration > 0 ? a.duration - a.trimStart : Infinity;
  return { start, end: Math.min(duration, start + Math.max(0, left)) };
}

// Where in its file the music is at time t.
export function musicTimeAt(a: AudioTrack, t: number, duration: number): number {
  const { start } = musicSpan(a, duration);
  return a.trimStart + Math.max(0, t - start);
}

// How loud the music plays at time t, with its fades.
export function musicGainAt(a: AudioTrack, t: number, duration: number): number {
  const { start, end } = musicSpan(a, duration);
  if (t < start || t >= end) return 0;
  return a.volume * edgeFade(t, start, end, a.fadeIn, a.fadeOut);
}

// 0 to 1 over fadeIn seconds after start, 1 to 0 over fadeOut before end.
function edgeFade(t: number, start: number, end: number, fadeIn: number, fadeOut: number): number {
  let g = 1;
  if (fadeIn > 0 && t < start + fadeIn) g *= clamp((t - start) / fadeIn, 0, 1);
  if (fadeOut > 0 && t > end - fadeOut) g *= clamp((end - t) / fadeOut, 0, 1);
  return g;
}
