import type { ShapeLayer } from './editorModel';

// The equalizer: a shape of bars that dance to the video's music, for a reel
// that has only sound (a podcast clip, an audio play) and nothing moving of
// its own. The music is analysed once into the loudness of a few frequency
// bands per video frame; at time t each bar takes the level of its band, so
// the stage, a scrub and the export draw the same bars. Without music (a
// template's preview, a design still being set up) the bars move in a
// made-up pattern, and a still design shows them at rest. The server's
// template renderer (services/compose_paint.go) draws that resting pattern.

export const DEFAULT_BARS = 24;
export const MIN_BARS = 4;
export const MAX_BARS = 64;
// The gap between two bars, as a share of a bar's width.
const GAP = 0.6;

// The music's loudness per band and frame: frames × bands values from 0 to
// 1, LEVELS_FPS (the video's FPS) frames a second from the start of the file.
export const LEVELS_FPS = 30;
export interface AudioLevels {
  bands: number;
  count: number;
  values: Float32Array;
}

const BANDS = 32;
const SAMPLE_RATE = 22050;
const FFT_SIZE = 1024;
const LOW_HZ = 50;
const HIGH_HZ = 9000;
// Longer files are analysed this far; a reel never runs that long.
const MAX_SECONDS = 600;

// How many bars a layer has.
export function barCount(l: ShapeLayer): number {
  return Math.round(clamp(l.bars ?? DEFAULT_BARS, MIN_BARS, MAX_BARS));
}

// The bars at rest: a fixed, lively looking pattern, louder in the lows.
// compose_paint.go's equalizerRest computes the same.
export function restLevels(n: number): number[] {
  return Array.from({ length: n }, (_, i) => {
    const p = n > 1 ? i / (n - 1) : 0;
    return 0.25 + 0.6 * (0.5 + 0.5 * Math.sin(i * 1.9 + 0.6)) * (1 - 0.45 * p) + 0.15 * (1 - p);
  });
}

// A made-up dance for when there is no music to follow: each bar swings on
// two waves of its own, the lows higher and slower than the highs.
export function idleLevels(n: number, since: number): number[] {
  return Array.from({ length: n }, (_, i) => {
    const p = n > 1 ? i / (n - 1) : 0;
    const a = Math.sin(since * (5.1 + 3.3 * p) + i * 2.17);
    const b = Math.sin(since * (8.3 + 4.1 * p) + i * 0.73 + 1.3);
    const beat = 0.75 + 0.25 * Math.abs(Math.sin(since * Math.PI * 2));
    return clamp((0.2 + 0.8 * Math.abs(0.6 * a + 0.4 * b)) * (1 - 0.4 * p) * beat, 0.08, 1);
  });
}

// The levels of n bars at music time mt (seconds into the file), the bands
// spread across the bars.
export function levelsAtTime(lv: AudioLevels, mt: number, n: number): number[] {
  const f = clamp(mt * LEVELS_FPS, 0, lv.count - 1);
  const f0 = Math.floor(f);
  const f1 = Math.min(lv.count - 1, f0 + 1);
  const ft = f - f0;
  const band = (b: number) => {
    const v0 = lv.values[f0 * lv.bands + b];
    const v1 = lv.values[f1 * lv.bands + b];
    return v0 + (v1 - v0) * ft;
  };
  return Array.from({ length: n }, (_, i) => {
    const pos = n > 1 ? (i / (n - 1)) * (lv.bands - 1) : 0;
    const b0 = Math.floor(pos);
    const b1 = Math.min(lv.bands - 1, b0 + 1);
    return band(b0) + (band(b1) - band(b0)) * (pos - b0);
  });
}

// The bars of an equalizer w × h at levels, as boxes in the layer's box: as
// wide as fits with their gaps, at least as high as wide (a dot when quiet),
// standing on the bottom edge or, mirrored, about the middle.
export function equalizerBars(w: number, h: number, levels: number[], mirror: boolean): { x: number; y: number; width: number; height: number }[] {
  const n = levels.length;
  const bw = w / (n + (n - 1) * GAP);
  return levels.map((v, i) => {
    const bh = clamp(Math.max(bw, h * clamp(v, 0, 1)), 0, h);
    return { x: i * bw * (1 + GAP), y: mirror ? (h - bh) / 2 : h - bh, width: bw, height: bh };
  });
}

const clamp = (v: number, min: number, max: number) => Math.max(min, Math.min(max, v));

// --- Analysis ----------------------------------------------------------------

const analysed = new Map<string, Promise<AudioLevels | null>>();

// The music at url analysed, once per session; null when it cannot be
// decoded.
export function audioLevels(url: string): Promise<AudioLevels | null> {
  let p = analysed.get(url);
  if (!p) {
    p = analyse(url).catch(() => null);
    analysed.set(url, p);
  }
  return p;
}

