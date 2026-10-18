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

// How many words a caption holds at most: one at a time in the punchy reel
// style, or a short phrase.
export const CAPTION_LENGTHS = [
  { id: 1, label: 'One word' },
  { id: 3, label: 'Few words' },
  { id: 6, label: 'Phrases' },
] as const;

// A pause this long (seconds) between two words starts a new caption.
const PAUSE = 0.6;
// A caption stays up this much past its last word, unless the next one
// comes sooner.
const LINGER = 0.3;
// A gap to the next caption this short is closed, so captions do not blink.
const BRIDGE = 0.35;
const MIN_LENGTH = 0.25;

const endsSentence = (w: string) => /[.!?…]["»”']?$/.test(w);

// Cuts the words into captions of at most maxWords, also at a pause and at
// the end of a sentence, timed to their words and kept within the video.
export function captionCues(words: TranscriptWord[], maxWords: number, duration: number): CaptionCue[] {
  const groups: TranscriptWord[][] = [];
  let group: TranscriptWord[] = [];
  for (const w of words) {
    const prev = group[group.length - 1];
    if (prev && (group.length >= maxWords || w.start - prev.end > PAUSE || endsSentence(prev.word))) {
      groups.push(group);
      group = [];
    }
    group.push(w);
  }
  if (group.length) groups.push(group);

  const cues: CaptionCue[] = [];
  groups.forEach((g, i) => {
    const start = Math.max(0, g[0].start);
    if (start >= duration) return;
    const next = groups[i + 1]?.[0].start ?? Infinity;
    let end = g[g.length - 1].end + LINGER;
    if (next - end < BRIDGE) end = next;
    end = Math.min(end, next, duration);
    end = Math.max(end, Math.min(start + MIN_LENGTH, next, duration));
    cues.push({ text: g.map(w => w.word).join(' '), start, end });
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

export interface CaptionOptions {
  style: CaptionStyle;
  fontFamily: string;
  place: CaptionPlace;
}

// Captions stay within this share of the frame's width, which on a reel
// keeps them clear of Instagram's icon column.
const MAX_WIDTH = 0.74;

// A caption's text layer: its words in the style, broken into two lines
// when one would be too wide and shrunk if even that is, centred across the
// frame at the place, showing for the cue's time.
export function captionLayer(cue: CaptionCue, o: CaptionOptions, frame: CropBox, id: string): TextLayer {
  let text = o.style.upper ? cue.text.toLocaleUpperCase() : cue.text;
  const base: TextLayer = {
    id, kind: 'text', caption: true, text,
    fontFamily: o.fontFamily,
    fontSize: Math.round(frame.width * 0.075),
    fill: o.style.fill, bold: o.style.bold, outline: o.style.outline, shadow: o.style.shadow,
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

// The text broken at the space nearest its middle.
function twoLines(text: string): string {
  const mid = text.length / 2;
  let best = -1;
  for (let i = 0; i < text.length; i++) {
    if (text[i] === ' ' && (best < 0 || Math.abs(i - mid) < Math.abs(best - mid))) best = i;
  }
  return best < 0 ? text : `${text.slice(0, best)}\n${text.slice(best + 1)}`;
}

export const isCaption = (l: EditorLayer): l is TextLayer => l.kind === 'text' && !!l.caption;

// A caption layer's cue: its text on one line again (a style may have
// upper-cased it, which a restyle keeps) and its time.
export function cueOf(l: TextLayer, duration: number): CaptionCue {
  return { text: l.text.replace(/\s*\n\s*/g, ' '), start: l.anim?.start ?? 0, end: l.anim?.end ?? duration };
}
