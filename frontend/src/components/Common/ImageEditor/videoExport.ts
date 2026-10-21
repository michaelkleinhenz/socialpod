import Konva from 'konva';
import {
  ALL_FORMATS, AudioBufferSource, BlobSource, BufferTarget, CanvasSink, CanvasSource, canEncodeAudio, canEncodeVideo, Input, Mp4OutputFormat, Output,
  QUALITY_HIGH, type WrappedCanvas,
} from 'mediabunny';
import {
  animStateAt, buildAnimatedLayerNode, clipFadeIn, clipFadeOut, clipTimeAt, equalizerLevels, FPS, isEqualizer, layerSpan, musicSpan,
} from './animation';
import { audioLevels } from './equalizer';
import { applyAdjustments, type Adjustments, type AudioTrack, type CropBox, type EditorLayer, type ImageLayer } from './editorModel';

export interface VideoInput {
  // The photo under the layers, or null for a blank canvas.
  source: HTMLImageElement | null;
  background?: string;
  crop: CropBox;
  adjustments: Adjustments;
  layers: EditorLayer[];
  duration: number;
  // The music under the clips' own sound.
  audio?: AudioTrack | null;
  // Holds the design at its frame at hold.at (seconds) from then on: its
  // layers' timing and effects run over hold.at as if that were the video's
  // length, and its clips pause there, except the layer hold.playOn names,
  // which runs the whole video. The music plays on.
  hold?: { at: number; playOn: string };
  // Turns a stored URL (/api/uploads/...) into one to fetch.
  resolve: (url: string) => string;
  // Longest side of the video; 1920 is Instagram's own size for a reel.
  maxSide?: number;
  onProgress?: (share: number) => void;
  signal?: AbortSignal;
}

// The sample rate of the sound track.
const SAMPLE_RATE = 48000;

// The time and length layer l is animated by at time t of the video:
// clamped to the hold for a held layer.
type Clock = (l: EditorLayer, t: number) => { t: number; duration: number };
const plainClock = (duration: number): Clock => (_l, t) => ({ t, duration });
function holdClock(duration: number, hold: VideoInput['hold']): Clock {
  if (!hold) return plainClock(duration);
  const at = Math.min(hold.at, duration);
  return (l, t) => (l.id === hold.playOn ? { t, duration } : { t: Math.min(t, at), duration: at });
}

// The video's size: the crop scaled to fit maxSide, both sides even, as
// H.264 in 4:2:0 needs.
function videoSize(crop: CropBox, maxSide: number) {
  const scale = Math.min(1, maxSide / Math.max(crop.width, crop.height));
  const even = (v: number) => Math.max(2, Math.round(v / 2) * 2);
  return { width: even(crop.width * scale), height: even(crop.height * scale), scale };
}

// Whether this browser can make the video at all (WebCodecs with H.264).
export async function canExportVideo(crop: CropBox, maxSide = 1920): Promise<boolean> {
  if (typeof VideoEncoder === 'undefined') return false;
  const { width, height } = videoSize(crop, maxSide);
  try {
    return await canEncodeVideo('avc', { width, height, bitrate: QUALITY_HIGH });
  } catch {
    return false;
  }
}

const aacEncodable = () =>
  canEncodeAudio('aac', { numberOfChannels: 2, sampleRate: SAMPLE_RATE }).catch(() => false);

// Whether the sound track can be encoded. Browsers without an AAC encoder of
// their own (Chromium on Linux, Brave, Firefox) get FFmpeg's, built to WASM,
// which is loaded only then.
let aacFallback: Promise<void> | null = null;
async function canEncodeAac(): Promise<boolean> {
  if (await aacEncodable()) return true;
  aacFallback ??= import('@mediabunny/aac-encoder').then(m => m.registerAacEncoder());
  try {
    await aacFallback;
  } catch {
    aacFallback = null;
    return false;
  }
  return aacEncodable();
}

