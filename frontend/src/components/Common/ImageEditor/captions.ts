// Auto-captions for a video: the clips' speech, transcribed with word times
// (POST /api/transcribe), cut into short cues and laid on the video as text
// layers marked `caption`, each showing while its words are said. The layers
// are ordinary texts, so they can be retyped, moved and restyled one by one;
// the Captions section restyles or removes them all at once.

import type { EnterEffect, LayerAnim } from './animation';
import { measureText, type CropBox, type EditorLayer, type TextLayer } from './editorModel';

export interface TranscriptWord {
  word: string;
  start: number;
  end: number;
}

// One caption: its text and when it shows, in seconds of the video.
export interface CaptionCue {
  text: string;
  start: number;
  end: number;
}

// How much a caption holds: one word at a time in the punchy reel style, a
// few words on one line, or a phrase on up to two lines.
export const CAPTION_LENGTHS = [
  { id: 1, label: 'One word' },
  { id: 3, label: 'Few words' },
  { id: 6, label: 'Phrases' },
] as const;

interface CaptionLimits {
  words: number;
  // Characters, spaces included: about one line of the caption font for a
  // few words, two for a phrase (subtitle practice is ~40 a caption).
  chars: number;
  // The longest a caption may span, in seconds.
  seconds: number;
}

const LIMITS: Record<number, CaptionLimits> = {
  1: { words: 1, chars: Infinity, seconds: Infinity },
  3: { words: 4, chars: 20, seconds: 2.5 },
  6: { words: 10, chars: 38, seconds: 5 },
};

// A pause this long (seconds) between two words starts a new caption.
const PAUSE = 0.6;
// A caption stays up this much past its last word, unless the next one
// comes sooner.
const LINGER = 0.3;
// A gap to the next caption this short is closed, so captions do not blink.
const BRIDGE = 0.35;
const MIN_LENGTH = 0.25;
// Reading speed: a caption of several words stays up long enough to read at
// this many characters a second (where the next one leaves the time), and at
// least MIN_READ seconds.
const READ_CPS = 17;
const MIN_READ = 0.8;

const endsSentence = (w: string) => /[.!?…]["»”“'’)]*$/.test(w);
const endsClause = (w: string) => /[,;:–—]["»”“'’)]*$/.test(w);

// Words a caption or a line should not end on: they belong to what follows
// (articles, prepositions, conjunctions, possessives; English and German).
const WEAK = new Set(`a an the and or but nor so of to in on at for with from by as into onto about over under
than that this these those my your our their his her its if when while because which who where
der die das den dem des ein eine einen einem einer eines und oder aber denn sondern mit von vom zu zum zur
im in am an auf für aus bei beim nach über unter vor hinter neben zwischen durch gegen ohne um dass ob wenn weil als wie
mein meine meinen meinem meiner dein deine unser unsere euer eure welche welcher welches`.split(/\s+/));

const isWeak = (w: string) => !/[^\p{L}\p{N}'’]$/u.test(w) && WEAK.has(w.toLocaleLowerCase().replace(/['’]/g, ''));

const joined = (ws: TranscriptWord[]) => ws.map(w => w.word).join(' ');

// What a caption of these words costs as part of a split: one for being a
// caption at all (fewer, fuller captions read more calmly), more the emptier
// it is next to the limit, so a split comes out even rather than leaving a
// word or two behind, and a lot for ending on a word that leads into the
// next one. Cutting before a word that starts a phrase, after a comma or at
// a short pause is cheaper.
function chunkCost(ws: TranscriptWord[], next: TranscriptWord | undefined, lim: CaptionLimits): number {
  const last = ws[ws.length - 1];
  const fill = Number.isFinite(lim.chars) ? joined(ws).length / lim.chars : 1;
  let cost = 1 + (1 - Math.min(1, fill)) ** 2;
  if (next) {
    if (isWeak(last.word)) cost += 2;
    // A phrase starts with such a word ("in the autumn", "mit vielen"),
    // so a cut before one falls between phrases; "of" and "von" mostly
    // belong to the word before them.
    else if (isWeak(next.word) && !/^(of|von)$/i.test(next.word)) cost -= 0.4;
    if (endsClause(last.word)) cost -= 0.5;
    if (next.start - last.end > 0.25) cost -= 0.3;
  }
  return cost;
}

