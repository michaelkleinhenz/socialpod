// Smart guides for moving a layer: snapping to the page (the crop) and to the
// other layers' edges and centres, and to equal spacing. Everything is in
// source-image pixels, like the rest of the editor's model; boxes are the
// axis-aligned bounds a layer covers, rotation included.

export interface Box {
  x: number;
  y: number;
  width: number;
  height: number;
}

// A line two things align on: vertical ('v') at x = pos, or horizontal ('h')
// at y = pos, drawn from `from` to `to` along it.
export interface Guide {
  orientation: 'v' | 'h';
  pos: number;
  from: number;
  to: number;
}

// A measured gap: along x ('x') or y ('y') from `from` to `to`, drawn at the
// cross position `at`. Shown when the dragged layer's spacing matches another.
export interface GapMark {
  axis: 'x' | 'y';
  from: number;
  to: number;
  at: number;
}

export interface SnapResult {
  dx: number;
  dy: number;
  guides: Guide[];
  gaps: GapMark[];
}

// Values closer than this count as aligned when the guides are drawn.
const EPS = 0.5;

// One axis of a box, so x and y share the code.
interface Span { start: number; end: number; cStart: number; cEnd: number }

const span = (b: Box, axis: 'x' | 'y'): Span => (axis === 'x'
  ? { start: b.x, end: b.x + b.width, cStart: b.y, cEnd: b.y + b.height }
  : { start: b.y, end: b.y + b.height, cStart: b.x, cEnd: b.x + b.width });

const lines = (s: Span) => [s.start, (s.start + s.end) / 2, s.end];

const crossOverlap = (a: Span, b: Span) => Math.min(a.cEnd, b.cEnd) - Math.max(a.cStart, b.cStart) > 0;

// The page's edges, as zero-thickness neighbours, so margins count as gaps.
const pageEdges = (page: Span): Span[] => [
  { start: page.start, end: page.start, cStart: page.cStart, cEnd: page.cEnd },
  { start: page.end, end: page.end, cStart: page.cStart, cEnd: page.cEnd },
];

// Neighbouring pairs among `items` along the axis: each item and the nearest
// one after it that shares some of its cross range. Their gap is what the
// dragged layer may match.
function neighbourGaps(items: Span[]): { a: Span; b: Span; gap: number }[] {
  const out: { a: Span; b: Span; gap: number }[] = [];
  for (const a of items) {
    let best: Span | null = null;
    for (const b of items) {
      if (b === a || b.start < a.end || !crossOverlap(a, b)) continue;
      if (!best || b.start < best.start) best = b;
    }
    if (best && best.start - a.end > EPS) out.push({ a, b: best, gap: best.start - a.end });
  }
  return out;
}

// The nearest neighbours before and after `d` that share its cross range.
function around(d: Span, items: Span[]): { before: Span | null; after: Span | null } {
  let before: Span | null = null;
  let after: Span | null = null;
  for (const o of items) {
    if (!crossOverlap(d, o)) continue;
    if (o.end <= d.start + EPS && (!before || o.end > before.end)) before = o;
    if (o.start >= d.end - EPS && (!after || o.start < after.start)) after = o;
  }
  return { before, after };
}

function snapAxis(d: Span, page: Span, others: Span[], threshold: number): number {
  let best = Infinity;
  const consider = (delta: number) => { if (Math.abs(delta) <= threshold && Math.abs(delta) < Math.abs(best)) best = delta; };

  // Edges and centres: the page's and every other layer's.
  for (const t of [page, ...others]) {
    for (const tl of lines(t)) for (const dl of lines(d)) consider(tl - dl);
  }

  // Equal spacing: halfway between the neighbours, or the same gap as a
  // neighbouring pair elsewhere.
  const items = [...others, ...pageEdges(page)];
  const { before, after } = around(d, items);
  if (before && after) consider((before.end + after.start - (d.end - d.start)) / 2 - d.start);
  for (const { a, b, gap } of neighbourGaps(items)) {
    if (before && a !== before) consider(before.end + gap - d.start);
    if (after && b !== after) consider(after.start - gap - d.end);
  }
  return Number.isFinite(best) ? best : 0;
}

// The guides and gap marks a (snapped) box shows along one axis.
function marksAxis(axis: 'x' | 'y', d: Span, page: Span, others: Span[]): { guides: Guide[]; gaps: GapMark[] } {
  const orientation = axis === 'x' ? 'v' : 'h';
  const guides = new Map<number, Guide>();
  const addGuide = (pos: number, from: number, to: number) => {
    const key = Math.round(pos * 2);
    const g = guides.get(key);
    if (g) { g.from = Math.min(g.from, from); g.to = Math.max(g.to, to); } else guides.set(key, { orientation, pos, from, to });
  };
  for (const dl of lines(d)) {
    for (const pl of lines(page)) if (Math.abs(pl - dl) < EPS) addGuide(pl, page.cStart, page.cEnd);
    for (const o of others) {
      for (const ol of lines(o)) {
        if (Math.abs(ol - dl) < EPS) addGuide(ol, Math.min(d.cStart, o.cStart), Math.max(d.cEnd, o.cEnd));
      }
    }
  }

  const gaps: GapMark[] = [];
  const seen = new Set<string>();
  const addGap = (a: Span, b: Span) => {
    const at = (Math.max(a.cStart, b.cStart) + Math.min(a.cEnd, b.cEnd)) / 2;
    const key = `${Math.round(a.end)}:${Math.round(b.start)}:${Math.round(at)}`;
    if (seen.has(key)) return;
    seen.add(key);
    gaps.push({ axis, from: a.end, to: b.start, at });
  };
  const items = [...others, ...pageEdges(page)];
  const { before, after } = around(d, items);
  const gapBefore = before ? d.start - before.end : NaN;
  const gapAfter = after ? after.start - d.end : NaN;
  if (before && after && gapBefore > EPS && Math.abs(gapBefore - gapAfter) < EPS) {
    addGap(before, d);
    addGap(d, after);
  }
  for (const { a, b, gap } of neighbourGaps(items)) {
    // A pair starting where the dragged gap starts (or ending where it ends)
    // runs beside it rather than along the row, and only adds noise.
    if (before && a !== before && gapBefore > EPS && Math.abs(gap - gapBefore) < EPS) { addGap(before, d); addGap(a, b); }
    if (after && b !== after && gapAfter > EPS && Math.abs(gap - gapAfter) < EPS) { addGap(d, after); addGap(a, b); }
  }
  return { guides: [...guides.values()], gaps };
}

// Snaps `box` (the dragged layer) within `threshold` to the page and the
// other layers, and returns the shift to apply plus what to draw.
export function snapBox(box: Box, page: Box, others: Box[], threshold: number): SnapResult {
  const dx = snapAxis(span(box, 'x'), span(page, 'x'), others.map(o => span(o, 'x')), threshold);
  const dy = snapAxis(span(box, 'y'), span(page, 'y'), others.map(o => span(o, 'y')), threshold);
  const moved = { ...box, x: box.x + dx, y: box.y + dy };
  const mx = marksAxis('x', span(moved, 'x'), span(page, 'x'), others.map(o => span(o, 'x')));
  const my = marksAxis('y', span(moved, 'y'), span(page, 'y'), others.map(o => span(o, 'y')));
  return { dx, dy, guides: [...mx.guides, ...my.guides], gaps: [...mx.gaps, ...my.gaps] };
}