async function analyse(url: string): Promise<AudioLevels | null> {
  const res = await fetch(url, { credentials: 'omit' });
  if (!res.ok) return null;
  const data = await res.arrayBuffer();
  // Decoded into mono at a low rate: the bands go up to 9 kHz.
  const ctx = new OfflineAudioContext(1, 1, SAMPLE_RATE);
  const buffer = await ctx.decodeAudioData(data);
  const length = Math.min(buffer.length, MAX_SECONDS * buffer.sampleRate);
  const mono = new Float32Array(length);
  for (let c = 0; c < buffer.numberOfChannels; c++) {
    const ch = buffer.getChannelData(c);
    for (let i = 0; i < length; i++) mono[i] += ch[i] / buffer.numberOfChannels;
  }
  return spectrumLevels(mono, buffer.sampleRate);
}

// The band levels of a mono signal, LEVELS_FPS frames a second: the power of
// each band (log-spaced) in dB, scaled between that band's quiet and loud ends
// over the whole file so every bar moves, then falling back slower than it
// rises, as an equalizer's bars do.
export async function spectrumLevels(mono: Float32Array, rate: number): Promise<AudioLevels> {
  const hop = rate / LEVELS_FPS;
  const count = Math.max(1, Math.floor(mono.length / hop));
  const values = new Float32Array(count * BANDS);
  const window = new Float32Array(FFT_SIZE);
  for (let i = 0; i < FFT_SIZE; i++) window[i] = 0.5 - 0.5 * Math.cos((2 * Math.PI * i) / (FFT_SIZE - 1));
  // Which FFT bins each band sums.
  const edges = Array.from({ length: BANDS + 1 }, (_, b) => LOW_HZ * Math.pow(HIGH_HZ / LOW_HZ, b / BANDS));
  const binOf = (hz: number) => Math.round((hz / rate) * FFT_SIZE);
  const re = new Float32Array(FFT_SIZE);
  const im = new Float32Array(FFT_SIZE);
  for (let f = 0; f < count; f++) {
    // Let the page breathe on a long file.
    if (f % 400 === 399) await new Promise(r => setTimeout(r, 0));
    const centre = Math.floor(f * hop);
    for (let i = 0; i < FFT_SIZE; i++) {
      const s = centre - FFT_SIZE / 2 + i;
      re[i] = s >= 0 && s < mono.length ? mono[s] * window[i] : 0;
      im[i] = 0;
    }
    fft(re, im);
    for (let b = 0; b < BANDS; b++) {
      const lo = Math.max(1, binOf(edges[b]));
      const hi = Math.max(lo, binOf(edges[b + 1]) - 1);
      let power = 0;
      for (let k = lo; k <= hi; k++) power += re[k] * re[k] + im[k] * im[k];
      values[f * BANDS + b] = 10 * Math.log10(power / (hi - lo + 1) + 1e-12);
    }
  }
  // Each band between its own quiet and loud ends.
  const sorted = new Float32Array(count);
  for (let b = 0; b < BANDS; b++) {
    for (let f = 0; f < count; f++) sorted[f] = values[f * BANDS + b];
    sorted.sort();
    const floor = Math.max(sorted[Math.floor(count * 0.2)], sorted[Math.floor(count * 0.98)] - 45);
    const ceil = sorted[Math.floor(count * 0.98)];
    const range = Math.max(6, ceil - floor);
    let prev = 0;
    for (let f = 0; f < count; f++) {
      const v = clamp((values[f * BANDS + b] - floor) / range, 0, 1);
      // Up at once, down by a fifth a frame.
      prev = v >= prev ? v : prev * 0.8 + v * 0.2;
      values[f * BANDS + b] = prev;
    }
  }
  return { bands: BANDS, count, values };
}

// In-place radix-2 FFT; the length is a power of two.
function fft(re: Float32Array, im: Float32Array) {
  const n = re.length;
  for (let i = 1, j = 0; i < n; i++) {
    let bit = n >> 1;
    for (; j & bit; bit >>= 1) j ^= bit;
    j ^= bit;
    if (i < j) {
      [re[i], re[j]] = [re[j], re[i]];
      [im[i], im[j]] = [im[j], im[i]];
    }
  }
  for (let len = 2; len <= n; len <<= 1) {
    const ang = (-2 * Math.PI) / len;
    const wr = Math.cos(ang);
    const wi = Math.sin(ang);
    for (let i = 0; i < n; i += len) {
      let cr = 1;
      let ci = 0;
      for (let k = 0; k < len / 2; k++) {
        const a = i + k;
        const b = a + len / 2;
        const xr = re[b] * cr - im[b] * ci;
        const xi = re[b] * ci + im[b] * cr;
        re[b] = re[a] - xr;
        im[b] = im[a] - xi;
        re[a] += xr;
        im[a] += xi;
        const nr = cr * wr - ci * wi;
        ci = cr * wi + ci * wr;
        cr = nr;
      }
    }
  }
}
