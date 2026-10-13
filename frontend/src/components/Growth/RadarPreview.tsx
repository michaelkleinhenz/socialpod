import type { ReactNode } from 'react';
import type { RadarSettings } from '../../types';
import { radarColor, resolveUrl, useRadarFont } from './radarSettings';

const W = 1080;
const H = 1350;
const FALLBACK_FAMILY = 'system-ui, -apple-system, "Segoe UI", Roboto, sans-serif';

let measureCtx: CanvasRenderingContext2D | null = null;

// measureText is the width of s in a CSS font ("700 62px sans-serif").
function measureText(s: string, font: string): number {
  measureCtx ??= document.createElement('canvas').getContext('2d');
  if (!measureCtx) return s.length * 0.55 * parseFloat(font.split('px')[0].split(' ').pop() || '40');
  measureCtx.font = font;
  return measureCtx.measureText(s).width;
}

// wrapLines mirrors wrapText in radar_render.go: greedy word wrap, the last
// allowed line cut with an ellipsis.
function wrapLines(text: string, font: string, maxWidth: number, maxLines: number): string[] {
  const width = (s: string) => measureText(s, font);
  const words = text.split(/\s+/).filter(Boolean);
  const lines: string[] = [];
  let cur = '';
  for (let i = 0; i < words.length; i++) {
    const next = cur ? `${cur} ${words[i]}` : words[i];
    if (width(next) <= maxWidth || !cur) {
      cur = next;
      continue;
    }
    lines.push(cur);
    cur = words[i];
    if (lines.length === maxLines) {
      cur = '';
      let last = lines[maxLines - 1];
      while (last && width(`${last}…`) > maxWidth) last = last.slice(0, -1).trimEnd();
      lines[maxLines - 1] = `${last}…`;
      break;
    }
  }
  if (cur) lines.push(cur);
  return lines;
}

interface SlideProps {
  radar: RadarSettings;
  family: string | null;
}

function slideFonts(family: string | null) {
  // An uploaded font serves every text, as on the real slides; the built-in
  // Go fonts are approximated with the system sans in bold and regular.
  const fam = family ? `"${family}"` : FALLBACK_FAMILY;
  const boldWeight = family ? 400 : 700;
  return {
    fam,
    bold: (size: number) => ({ fontFamily: fam, fontWeight: boldWeight, fontSize: size }),
    regular: (size: number) => ({ fontFamily: fam, fontWeight: 400, fontSize: size }),
    boldCss: (size: number) => `${boldWeight} ${size}px ${fam}`,
  };
}

function CoverSlide({ radar, family }: SlideProps) {
  const f = slideFonts(family);
  const title = radar.title || 'Radar der Woche';
  const titleFont = f.boldCss(110);
  const lines = wrapLines(title, titleFont, W - 160, 3);
  if (radar.coverImageUrl) {
    return <image href={resolveUrl(radar.coverImageUrl)} width={W} height={H} preserveAspectRatio="xMidYMid slice" />;
  }
  const text = radarColor(radar, 'textColor');
  const y = H / 2 - (lines.length * 124) / 2 + 90;
  return (
    <>
      <rect width={W} height={H} fill={radarColor(radar, 'accentColor')} />
      {lines.map((l, i) => <text key={i} x={W / 2} y={y + i * 124} textAnchor="middle" fill={text} {...f.bold(110)}>{l}</text>)}
      <text x={W / 2} y={y + lines.length * 124 + 30} textAnchor="middle" fill={text} {...f.regular(46)}>KW 42 · 5 Ankündigungen</text>
    </>
  );
}

const BADGES = {
  ja: { label: 'JA', key: 'badgeYesColor' },
  nein: { label: 'NEIN', key: 'badgeNoColor' },
  offen: { label: 'OFFEN', key: 'badgeOpenColor' },
} as const;

