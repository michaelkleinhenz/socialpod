import { NO_ANIM, type EnterEffect, type LayerAnim } from './animation';
import {
  BUILTIN_FONTS, instantiateTemplate, measureText, NO_ADJUSTMENTS, renderEdit, type BlendMode, type CropBox, type EditorTemplate, type ImageBorder, type ImageLayer, type PaintLayer, type ShapeKind,
  type ShapeLayer, type TemplateLayer, type TextAlign, type TextLayer,
} from './editorModel';

// The templates SocialPod ships with, beside the team's own: generic designs
// for posts, stories and reels, in plain colours and the built-in fonts, which
// the user changes once a template is on the canvas. Each design is a function
// of the format, so it comes in 1:1, 4:5 and 9:16 (kept inside the area
// Instagram's interface leaves free), and in a video with its layers coming
// in one after the other. Pictures are placeholders (public/template-assets),
// which a paste or Replace takes the place of.

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

interface PhotoOpts { corner?: number; border?: ImageBorder; rotation?: number; bg?: boolean }

// A placeholder picture filling the box (top-left x, y).
function photo(p: PhotoKey, x: number, y: number, w: number, h: number, o: PhotoOpts = {}): Item {
  const layer: Omit<ImageLayer, 'id' | 'img'> = {
    kind: 'image', origin: 'media', src: PHOTOS[p],
    x: x + w / 2, y: y + h / 2, width: w, height: h, rotation: o.rotation ?? 0, opacity: 1,
    srcCrop: coverCrop(w, h),
    ...(o.corner ? { cornerRadius: o.corner } : {}),
    ...(o.border ? { border: o.border } : {}),
  };
  return { layer, box: { x, y, w, h }, bg: o.bg };
}

interface ShapeOpts { corner?: number; stroke?: string; strokeWidth?: number; opacity?: number; rotation?: number; blend?: BlendMode; bg?: boolean; anim?: Partial<LayerAnim> | null }

// A box, ellipse or triangle filling the box (top-left x, y); fill '' for none.
function shape(kind: ShapeKind, x: number, y: number, w: number, h: number, fill: string, o: ShapeOpts = {}): Item {
  const layer: Omit<ShapeLayer, 'id'> = {
    kind: 'shape', shape: kind, x: x + w / 2, y: y + h / 2, width: w, height: h, rotation: o.rotation ?? 0, opacity: o.opacity ?? 1,
    fill, stroke: o.stroke ?? '#000000', strokeWidth: o.strokeWidth ?? 0,
    ...(kind === 'rect' && o.corner ? { cornerRadius: o.corner } : {}),
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
    if (!video) return it.layer;
    const anim = videoAnim(it, k, f);
    if (anim) k++;
    return anim ? { ...it.layer, anim } : it.layer;
  });
  return {
    version: 1, width: f.w, height: f.h, background: d.background, layers,
    ...(video ? { duration: VIDEO_LENGTH } : {}),
  };
}

export const SYSTEM_TEMPLATES: SystemTemplate[] = DESIGNS.flatMap(d => SYSTEM_FORMATS.map(({ id: fmt }) => ({
  id: `${d.key}-${fmt}`,
  name: d.name,
  category: d.category,
  format: fmt,
  overPhoto: !!d.overPhoto,
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

// Previews, rendered in the browser the first time the library shows them
// (one after the other, so a long list does not block the page) and kept
// for the session as object URLs.
const previews = new Map<string, Promise<string>>();
let queue: Promise<unknown> = Promise.resolve();

export function systemTemplatePreview(s: SystemTemplate): Promise<string> {
  let p = previews.get(s.id);
  if (!p) {
    p = queue.then(async () => {
      const t = s.build({ video: false, photo: false });
      const box = { x: 0, y: 0, width: t.width, height: t.height };
      const { layers } = await instantiateTemplate(t, box, url => url);
      const blob = await renderEdit({ source: null, background: t.background, crop: box, adjustments: NO_ADJUSTMENTS, layers, maxSide: 320, quality: 0.85 });
      return URL.createObjectURL(blob);
    });
    // A failed preview is tried again next time.
    p.catch(() => previews.delete(s.id));
    queue = p.catch(() => {});
    previews.set(s.id, p);
  }
  return p;
}
