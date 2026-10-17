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

// The share of the canvas a layer's box covers (its turn ignored).
function coverage(l: ImageLayer, width: number, height: number): number {
  const w = Math.min(l.x + l.width / 2, width) - Math.max(l.x - l.width / 2, 0);
  const h = Math.min(l.y + l.height / 2, height) - Math.max(l.y - l.height / 2, 0);
  return w > 0 && h > 0 ? (w * h) / (width * height) : 0;
}

// An image smaller than this share of the canvas is a logo or decoration,
// not a frame meant for the picture.
const MIN_PHOTO_COVERAGE = 1 / 3;

// The layer the clip goes into: the template's largest clip, or else its
// largest image when that covers a good part of the canvas (a photo
// placeholder or full-frame backdrop). -1 when there is neither, and the
// clip goes full frame under everything instead.
export function clipTarget(layers: EditorLayer[], width: number, height: number): number {
  let best = -1;
  let bestClip = false;
  let bestCover = 0;
  layers.forEach((l, i) => {
    if (l.kind !== 'image') return;
    const isClip = !!l.clip;
    const cover = coverage(l, width, height);
    if (!isClip && cover < MIN_PHOTO_COVERAGE) return;
    if (best < 0 || (isClip && !bestClip) || (isClip === bestClip && cover > bestCover)) {
      best = i;
      bestClip = isClip;
      bestCover = cover;
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
  // The clip sets the reel's length. The template plays over its own length
  // and then holds its last frame while the clip runs on.
  const duration = clamp(length || (template.duration ?? 0), MIN_DURATION, MAX_DURATION);
  const designLength = template.duration ?? duration;

  // The clip fills its frame, cropped to its shape, from its start.
  const i = clipTarget(layers, box.width, box.height);
  const clipId = i >= 0 ? layers[i].id : newLayerId();
  if (i >= 0) {
    const target = layers[i] as ImageLayer;
    // A frame that lasts to the template's end lasts to the clip's.
    const anim = target.anim && target.anim.end !== undefined && target.anim.end >= designLength - 0.05
      ? { ...target.anim, end: undefined } : target.anim;
    layers[i] = {
      ...target, img: video, src: clipUrl, origin: 'media', anim,
      srcCrop: centredCrop(own.width, own.height, target.width / target.height),
      clip: { trimStart: 0, volume: target.clip?.volume ?? 1, duration: length },
    };
  } else {
    layers.unshift({
      id: clipId, kind: 'image', origin: 'media', src: clipUrl, img: video,
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
      hold: { at: designLength, playOn: clipId },
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