function ItemSlide({ radar, family, index, title, badge }: SlideProps & { index: number; title: string; badge: keyof typeof BADGES }) {
  const f = slideFonts(family);
  const text = radarColor(radar, 'textColor');
  const muted = radarColor(radar, 'mutedColor');
  const titleFont = f.boldCss(62);
  const pillFont = f.boldCss(52);
  const titleLines = wrapLines(title, titleFont, W - 120, 2);
  const b = BADGES[badge];
  const pillW = Math.round(measureText(b.label, pillFont)) + 80;
  const pillX = W - 60 - pillW;
  const question = wrapLines(radar.badgeQuestion || "Kommt's auf Deutsch?", `400 44px ${f.fam}`, pillX - 90, 1)[0] ?? '';
  return (
    <>
      <rect width={W} height={H} fill={radarColor(radar, 'backgroundColor')} />
      <text x={60} y={92} fill={radarColor(radar, 'accentColor')} {...f.bold(40)}>{(radar.title || 'Radar der Woche').toUpperCase()}</text>
      <text x={W - 60} y={92} textAnchor="end" fill={muted} {...f.bold(40)}>{index}/5</text>
      <rect x={60} y={130} width={W - 120} height={880} rx={28} fill={radarColor(radar, 'panelColor')} />
      <text x={W / 2} y={590} textAnchor="middle" fill={muted} opacity={0.6} {...f.regular(44)}>game image</text>
      {titleLines.map((l, i) => <text key={i} x={60} y={1100 + i * 72} fill={text} {...f.bold(62)}>{l}</text>)}
      <rect x={pillX} y={1222} width={pillW} height={84} rx={42} fill={radarColor(radar, b.key)} />
      <text x={pillX + 40} y={1283} fill={text} {...f.bold(52)}>{b.label}</text>
      <text x={60} y={1279} fill={muted} {...f.regular(44)}>{question}</text>
    </>
  );
}

const DEFAULT_CLOSING_TEXT = 'Welche Neuheit interessiert euch am meisten? Schreibt es in die Kommentare!';

function ClosingSlide({ radar, family }: SlideProps) {
  const f = slideFonts(family);
  const closing = radar.closingText || DEFAULT_CLOSING_TEXT;
  const textFont = f.boldCss(76);
  const lines = wrapLines(closing, textFont, W - 160, 6);
  if (radar.closingImageUrl) {
    return <image href={resolveUrl(radar.closingImageUrl)} width={W} height={H} preserveAspectRatio="xMidYMid slice" />;
  }
  const y = H / 2 - (lines.length * 92) / 2 + 60;
  return (
    <>
      <rect width={W} height={H} fill={radarColor(radar, 'backgroundColor')} />
      <rect x={60} y={60} width={W - 120} height={12} rx={6} fill={radarColor(radar, 'accentColor')} />
      {lines.map((l, i) => (
        <text key={i} x={W / 2} y={y + i * 92} textAnchor="middle" fill={radarColor(radar, 'textColor')} {...f.bold(76)}>{l}</text>
      ))}
    </>
  );
}

function Frame({ label, children }: { label: string; children: ReactNode }) {
  return (
    <figure className="radar-preview-slide">
      <svg viewBox={`0 0 ${W} ${H}`} role="img" aria-label={label}>{children}</svg>
      <figcaption>{label}</figcaption>
    </figure>
  );
}

/**
 * A live sketch of the Radar carousel drawn with the settings being edited:
 * same layout, colours, title, badge question, slide images and font as
 * services/radar_render.go, with placeholder news.
 */
export function RadarPreview({ radar }: { radar: RadarSettings }) {
  const family = useRadarFont(radar.fontUrl);
  return (
    <div className="radar-preview">
      <Frame label="Opening"><CoverSlide radar={radar} family={family} /></Frame>
      <Frame label="News, Ja"><ItemSlide radar={radar} family={family} index={1} title="Arche Nova: Meereswelten" badge="ja" /></Frame>
      <Frame label="News, Nein"><ItemSlide radar={radar} family={family} index={2} title="Ein ziemlich langer Spieltitel, der umbrechen muss" badge="nein" /></Frame>
      <Frame label="News, Offen"><ItemSlide radar={radar} family={family} index={3} title="Größenwahn" badge="offen" /></Frame>
      <Frame label="Closing"><ClosingSlide radar={radar} family={family} /></Frame>
    </div>
  );
}
