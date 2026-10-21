import { MAX_DURATION, NO_ANIM, TRANSITIONS, type TransitionKind } from './animation';
import type { ClipTransition, EditorLayer, ImageLayer } from './editorModel';

// The clip sequence: the clips that play one after the other, like the cuts
// of a film. They are ordinary clip layers marked seq, in the order they are
// stacked (a later one lies on top, as an incoming clip must during a
// transition), and their spans are laid out from their lengths: each starts
// where the one before it ends, less the transition between them, during
// which both show.

export type SeqClip = ImageLayer & { clip: NonNullable<ImageLayer['clip']> };

// The shortest a clip may be trimmed to.
export const MIN_CLIP = 0.5;
export const DEFAULT_TRANSITION: ClipTransition = { kind: 'cut', duration: 0.5 };
export const MIN_TRANSITION = 0.2;
export const MAX_TRANSITION = 2;
// A clip of unknown length is given this many seconds.
const UNKNOWN_LENGTH = 5;

export const isSeqClip = (l: EditorLayer): l is SeqClip => l.kind === 'image' && !!l.clip?.seq;

export function seqClips(layers: EditorLayer[]): SeqClip[] {
  return layers.filter(isSeqClip);
}

// How long a sequence clip plays: its span, or for one just joined the rest
// of its video.
export function clipLength(l: SeqClip): number {
  const a = l.anim;
  if (a?.end !== undefined) return Math.max(MIN_CLIP, a.end - a.start);
  const left = l.clip.duration > 0 ? l.clip.duration - l.clip.trimStart : UNKNOWN_LENGTH;
  return Math.min(MAX_DURATION, Math.max(MIN_CLIP, left));
}

// How long the transition between two clips lasts: as chosen, but at most
// half of either clip, and none for a cut.
export function transitionLength(t: ClipTransition | undefined, before: number, after: number): number {
  if (!t || t.kind === 'cut') return 0;
  return Math.max(0, Math.min(t.duration, before / 2, after / 2));
}

const sameTransition = (a: ClipTransition | undefined, b: ClipTransition | undefined) =>
  a === b || (!!a && !!b && a.kind === b.kind && Math.abs(a.duration - b.duration) < 1e-9);

// The layers with the sequence laid out from 0 on. The same array when
// nothing changes, so it can run on every change of the layers.
export function layoutSequence(layers: EditorLayer[]): EditorLayer[] {
  const clips = seqClips(layers);
  const placed = new Map<string, SeqClip>();
  let cursor = 0;
  let prevLength = 0;
  clips.forEach((l, i) => {
    const length = clipLength(l);
    const next = clips[i + 1];
    const inLen = i > 0 ? transitionLength(l.clip.transition, prevLength, length) : 0;
    const outLen = next ? transitionLength(next.clip.transition, length, clipLength(next)) : 0;
    const start = Math.max(0, cursor - inLen);
    const end = start + length;
    const into = inLen > 0 && l.clip.transition ? { kind: l.clip.transition.kind, duration: inLen } : undefined;
    const out = outLen > 0 && next?.clip.transition ? { kind: next.clip.transition.kind, duration: outLen } : undefined;
    const a = l.anim ?? NO_ANIM;
    if (Math.abs(a.start - start) > 1e-9 || a.end === undefined || Math.abs(a.end - end) > 1e-9 || !sameTransition(l.clip.into, into) || !sameTransition(l.clip.out, out)) {
      const clip = { ...l.clip, into, out };
      if (!into) delete clip.into;
      if (!out) delete clip.out;
      placed.set(l.id, { ...l, clip, anim: { ...a, start, end } });
    }
    cursor = end;
    prevLength = length;
  });
  // A clip taken out of the sequence keeps its span but loses the
  // transitions it had there.
  const stale = layers.some(l => l.kind === 'image' && l.clip && !l.clip.seq && (l.clip.into || l.clip.out));
  if (placed.size === 0 && !stale) return layers;
  return layers.map(l => {
    const p = placed.get(l.id);
    if (p) return p;
    if (l.kind === 'image' && l.clip && !l.clip.seq && (l.clip.into || l.clip.out)) {
      const clip = { ...l.clip };
      delete clip.into;
      delete clip.out;
      return { ...l, clip };
    }
    return l;
  });
}

