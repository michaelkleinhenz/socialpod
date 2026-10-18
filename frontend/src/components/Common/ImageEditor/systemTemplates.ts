import Konva from 'konva';
import { animStateAt, buildAnimatedLayerNode, equalizerLevels, isEqualizer, NO_ANIM, type EnterEffect, type ExitEffect, type LayerAnim, type LoopEffect } from './animation';
import {
  BUILTIN_FONTS, instantiateTemplate, measureText, type BlendMode, type CropBox, type EditorLayer, type EditorTemplate, type ImageBorder, type ImageLayer, type PaintLayer,
  type ShapeKind, type ShapeLayer, type TemplateLayer, type TextAlign, type TextLayer,
} from './editorModel';

// The templates SocialPod ships with, beside the team's own: generic designs
// for posts, stories and reels, in plain colours and the built-in fonts, which
// the user changes once a template is on the canvas. Each design is a function
// of the format, so it comes in 1:1, 4:5 and 9:16 (kept inside the area
// Instagram's interface leaves free), and in a video with its layers coming
// in one after the other. Reel designs (REELS) are videos only, in 9:16: their
// layers have their own timing, scene after scene. Pictures are placeholders
// (public/template-assets), which a paste or Replace takes the place of.

export type SystemFormat = '1:1' | '4:5' | '9:16';

export const SYSTEM_FORMATS: { id: SystemFormat; label: string; width: number; height: number }[] = [
  { id: '1:1', label: '1:1', width: 1080, height: 1080 },
  { id: '4:5', label: '4:5', width: 1080, height: 1350 },
  { id: '9:16', label: '9:16', width: 1080, height: 1920 },
];

export type SystemCategory = 'news' | 'review' | 'episode' | 'list' | 'quote' | 'question' | 'giveaway' | 'event' | 'promo' | 'photo';

export const SYSTEM_CATEGORIES: { id: SystemCategory; label: string }[] = [
  { id: 'news', label: 'News' },
  { id: 'review', label: 'Review' },
  { id: 'episode', label: 'Episode' },
  { id: 'list', label: 'List' },
  { id: 'quote', label: 'Quote' },
  { id: 'question', label: 'Question' },
  { id: 'giveaway', label: 'Giveaway' },
  { id: 'event', label: 'Event' },
  { id: 'promo', label: 'Promo' },
  { id: 'photo', label: 'Photo' },
];

export interface SystemTemplate {
  id: string;
  name: string;
  category: SystemCategory;
  format: SystemFormat;
  // Laid over a full-frame picture (its bottom layer). On a photo being
  // edited the photo takes that picture's place; the others need a canvas.
  overPhoto: boolean;
  // A reel design, offered in a video only.
  videoOnly: boolean;
  // How long its video runs, and the moment its preview shows.
  duration: number;
  poster: number;
  // The design; video adds the layers' timing and a length.
  build(opts: { video: boolean; photo: boolean }): EditorTemplate;
}

// Where the placeholder pictures are served: by the frontend itself, which
// may not be the API's origin in development.
export const TEMPLATE_ASSETS = '/template-assets/';

// How long a video version runs.
const VIDEO_LENGTH = 8;

const font = (label: string) => BUILTIN_FONTS.find(f => f.label === label)!.family;
const SANS = font('Arial');
const SERIF = font('Georgia');
const TREBUCHET = font('Trebuchet MS');
const MONO = font('Courier New');

// The placeholder pictures and their pixel sizes.
const PHOTOS = {
  warm: `${TEMPLATE_ASSETS}photo-warm.jpg`,
  cool: `${TEMPLATE_ASSETS}photo-cool.jpg`,
  dark: `${TEMPLATE_ASSETS}photo-dark.jpg`,
  green: `${TEMPLATE_ASSETS}photo-green.jpg`,
};
type PhotoKey = keyof typeof PHOTOS;
const PHOTO_W = 1500;
const PHOTO_H = 2000;

// --- Building blocks --------------------------------------------------------

interface Box { x: number; y: number; w: number; h: number }

interface Item {
  layer: TemplateLayer;
  box: Box;
  // The backdrop: drawn first and never animated.
  bg?: boolean;
  // A video's timing for this layer, on top of the default cascade; null
  // keeps it still.
  anim?: Partial<LayerAnim> | null;
}

// The canvas and the part of it that content keeps inside: margins, and on
// 9:16 the header, caption and icon column Instagram lays over a reel or a
// story.
interface Frame {
  w: number;
  h: number;
  fmt: SystemFormat;
  x0: number;
  x1: number;
  y0: number;
  y1: number;
  cx: number;
}

function frameOf(fmt: SystemFormat): Frame {
  const { width: w, height: h } = SYSTEM_FORMATS.find(f => f.id === fmt)!;
  if (fmt === '9:16') return { w, h, fmt, x0: 140, x1: w - 140, y0: 280, y1: 1470, cx: w / 2 };
  return { w, h, fmt, x0: 80, x1: w - 80, y0: 80, y1: h - 80, cx: w / 2 };
}

// The part of a placeholder picture that fills a box of the given aspect.
function coverCrop(w: number, h: number): CropBox {
  const r = w / h;
  if (r > PHOTO_W / PHOTO_H) {
    const ch = PHOTO_W / r;
    return { x: 0, y: (PHOTO_H - ch) / 2, width: PHOTO_W, height: ch };
  }
  const cw = PHOTO_H * r;
  return { x: (PHOTO_W - cw) / 2, y: 0, width: cw, height: PHOTO_H };
}

interface PhotoOpts { corner?: number; border?: ImageBorder; rotation?: number; bg?: boolean; anim?: Partial<LayerAnim> | null }

// A placeholder picture filling the box (top-left x, y).
function photo(p: PhotoKey, x: number, y: number, w: number, h: number, o: PhotoOpts = {}): Item {
  const layer: Omit<ImageLayer, 'id' | 'img'> = {
    kind: 'image', origin: 'media', src: PHOTOS[p],
    x: x + w / 2, y: y + h / 2, width: w, height: h, rotation: o.rotation ?? 0, opacity: 1,
    srcCrop: coverCrop(w, h),
    ...(o.corner ? { cornerRadius: o.corner } : {}),
    ...(o.border ? { border: o.border } : {}),
  };
  return { layer, box: { x, y, w, h }, bg: o.bg, anim: o.anim };
}

interface ShapeOpts { corner?: number; stroke?: string; strokeWidth?: number; opacity?: number; rotation?: number; blend?: BlendMode; bg?: boolean; anim?: Partial<LayerAnim> | null }

// A box, ellipse or triangle filling the box (top-left x, y); fill '' for none.
function shape(kind: ShapeKind, x: number, y: number, w: number, h: number, fill: string, o: ShapeOpts = {}): Item {
  const layer: Omit<ShapeLayer, 'id'> = {
    kind: 'shape', shape: kind, x: x + w / 2, y: y + h / 2, width: w, height: h, rotation: o.rotation ?? 0, opacity: o.opacity ?? 1,
    fill, stroke: o.stroke ?? '#000000', strokeWidth: o.strokeWidth ?? 0,
    ...((kind === 'rect' || kind === 'equalizer') && o.corner ? { cornerRadius: o.corner } : {}),
    ...(o.blend ? { blend: o.blend } : {}),
  };
  return { layer, box: { x, y, w, h }, bg: o.bg, anim: o.anim };
}

const rect = (x: number, y: number, w: number, h: number, fill: string, o?: ShapeOpts) => shape('rect', x, y, w, h, fill, o);
// A circle or ellipse by its centre.
const circle = (cx: number, cy: number, d: number, fill: string, o?: ShapeOpts & { h?: number }) =>
  shape('ellipse', cx - d / 2, cy - (o?.h ?? d) / 2, d, o?.h ?? d, fill, o);

interface TextOpts {
  x: number;
  y: number;
  font?: string;
  size: number;
  color: string;
  bold?: boolean;
  // How the lines line up, and which point of the text x is: its left
  // edge, centre or right edge.
  align?: TextAlign;
  // Which point of the text y is.
  valign?: 'top' | 'middle' | 'bottom';
  // The text shrinks to fit this width (fonts differ between devices).
  maxWidth?: number;
  rotation?: number;
  outline?: boolean;
  shadow?: boolean;
  anim?: Partial<LayerAnim> | null;
}

function text(s: string, o: TextOpts): Item {
  const align = o.align ?? 'center';
  const base: TextLayer = {
    id: '', kind: 'text', text: s, fontFamily: o.font ?? SANS, fontSize: o.size, fill: o.color, bold: o.bold ?? false,
    outline: o.outline ?? false, shadow: o.shadow ?? false, align, x: 0, y: 0, rotation: o.rotation ?? 0,
  };
  let m = measureText(base);
  if (o.maxWidth && m.width > o.maxWidth) {
    base.fontSize = Math.floor(o.size * (o.maxWidth / m.width));
    m = measureText(base);
  }
  const left = align === 'left' ? o.x : align === 'right' ? o.x - m.width : o.x - m.width / 2;
  const top = o.valign === 'bottom' ? o.y - m.height : o.valign === 'middle' ? o.y - m.height / 2 : o.y;
  let x = left;
  let y = top;
  if (base.rotation) {
    // Konva turns a text about its top-left corner: place that corner so the
    // centre stays where it would be unturned.
    const rad = (base.rotation * Math.PI) / 180;
    const cx = left + m.width / 2;
    const cy = top + m.height / 2;
    x = cx - ((m.width / 2) * Math.cos(rad) - (m.height / 2) * Math.sin(rad));
    y = cy - ((m.width / 2) * Math.sin(rad) + (m.height / 2) * Math.cos(rad));
  }
  const { id: _id, ...layer } = { ...base, x, y };
  void _id;
  return { layer, box: { x: left, y: top, w: m.width, h: m.height }, anim: o.anim };
}