const isClip = (l: EditorLayer): l is ImageLayer & { clip: NonNullable<ImageLayer['clip']> } => l.kind === 'image' && !!l.clip;

// Each file fetched once, however many layers use it.
function fetcher(resolve: (url: string) => string, signal?: AbortSignal) {
  const files = new Map<string, Promise<Blob>>();
  return (src: string) => {
    let p = files.get(src);
    if (!p) {
      p = fetch(resolve(src), { signal }).then(r => {
        if (!r.ok) throw new Error(`Failed to load ${src.split('/').pop()}`);
        return r.blob();
      });
      files.set(src, p);
    }
    return p;
  };
}

// Mixes the clips' sound and the music into one track, as the preview plays
// it: each clip from its trim point while its layer shows, faded with the
// layer's in and out effects, the music with its own fades. Null when there
// is nothing to hear.
async function mixAudio(
  layers: EditorLayer[], audio: AudioTrack | null | undefined, duration: number, clock: Clock, load: (src: string) => Promise<Blob>,
  { channels = 2, sampleRate = SAMPLE_RATE } = {},
): Promise<AudioBuffer | null> {
  const ctx = new OfflineAudioContext(channels, Math.ceil(duration * sampleRate), sampleRate);
  // Decoding needs its own copy: decodeAudioData takes the buffer over.
  const decode = async (src: string) => {
    try {
      return await ctx.decodeAudioData(await (await load(src)).arrayBuffer());
    } catch {
      // A clip without sound, or a format this browser cannot decode.
      return null;
    }
  };
  // Plays buffer from offset at when for length seconds, through a gain that
  // ramps up over fadeIn and down over fadeOut.
  const play = (buffer: AudioBuffer, when: number, offset: number, length: number, volume: number, fadeIn: number, fadeOut: number) => {
    length = Math.min(length, buffer.duration - offset);
    if (length <= 0 || volume <= 0) return false;
    const node = ctx.createBufferSource();
    node.buffer = buffer;
    const gain = ctx.createGain();
    // A few milliseconds of ramp even without a fade, so nothing clicks.
    const fi = Math.max(fadeIn, 0.01);
    const fo = Math.max(fadeOut, 0.01);
    gain.gain.setValueAtTime(0, when);
    gain.gain.linearRampToValueAtTime(volume, when + Math.min(fi, length / 2));
    gain.gain.setValueAtTime(volume, Math.max(when + Math.min(fi, length / 2), when + length - Math.min(fo, length / 2)));
    gain.gain.linearRampToValueAtTime(0, when + length);
    node.connect(gain).connect(ctx.destination);
    node.start(when, offset, length);
    return true;
  };

  let any = false;
  for (const l of layers) {
    if (!isClip(l) || l.clip.volume <= 0) continue;
    const buffer = await decode(l.src);
    if (!buffer) continue;
    // A held clip falls silent where it pauses.
    const { start, end } = layerSpan(l, clock(l, duration).duration);
    any = play(buffer, start, l.clip.trimStart, end - start, l.clip.volume, clipFadeIn(l, start, end), clipFadeOut(l, start, end)) || any;
  }
  if (audio) {
    const buffer = await decode(audio.src);
    if (!buffer) throw new Error(`The music "${audio.name}" cannot be played in this browser`);
    const { start, end } = musicSpan(audio, duration);
    any = play(buffer, start, audio.trimStart, end - start, audio.volume, audio.fadeIn, audio.fadeOut) || any;
  }
  return any ? ctx.startRendering() : null;
}

// What auto-captions transcribe, as the video plays it, so the words' times
// are times of the video: the clips' sound alone, without the music, which is
// usually a song. A video whose clips are silent (or that has none, a reel of
// only sound such as a podcast snippet) has its music transcribed instead.
// 16 kHz mono WAV, which speech recognition needs no more than. Null when
// neither has any sound.
export async function speechAudio(
  layers: EditorLayer[], audio: AudioTrack | null | undefined, duration: number, resolve: (url: string) => string, signal?: AbortSignal,
): Promise<Blob | null> {
  const load = fetcher(resolve, signal);
  const options = { channels: 1, sampleRate: 16000 };
  let buffer = await mixAudio(layers, null, duration, plainClock(duration), load, options);
  if ((!buffer || isSilent(buffer)) && audio) buffer = await mixAudio([], audio, duration, plainClock(duration), load, options);
  if (!buffer || isSilent(buffer)) return null;
  return wavBlob(buffer);
}