// The cheapest split of a run of words (no sentence end or long pause
// inside) into captions within the limits; a single word that is too long
// is a caption of its own.
function splitRun(run: TranscriptWord[], lim: CaptionLimits): TranscriptWord[][] {
  const fits = (g: TranscriptWord[]) => g.length === 1 || (g.length <= lim.words
    && joined(g).length <= lim.chars && g[g.length - 1].end - g[0].start <= lim.seconds);
  const best = [0];
  const from = [0];
  for (let i = 1; i <= run.length; i++) {
    best[i] = Infinity;
    for (let j = i - 1; j >= 0; j--) {
      const g = run.slice(j, i);
      if (!fits(g)) break;
      const cost = best[j] + chunkCost(g, run[i], lim);
      if (cost < best[i]) { best[i] = cost; from[i] = j; }
    }
  }
  const out: TranscriptWord[][] = [];
  for (let i = run.length; i > 0; i = from[i]) out.unshift(run.slice(from[i], i));
  return out;
}

// Cuts the words into captions within the length's limits: always at the
// end of a sentence and at a pause, and in between where the captions come
// out even and read naturally (after a comma, not after "the" or "und").
// Each is timed to its words, given time to be read and kept within the
// video.
export function captionCues(words: TranscriptWord[], length: number, duration: number): CaptionCue[] {
  const lim = LIMITS[length] ?? LIMITS[6];
  const groups: TranscriptWord[][] = [];
  let run: TranscriptWord[] = [];
  for (const w of words) {
    const prev = run[run.length - 1];
    if (prev && (w.start - prev.end > PAUSE || endsSentence(prev.word))) {
      groups.push(...splitRun(run, lim));
      run = [];
    }
    run.push(w);
  }
  if (run.length) groups.push(...splitRun(run, lim));

  const cues: CaptionCue[] = [];
  groups.forEach((g, i) => {
    const start = Math.max(0, g[0].start);
    if (start >= duration) return;
    const text = joined(g);
    const next = groups[i + 1]?.[0].start ?? Infinity;
    const read = g.length > 1 ? Math.max(MIN_READ, text.length / READ_CPS) : MIN_LENGTH;
    let end = Math.max(g[g.length - 1].end + LINGER, start + read);
    if (next - end < BRIDGE) end = next;
    end = Math.min(end, next, duration);
    end = Math.max(end, Math.min(start + MIN_LENGTH, next, duration));
    cues.push({ text, start, end });
  });
  return cues;
}

export type CaptionPlace = 'lower' | 'middle' | 'upper';

export const CAPTION_PLACES: { id: CaptionPlace; label: string }[] = [
  { id: 'upper', label: 'Top' },
  { id: 'middle', label: 'Middle' },
  { id: 'lower', label: 'Bottom' },
];

// Where a caption's centre sits, as a fraction of the frame's height. Bottom
// is above a reel's caption area (the lowest 22%), the Top below its header.
const PLACE_Y: Record<CaptionPlace, number> = { upper: 0.22, middle: 0.5, lower: 0.68 };

export interface CaptionStyle {
  id: string;
  label: string;
  fill: string;
  bold: boolean;
  outline: boolean;
  shadow: boolean;
  upper: boolean;
  enter: EnterEffect;
}

export const CAPTION_STYLES: CaptionStyle[] = [
  { id: 'bold', label: 'Bold', fill: '#ffffff', bold: true, outline: true, shadow: false, upper: false, enter: 'pop' },
  { id: 'yellow', label: 'Yellow', fill: '#ffe14d', bold: true, outline: true, shadow: false, upper: false, enter: 'pop' },
  { id: 'loud', label: 'Loud', fill: '#ffffff', bold: true, outline: true, shadow: true, upper: true, enter: 'bungee' },
  { id: 'soft', label: 'Soft', fill: '#ffffff', bold: true, outline: false, shadow: true, upper: false, enter: 'fade' },
  { id: 'typewriter', label: 'Typewriter', fill: '#ffffff', bold: false, outline: true, shadow: false, upper: false, enter: 'typewriter' },
];