// A soft band of colour across the canvas around y, size high, fading out
// over feather: the shade behind text on a photo.
function shade(f: Frame, y: number, size: number, feather: number, color: string, opacity: number): Item {
  const layer: Omit<PaintLayer, 'id'> = {
    kind: 'paint', x: 0, y: 0,
    strokes: [{ points: [-feather, y, f.w / 2, y, f.w + feather, y], size, feather, color, opacity }],
  };
  return { layer, box: { x: 0, y: y - size / 2, w: f.w, h: size }, anim: { enter: 'fade', enterDuration: 0.6 } };
}

// A reel layer's timing: it shows from start until end (the end of the
// video when absent), coming in and going out with the effects named.
function at(start: number, end: number | undefined, enter: EnterEffect, exit: ExitEffect = 'none', o: { in?: number; out?: number; loop?: LoopEffect } = {}): Partial<LayerAnim> {
  return {
    start, ...(end !== undefined ? { end } : {}), enter, exit,
    enterDuration: o.in ?? 0.5, exitDuration: o.out ?? 0.35, loop: o.loop ?? 'none',
  };
}

// Gives items the timing of a scene, on top of what they set themselves.
function timed(items: Item[], anim: Partial<LayerAnim>): Item[] {
  for (const it of items) it.anim = { ...anim, ...it.anim };
  return items;
}

// Moves items (and their boxes) by dx, dy.
function move(items: Item[], dx: number, dy: number): Item[] {
  for (const it of items) {
    const l = it.layer as { x: number; y: number; kind: string; strokes?: { points: number[] }[] };
    if (l.kind === 'paint') {
      for (const st of l.strokes!) st.points = st.points.map((v, i) => v + (i % 2 === 0 ? dx : dy));
    } else {
      l.x += dx;
      l.y += dy;
    }
    it.box = { ...it.box, x: it.box.x + dx, y: it.box.y + dy };
  }
  return items;
}

// The box around items.
function bounds(items: Item[]): Box {
  const x = Math.min(...items.map(i => i.box.x));
  const y = Math.min(...items.map(i => i.box.y));
  const x1 = Math.max(...items.map(i => i.box.x + i.box.w));
  const y1 = Math.max(...items.map(i => i.box.y + i.box.h));
  return { x, y, w: x1 - x, h: y1 - y };
}

// Items centred vertically between top and bottom.
function centreY(items: Item[], top: number, bottom: number): Item[] {
  const b = bounds(items);
  return move(items, 0, top + (bottom - top - b.h) / 2 - b.y);
}

// A pill with a centred label: the label's box sets the pill's size.
function pill(label: string, x: number, y: number, o: { fill: string; color: string; size?: number; align?: 'left' | 'center'; font?: string; stroke?: string }): Item[] {
  const size = o.size ?? 30;
  const t = text(label, { x: 0, y: 0, size, color: o.color, bold: true, font: o.font, align: 'left' });
  const padX = size * 1.1;
  const padY = size * 0.55;
  const w = t.box.w + padX * 2;
  const h = t.box.h + padY * 2;
  const left = o.align === 'center' ? x - w / 2 : x;
  const bgItem = rect(left, y, w, h, o.fill, { corner: 50, ...(o.stroke ? { stroke: o.stroke, strokeWidth: 4 } : {}) });
  return [bgItem, ...move([t], left + padX, y + padY)];
}

// --- Designs ----------------------------------------------------------------

interface Design {
  key: string;
  name: string;
  category: SystemCategory;
  background: string;
  overPhoto?: boolean;
  // A reel: its length, the moment its preview shows, and the formats it
  // comes in (9:16 when absent). Its layers carry their own timing.
  reel?: { duration: number; poster: number; formats?: SystemFormat[] };
  draw(f: Frame): Item[];
}

const tall = (f: Frame) => f.fmt === '9:16';
const square = (f: Frame) => f.fmt === '1:1';