// Where the sequence ends, 0 without one.
export function sequenceEnd(layers: EditorLayer[]): number {
  const clips = seqClips(layers);
  const last = clips[clips.length - 1];
  return last?.anim?.end ?? 0;
}

// The layers with sequence clip id moved to place index of the sequence. The
// clips swap their places in the stack, so everything else stays where it is.
export function moveInSequence(layers: EditorLayer[], id: string, index: number): EditorLayer[] {
  const clips = seqClips(layers);
  const from = clips.findIndex(c => c.id === id);
  if (from < 0) return layers;
  const to = Math.max(0, Math.min(clips.length - 1, index));
  if (to === from) return layers;
  const order = [...clips];
  const [moved] = order.splice(from, 1);
  order.splice(to, 0, moved);
  let i = 0;
  return layoutSequence(layers.map(l => (isSeqClip(l) ? order[i++] : l)));
}

// The layers with sequence clip id trimmed to play length seconds from
// trimStart into its video, the clips after it moving up or back.
export function trimClip(layers: EditorLayer[], id: string, trimStart: number, length: number): EditorLayer[] {
  return layoutSequence(layers.map(l => {
    if (l.id !== id || !isSeqClip(l)) return l;
    const max = l.clip.duration > 0 ? l.clip.duration : Infinity;
    const from = Math.max(0, Math.min(trimStart, max - MIN_CLIP));
    const len = Math.max(MIN_CLIP, Math.min(length, max - from, MAX_DURATION));
    const a = l.anim ?? NO_ANIM;
    return { ...l, clip: { ...l.clip, trimStart: from }, anim: { ...a, end: a.start + len } };
  }));
}

// The layers with the transition into sequence clip id changed.
export function setTransition(layers: EditorLayer[], id: string, patch: Partial<ClipTransition>): EditorLayer[] {
  return layoutSequence(layers.map(l => {
    if (l.id !== id || !isSeqClip(l)) return l;
    return { ...l, clip: { ...l.clip, transition: { ...DEFAULT_TRANSITION, ...l.clip.transition, ...patch } } };
  }));
}

// The layers with clip id joined to the sequence (at its end) or taken out
// of it, keeping the span it has at the moment.
export function setInSequence(layers: EditorLayer[], id: string, inSeq: boolean): EditorLayer[] {
  const target = layers.find(l => l.id === id);
  if (!target || target.kind !== 'image' || !target.clip || !!target.clip.seq === inSeq) return layers;
  if (!inSeq) {
    return layoutSequence(layers.map(l => {
      if (l.id !== id || l.kind !== 'image' || !l.clip) return l;
      const clip = { ...l.clip };
      delete clip.seq;
      return { ...l, clip };
    }));
  }
  // Joined: right above the last clip of the sequence, which makes it the
  // last one, playing from its start point for its own length.
  const joined: SeqClip = { ...target, clip: { ...target.clip, seq: true }, anim: { ...(target.anim ?? NO_ANIM), end: undefined } };
  return layoutSequence(insertIntoSequence(layers.filter(l => l.id !== id), [joined]));
}

// The layers with new sequence clips added after the last one, in the stack
// right above it (or on top when there is none yet).
export function insertIntoSequence(layers: EditorLayer[], added: EditorLayer[]): EditorLayer[] {
  let last = -1;
  layers.forEach((l, i) => { if (isSeqClip(l)) last = i; });
  const at = last < 0 ? layers.length : last + 1;
  return layoutSequence([...layers.slice(0, at), ...added, ...layers.slice(at)]);
}

// The label of a transition, for the timeline's marks.
export function transitionLabel(kind: TransitionKind | undefined): string {
  return TRANSITIONS.find(t => t.id === kind)?.label ?? 'Cut';
}
