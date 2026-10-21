import Konva from 'konva';
import { buildLayerNode, clamp, measureText, textLayerAttrs, type AudioTrack, type CropBox, type EditorLayer, type ImageLayer, type ShapeLayer, type TextLayer } from './editorModel';
import { barCount, idleLevels, levelsAtTime, LEVELS_FPS, restLevels, type AudioLevels } from './equalizer';

// Reels: a design with a length, whose layers each show for a span of it and
// come in, move and go out with a preset effect. Everything is a pure function
// of the time, so the preview, a scrub and the export draw the same frame.

export type EnterEffect = 'none' | 'fade' | 'slide-up' | 'slide-down' | 'slide-left' | 'slide-right' | 'zoom' | 'pop' | 'wipe-right' | 'wipe-left'
  | 'flicker' | 'tumble' | 'spin' | 'bungee' | 'shrink' | 'typewriter';
export type ExitEffect = 'none' | 'fade' | 'slide-up' | 'slide-down' | 'slide-left' | 'slide-right' | 'zoom' | 'wipe-right' | 'wipe-left'
  | 'flicker' | 'tumble' | 'spin' | 'pop' | 'grow';
export type LoopEffect = 'none' | 'pulse' | 'float' | 'wiggle' | 'spin' | 'breathe' | 'bob' | 'yoyo' | 'jitter' | 'blink' | 'flicker' | 'slow-zoom';

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
  // Keyframes: where the layer is, how large, turned and opaque at moments
  // of its span, eased from one to the next. Sorted by time.
  keys?: Keyframe[];
}

// How a keyframe eases to the next one: hold stays put and jumps.
export type KeyEase = 'smooth' | 'linear' | 'ease-in' | 'ease-out' | 'hold';

// The layer at one moment, on top of its own attributes: moved by x/y
// picture pixels, scaled and turned about its centre, its opacity scaled.
// t is in seconds after the layer comes in, so a key moves with its bar.
export interface Keyframe {
  t: number;
  x: number;
  y: number;
  scale: number;
  rotation: number;
  opacity: number;
  ease?: KeyEase;
}

export type KeyValues = Pick<Keyframe, 'x' | 'y' | 'scale' | 'rotation' | 'opacity'>;

export const KEY_EASES: { id: KeyEase; label: string }[] = [
  { id: 'smooth', label: 'Smooth' },
  { id: 'linear', label: 'Linear' },
  { id: 'ease-in', label: 'Ease in' },
  { id: 'ease-out', label: 'Ease out' },
  { id: 'hold', label: 'Hold' },
];

// How one clip of the sequence takes over from the one before it.
export type TransitionKind = 'cut' | 'crossfade' | 'dip' | 'push-left' | 'push-right' | 'push-up' | 'push-down' | 'wipe-right' | 'wipe-left' | 'zoom' | 'cover';

export const TRANSITIONS: { id: TransitionKind; label: string }[] = [
  { id: 'cut', label: 'Cut' },
  { id: 'crossfade', label: 'Crossfade' },
  { id: 'dip', label: 'Dip' },
  { id: 'push-left', label: 'Push left' },
  { id: 'push-right', label: 'Push right' },
  { id: 'push-up', label: 'Push up' },
  { id: 'push-down', label: 'Push down' },
  { id: 'wipe-right', label: 'Wipe right' },
  { id: 'wipe-left', label: 'Wipe left' },
  { id: 'zoom', label: 'Zoom' },
  { id: 'cover', label: 'Slide over' },
];

export const KEY_IDENTITY: KeyValues = { x: 0, y: 0, scale: 1, rotation: 0, opacity: 1 };

// Two keys closer than this are the same moment: a playhead on a key edits
// that key rather than adding one beside it.
export const KEY_SNAP = 1 / (2 * 30);

const easeKey = (ease: KeyEase | undefined, p: number) => {
  switch (ease) {
    case 'linear': return p;
    case 'ease-in': return p * p * p;
    case 'ease-out': return 1 - Math.pow(1 - p, 3);
    case 'hold': return 0;
    default: return p < 0.5 ? 4 * p * p * p : 1 - Math.pow(-2 * p + 2, 3) / 2;
  }
};