const DESIGNS: Design[] = [
  {
    key: 'headline', name: 'Headline', category: 'news', background: '#1a1a1a', overPhoto: true,
    draw: f => {
      const items: Item[] = [photo('warm', 0, 0, f.w, f.h, { bg: true })];
      items.push(shade(f, f.h, f.h * (tall(f) ? 0.75 : 0.9), f.h * 0.35, '#000000', 0.85));
      const sub = text('A short line with the key detail', { x: f.x0, y: f.y1, valign: 'bottom', align: 'left', size: 40, color: '#e8e8e8', maxWidth: f.x1 - f.x0 });
      const head = text('Your headline\ngoes right here', { x: f.x0, y: sub.box.y - 28, valign: 'bottom', align: 'left', size: square(f) ? 100 : 118, bold: true, color: '#ffffff', maxWidth: f.x1 - f.x0 });
      const tag = pill('NEWS', f.x0, 0, { fill: '#ffd23f', color: '#111111', size: 30 });
      move(tag, 0, head.box.y - 36 - bounds(tag).h);
      items.push(...tag, head, sub);
      return items;
    },
  },
  {
    key: 'breaking', name: 'Breaking', category: 'news', background: '#111111',
    draw: f => {
      const ph = Math.round(f.h * (square(f) ? 0.5 : tall(f) ? 0.58 : 0.56));
      const items: Item[] = [photo('cool', 0, 0, f.w, ph)];
      const label = text('BREAKING NEWS', { x: 0, y: 0, align: 'left', size: 46, bold: true, color: '#ffffff' });
      const band = rect(f.x0, ph - (label.box.h + 40) / 2, label.box.w + 64, label.box.h + 40, '#e63946');
      move([label], f.x0 + 32, ph - label.box.h / 2);
      const head = text('Big announcement\nin two short lines', { x: f.x0, y: ph + band.box.h / 2 + 50, align: 'left', size: square(f) ? 80 : 98, bold: true, color: '#ffffff', maxWidth: f.x1 - f.x0 });
      const sub = text('Add the one detail that matters most.', { x: f.x0, y: head.box.y + head.box.h + 24, align: 'left', size: 40, color: '#b8b8b8', maxWidth: f.x1 - f.x0 });
      const handle = text('@yourhandle', { x: f.x0, y: f.y1, valign: 'bottom', align: 'left', size: 32, bold: true, color: '#e63946' });
      items.push(band, label, head, sub);
      if (sub.box.y + sub.box.h + 60 < handle.box.y) items.push(handle);
      return items;
    },
  },
  {
    key: 'spotlight', name: 'Spotlight', category: 'review', background: '#f4efe6',
    draw: f => {
      const cw = Math.round((f.x1 - f.x0) * (square(f) ? 0.62 : tall(f) ? 0.96 : 0.9));
      const ch = Math.round(cw * (tall(f) ? 1.0 : 0.75));
      const kicker = text('GAME SPOTLIGHT', { x: f.cx, y: 0, size: 32, bold: true, color: '#b5651d' });
      const card = photo('warm', f.cx - cw / 2, kicker.box.h + 44, cw, ch, { corner: 3, border: { style: 'offset', width: 16, color: '#1d1d1d' } });
      const title = text('Game Title', { x: f.cx, y: card.box.y + ch + 56, font: SERIF, size: square(f) ? 92 : 104, bold: true, color: '#1d1d1d', maxWidth: f.x1 - f.x0 });
      const stars = text('★★★★☆', { x: f.cx, y: title.box.y + title.box.h + 18, size: 60, color: '#e9a23b' });
      const info = text('2 to 4 players  |  45 min  |  age 10+', { x: f.cx, y: stars.box.y + stars.box.h + 22, size: 36, color: '#5a5a5a', maxWidth: f.x1 - f.x0 });
      return centreY([kicker, card, title, stars, info], f.y0, f.y1);
    },
  },
  {
    key: 'score', name: 'Review Score', category: 'review', background: '#1b1b2f',
    draw: f => {
      const ph = Math.round(f.h * (tall(f) ? 0.54 : square(f) ? 0.46 : 0.56));
      const d = square(f) ? 200 : 230;
      const items: Item[] = [photo('dark', 0, 0, f.w, ph)];
      const cx = f.x1 - d / 2;
      items.push(circle(cx, ph, d, '#ffb703', { stroke: '#1b1b2f', strokeWidth: 12 }));
      const score = text('8.5', { x: cx, y: ph - 8, valign: 'middle', size: d * 0.4, bold: true, color: '#1b1b2f' });
      const of = text('OUT OF 10', { x: cx, y: score.box.y + score.box.h - 2, size: d * 0.09, bold: true, color: '#1b1b2f' });
      const kicker = text('REVIEW', { x: f.x0, y: ph + 50, align: 'left', size: 34, bold: true, color: '#ffb703' });
      const title = text('Game Title', { x: f.x0, y: kicker.box.y + kicker.box.h + 16, align: 'left', size: square(f) ? 84 : 104, bold: true, color: '#ffffff', maxWidth: f.x1 - f.x0 - d * 0.6 });
      const verdict = text('Clever, quick and easy to teach.\nA new favourite on our table.', { x: f.x0, y: title.box.y + title.box.h + 22, align: 'left', size: square(f) ? 34 : 42, color: '#c9c9d9', maxWidth: f.x1 - f.x0 });
      items.push(score, of, kicker, title, verdict);
      return items;
    },
  },
  {
    key: 'episode', name: 'New Episode', category: 'episode', background: '#4c2a85',
    draw: f => {
      const items: Item[] = [
        circle(f.w * 0.92, f.h * 0.08, f.w * 0.62, '#5e3a9e', { bg: true }),
        circle(f.w * 0.05, f.h * 0.95, f.w * 0.5, '#5e3a9e', { bg: true }),
      ];
      const d = Math.round(f.w * (square(f) ? 0.4 : tall(f) ? 0.6 : 0.52));
      const tag = pill('NEW EPISODE', f.cx, 0, { fill: '#ff6f59', color: '#ffffff', size: 30, align: 'center' });
      const cover = photo('cool', f.cx - d / 2, bounds(tag).h + 50, d, d, { corner: 50, border: { style: 'solid', width: 14, color: '#ffffff' } });
      const ep = text('EPISODE 42', { x: f.cx, y: cover.box.y + d + 60, size: 40, bold: true, color: '#ffd166' });
      const title = text('The episode title\nin two lines', { x: f.cx, y: ep.box.y + ep.box.h + 16, size: square(f) ? 70 : 88, bold: true, color: '#ffffff', maxWidth: f.x1 - f.x0 });
      const listen = text('Listen now wherever you get your podcasts', { x: f.cx, y: title.box.y + title.box.h + 26, size: square(f) ? 30 : 36, color: '#e0d4f5', maxWidth: f.x1 - f.x0 });
      items.push(...centreY([...tag, cover, ep, title, listen], f.y0, f.y1));
      return items;
    },
  },
  {
    key: 'top5', name: 'Top 5', category: 'list', background: '#ffd23f',
    draw: f => {
      const items: Item[] = [
        circle(f.w - 60, 60, 300, '', { stroke: '#111111', strokeWidth: 10, bg: true }),
        circle(80, f.h - 80, 220, '', { stroke: '#111111', strokeWidth: 10, bg: true }),
      ];
      const top = text('TOP', { x: f.cx, y: 0, size: square(f) ? 120 : 140, bold: true, color: '#111111' });
      const five = text('5', { x: f.cx, y: top.box.h - 40, size: square(f) ? 420 : tall(f) ? 560 : 520, bold: true, color: '#111111' });
      const sub = text('games for two players', { x: 0, y: 0, align: 'left', size: square(f) ? 50 : 56, bold: true, color: '#ffd23f', maxWidth: f.x1 - f.x0 - 80 });
      const bar = rect(f.cx - sub.box.w / 2 - 36, five.box.y + five.box.h - 20, sub.box.w + 72, sub.box.h + 40, '#111111');
      move([sub], f.cx - sub.box.w / 2, bar.box.y + 20);
      const swipe = text('Swipe to see them all  →', { x: f.cx, y: bar.box.y + bar.box.h + 44, size: 38, bold: true, color: '#111111' });
      items.push(...centreY([top, five, bar, sub, swipe], f.y0, f.y1));
      return items;
    },
  },
  {
    key: 'listitem', name: 'List Item', category: 'list', background: '#ffffff',
    draw: f => {
      const items: Item[] = [rect(0, 0, 24, f.h, '#ffd23f', { bg: true })];
      // On a square the picture takes the right half, else the space below.
      const colW = square(f) ? (f.x1 - f.x0) * 0.5 : f.x1 - f.x0;
      const num = text('01', { x: f.x0, y: f.y0 - 20, align: 'left', size: square(f) ? 200 : 240, bold: true, color: '#ffd23f', outline: true });
      const title = text('Game Title', { x: f.x0, y: num.box.y + num.box.h, align: 'left', size: square(f) ? 72 : 84, bold: true, color: '#111111', maxWidth: colW });
      const desc = text(square(f) ? 'Why it made the list,\nin a sentence or two\nthat sells it.' : 'Why it made the list, in a sentence\nor two that sell it to the reader.', { x: f.x0, y: title.box.y + title.box.h + 18, align: 'left', size: square(f) ? 34 : 38, color: '#555555', maxWidth: colW });
      if (square(f)) {
        items.push(...centreY([num, title, desc], f.y0, f.y1));
        const px = f.x0 + colW + 40;
        items.push(photo('green', px, f.y0, f.x1 - px, f.y1 - f.y0, { corner: 4 }));
      } else {
        items.push(num, title, desc);
        const top = desc.box.y + desc.box.h + 50;
        items.push(photo('green', f.x0, top, f.x1 - f.x0, f.y1 - top, { corner: 4 }));
      }
      return items;
    },
  },
  {
    key: 'quote', name: 'Quote', category: 'quote', background: '#f7f3ee',
    draw: f => {
      const mark = text('“', { x: f.cx, y: 0, font: SERIF, size: square(f) ? 260 : 320, bold: true, color: '#e76f51' });
      const q = text('Put a memorable quote\nfrom the episode or\na review right here.', { x: f.cx, y: mark.box.h * 0.55, font: SERIF, size: square(f) ? 64 : 80, color: '#222222', maxWidth: f.x1 - f.x0 });
      const line = rect(f.cx - 40, q.box.y + q.box.h + 50, 80, 6, '#e76f51');
      const who = text('Name Surname', { x: f.cx, y: line.box.y + 40, size: 38, bold: true, color: '#222222' });
      const role = text('Role or source', { x: f.cx, y: who.box.y + who.box.h + 8, size: 32, color: '#777777' });
      return centreY([mark, q, line, who, role], f.y0, f.y1);
    },
  },
  {
    key: 'thisorthat', name: 'This or That', category: 'question', background: '#2ec4b6',
    draw: f => {
      const title = text('WHICH ONE\nWOULD YOU PICK?', { x: f.cx, y: 0, size: square(f) ? 72 : 84, bold: true, color: '#ffffff', maxWidth: f.x1 - f.x0 });
      const gap = 36;
      const pw = (f.x1 - f.x0 - gap) / 2;
      const ph = pw * (square(f) ? 1.0 : tall(f) ? 1.45 : 1.25);
      const py = title.box.h + 60;
      const a = photo('warm', f.x0, py, pw, ph, { corner: 6, border: { style: 'solid', width: 10, color: '#ffffff' } });
      const b = photo('cool', f.x0 + pw + gap, py, pw, ph, { corner: 6, border: { style: 'solid', width: 10, color: '#ffffff' } });
      const d = 110;
      const badge = (cx: number, l: string) => [
        circle(cx, py + ph, d, '#ff9f1c', { stroke: '#ffffff', strokeWidth: 8 }),
        text(l, { x: cx, y: py + ph, valign: 'middle', size: 56, bold: true, color: '#ffffff' }),
      ];
      const ask = text('Tell us in the comments', { x: f.cx, y: py + ph + d / 2 + 40, size: 40, bold: true, color: '#ffffff' });
      return centreY([title, a, b, ...badge(f.x0 + pw / 2, 'A'), ...badge(f.x0 + pw + gap + pw / 2, 'B'), ask], f.y0, f.y1);
    },
  },
  {
    key: 'giveaway', name: 'Giveaway', category: 'giveaway', background: '#7b2ff7',
    draw: f => {
      const confetti: [ShapeKind, number, number, number, string, number][] = [
        ['rect', 0.08, 0.06, 34, '#ffd23f', 20], ['ellipse', 0.22, 0.03, 26, '#ff6fb5', 0], ['triangle', 0.82, 0.05, 40, '#2ec4b6', 15],
        ['rect', 0.94, 0.16, 30, '#ff6fb5', -25], ['ellipse', 0.05, 0.3, 22, '#2ec4b6', 0], ['triangle', 0.95, 0.5, 34, '#ffd23f', -10],
        ['rect', 0.04, 0.7, 28, '#ffd23f', 35], ['ellipse', 0.93, 0.8, 30, '#ff6fb5', 0], ['triangle', 0.1, 0.93, 38, '#ff6fb5', 30],
        ['rect', 0.85, 0.95, 32, '#2ec4b6', -30], ['ellipse', 0.5, 0.02, 20, '#ffd23f', 0], ['ellipse', 0.6, 0.97, 24, '#2ec4b6', 0],
      ];
      const items: Item[] = confetti.map(([k, x, y, s, c, r]) =>
        shape(k, x * f.w - s / 2, y * f.h - s / 2, s, k === 'rect' ? s * 0.5 : s, c, { rotation: r, anim: { loop: 'float' } }));
      const head = text('GIVEAWAY', { x: f.cx, y: 0, size: square(f) ? 130 : 150, bold: true, color: '#ffffff', shadow: true, rotation: -4, maxWidth: f.x1 - f.x0, anim: { enter: 'pop', loop: 'pulse' } });
      const pw = (f.x1 - f.x0) * (square(f) ? 0.5 : 0.8);
      const ph = pw * (square(f) ? 0.8 : tall(f) ? 1.0 : 0.75);
      const pic = photo('warm', f.cx - pw / 2, head.box.h + 50, pw, ph, { corner: 5, border: { style: 'solid', width: 12, color: '#ffffff' } });
      const how = text('HOW TO ENTER', { x: f.cx, y: pic.box.y + ph + 50, size: 38, bold: true, color: '#ffd23f' });
      const steps = text('1. Follow us\n2. Like this post\n3. Tag a friend', { x: f.cx, y: how.box.y + how.box.h + 16, size: square(f) ? 34 : 44, color: '#ffffff' });
      items.push(...centreY([head, pic, how, steps], f.y0, f.y1));
      return items;
    },
  },
  {
    key: 'comingsoon', name: 'Coming Soon', category: 'promo', background: '#111111', overPhoto: true,
    draw: f => {
      const items: Item[] = [
        photo('dark', 0, 0, f.w, f.h, { bg: true }),
        rect(0, 0, f.w, f.h, '#000000', { opacity: 0.35, bg: true }),
      ];
      const kicker = text('SOMETHING NEW IS', { x: f.cx, y: 0, size: 34, bold: true, color: '#ffd23f' });
      const lineA = rect(f.cx - 60, kicker.box.h + 30, 120, 4, '#ffffff');
      const head = text('COMING\nSOON', { x: f.cx, y: lineA.box.y + 40, size: square(f) ? 140 : 170, bold: true, color: '#ffffff', maxWidth: f.x1 - f.x0 });
      const lineB = rect(f.cx - 60, head.box.y + head.box.h + 30, 120, 4, '#ffffff');
      const date = pill('SPRING 2027', f.cx, lineB.box.y + 50, { fill: '', stroke: '#ffffff', color: '#ffffff', size: 32, align: 'center' });
      items.push(...centreY([kicker, lineA, head, lineB, ...date], f.y0, f.y1));
      items.push(text('@yourhandle', { x: f.cx, y: f.y1, valign: 'bottom', size: 32, color: '#cccccc' }));
      return items;
    },
  },
  {
    key: 'live', name: 'Live From', category: 'event', background: '#111111', overPhoto: true,
    draw: f => {
      const items: Item[] = [photo('cool', 0, 0, f.w, f.h, { bg: true })];
      items.push(shade(f, f.h, f.h * (tall(f) ? 0.75 : 0.85), f.h * 0.35, '#000000', 0.85));
      const label = text('LIVE', { x: 0, y: 0, align: 'left', size: 32, bold: true, color: '#ffffff' });
      const h = label.box.h + 32;
      const w = label.box.w + 96;
      const badge = rect(f.x0, f.y0, w, h, '#e63946', { corner: 50 });
      const dot = circle(f.x0 + 36, f.y0 + h / 2, 18, '#ffffff', { anim: { loop: 'blink' } });
      move([label], f.x0 + 60, f.y0 + 16);
      const place = text('Hall 3  |  Booth B-12', { x: f.x0, y: f.y1, valign: 'bottom', align: 'left', size: 42, color: '#e8e8e8', maxWidth: f.x1 - f.x0 });
      const name = text('CONVENTION\nNAME 2026', { x: f.x0, y: place.box.y - 20, valign: 'bottom', align: 'left', size: square(f) ? 96 : 116, bold: true, color: '#ffffff', maxWidth: f.x1 - f.x0 });
      const from = text('LIVE FROM', { x: f.x0, y: name.box.y - 14, valign: 'bottom', align: 'left', size: 38, bold: true, color: '#ffd23f' });
      items.push(badge, dot, label, from, name, place);
      return items;
    },
  },
  {
    key: 'crowdfunding', name: 'Crowdfunding', category: 'promo', background: '#0b3d2e',
    draw: f => {
      const kicker = pill('CROWDFUNDING ALERT', f.x0, 0, { fill: '#3ddc84', color: '#0b3d2e', size: 28 });
      const top = bounds(kicker).h + 40;
      const pw = f.x1 - f.x0;
      const ph = pw * (square(f) ? 0.42 : tall(f) ? 0.75 : 0.56);
      const pic = photo('green', f.x0, top, pw, ph, { corner: 4 });
      const title = text('Game Title', { x: f.x0, y: top + ph + 40, align: 'left', size: square(f) ? 76 : 88, bold: true, color: '#ffffff', maxWidth: pw });
      const gap = 24;
      const sw = (pw - gap * 2) / 3;
      const sy = title.box.y + title.box.h + 36;
      const stats: Item[] = [];
      const sh = square(f) ? 140 : 160;
      [['250%', 'funded'], ['1,200', 'backers'], ['12', 'days left']].forEach(([v, l], i) => {
        const x = f.x0 + i * (sw + gap);
        stats.push(rect(x, sy, sw, sh, '#145a43', { corner: 10 }));
        const val = text(v, { x: x + sw / 2, y: sy + sh * 0.42, valign: 'middle', size: square(f) ? 52 : 58, bold: true, color: '#3ddc84', maxWidth: sw - 30 });
        stats.push(val, text(l, { x: x + sw / 2, y: val.box.y + val.box.h + 6, size: 28, color: '#cfe9dc' }));
      });
      return centreY([...kicker, pic, title, ...stats], f.y0, f.y1);
    },
  },
  {
    key: 'polaroid', name: 'Snapshot', category: 'photo', background: '#e9e4da',
    draw: f => {
      const pw = (f.x1 - f.x0) * (square(f) ? 0.62 : tall(f) ? 0.92 : 0.82);
      const ph = pw * (tall(f) ? 1.15 : 0.9);
      const m = pw * 0.06;
      const cw = pw + m * 2;
      const ch = ph + m + pw * 0.24;
      const cx = f.cx;
      const cy = (f.y0 + f.y1) / 2;
      const rot = -3;
      const rad = (rot * Math.PI) / 180;
      // A point of the card, relative to its centre, turned with it.
      const at = (dx: number, dy: number) => ({ x: cx + dx * Math.cos(rad) - dy * Math.sin(rad), y: cy + dx * Math.sin(rad) + dy * Math.cos(rad) });
      const shadow = rect(cx - cw / 2 + 14, cy - ch / 2 + 18, cw, ch, '#000000', { opacity: 0.12, rotation: rot });
      const card = rect(cx - cw / 2, cy - ch / 2, cw, ch, '#ffffff', { rotation: rot });
      const pc = at(0, -ch / 2 + m + ph / 2);
      const pic = photo('warm', pc.x - pw / 2, pc.y - ph / 2, pw, ph, { rotation: rot });
      const tc = at(0, ch / 2 - pw * 0.12);
      const caption = text('game night, round two', { x: tc.x, y: tc.y, valign: 'middle', font: MONO, size: square(f) ? 34 : 40, bold: true, color: '#333333', rotation: rot, maxWidth: pw });
      return [shadow, card, pic, caption];
    },
  },
  {
    key: 'gamenight', name: 'Game Night', category: 'event', background: '#14213d',
    draw: f => {
      const items: Item[] = [];
      const bw = 220;
      const bh = 240;
      const block = rect(f.x0, f.y0, bw, bh, '#fca311', { corner: 8 });
      const day = text('24', { x: f.x0 + bw / 2, y: f.y0 + bh * 0.42, valign: 'middle', size: 130, bold: true, color: '#14213d' });
      const month = text('OCTOBER', { x: f.x0 + bw / 2, y: day.box.y + day.box.h, size: 34, bold: true, color: '#14213d', maxWidth: bw - 30 });
      const colW = square(f) ? (f.x1 - f.x0) * 0.5 : f.x1 - f.x0;
      const title = text('GAME\nNIGHT', { x: f.x0, y: f.y0 + bh + 40, align: 'left', size: square(f) ? 120 : 140, bold: true, color: '#ffffff', maxWidth: colW });
      const details = text('Friday, 7 pm\nYour venue name\nEveryone welcome', { x: f.x0, y: title.box.y + title.box.h + 30, align: 'left', size: square(f) ? 38 : 44, color: '#e5e5e5', maxWidth: colW });
      items.push(block, day, month, title, details);
      if (square(f)) {
        items.unshift(photo('dark', f.w * 0.56, 0, f.w * 0.44, f.h, { bg: true }));
      } else {
        const top = details.box.y + details.box.h + 60;
        items.push(photo('dark', f.x0, top, f.x1 - f.x0, f.y1 - top, { corner: 4 }));
      }
      return items;
    },
  },
  {
    key: 'didyouknow', name: 'Did You Know', category: 'quote', background: '#fdf0d5',
    draw: f => {
      const items: Item[] = [
        circle(f.w, f.h, f.w * 0.8, '#f6dfb8', { bg: true }),
        circle(0, 0, f.w * 0.45, '#f6dfb8', { bg: true }),
      ];
      const d = 160;
      const badge = circle(f.cx, d / 2, d, '#c1121f', { anim: { enter: 'pop' } });
      const q = text('?', { x: f.cx, y: d / 2, valign: 'middle', size: 110, bold: true, color: '#ffffff' });
      const head = text('DID YOU KNOW?', { x: f.cx, y: d + 50, size: square(f) ? 72 : 80, bold: true, color: '#003049', maxWidth: f.x1 - f.x0 });
      const body = text('Share a fun fact about\na game, its designer\nor its history here.', { x: f.cx, y: head.box.y + head.box.h + 30, font: SERIF, size: square(f) ? 50 : 60, color: '#003049', maxWidth: f.x1 - f.x0 });
      const handle = text('@yourhandle', { x: f.cx, y: body.box.y + body.box.h + 60, font: TREBUCHET, size: 34, bold: true, color: '#669bbc' });
      items.push(...centreY([badge, q, head, body, handle], f.y0, f.y1));
      return items;
    },
  },
];

