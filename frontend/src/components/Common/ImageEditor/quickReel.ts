import { api } from '../../../services/api';
import { centredCrop, clamp, instantiateTemplate, loadVideo, newLayerId, NO_ADJUSTMENTS, sourceSize, type EditorLayer, type EditorTemplate, type ImageLayer } from './editorModel';
import { MAX_DURATION, MIN_DURATION } from './animation';
import { canExportVideo, renderVideo } from './videoExport';

// A clip put into a video template and exported as the editor's "Export
// video" would: the template's layers instantiated as the editor does, the
// clip in its main picture, then renderVideo frame by frame. Nothing is
// shown or edited on the way.

const apiBase = import.meta.env.VITE_API_URL || '';
const resolveUrl = (url: string) => (url.startsWith('/') ? apiBase + url : url);

// The media library's fonts, registered under the family names the editor
// gives them (ImageEditor's font loading), so a template's texts are set in
// them. A font that fails to load falls back like it does in the editor.
let libraryFonts: Promise<void> | null = null;
function loadLibraryFonts(): Promise<void> {
  libraryFonts ??= api.getMedia({ kind: 'font', limit: 200 }).then(async items => {
    await Promise.all(items.filter(m => m.url).map(async m => {
      try {
        const face = await new FontFace(`SocialPod Media ${m.id}`, `url(${resolveUrl(m.url)})`).load();
        document.fonts.add(face);
      } catch {
        // Left out, as in the editor.
      }
    }));
  }).catch(() => { libraryFonts = null; });
  return libraryFonts;
}

const area = (l: ImageLayer) => l.width * l.height;

// The layer the clip goes into: the template's largest clip, or else its
// largest image (a template's photo placeholder, not a logo). -1 when the
// template has neither, and the clip goes under everything instead.
export function clipTarget(layers: EditorLayer[]): number {
  let best = -1;
  let bestClip = false;
  layers.forEach((l, i) => {
    if (l.kind !== 'image') return;
    const isClip = !!l.clip;
    const current = best >= 0 ? layers[best] as ImageLayer : null;
    if (!current || (isClip && !bestClip) || (isClip === bestClip && area(l) > area(current))) {
      best = i;
      bestClip = isClip;
    }
  });
  return best;
}

export interface QuickReelInput {
  template: EditorTemplate;
  // The uploaded clip, a stored URL (/api/uploads/...).
  clipUrl: string;
  onProgress?: (share: number) => void;
  signal?: AbortSignal;
}

export async function renderQuickReel({ template, clipUrl, onProgress, signal }: QuickReelInput): Promise<Blob> {
  const box = { x: 0, y: 0, width: template.width, height: template.height };
  if (!(await canExportVideo(box))) {
    throw new Error('This browser cannot make videos. Use a current Chrome, Edge or Safari.');
  }
  await loadLibraryFonts();
  const [{ layers }, video] = await Promise.all([
    instantiateTemplate(template, box, resolveUrl, { placeholders: false }),
    loadVideo(resolveUrl(clipUrl)).catch(() => { throw new Error('This clip cannot be played in this browser'); }),
  ]);
  const own = sourceSize(video);
  const length = Number.isFinite(video.duration) ? video.duration : 0;
  const duration = clamp(template.duration ?? length, MIN_DURATION, MAX_DURATION);

  // The clip fills its frame, cropped to its shape, from its start.
  const i = clipTarget(layers);
  if (i >= 0) {
    const target = layers[i] as ImageLayer;
    layers[i] = {
      ...target, img: video, src: clipUrl, origin: 'media',
      srcCrop: centredCrop(own.width, own.height, target.width / target.height),
      clip: { trimStart: 0, volume: target.clip?.volume ?? 1, duration: length },
    };
  } else {
    layers.unshift({
      id: newLayerId(), kind: 'image', origin: 'media', src: clipUrl, img: video,
      x: box.width / 2, y: box.height / 2, width: box.width, height: box.height, rotation: 0, opacity: 1,
      srcCrop: centredCrop(own.width, own.height, box.width / box.height),
      clip: { trimStart: 0, volume: 1, duration: length },
    });
  }

  try {
    return await renderVideo({
      source: null,
      background: template.background,
      crop: box,
      adjustments: NO_ADJUSTMENTS,
      layers,
      duration,
      audio: template.audio ?? null,
      resolve: resolveUrl,
      onProgress,
      signal,
    });
  } finally {
    // The export decodes the file itself; the element only gave its size.
    video.removeAttribute('src');
    video.load();
  }
}