// Caption text sizes, as a factor of the default size.
export const CAPTION_SIZES = [
  { id: 0.8, label: 'Small' },
  { id: 1, label: 'Medium' },
  { id: 1.25, label: 'Large' },
] as const;

// How captions look. A style preset sets the colour and the text styles and
// brings its enter effect; each of them can then be changed on its own.
export interface CaptionOptions {
  style: CaptionStyle;
  fontFamily: string;
  place: CaptionPlace;
  fill: string;
  bold: boolean;
  outline: boolean;
  shadow: boolean;
  upper: boolean;
  size: number;
}

// The options a style preset sets.
export function styleOptions(style: CaptionStyle): Pick<CaptionOptions, 'style' | 'fill' | 'bold' | 'outline' | 'shadow' | 'upper'> {
  return { style, fill: style.fill, bold: style.bold, outline: style.outline, shadow: style.shadow, upper: style.upper };
}

// Captions stay within this share of the frame's width, which on a reel
// keeps them clear of Instagram's icon column.
const MAX_WIDTH = 0.74;

// A caption's text layer: its words in the options' look, broken into two lines
// when one would be too wide and shrunk if even that is, centred across the
// frame at the place, showing for the cue's time.
export function captionLayer(cue: CaptionCue, o: CaptionOptions, frame: CropBox, id: string): TextLayer {
  let text = o.upper ? cue.text.toLocaleUpperCase() : cue.text;
  const base: TextLayer = {
    id, kind: 'text', caption: true, text, spoken: cue.text,
    fontFamily: o.fontFamily,
    fontSize: Math.round(frame.width * 0.075 * (o.size || 1)),
    fill: o.fill, bold: o.bold, outline: o.outline, shadow: o.shadow,
    align: 'center', x: 0, y: 0, rotation: 0,
  };
  const maxWidth = frame.width * MAX_WIDTH;
  if (measureText(base).width > maxWidth) text = twoLines(text);
  let layer = { ...base, text };
  const m = measureText(layer);
  if (m.width > maxWidth) layer = { ...layer, fontSize: Math.max(8, Math.floor(layer.fontSize * maxWidth / m.width)) };
  const size = measureText(layer);
  const length = cue.end - cue.start;
  const enterDuration = o.style.enter === 'typewriter'
    ? Math.min(length * 0.7, text.length * 0.035)
    : Math.min(0.25, length / 3);
  const anim: LayerAnim = {
    start: cue.start, end: cue.end,
    enter: o.style.enter, enterDuration,
    exit: 'none', exitDuration: 0.2, loop: 'none',
  };
  return {
    ...layer,
    x: frame.x + (frame.width - size.width) / 2,
    y: frame.y + frame.height * PLACE_Y[o.place] - size.height / 2,
    anim,
  };
}

// The text broken into two lines of about even length, preferably after a
// comma and never after a word that leads into the next one.
export function twoLines(text: string): string {
  const words = text.split(' ');
  if (words.length < 2) return text;
  let best = -1;
  let bestCost = Infinity;
  let top = 0;
  for (let k = 1; k < words.length; k++) {
    top += words[k - 1].length + (k > 1 ? 1 : 0);
    const bottom = text.length - top - 1;
    let cost = Math.abs(top - bottom) / text.length;
    if (endsClause(words[k - 1]) || endsSentence(words[k - 1])) cost -= 0.25;
    if (isWeak(words[k - 1])) cost += 0.5;
    if (cost < bestCost) { best = k; bestCost = cost; }
  }
  return `${words.slice(0, best).join(' ')}\n${words.slice(best).join(' ')}`;
}

export const isCaption = (l: EditorLayer): l is TextLayer => l.kind === 'text' && !!l.caption;

// A caption layer's cue: its text on one line again and its time. The
// words as spoken while the text is still them (all caps or not), otherwise
// the text as retyped.
export function cueOf(l: TextLayer, duration: number): CaptionCue {
  const text = l.text.replace(/\s*\n\s*/g, ' ');
  const spoken = l.spoken && (text === l.spoken || text === l.spoken.toLocaleUpperCase()) ? l.spoken : text;
  return { text: spoken, start: l.anim?.start ?? 0, end: l.anim?.end ?? duration };
}