// --- Reels ------------------------------------------------------------------

// Confetti bits around the edge of the frame, floating once they are in.
function confetti(f: Frame, start: number, colours: string[]): Item[] {
  const bits: [ShapeKind, number, number, number, number][] = [
    ['rect', 0.08, 0.1, 34, 20], ['ellipse', 0.24, 0.06, 26, 0], ['triangle', 0.84, 0.08, 40, 15], ['rect', 0.93, 0.2, 30, -25],
    ['ellipse', 0.06, 0.34, 22, 0], ['triangle', 0.94, 0.42, 34, -10], ['rect', 0.05, 0.58, 28, 35], ['ellipse', 0.92, 0.62, 30, 0],
    ['triangle', 0.1, 0.74, 38, 30], ['rect', 0.88, 0.74, 32, -30], ['ellipse', 0.55, 0.05, 20, 0], ['rect', 0.4, 0.09, 26, 50],
  ];
  return bits.map(([k, x, y, size, r], i) => shape(k, x * f.w - size / 2, y * f.h - size / 2, size, k === 'rect' ? size * 0.5 : size, colours[i % colours.length],
    { rotation: r, anim: at(start + (i % 4) * 0.08, undefined, 'pop', 'none', { in: 0.4, loop: i % 2 ? 'float' : 'wiggle' }) }));
}

