import Konva from 'konva';
import { BufferTarget, CanvasSource, canEncodeVideo, Mp4OutputFormat, Output, QUALITY_HIGH } from 'mediabunny';
import { animStateAt, buildAnimatedLayerNode, FPS } from './animation';
import { applyAdjustments, type Adjustments, type CropBox, type EditorLayer } from './editorModel';

export interface VideoInput {
  // The photo under the layers, or null for a blank canvas.
  source: HTMLImageElement | null;
  background?: string;
  crop: CropBox;
  adjustments: Adjustments;
  layers: EditorLayer[];
  duration: number;
  // Longest side of the video; 1920 is Instagram's own size for a reel.
  maxSide?: number;
  onProgress?: (share: number) => void;
  signal?: AbortSignal;
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

// Renders the design frame by frame and encodes it as an H.264 MP4 with its
// index up front, the way Instagram wants a reel. Frames are drawn offline,
// one after the other, so the video is exact however slow the machine is.
export async function renderVideo({
  source, background = '#ffffff', crop, adjustments, layers, duration, maxSide = 1920, onProgress, signal,
}: VideoInput): Promise<Blob> {
  const { width, height, scale } = videoSize(crop, maxSide);

  const container = document.createElement('div');
  const stage = new Konva.Stage({ container, width, height });
  const output = new Output({ format: new Mp4OutputFormat({ fastStart: 'in-memory' }), target: new BufferTarget() });
  let started = false;
  try {
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
    await output.start();
    started = true;

    const frames = Math.max(1, Math.round(duration * FPS));
    for (let i = 0; i < frames; i++) {
      if (signal?.aborted) throw new DOMException('Export cancelled', 'AbortError');
      const t = i / FPS;
      frameLayers.destroyChildren();
      for (const l of layers) {
        const s = animStateAt(l, t, duration, crop);
        if (s.visible) frameLayers.add(buildAnimatedLayerNode(l, s));
      }
      layer.draw();
      ctx.clearRect(0, 0, width, height);
      ctx.drawImage(layer.getCanvas()._canvas, 0, 0);
      await videoSource.add(t, 1 / FPS);
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
    stage.destroy();
  }
}