// Whether a mix is (practically) silent, e.g. clips whose files carry an
// empty sound track.
function isSilent(buffer: AudioBuffer): boolean {
  const data = buffer.getChannelData(0);
  for (let i = 0; i < data.length; i++) if (Math.abs(data[i]) > 0.003) return false;
  return true;
}

// A mono buffer as a 16-bit PCM WAV file.
function wavBlob(buffer: AudioBuffer): Blob {
  const data = buffer.getChannelData(0);
  const view = new DataView(new ArrayBuffer(44 + data.length * 2));
  const text = (at: number, s: string) => { for (let i = 0; i < s.length; i++) view.setUint8(at + i, s.charCodeAt(i)); };
  text(0, 'RIFF');
  view.setUint32(4, 36 + data.length * 2, true);
  text(8, 'WAVE');
  text(12, 'fmt ');
  view.setUint32(16, 16, true);
  view.setUint16(20, 1, true);
  view.setUint16(22, 1, true);
  view.setUint32(24, buffer.sampleRate, true);
  view.setUint32(28, buffer.sampleRate * 2, true);
  view.setUint16(32, 2, true);
  view.setUint16(34, 16, true);
  text(36, 'data');
  view.setUint32(40, data.length * 2, true);
  for (let i = 0; i < data.length; i++) {
    const v = Math.max(-1, Math.min(1, data[i]));
    view.setInt16(44 + i * 2, v < 0 ? v * 0x8000 : v * 0x7fff, true);
  }
  return new Blob([view], { type: 'audio/wav' });
}

// A clip's frames for the video, one per frame in which its layer shows, in
// order: decoded from the file itself, so they are exact whatever the
// browser's video element would show.
async function clipFrames(l: ImageLayer, frames: number, clock: Clock, load: (src: string) => Promise<Blob>) {
  const input = new Input({ source: new BlobSource(await load(l.src)), formats: ALL_FORMATS });
  const track = await input.getPrimaryVideoTrack();
  if (!track) throw new Error('A clip has no video in it');
  const first = await track.getFirstTimestamp();
  const times: number[] = [];
  for (let i = 0; i < frames; i++) {
    const { t, duration } = clock(l, i / FPS);
    const { start, end } = layerSpan(l, duration);
    if (t >= start && t <= end) times.push(first + clipTimeAt(l, t, duration));
  }
  const sink = new CanvasSink(track, { poolSize: 2 });
  return { input, frames: sink.canvasesAtTimestamps(times) };
}