const REELS: Design[] = [
  {
    key: 'reel-kinetic', name: 'Kinetic Intro', category: 'promo', background: '#111111',
    reel: { duration: 8, poster: 5 },
    draw: f => {
      const items: Item[] = [];
      // Three words slam in, each on a colour of its own, then the reveal.
      const words: [string, string, string, LoopEffect][] = [['NEW', '#ff3d68', '#ffffff', 'none'], ['GAME', '#ffd23f', '#111111', 'none'], ['ALERT', '#3d5afe', '#ffffff', 'jitter']];
      words.forEach(([word, bg, colour, loop], i) => {
        const t0 = i * 1.1;
        items.push(rect(0, 0, f.w, f.h, bg, { anim: at(t0, t0 + 1.1, 'none', 'none') }));
        items.push(text(word, { x: f.cx, y: f.h / 2, valign: 'middle', size: 300, bold: true, color: colour, maxWidth: f.x1 - f.x0, anim: at(t0, t0 + 1.1, 'shrink', 'none', { in: 0.3, loop }) }));
      });
      const pw = f.x1 - f.x0;
      const pic = photo('warm', f.x0, 0, pw, pw * 1.05, { corner: 5, anim: at(3.3, undefined, 'zoom', 'none', { in: 0.6 }) });
      const title = text('Game Title', { x: f.cx, y: pic.box.h + 60, size: 140, bold: true, color: '#ffffff', maxWidth: pw, anim: at(3.6, undefined, 'slide-up', 'none', { in: 0.6 }) });
      const sub = text('Out now. Here is why you want it.', { x: f.cx, y: title.box.y + title.box.h + 20, size: 50, color: '#ffd23f', maxWidth: pw, anim: at(3.9, undefined, 'slide-up', 'none', { in: 0.6 }) });
      const cta = timed(pill('LINK IN BIO', f.cx, sub.box.y + sub.box.h + 50, { fill: '#ff3d68', color: '#ffffff', size: 38, align: 'center' }), at(4.4, undefined, 'pop', 'none', { in: 0.5, loop: 'pulse' }));
      items.push(...centreY([pic, title, sub, ...cta], f.y0, f.y1));
      return items;
    },
  },
  {
    key: 'reel-countdown', name: 'Countdown Reveal', category: 'news', background: '#0d0d0d',
    reel: { duration: 8, poster: 5 },
    draw: f => {
      const cy = f.h / 2 - 60;
      const items: Item[] = [
        circle(f.cx, cy, 560, '', { stroke: '#ffd23f', strokeWidth: 14, anim: at(0, 2.4, 'zoom', 'zoom', { in: 0.4, out: 0.3, loop: 'pulse' }) }),
      ];
      ['3', '2', '1'].forEach((n, i) => {
        items.push(text(n, { x: f.cx, y: cy, valign: 'middle', size: 380, bold: true, color: '#ffffff', anim: at(i * 0.8, (i + 1) * 0.8, 'pop', 'zoom', { in: 0.3, out: 0.2 }) }));
      });
      items.push(photo('cool', 0, 0, f.w, f.h, { anim: at(2.4, undefined, 'shrink', 'none', { in: 0.8, loop: 'slow-zoom' }) }));
      items.push(timed([shade(f, f.h, f.h * 0.8, f.h * 0.35, '#000000', 0.9)], at(2.6, undefined, 'fade', 'none', { in: 0.6 }))[0]);
      const sub = text('Add the one detail that matters most.', { x: f.x0, y: f.y1, valign: 'bottom', align: 'left', size: 44, color: '#e8e8e8', maxWidth: f.x1 - f.x0, anim: at(3.5, undefined, 'fade', 'none', { in: 0.6 }) });
      const head = text('The big reveal\ngoes right here', { x: f.x0, y: sub.box.y - 28, valign: 'bottom', align: 'left', size: 110, bold: true, color: '#ffffff', maxWidth: f.x1 - f.x0, anim: at(3.1, undefined, 'slide-up', 'none', { in: 0.6 }) });
      const kicker = text("IT'S HERE", { x: f.x0, y: head.box.y - 24, valign: 'bottom', align: 'left', size: 48, bold: true, color: '#ffd23f', anim: at(2.9, undefined, 'wipe-right', 'none', { in: 0.5 }) });
      items.push(kicker, head, sub);
      return items;
    },
  },
  {
    key: 'reel-top3', name: 'Top 3 Countdown', category: 'list', background: '#101820',
    reel: { duration: 13, poster: 3.4 },
    draw: f => {
      const items: Item[] = [];
      const intro = at(0, 2.2, 'slide-up', 'zoom', { in: 0.5, out: 0.3 });
      const top = text('TOP', { x: f.cx, y: 0, size: 150, bold: true, color: '#fee715', anim: { ...intro, enter: 'slide-down' } });
      const three = text('3', { x: f.cx, y: top.box.h - 60, size: 560, bold: true, color: '#ffffff', anim: { ...intro, enter: 'bungee', enterDuration: 0.9 } });
      const sub = text('games for two players', { x: f.cx, y: three.box.y + three.box.h - 20, size: 64, bold: true, color: '#fee715', maxWidth: f.x1 - f.x0, anim: { ...intro, start: 0.3 } });
      items.push(...centreY([top, three, sub], f.y0, f.y1));
      const pw = f.x1 - f.x0;
      const ph = pw * 1.1;
      (['warm', 'cool', 'green'] as PhotoKey[]).forEach((p, i) => {
        const t0 = 2.2 + i * 3;
        const scene = at(t0, t0 + 3, 'slide-left', 'slide-left', { in: 0.5, out: 0.35 });
        const pic = photo(p, f.x0, 0, pw, ph, { corner: 4, anim: scene });
        const d = 190;
        const badge = circle(f.x0 + 40, 40, d, '#fee715', { stroke: '#101820', strokeWidth: 12, anim: { ...scene, start: t0 + 0.2, enter: 'pop' } });
        const num = text(`#${3 - i}`, { x: f.x0 + 40, y: 40, valign: 'middle', size: 84, bold: true, color: '#101820', anim: { ...scene, start: t0 + 0.2, enter: 'pop' } });
        const title = text('Game Title', { x: f.x0, y: ph + 50, align: 'left', size: 100, bold: true, color: '#ffffff', maxWidth: pw, anim: { ...scene, start: t0 + 0.3, enter: 'slide-up' } });
        const info = text('2 players  |  30 min  |  easy to learn', { x: f.x0, y: title.box.y + title.box.h + 16, align: 'left', size: 42, color: '#a8b3bd', maxWidth: pw, anim: { ...scene, start: t0 + 0.45, enter: 'slide-up' } });
        items.push(...centreY([pic, badge, num, title, info], f.y0 + 40, f.y1));
      });
      const ask = text('Which one is\nyour #1?', { x: f.cx, y: f.h / 2 - 140, valign: 'middle', size: 120, bold: true, color: '#ffffff', maxWidth: pw, anim: at(11.2, undefined, 'pop', 'none', { in: 0.5 }) });
      const tell = text('Tell us in the comments', { x: f.cx, y: ask.box.y + ask.box.h + 40, size: 50, bold: true, color: '#fee715', anim: at(11.6, undefined, 'slide-up', 'none', { in: 0.5, loop: 'pulse' }) });
      items.push(ask, tell);
      return items;
    },
  },
  {
    key: 'reel-verdict', name: 'Review Verdict', category: 'review', background: '#1b1b2f',
    reel: { duration: 9, poster: 6 },
    draw: f => {
      const items: Item[] = [
        photo('dark', 0, 0, f.w, f.h, { anim: at(0, undefined, 'fade', 'none', { in: 0.6, loop: 'slow-zoom' }) }),
        timed([shade(f, f.h, f.h * 0.95, f.h * 0.4, '#1b1b2f', 0.95)], at(0, undefined, 'none'))[0],
      ];
      const verdict = text('Clever, quick and easy to teach.\nA new favourite on our table.', { x: f.x0, y: f.y1, valign: 'bottom', align: 'left', size: 44, color: '#d8d8e8', maxWidth: f.x1 - f.x0, anim: at(3.4, undefined, 'typewriter', 'none', { in: 2.2 }) });
      const starSize = 96;
      const starY = verdict.box.y - 60 - starSize;
      for (let i = 0; i < 5; i++) {
        items.push(text(i < 4 ? '★' : '☆', { x: f.x0 + i * starSize * 1.05, y: starY, align: 'left', size: starSize, color: '#ffb703', anim: at(1.4 + i * 0.2, undefined, 'pop', 'none', { in: 0.4 }) }));
      }
      const title = text('Game Title', { x: f.x0, y: starY - 24, valign: 'bottom', align: 'left', size: 120, bold: true, color: '#ffffff', maxWidth: f.x1 - f.x0 - 120, anim: at(0.6, undefined, 'slide-up', 'none', { in: 0.6 }) });
      const kicker = text('OUR VERDICT', { x: f.x0, y: title.box.y - 16, valign: 'bottom', align: 'left', size: 44, bold: true, color: '#ffb703', anim: at(0.3, undefined, 'wipe-right', 'none', { in: 0.5 }) });
      const d = 250;
      const cx = f.x1 - d / 2;
      const cy = kicker.box.y - d / 2 - 40;
      items.push(
        circle(cx, cy, d, '#ffb703', { anim: at(2.6, undefined, 'bungee', 'none', { in: 1.0, loop: 'pulse' }) }),
        text('8.5', { x: cx, y: cy - 10, valign: 'middle', size: 104, bold: true, color: '#1b1b2f', anim: at(2.6, undefined, 'bungee', 'none', { in: 1.0, loop: 'pulse' }) }),
        text('OUT OF 10', { x: cx, y: cy + 52, valign: 'middle', size: 24, bold: true, color: '#1b1b2f', anim: at(2.6, undefined, 'bungee', 'none', { in: 1.0, loop: 'pulse' }) }),
        kicker, title, verdict,
      );
      return items;
    },
  },
  {
    key: 'reel-podcast', name: 'Podcast Clip', category: 'episode', background: '#4c2a85',
    reel: { duration: 10, poster: 3 },
    draw: f => {
      const items: Item[] = [
        circle(f.w * 0.95, f.h * 0.12, f.w * 0.7, '#5e3a9e', { anim: at(0, undefined, 'none', 'none', { loop: 'float' }) }),
        circle(f.w * 0.02, f.h * 0.9, f.w * 0.6, '#5e3a9e', { anim: at(0, undefined, 'none', 'none', { loop: 'float' }) }),
      ];
      const tag = timed(pill('NEW EPISODE', f.cx, 0, { fill: '#ff6f59', color: '#ffffff', size: 36, align: 'center' }), at(0.2, undefined, 'pop', 'none', { in: 0.5 }));
      // The cover as a record: it turns on a black disc with grooves.
      const rd = 640;
      const rcy = bounds(tag).h + 50 + rd / 2;
      const spinIn = at(0.4, undefined, 'zoom', 'none', { in: 0.6, loop: 'spin' });
      const record: Item[] = [
        circle(f.cx, rcy, rd, '#141414', { anim: spinIn }),
        circle(f.cx, rcy, rd * 0.86, '', { stroke: '#2a2a2a', strokeWidth: 3, anim: spinIn }),
        circle(f.cx, rcy, rd * 0.74, '', { stroke: '#2a2a2a', strokeWidth: 3, anim: spinIn }),
        photo('cool', f.cx - rd * 0.31, rcy - rd * 0.31, rd * 0.62, rd * 0.62, { corner: 50, anim: spinIn }),
        circle(f.cx, rcy, 28, '#141414', { anim: spinIn }),
      ];
      const ep = text('EPISODE 42', { x: f.cx, y: rcy + rd / 2 + 60, size: 44, bold: true, color: '#ffd166', anim: at(1.0, undefined, 'slide-up', 'none', { in: 0.5 }) });
      const title = text('The episode title\nin two lines', { x: f.cx, y: ep.box.y + ep.box.h + 16, size: 92, bold: true, color: '#ffffff', maxWidth: f.x1 - f.x0, anim: at(1.2, undefined, 'slide-up', 'none', { in: 0.6 }) });
      // A row of dots that ripple like a voice.
      const dots: Item[] = [];
      const n = 11;
      const gap = 52;
      const dy = title.box.y + title.box.h + 60;
      for (let i = 0; i < n; i++) {
        dots.push(circle(f.cx + (i - (n - 1) / 2) * gap, dy, 24, i % 3 === 1 ? '#ffd166' : '#ffffff', { anim: at(1.6 + i * 0.09, undefined, 'fade', 'none', { in: 0.3, loop: 'bob' }) }));
      }
      const listen = text('Listen now, link in bio', { x: f.cx, y: dy + 60, size: 40, color: '#e0d4f5', anim: at(2.2, undefined, 'fade', 'none', { in: 0.6 }) });
      items.push(...centreY([...tag, ...record, ep, title, ...dots, listen], f.y0, f.y1));
      return items;
    },
  },
  {
    // Tabulaludo's weekly review: its intro is a short audio play, so the
    // reel is the game's cover and an equalizer moving to the sound, in the
    // podcast's colours, with room at the bottom for the auto-captions.
    key: 'reel-tabulaludo-review', name: 'Tabulaludo Review', category: 'review', background: '#eee4b8',
    reel: { duration: 30, poster: 4 },
    draw: f => {
      const red = '#a31b22';
      const ink = '#3b2a1a';
      const sand = '#d3c798';
      const slab = font('Rockwell');
      const items: Item[] = [
        circle(f.w * 0.98, f.h * 0.14, f.w * 0.8, sand, { anim: at(0, undefined, 'none', 'none', { loop: 'breathe' }) }),
        circle(f.w * 0.02, f.h * 0.88, f.w * 0.7, sand, { anim: at(0, undefined, 'none', 'none', { loop: 'breathe' }) }),
        rect(0, 0, f.w, 22, red),
        rect(0, f.h - 22, f.w, 22, red),
      ];
      const brand = text('TABULALUDO', { x: f.cx, y: 0, font: slab, size: 84, bold: true, color: red, maxWidth: f.x1 - f.x0, anim: at(0.2, undefined, 'wipe-right', 'none', { in: 0.7 }) });
      const pod = text('PODCAST', { x: f.cx, y: brand.box.h + 2, font: slab, size: 32, bold: true, color: ink, anim: at(0.6, undefined, 'fade', 'none', { in: 0.5 }) });
      const rule = (x: number) => rect(x, pod.box.y + pod.box.h / 2 - 2, 80, 4, red, { anim: at(0.6, undefined, 'wipe-right', 'none', { in: 0.5 }) });
      const rules = [rule(f.cx - pod.box.w / 2 - 104), rule(f.cx + pod.box.w / 2 + 24)];
      const tag = timed(pill('DAS WÖCHENTLICHE REVIEW', f.cx, pod.box.y + pod.box.h + 30, { fill: red, color: '#eee4b8', size: 28, align: 'center', font: slab }), at(0.9, undefined, 'pop', 'none', { in: 0.5 }));
      // The game's cover, on a card that floats a little.
      const cd = 420;
      const cy = bounds(tag).y + bounds(tag).h + 36;
      const float = at(1.2, undefined, 'zoom', 'none', { in: 0.7, loop: 'float' });
      const cover: Item[] = [
        rect(f.cx - cd / 2 + 18, cy + 22, cd, cd, ink, { corner: 3, opacity: 0.25, anim: float }),
        photo('warm', f.cx - cd / 2, cy, cd, cd, { corner: 3, border: { style: 'solid', width: 14, color: '#ffffff' }, anim: float }),
      ];
      const title = text('"Spieltitel"', { x: f.cx, y: cy + cd + 36, font: slab, size: 72, bold: true, color: ink, maxWidth: f.x1 - f.x0, anim: at(1.6, undefined, 'slide-up', 'none', { in: 0.6 }) });
      // The intro's sound, as bars that move to it.
      const eq = shape('equalizer', f.x0 + 20, title.box.y + title.box.h + 26, f.x1 - f.x0 - 40, 100, red, { corner: 50, anim: at(2.0, undefined, 'fade', 'none', { in: 0.6 }) });
      (eq.layer as ShapeLayer).bars = 28;
      (eq.layer as ShapeLayer).mirror = true;
      // Kept clear of the auto-captions (two lines about 0.68 of the way
      // down, captions.ts).
      items.push(...centreY([brand, pod, ...rules, ...tag, ...cover, title, eq], f.y0 - 50, f.h * 0.617));
      return items;
    },
  },
  {
    key: 'reel-breaking', name: 'Breaking Live', category: 'news', background: '#111111', overPhoto: true,
    reel: { duration: 8, poster: 4 },
    draw: f => {
      const items: Item[] = [
        photo('cool', 0, 0, f.w, f.h, { bg: true, anim: at(0, undefined, 'none', 'none', { loop: 'slow-zoom' }) }),
        timed([shade(f, f.h, f.h * 0.85, f.h * 0.4, '#000000', 0.9)], at(0, undefined, 'fade', 'none', { in: 0.8 }))[0],
      ];
      const label = text('LIVE', { x: 0, y: 0, align: 'left', size: 36, bold: true, color: '#ffffff' });
      const bh = label.box.h + 32;
      const live = at(0.3, undefined, 'pop', 'none', { in: 0.4 });
      items.push(rect(f.x0, f.y0, label.box.w + 100, bh, '#e63946', { corner: 50, anim: live }));
      items.push(circle(f.x0 + 38, f.y0 + bh / 2, 20, '#ffffff', { anim: { ...live, loop: 'blink' } }));
      items.push(...timed(move([label], f.x0 + 64, f.y0 + 16), live));
      const sub = text('Add the one detail that matters most.', { x: f.x0, y: f.y1, valign: 'bottom', align: 'left', size: 44, color: '#d8d8d8', maxWidth: f.x1 - f.x0, anim: at(3.4, undefined, 'fade', 'none', { in: 0.6 }) });
      const head = text('Big announcement\nin two short lines', { x: f.x0, y: sub.box.y - 30, valign: 'bottom', align: 'left', size: 104, bold: true, color: '#ffffff', maxWidth: f.x1 - f.x0, anim: at(1.3, undefined, 'typewriter', 'none', { in: 1.8 }) });
      const tag = text('BREAKING NEWS', { x: 0, y: 0, align: 'left', size: 50, bold: true, color: '#ffffff' });
      const band = rect(f.x0, head.box.y - 40 - (tag.box.h + 40), tag.box.w + 70, tag.box.h + 40, '#e63946', { anim: at(0.7, undefined, 'wipe-right', 'none', { in: 0.5 }) });
      move([tag], f.x0 + 35, band.box.y + 20);
      tag.anim = at(0.9, undefined, 'flicker', 'none', { in: 0.5 });
      items.push(band, tag, head, sub);
      return items;
    },
  },
  {
    key: 'reel-versus', name: 'Versus', category: 'question', background: '#111111',
    reel: { duration: 8, poster: 3 },
    draw: f => {
      const half = f.w / 2;
      const items: Item[] = [
        rect(0, 0, half, f.h, '#ff4d6d', { anim: at(0, undefined, 'slide-down', 'none', { in: 0.5 }) }),
        rect(half, 0, half, f.h, '#3a86ff', { anim: at(0, undefined, 'slide-up', 'none', { in: 0.5 }) }),
      ];
      const title = text('WHICH ONE\nWINS?', { x: f.cx, y: f.y0, size: 120, bold: true, color: '#ffffff', shadow: true, maxWidth: f.x1 - f.x0, anim: at(0.3, undefined, 'slide-down', 'none', { in: 0.6 }) });
      const pw = half - 110;
      const ph = pw * 1.4;
      const py = title.box.y + title.box.h + 80;
      const border = { style: 'solid' as const, width: 12, color: '#ffffff' };
      items.push(title,
        photo('warm', (half - pw) / 2 + 20, py, pw, ph, { corner: 6, border, rotation: -3, anim: at(0.6, undefined, 'slide-right', 'none', { in: 0.6 }) }),
        photo('cool', half + (half - pw) / 2 - 20, py, pw, ph, { corner: 6, border, rotation: 3, anim: at(0.8, undefined, 'slide-left', 'none', { in: 0.6 }) }));
      const label = (x: number, l: string, start: number) => text(l, { x, y: py + ph + 50, size: 60, bold: true, color: '#ffffff', anim: at(start, undefined, 'pop', 'none', { in: 0.4 }) });
      items.push(label(half / 2 + 20, 'A', 1.6), label(half * 1.5 - 20, 'B', 1.8));
      const vy = py + ph / 2;
      const vs = at(1.2, undefined, 'spin', 'none', { in: 0.6, loop: 'pulse' });
      items.push(circle(f.cx, vy, 220, '#ffffff', { stroke: '#111111', strokeWidth: 10, anim: vs }), text('VS', { x: f.cx, y: vy, valign: 'middle', size: 100, bold: true, color: '#111111', anim: vs }));
      items.push(text('Vote in the comments', { x: f.cx, y: py + ph + 160, size: 52, bold: true, color: '#ffffff', anim: at(2.2, undefined, 'bungee', 'none', { in: 1.0 }) }));
      return items;
    },
  },
  {
    key: 'reel-savethedate', name: 'Save the Date', category: 'event', background: '#14213d',
    reel: { duration: 8, poster: 5 },
    draw: f => {
      const items: Item[] = confetti(f, 2.4, ['#fca311', '#ffffff', '#e5e5e5', '#fca311']);
      const kicker = text('SAVE THE DATE', { x: f.cx, y: 0, size: 72, bold: true, color: '#fca311', anim: at(0.2, undefined, 'wipe-right', 'none', { in: 0.6 }) });
      const day = text('24', { x: f.cx, y: kicker.box.h + 20, size: 420, bold: true, color: '#ffffff', anim: at(0.7, undefined, 'shrink', 'none', { in: 0.5 }) });
      const month = text('OCTOBER', { x: f.cx, y: day.box.y + day.box.h - 30, size: 96, bold: true, color: '#fca311', anim: at(1.1, undefined, 'slide-up', 'none', { in: 0.5 }) });
      const line = rect(f.cx - 120, month.box.y + month.box.h + 50, 240, 8, '#ffffff', { anim: at(1.4, undefined, 'wipe-right', 'none', { in: 0.5 }) });
      const details = text('Friday, 7 pm\nYour venue name', { x: f.cx, y: line.box.y + 60, size: 56, color: '#e5e5e5', anim: at(1.7, undefined, 'typewriter', 'none', { in: 1.2 }) });
      const cta = timed(pill('EVERYONE WELCOME', f.cx, details.box.y + details.box.h + 60, { fill: '#fca311', color: '#14213d', size: 40, align: 'center' }), at(3.1, undefined, 'pop', 'none', { in: 0.5, loop: 'pulse' }));
      items.push(...centreY([kicker, day, month, line, details, ...cta], f.y0, f.y1));
      return items;
    },
  },
  {
    key: 'reel-quote', name: 'Quote Typewriter', category: 'quote', background: '#f7f3ee',
    reel: { duration: 8, poster: 6 },
    draw: f => {
      const mark = text('“', { x: f.cx, y: 0, font: SERIF, size: 360, bold: true, color: '#e76f51', anim: at(0.2, undefined, 'pop', 'none', { in: 0.5 }) });
      const q = text('Put a memorable quote\nfrom the episode or\na review right here.', { x: f.cx, y: mark.box.h * 0.55, font: SERIF, size: 84, color: '#222222', maxWidth: f.x1 - f.x0, anim: at(0.6, undefined, 'typewriter', 'none', { in: 3.0 }) });
      const line = rect(f.cx - 50, q.box.y + q.box.h + 60, 100, 8, '#e76f51', { anim: at(3.8, undefined, 'wipe-right', 'none', { in: 0.4 }) });
      const who = text('Name Surname', { x: f.cx, y: line.box.y + 48, size: 46, bold: true, color: '#222222', anim: at(4.0, undefined, 'slide-up', 'none', { in: 0.5 }) });
      const role = text('Role or source', { x: f.cx, y: who.box.y + who.box.h + 10, size: 38, color: '#777777', anim: at(4.3, undefined, 'fade', 'none', { in: 0.5 }) });
      return centreY([mark, q, line, who, role], f.y0, f.y1);
    },
  },
  {
    key: 'reel-giveaway', name: 'Giveaway Hype', category: 'giveaway', background: '#7b2ff7',
    reel: { duration: 10, poster: 6.5 },
    draw: f => {
      const items: Item[] = [];
      const words: [string, string, string][] = [['WIN', '#ff6fb5', '#ffffff'], ['THIS', '#ffd23f', '#111111'], ['GAME', '#2ec4b6', '#ffffff']];
      words.forEach(([word, bg, colour], i) => {
        const t0 = i * 0.9;
        items.push(rect(0, 0, f.w, f.h, bg, { anim: at(t0, t0 + 0.9, 'none', 'none') }));
        items.push(text(word, { x: f.cx, y: f.h / 2, valign: 'middle', size: 320, bold: true, color: colour, maxWidth: f.x1 - f.x0, anim: at(t0, t0 + 0.9, 'pop', 'none', { in: 0.3 }) }));
      });
      items.push(...confetti(f, 2.7, ['#ffd23f', '#ff6fb5', '#2ec4b6']));
      const head = text('GIVEAWAY', { x: f.cx, y: 0, size: 150, bold: true, color: '#ffffff', shadow: true, rotation: -4, maxWidth: f.x1 - f.x0, anim: at(2.7, undefined, 'pop', 'none', { in: 0.5, loop: 'wiggle' }) });
      const pw = (f.x1 - f.x0) * 0.85;
      const pic = photo('warm', f.cx - pw / 2, head.box.h + 50, pw, pw * 0.95, { corner: 5, border: { style: 'solid', width: 12, color: '#ffffff' }, anim: at(3.0, undefined, 'zoom', 'none', { in: 0.6, loop: 'float' }) });
      const how = text('HOW TO ENTER', { x: f.cx, y: pic.box.y + pic.box.h + 50, size: 46, bold: true, color: '#ffd23f', anim: at(3.6, undefined, 'fade', 'none', { in: 0.4 }) });
      const steps = ['1. Follow us', '2. Like this post', '3. Tag a friend'];
      let y = how.box.y + how.box.h + 16;
      const lines = steps.map((st, i) => {
        const t = text(st, { x: f.cx, y, size: 50, color: '#ffffff', anim: at(4.0 + i * 0.4, undefined, 'slide-left', 'none', { in: 0.4 }) });
        y += t.box.h + 8;
        return t;
      });
      const ends = timed(pill('ENDS FRIDAY', f.cx, y + 40, { fill: '#ff6fb5', color: '#ffffff', size: 36, align: 'center' }), at(5.4, undefined, 'pop', 'none', { in: 0.4, loop: 'pulse' }));
      items.push(...centreY([head, pic, how, ...lines, ...ends], f.y0, f.y1));
      return items;
    },
  },
  {
    key: 'reel-recap', name: 'Photo Recap', category: 'photo', background: '#000000',
    reel: { duration: 9, poster: 1.5 },
    draw: f => {
      const items: Item[] = [];
      const shots: [PhotoKey, string][] = [['warm', 'Setting up'], ['green', 'Mid-game chaos'], ['dark', 'And the winner is...']];
      shots.forEach(([p], i) => {
        const t0 = i * 3;
        const end = i < 2 ? t0 + 3.4 : undefined;
        items.push(photo(p, 0, 0, f.w, f.h, { anim: at(t0, end, i ? 'fade' : 'none', 'none', { in: 0.5, loop: 'slow-zoom' }) }));
      });
      items.push(timed([shade(f, f.h, f.h * 0.75, f.h * 0.35, '#000000', 0.85)], at(0, undefined, 'none'))[0]);
      const label = text('GAME NIGHT RECAP', { x: f.x0, y: f.y0, align: 'left', size: 40, bold: true, color: '#ffffff', anim: at(0.2, undefined, 'wipe-right', 'none', { in: 0.6 }) });
      items.push(label, rect(f.x0, label.box.y + label.box.h + 16, 90, 6, '#ffd23f', { anim: at(0.5, undefined, 'wipe-right', 'none', { in: 0.4 }) }));
      shots.forEach(([, caption], i) => {
        const t0 = i * 3;
        const end = i < 2 ? t0 + 3 : undefined;
        items.push(text(caption, { x: f.x0, y: f.y1, valign: 'bottom', align: 'left', size: 100, bold: true, color: '#ffffff', maxWidth: f.x1 - f.x0, anim: at(t0 + 0.3, end, 'slide-up', 'fade', { in: 0.5, out: 0.3 }) }));
      });
      const counter = (i: number) => text(`${i + 1} / 3`, { x: f.x1, y: f.y0, align: 'right', size: 36, bold: true, color: '#ffd23f', anim: at(i * 3, i < 2 ? i * 3 + 3 : undefined, 'fade', 'none', { in: 0.3 }) });
      items.push(counter(0), counter(1), counter(2));
      return items;
    },
  },
];