// The keyed values at `since` seconds into the span: the first key's before
// it, the last key's after it, eased between.
export function keyValuesAt(keys: Keyframe[] | undefined, since: number): KeyValues {
  if (!keys?.length) return KEY_IDENTITY;
  if (since <= keys[0].t) return pickValues(keys[0]);
  const last = keys[keys.length - 1];
  if (since >= last.t) return pickValues(last);
  let i = 0;
  while (keys[i + 1].t < since) i++;
  const a = keys[i];
  const b = keys[i + 1];
  const p = easeKey(a.ease, b.t > a.t ? (since - a.t) / (b.t - a.t) : 1);
  const mix = (u: number, v: number) => u + (v - u) * p;
  return { x: mix(a.x, b.x), y: mix(a.y, b.y), scale: mix(a.scale, b.scale), rotation: mix(a.rotation, b.rotation), opacity: mix(a.opacity, b.opacity) };
}

function pickValues(k: Keyframe): KeyValues {
  return { x: k.x, y: k.y, scale: k.scale, rotation: k.rotation, opacity: k.opacity };
}

// The index of the key at `since`, or -1.
export function keyIndexAt(keys: Keyframe[] | undefined, since: number): number {
  return keys?.findIndex(k => Math.abs(k.t - since) <= KEY_SNAP) ?? -1;
}

// The keys with the one at `since` changed by patch, added there (from the
// values it has at that moment) when there is none.
export function setKeyAt(keys: Keyframe[] | undefined, since: number, patch: Partial<Keyframe>): Keyframe[] {
  const list = keys ?? [];
  const i = keyIndexAt(list, since);
  if (i >= 0) return list.map((k, j) => (j === i ? { ...k, ...patch } : k));
  const prev = [...list].reverse().find(k => k.t < since);
  const added: Keyframe = { ...keyValuesAt(list, since), t: since, ...(prev?.ease ? { ease: prev.ease } : {}), ...patch };
  return sortKeys([...list, added]);
}

export function sortKeys(keys: Keyframe[]): Keyframe[] {
  return [...keys].sort((a, b) => a.t - b.t);
}

// Whether a layer is moved by keyframes, so moving, resizing and turning it
// on the picture set the key at the playhead instead of the layer itself.
export function hasKeys(l: EditorLayer): boolean {
  return !!l.anim?.keys?.length;
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
  { id: 'flicker', label: 'Flicker' },
  { id: 'tumble', label: 'Tumble' },
  { id: 'spin', label: 'Spin' },
  { id: 'bungee', label: 'Bungee' },
  { id: 'shrink', label: 'Shrink' },
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
  { id: 'flicker', label: 'Flicker' },
  { id: 'tumble', label: 'Tumble' },
  { id: 'spin', label: 'Spin' },
  { id: 'pop', label: 'Pop' },
  { id: 'grow', label: 'Grow' },
];

export const LOOP_EFFECTS: { id: LoopEffect; label: string }[] = [
  { id: 'none', label: 'None' },
  { id: 'pulse', label: 'Pulse' },
  { id: 'float', label: 'Float' },
  { id: 'wiggle', label: 'Wiggle' },
  { id: 'spin', label: 'Spin' },
  { id: 'breathe', label: 'Breathe' },
  { id: 'bob', label: 'Bob' },
  { id: 'yoyo', label: 'Yoyo' },
  { id: 'jitter', label: 'Jitter' },
  { id: 'blink', label: 'Blink' },
  { id: 'flicker', label: 'Flicker' },
  { id: 'slow-zoom', label: 'Slow zoom' },
];

// Instagram takes reels of 3 seconds and more; 90 is plenty for a design.
export const MIN_DURATION = 3;
export const MAX_DURATION = 90;
export const DEFAULT_DURATION = 10;
export const DURATION_PRESETS = [5, 10, 15, 30];
export const FPS = LEVELS_FPS;

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

// The first moment a layer shows in full, its entrance over (shared with
// the exit when its span is too short for both, as animStateAt does).
export function settledAt(l: EditorLayer, duration: number): number {
  const { start, end } = layerSpan(l, duration);
  const a = layerAnim(l);
  return a.enter === 'none' ? start : Math.min(start + a.enterDuration, (start + end) / 2);
}