// Renders the design frame by frame and encodes it as an H.264 MP4 with its
// index up front, the way Instagram wants a reel, with an AAC sound track
// when a clip or the music has sound. Frames are drawn offline, one after the
// other, so the video is exact however slow the machine is.
export async function renderVideo({
  source, background = '#ffffff', crop, adjustments, layers, duration, audio, hold, resolve, maxSide = 1920, onProgress, signal,
}: VideoInput): Promise<Blob> {
  const { width, height, scale } = videoSize(crop, maxSide);
  const frames = Math.max(1, Math.round(duration * FPS));
  const load = fetcher(resolve, signal);
  const clock = holdClock(duration, hold);

  const sound = await mixAudio(layers, audio, duration, clock, load);
  // An equalizer follows the music, which plays on through a hold.
  const levels = audio && layers.some(isEqualizer) ? await audioLevels(resolve(audio.src)) : null;
  if (sound && !(await canEncodeAac())) {
    throw new Error('This browser cannot encode the sound (AAC). Use a current Chrome, Edge or Safari.');
  }

  const clips = new Map<string, Awaited<ReturnType<typeof clipFrames>>>();
  const container = document.createElement('div');
  const stage = new Konva.Stage({ container, width, height });
  const output = new Output({ format: new Mp4OutputFormat({ fastStart: 'in-memory' }), target: new BufferTarget() });
  let started = false;
  try {
    for (const l of layers) {
      if (isClip(l)) clips.set(l.id, await clipFrames(l, frames, clock, load));
    }

    const layer = new Konva.Layer({ listening: false });
    stage.add(layer);
    // One canvas pixel per video pixel, whatever the screen's density.
    layer.getCanvas().setPixelRatio(1);
    layer.getCanvas().setSize(width, height);
    const group = new Konva.Group({ x: -crop.x * scale, y: -crop.y * scale, scaleX: scale, scaleY: scale });
    layer.add(group);

    // The base is drawn once; only the layers change from frame to frame.
    if (source) {
      const base = new Konva.Image({ image: source, width: source.naturalWidth, height: source.naturalHeight });
      group.add(base);
      applyAdjustments(base, adjustments, scale);
    } else {
      group.add(new Konva.Rect({ x: crop.x, y: crop.y, width: crop.width, height: crop.height, fill: background }));
    }
    const frameLayers = new Konva.Group();
    group.add(frameLayers);

    const canvas = document.createElement('canvas');
    canvas.width = width;
    canvas.height = height;
    const ctx = canvas.getContext('2d');
    if (!ctx) throw new Error('Failed to prepare the video canvas');

    const videoSource = new CanvasSource(canvas, { codec: 'avc', bitrate: QUALITY_HIGH, keyFrameInterval: 2 });
    output.addVideoTrack(videoSource, { frameRate: FPS });
    const audioSource = sound ? new AudioBufferSource({ codec: 'aac', bitrate: QUALITY_HIGH }) : null;
    if (audioSource) output.addAudioTrack(audioSource);
    await output.start();
    started = true;
    if (audioSource && sound) {
      await audioSource.add(sound);
      audioSource.close();
    }

    // The frame a clip shows now; kept when the decoder has none for a time.
    const lastFrame = new Map<string, WrappedCanvas>();
    for (let i = 0; i < frames; i++) {
      if (signal?.aborted) throw new DOMException('Export cancelled', 'AbortError');
      const time = i / FPS;
      frameLayers.destroyChildren();
      for (const l of layers) {
        const { t, duration: length } = clock(l, time);
        const { start, end } = layerSpan(l, length);
        const clip = clips.get(l.id);
        let drawn: EditorLayer = l;
        if (clip && t >= start && t <= end) {
          const next = await clip.frames.next();
          const frame = (!next.done && next.value) || lastFrame.get(l.id);
          if (frame) {
            lastFrame.set(l.id, frame);
            drawn = { ...l, img: frame.canvas } as ImageLayer;
          }
        }
        const s = animStateAt(l, t, length, crop);
        if (s.visible) frameLayers.add(buildAnimatedLayerNode(drawn, s, isEqualizer(l) ? equalizerLevels(l, time, duration, audio, levels) : undefined));
      }
      layer.draw();
      ctx.clearRect(0, 0, width, height);
      ctx.drawImage(layer.getCanvas()._canvas, 0, 0);
      await videoSource.add(time, 1 / FPS);
      onProgress?.((i + 1) / frames);
    }
    videoSource.close();
    await output.finalize();
    const buffer = (output.target as BufferTarget).buffer;
    if (!buffer) throw new Error('Failed to encode the video');
    return new Blob([buffer], { type: 'video/mp4' });
  } catch (err) {
    if (started) await output.cancel().catch(() => {});
    throw err;
  } finally {
    for (const c of clips.values()) {
      await c.frames.return(undefined).catch(() => {});
      c.input.dispose();
    }
    stage.destroy();
  }
}