// --- The library ------------------------------------------------------------

// The cascade a video version comes in with: every layer from the start, each
// taking a little longer than the one before, so they arrive one by one and
// the design is complete where the editor opens it (openingTime).
function videoAnim(it: Item, k: number, f: Frame): LayerAnim | undefined {
  if (it.bg || it.anim === null) return undefined;
  let enter: EnterEffect;
  const area = (it.box.w * it.box.h) / (f.w * f.h);
  if (it.layer.kind === 'text') enter = 'slide-up';
  else if (it.layer.kind === 'image') enter = 'zoom';
  else if (it.layer.kind === 'paint') enter = 'fade';
  else enter = area < 0.05 ? 'pop' : area < 0.4 ? 'wipe-right' : 'fade';
  return { ...NO_ANIM, start: 0, enter, enterDuration: Math.min(0.5 + k * 0.12, 1.8), ...it.anim };
}

function buildTemplate(d: Design, fmt: SystemFormat, { video, photo: onPhoto }: { video: boolean; photo: boolean }): EditorTemplate {
  const f = frameOf(fmt);
  let items = d.draw(f);
  // On a photo, the photo is the backdrop.
  if (onPhoto && d.overPhoto) items = items.filter((it, i) => !(i === 0 && it.layer.kind === 'image'));
  let k = 0;
  const layers = items.map(it => {
    if (d.reel) return it.anim ? { ...it.layer, anim: { ...NO_ANIM, ...it.anim } } : it.layer;
    if (!video) return it.layer;
    const anim = videoAnim(it, k, f);
    if (anim) k++;
    return anim ? { ...it.layer, anim } : it.layer;
  });
  return {
    version: 1, width: f.w, height: f.h, background: d.background, layers,
    ...(video || d.reel ? { duration: d.reel?.duration ?? VIDEO_LENGTH } : {}),
  };
}