// Where a design opens: once the layers that come in at the very start have
// settled, so a reopened video does not show them before they are in (faded
// out and shifted off their place), which reads as lost.
export function openingTime(layers: EditorLayer[], duration: number): number {
  let t = 0;
  for (const l of layers) {
    if (l.anim && layerSpan(l, duration).start < 0.05) t = Math.max(t, settledAt(l, duration));
  }
  return clamp(t, 0, duration);
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
// Overshoots and springs back a few times, for Bungee.
const easeOutElastic = (p: number) => (p <= 0 ? 0 : p >= 1 ? 1 : Math.pow(2, -10 * p) * Math.sin((p * 10 - 0.75) * ((2 * Math.PI) / 3)) + 1);

// A fixed pseudo-random number in [0, 1) for step n, so a flicker or a
// jitter is the same on every play and in the export.
const noise = (n: number) => {
  const x = Math.sin(n * 12.9898) * 43758.5453;
  return x - Math.floor(x);
};

// A number per layer, so layers that flicker at once do not flicker alike.
function layerSeed(l: EditorLayer): number {
  let h = 0;
  for (let i = 0; i < l.id.length; i++) h = (h * 31 + l.id.charCodeAt(i)) % 9973;
  return h;
}

// Whether a flickering layer is on at progress p (0 to 1) of an enter:
// off and on in ten steps, on more often as it goes, and on at the end.
function flickerOn(p: number, seed: number): boolean {
  if (p >= 0.9) return true;
  return noise(seed + Math.floor(p * 10)) < p + 0.1;
}

// The state of layer l at time t. box is the frame, which sets how far a
// slide travels.
export function animStateAt(l: EditorLayer, t: number, duration: number, box: CropBox): AnimState {
  if (!l.anim) return IDENTITY;
  const a = layerAnim(l);
  const { start, end } = layerSpan(l, duration);
  // The last frame of the video still shows what lasts to the end.
  if (t < start || t > end || (t === end && end < duration)) return { ...IDENTITY, visible: false };
  const s: AnimState = { ...IDENTITY };
  if (a.keys?.length) {
    const k = keyValuesAt(a.keys, t - start);
    s.dx = k.x;
    s.dy = k.y;
    s.scale = Math.max(0, k.scale);
    s.rotation = k.rotation;
    s.opacity = clamp(k.opacity, 0, 1);
  }
  if (l.kind === 'image' && l.clip) applyTransitions(s, l.clip, t, start, end, box);
  const travel = Math.min(box.width, box.height) * 0.25;
  const seed = layerSeed(l);
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
      case 'flicker': if (!flickerOn(p, seed)) s.opacity = 0; break;
      // Rolls in from a half turn back, growing.
      case 'tumble': s.rotation -= 180 * (1 - e); s.scale *= 0.3 + 0.7 * e; s.opacity *= e; break;
      case 'spin': s.rotation -= 360 * (1 - e); s.scale *= 0.5 + 0.5 * e; s.opacity *= e; break;
      // Drops from above and springs on its rope until it settles.
      case 'bungee': s.dy -= travel * 2 * (1 - easeOutElastic(p)); s.opacity *= clamp(p * 4, 0, 1); break;
      // Comes in large and shrinks into place.
      case 'shrink': s.scale *= 1.8 - 0.8 * e; s.opacity *= e; break;
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
      case 'flicker': if (!flickerOn(1 - q, seed + 50)) s.opacity = 0; break;
      case 'tumble': s.rotation += 180 * e; s.scale *= 1 - 0.7 * e; s.opacity *= 1 - e; break;
      case 'spin': s.rotation += 360 * e; s.scale *= 1 - 0.5 * e; s.opacity *= 1 - e; break;
      // Swells a little, then snaps away.
      case 'pop':
        s.scale *= q < 0.3 ? 1 + 0.15 * easeOut(q / 0.3) : 1.15 * (1 - easeIn((q - 0.3) / 0.7));
        break;
      case 'grow': s.scale *= 1 + 0.8 * e; s.opacity *= 1 - e; break;
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
    // Slow and soft, swelling and dimming a little like a breath.
    case 'breathe': {
      const b = (1 - Math.cos((2 * Math.PI * since) / 3.2)) / 2;
      s.scale *= 1 + 0.05 * b;
      s.opacity *= 1 - 0.15 * b;
      break;
    }
    // Hops up and drops back, landing each second.
    case 'bob': s.dy -= Math.min(box.width, box.height) * 0.03 * Math.abs(Math.sin(Math.PI * since)); break;
    case 'yoyo': s.dx += Math.min(box.width, box.height) * 0.03 * wave(2); break;
    // A nervous shake, a new offset twelve times a second.
    case 'jitter': {
      const n = seed + Math.floor(since * 12) * 3;
      const amp = Math.min(box.width, box.height) * 0.006;
      s.dx += (noise(n) - 0.5) * 2 * amp;
      s.dy += (noise(n + 1) - 0.5) * 2 * amp;
      s.rotation += (noise(n + 2) - 0.5) * 2;
      break;
    }
    // Off for the last quarter of every second.
    case 'blink': if (since % 1 >= 0.75) s.opacity = 0; break;
    // Mostly on, dimming now and then for a moment, like a failing bulb.
    case 'flicker': if (noise(seed + Math.floor(since * 15)) < 0.1) s.opacity *= 0.25; break;
    // Grows a tenth over the whole time it shows, the slow push-in of a
    // photo in a slideshow.
    case 'slow-zoom': s.scale *= 1 + 0.1 * clamp(since / Math.max(length, 0.1), 0, 1); break;
  }
  return s;
}

