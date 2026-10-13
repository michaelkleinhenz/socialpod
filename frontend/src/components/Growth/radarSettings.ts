import { useEffect, useState } from 'react';
import type { RadarSettings } from '../../types';

// The defaults the backend draws with when a colour is not set
// (services.DefaultRadar* in radar_render.go).
export const RADAR_COLOR_DEFAULTS = {
  accentColor: '#6366f1',
  backgroundColor: '#0f172a',
  panelColor: '#1e293b',
  textColor: '#f8fafc',
  mutedColor: '#94a3b8',
  badgeYesColor: '#16a34a',
  badgeNoColor: '#dc2626',
  badgeOpenColor: '#d97706',
} as const;

export type RadarColorKey = keyof typeof RADAR_COLOR_DEFAULTS;

export const RADAR_COLOR_FIELDS: { key: RadarColorKey; label: string; hint: string }[] = [
  { key: 'accentColor', label: 'Accent', hint: 'Drawn opening slide, header on the news slides, bar on the closing slide' },
  { key: 'backgroundColor', label: 'Background', hint: 'News and closing slides' },
  { key: 'panelColor', label: 'Image panel', hint: 'The box behind each game image' },
  { key: 'textColor', label: 'Text', hint: 'Titles, closing text and badge labels' },
  { key: 'mutedColor', label: 'Muted text', hint: 'Position (1/5) and the badge question' },
  { key: 'badgeYesColor', label: 'Badge “Ja”', hint: 'German release: yes' },
  { key: 'badgeNoColor', label: 'Badge “Nein”', hint: 'German release: no' },
  { key: 'badgeOpenColor', label: 'Badge “Offen”', hint: 'German release: open' },
];

export function radarColor(radar: RadarSettings, key: RadarColorKey): string {
  return radar[key] || RADAR_COLOR_DEFAULTS[key];
}

export function resolveUrl(url: string) {
  const apiUrl = import.meta.env.VITE_API_URL || '';
  return url.startsWith('/') ? apiUrl + url : url;
}

/** Loads the uploaded Radar font and returns its CSS family once it is ready. */
export function useRadarFont(url?: string): string | null {
  // The family is kept with the URL it was loaded for, so a font that is
  // replaced or removed stops showing at once.
  const [loaded, setLoaded] = useState<{ url: string; family: string } | null>(null);
  useEffect(() => {
    if (!url) return;
    let cancelled = false;
    const family = `radar-font-${url.replace(/[^a-z0-9]/gi, '')}`;
    const face = new FontFace(family, `url(${resolveUrl(url)})`);
    face.load().then(f => {
      if (cancelled) return;
      document.fonts.add(f);
      setLoaded({ url, family });
    }).catch(() => { /* previews fall back to the page font */ });
    return () => { cancelled = true; };
  }, [url]);
  return loaded && loaded.url === url ? loaded.family : null;
}