// Reels first, so a video editor lists them at the top.
export const SYSTEM_TEMPLATES: SystemTemplate[] = [...REELS, ...DESIGNS].flatMap(d =>
  SYSTEM_FORMATS.filter(({ id }) => !d.reel || (d.reel.formats ?? ['9:16']).includes(id)).map(({ id: fmt }) => ({
    id: `${d.key}-${fmt}`,
    name: d.name,
    category: d.category,
    format: fmt,
    overPhoto: !!d.overPhoto,
    videoOnly: !!d.reel,
    duration: d.reel?.duration ?? VIDEO_LENGTH,
    poster: d.reel?.poster ?? VIDEO_LENGTH,
    build: (opts: { video: boolean; photo: boolean }) => buildTemplate(d, fmt, opts),
  })));

// The library's format closest to a picture's shape.
export function nearestFormat(width: number, height: number): SystemFormat {
  const r = width / height;
  let best = SYSTEM_FORMATS[0];
  for (const f of SYSTEM_FORMATS) {
    if (Math.abs(Math.log(f.width / f.height / r)) < Math.abs(Math.log(best.width / best.height / r))) best = f;
  }
  return best.id;
}

// A template's layers, loaded (its pictures) once per session and kind: the
// still design, or the video with its timing.
const loadedLayers = new Map<string, Promise<{ template: EditorTemplate; layers: EditorLayer[] }>>();

