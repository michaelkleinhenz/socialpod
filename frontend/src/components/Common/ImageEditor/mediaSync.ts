import { clipGainAt, clipTimeAt, layerSpan, musicGainAt, musicSpan, musicTimeAt } from './animation';
import { clamp, type AudioTrack, type EditorLayer } from './editorModel';

// Plays or still-frames the preview's clip videos and music at time t of the
// video: while it plays, every clip that shows and the music play along, at
// the volume the export mixes them at, pulled back when they drift; paused,
// each clip shows its frame at t. The export does not use this; it decodes
// the files itself (videoExport.ts).
//
// known collects the clip videos seen, so one that left the design (removed,
// replaced) is paused too, and each gets redraw on seeking, since the canvas
// only draws a video's new frame when asked.
export function syncMedia(
  layers: EditorLayer[], audio: AudioTrack | null, music: HTMLAudioElement | null,
  t: number, duration: number, playing: boolean, known: Set<HTMLVideoElement>, redraw: () => void,
) {
  const live = new Set<HTMLVideoElement>();
  for (const l of layers) {
    if (l.kind !== 'image' || !l.clip || !(l.img instanceof HTMLVideoElement)) continue;
    const v = l.img;
    live.add(v);
    if (!known.has(v)) {
      known.add(v);
      v.addEventListener('seeked', redraw);
    }
    const { start, end } = layerSpan(l, duration);
    const want = clipTimeAt(l, t, duration);
    if (playing && t >= start && t < end) {
      v.volume = clamp(clipGainAt(l, t, duration), 0, 1);
      if (v.paused && !v.ended) {
        v.currentTime = want;
        void v.play().catch(() => {});
      } else if (Math.abs(v.currentTime - want) > 0.3) {
        v.currentTime = want;
      }
    } else {
      if (!v.paused) v.pause();
      if (Math.abs(v.currentTime - want) > 0.02) v.currentTime = want;
    }
  }
  for (const v of known) {
    if (!live.has(v) && !v.paused) v.pause();
  }

  if (!music) return;
  const span = audio ? musicSpan(audio, duration) : null;
  if (audio && span && playing && t >= span.start && t < span.end) {
    const want = musicTimeAt(audio, t, duration);
    music.volume = clamp(musicGainAt(audio, t, duration), 0, 1);
    if (music.paused) {
      music.currentTime = want;
      void music.play().catch(() => {});
    } else if (Math.abs(music.currentTime - want) > 0.3) {
      music.currentTime = want;
    }
  } else if (!music.paused) {
    music.pause();
  }
}

// Stops everything the preview played, for good: the editor is closing.
export function stopMedia(known: Set<HTMLVideoElement>, music: HTMLAudioElement | null) {
  for (const v of known) {
    v.pause();
    v.removeAttribute('src');
    v.load();
  }
  known.clear();
  music?.pause();
}

// A sound file's length in seconds, 0 if it cannot be read. Some files (an
// Ogg stream, say) do not say how long they are; those are decoded to find
// out.
export async function soundDuration(url: string): Promise<number> {
  const told = await new Promise<number>(resolve => {
    const a = new Audio();
    a.preload = 'metadata';
    a.onloadedmetadata = () => resolve(Number.isFinite(a.duration) ? a.duration : 0);
    a.onerror = () => resolve(0);
    a.src = url;
  });
  if (told > 0) return told;
  try {
    const ctx = new OfflineAudioContext(1, 1, 44100);
    const buffer = await ctx.decodeAudioData(await (await fetch(url)).arrayBuffer());
    return buffer.duration;
  } catch {
    return 0;
  }
}