const easeInOut = (p: number) => (p < 0.5 ? 4 * p * p * p : 1 - Math.pow(-2 * p + 2, 3) / 2);

// A sequence clip's transitions: the one it comes in with, over the start of
// its span, and the next clip's, over its end. The incoming clip lies on top
// (it is stacked above), so a crossfade only fades it in; a push moves both
// clips a whole frame, together.
function applyTransitions(s: AnimState, c: NonNullable<ImageLayer['clip']>, t: number, start: number, end: number, box: CropBox) {
  const { into, out } = c;
  if (into && into.duration > 0 && t < start + into.duration) {
    const p = clamp((t - start) / into.duration, 0, 1);
    const k = 1 - easeInOut(p);
    switch (into.kind) {
      case 'crossfade': s.opacity *= p; break;
      // Out of the clip before, which fades over the first half, to this
      // one, over the second.
      case 'dip': s.opacity *= clamp(p * 2 - 1, 0, 1); break;
      case 'push-left': case 'cover': s.dx += box.width * k; break;
      case 'push-right': s.dx -= box.width * k; break;
      case 'push-up': s.dy += box.height * k; break;
      case 'push-down': s.dy -= box.height * k; break;
      case 'wipe-right': s.reveal = { from: 'left', share: 1 - k }; break;
      case 'wipe-left': s.reveal = { from: 'right', share: 1 - k }; break;
      case 'zoom': s.scale *= 0.8 + 0.2 * (1 - k); s.opacity *= p; break;
    }
  }
  if (out && out.duration > 0 && t > end - out.duration) {
    const q = clamp((t - (end - out.duration)) / out.duration, 0, 1);
    const e = easeInOut(q);
    switch (out.kind) {
      case 'dip': s.opacity *= clamp(1 - q * 2, 0, 1); break;
      case 'push-left': s.dx -= box.width * e; break;
      case 'push-right': s.dx += box.width * e; break;
      case 'push-up': s.dy -= box.height * e; break;
      case 'push-down': s.dy += box.height * e; break;
      case 'zoom': s.scale *= 1 + 0.25 * e; break;
    }
  }
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

// A layer's node for one frame, wrapped in the group that animates it;
// levels are an equalizer's bars in that frame.
export function buildAnimatedLayerNode(l: EditorLayer, s: AnimState, levels?: number[]): Konva.Group {
  const g = new Konva.Group(animGroupAttrs(l, s));
  g.add(l.kind === 'text' ? new Konva.Text(animatedTextAttrs(l, s)) : buildLayerNode(l, levels));
  return g;
}

export function isEqualizer(l: EditorLayer): l is ShapeLayer {
  return l.kind === 'shape' && l.shape === 'equalizer';
}

// The levels (0 to 1) of an equalizer's bars: at rest in a still design
// (t null), following the music at time t once it has been analysed, and
// the made-up dance otherwise. Where the music does not play the bars rest
// at their lowest, and they fade with its fades.
export function equalizerLevels(
  l: ShapeLayer, t: number | null, duration: number, audio: AudioTrack | null | undefined, levels: AudioLevels | null | undefined,
): number[] {
  const n = barCount(l);
  if (t === null) return restLevels(n);
  if (audio && levels && levels.count > 0) {
    const { start, end } = musicSpan(audio, duration);
    if (t < start || t >= end) return new Array(n).fill(0);
    const gain = audio.volume > 0 ? clamp(musicGainAt(audio, t, duration) / audio.volume, 0, 1) : 0;
    return levelsAtTime(levels, musicTimeAt(audio, t, duration), n).map(v => v * gain);
  }
  return idleLevels(n, t);
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

// A sequence clip's sound crosses over with the next one's for the length
// of the transition between them, whichever it is (but a cut).
export function clipFadeIn(l: EditorLayer, start: number, end: number): number {
  const a = layerAnim(l);
  const own = a.enter === 'none' ? 0 : Math.min(a.enterDuration, (end - start) / 2);
  const into = l.kind === 'image' ? l.clip?.into : undefined;
  return Math.max(own, into?.duration ?? 0);
}

export function clipFadeOut(l: EditorLayer, start: number, end: number): number {
  const a = layerAnim(l);
  const own = a.exit === 'none' ? 0 : Math.min(a.exitDuration, (end - start) / 2);
  const out = l.kind === 'image' ? l.clip?.out : undefined;
  return Math.max(own, out?.duration ?? 0);
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