export function systemTemplateLayers(s: SystemTemplate, video: boolean): Promise<{ template: EditorTemplate; layers: EditorLayer[] }> {
  const key = `${s.id}:${video || s.videoOnly ? 'video' : 'still'}`;
  let p = loadedLayers.get(key);
  if (!p) {
    p = (async () => {
      const template = s.build({ video, photo: false });
      const { layers } = await instantiateTemplate(template, { x: 0, y: 0, width: template.width, height: template.height }, url => url);
      return { template, layers };
    })();
    p.catch(() => loadedLayers.delete(key));
    loadedLayers.set(key, p);
  }
  return p;
}

// Draws the design at time t (seconds; a still design whatever t) into
// group, in the design's own pixels: the background, then every layer that
// shows, as it moves then, like the editor's export.
export function drawSystemFrame(group: Konva.Group, template: EditorTemplate, layers: EditorLayer[], t: number): void {
  group.destroyChildren();
  const box = { x: 0, y: 0, width: template.width, height: template.height };
  group.add(new Konva.Rect({ ...box, fill: template.background }));
  // A still design's layers have no timing, so they show as they are.
  const duration = template.duration ?? 1;
  for (const l of layers) {
    const st = animStateAt(l, t, duration, box);
    // An equalizer dances without music in a video, and rests in a still.
    if (st.visible) group.add(buildAnimatedLayerNode(l, st, isEqualizer(l) ? equalizerLevels(l, template.duration ? t : null, duration, null, null) : undefined));
  }
}

// Previews, rendered in the browser the first time the library shows them
// (one after the other, so a long list does not block the page) and kept
// for the session as object URLs. A reel shows its poster frame.
const previews = new Map<string, Promise<string>>();
let queue: Promise<unknown> = Promise.resolve();

const PREVIEW_SIDE = 320;

export function systemTemplatePreview(s: SystemTemplate): Promise<string> {
  let p = previews.get(s.id);
  if (!p) {
    p = queue.then(async () => {
      const { template, layers } = await systemTemplateLayers(s, s.videoOnly);
      const scale = PREVIEW_SIDE / Math.max(template.width, template.height);
      const container = document.createElement('div');
      const stage = new Konva.Stage({ container, width: 1, height: 1 });
      try {
        const layer = new Konva.Layer({ listening: false });
        stage.add(layer);
        const group = new Konva.Group({ scaleX: scale, scaleY: scale });
        layer.add(group);
        drawSystemFrame(group, template, layers, s.poster);
        const canvas = group.toCanvas({ x: 0, y: 0, width: Math.round(template.width * scale), height: Math.round(template.height * scale), pixelRatio: 1 });
        const blob = await new Promise<Blob>((resolve, reject) => canvas.toBlob(b => (b ? resolve(b) : reject(new Error('Failed to draw the preview'))), 'image/jpeg', 0.85));
        return URL.createObjectURL(blob);
      } finally {
        stage.destroy();
      }
    });
    // A failed preview is tried again next time.
    p.catch(() => previews.delete(s.id));
    queue = p.catch(() => {});
    previews.set(s.id, p);
  }
  return p;
}
