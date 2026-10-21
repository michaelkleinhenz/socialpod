import { useEffect, useRef, useState } from 'react';
import { ALL_FORMATS, BlobSource, CanvasSink, Input } from 'mediabunny';
import { Pause, Play } from 'lucide-react';
import { MAX_DURATION, MIN_DURATION } from '../Common/ImageEditor/animation';
import { clamp } from '../Common/ImageEditor/editorModel';

// Where the reel cuts into the clip and out of it, picked on a filmstrip of
// the clip: the two handles set the lead in and lead out (the window between
// them drags both), the preview above shows the frame under a handle while
// it moves, and Play plays the part kept, over and over.

export interface ClipRange {
  start: number;
  end: number;
}

// A reel is 3 to 180 s, as far as the clip allows.
const minLength = (duration: number) => Math.min(MIN_DURATION, duration);

const THUMBS = 10;
const THUMB_HEIGHT = 112;

// Frames spread over the clip, decoded from the file itself.
async function filmstrip(file: Blob, duration: number, signal: { cancelled: boolean }): Promise<string[]> {
  const input = new Input({ source: new BlobSource(file), formats: ALL_FORMATS });
  try {
    const track = await input.getPrimaryVideoTrack();
    if (!track) return [];
    const first = await track.getFirstTimestamp();
    const sink = new CanvasSink(track, { height: THUMB_HEIGHT });
    const times = Array.from({ length: THUMBS }, (_, i) => first + (duration * (i + 0.5)) / THUMBS);
    const urls: string[] = [];
    for await (const frame of sink.canvasesAtTimestamps(times)) {
      if (signal.cancelled) break;
      if (!frame) { urls.push(''); continue; }
      const c = document.createElement('canvas');
      c.width = frame.canvas.width;
      c.height = frame.canvas.height;
      c.getContext('2d')?.drawImage(frame.canvas, 0, 0);
      urls.push(c.toDataURL('image/jpeg', 0.7));
    }
    return urls;
  } finally {
    input.dispose();
  }
}

type Drag = { part: 'start' | 'end' | 'window'; from: number; range: ClipRange };

const fmt = (t: number) => {
  const m = Math.floor(t / 60);
  const s = t - m * 60;
  return m > 0 ? `${m}:${s.toFixed(1).padStart(4, '0')}` : `${s.toFixed(1)}s`;
};

export function ClipTrimmer({ file, src, duration, range, onChange }: {
  file: Blob;
  // An object URL of file, for the preview.
  src: string;
  duration: number;
  range: ClipRange;
  onChange: (r: ClipRange) => void;
}) {
  const videoRef = useRef<HTMLVideoElement>(null);
  const stripRef = useRef<HTMLDivElement>(null);
  const dragRef = useRef<Drag | null>(null);
  const rangeRef = useRef(range);
  useEffect(() => { rangeRef.current = range; }, [range]);
  const [thumbs, setThumbs] = useState<string[]>([]);
  const [playing, setPlaying] = useState(false);
  const [playhead, setPlayhead] = useState(range.start);

  useEffect(() => {
    const signal = { cancelled: false };
    filmstrip(file, duration, signal)
      .then(urls => { if (!signal.cancelled) setThumbs(urls); })
      // Without frames the strip is a plain bar; trimming still works.
      .catch(() => {});
    return () => { signal.cancelled = true; };
  }, [file, duration]);

  // Plays the kept part, from its start again once it reaches the end.
  useEffect(() => {
    if (!playing) return;
    const v = videoRef.current;
    if (!v) return;
    let raf = 0;
    const tick = () => {
      const { start, end } = rangeRef.current;
      if (v.currentTime >= end || v.currentTime < start - 0.1) v.currentTime = start;
      setPlayhead(v.currentTime);
      raf = requestAnimationFrame(tick);
    };
    raf = requestAnimationFrame(tick);
    return () => cancelAnimationFrame(raf);
  }, [playing]);

  const seek = (t: number) => {
    const v = videoRef.current;
    if (v) v.currentTime = t;
    setPlayhead(t);
  };

  const togglePlay = () => {
    const v = videoRef.current;
    if (!v) return;
    if (playing) {
      v.pause();
      setPlaying(false);
      return;
    }
    if (v.currentTime < range.start || v.currentTime >= range.end - 0.05) v.currentTime = range.start;
    void v.play().then(() => setPlaying(true)).catch(() => setPlaying(false));
  };

  const stop = () => {
    videoRef.current?.pause();
    setPlaying(false);
  };

  // The range with one part moved, kept within the clip and the reel's limits.
  const moved = (part: Drag['part'], base: ClipRange, delta: number): ClipRange => {
    const min = minLength(duration);
    if (part === 'window') {
      const length = base.end - base.start;
      const start = clamp(base.start + delta, 0, duration - length);
      return { start, end: start + length };
    }
    if (part === 'start') {
      const start = clamp(base.start + delta, Math.max(0, base.end - MAX_DURATION), base.end - min);
      return { start, end: base.end };
    }
    const end = clamp(base.end + delta, base.start + min, Math.min(duration, base.start + MAX_DURATION));
    return { start: base.start, end };
  };

  const timeAt = (clientX: number) => {
    const rect = stripRef.current?.getBoundingClientRect();
    return rect && rect.width > 0 ? ((clientX - rect.left) / rect.width) * duration : 0;
  };

  const onPointerDown = (part: Drag['part']) => (e: React.PointerEvent) => {
    e.preventDefault();
    e.stopPropagation();
    stop();
    (e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
    dragRef.current = { part, from: timeAt(e.clientX), range };
    seek(part === 'end' ? range.end : range.start);
  };

  const onPointerMove = (e: React.PointerEvent) => {
    const d = dragRef.current;
    if (!d) return;
    const next = moved(d.part, d.range, timeAt(e.clientX) - d.from);
    onChange(next);
    seek(d.part === 'end' ? next.end : next.start);
  };

  const onPointerUp = () => {
    const d = dragRef.current;
    dragRef.current = null;
    if (d) seek(rangeRef.current.start);
  };

  // A tap on the strip outside the handles shows that frame.
  const onStripPointerDown = (e: React.PointerEvent) => {
    stop();
    seek(clamp(timeAt(e.clientX), 0, duration));
  };

  const onKey = (part: 'start' | 'end') => (e: React.KeyboardEvent) => {
    const step = e.shiftKey ? 1 : 0.1;
    const delta = e.key === 'ArrowLeft' || e.key === 'ArrowDown' ? -step : e.key === 'ArrowRight' || e.key === 'ArrowUp' ? step : 0;
    if (!delta) return;
    e.preventDefault();
    stop();
    const next = moved(part, range, delta);
    onChange(next);
    seek(part === 'end' ? next.end : next.start);
  };

  const pct = (t: number) => `${(t / duration) * 100}%`;

  return (
    <div className="qr-trim">
      <video
        ref={videoRef}
        className="qr-video qr-trim-video"
        src={src}
        playsInline
        preload="auto"
        onLoadedMetadata={e => { e.currentTarget.currentTime = range.start; }}
        onPause={() => setPlaying(false)}
        onClick={togglePlay}
      />
      <div className="qr-trim-bar">
        <button type="button" className="btn btn-secondary btn-sm qr-trim-play" onClick={togglePlay} aria-label={playing ? 'Pause' : 'Play the part kept'}>
          {playing ? <Pause size={16} /> : <Play size={16} />}
        </button>
        <div
          ref={stripRef}
          className="qr-trim-strip"
          onPointerDown={onStripPointerDown}
          onPointerMove={onPointerMove}
          onPointerUp={onPointerUp}
          onPointerCancel={onPointerUp}
        >
          <div className="qr-trim-thumbs">
            {Array.from({ length: THUMBS }, (_, i) => (
              <div key={i} className="qr-trim-thumb" style={thumbs[i] ? { backgroundImage: `url(${thumbs[i]})` } : undefined} />
            ))}
          </div>
          <div className="qr-trim-shade" style={{ left: 0, width: pct(range.start) }} />
          <div className="qr-trim-shade" style={{ left: pct(range.end), right: 0 }} />
          <div
            className="qr-trim-window"
            style={{ left: pct(range.start), width: pct(range.end - range.start) }}
            onPointerDown={onPointerDown('window')}
          />
          <div
            className="qr-trim-handle start"
            style={{ left: pct(range.start) }}
            role="slider"
            tabIndex={0}
            aria-label="Lead in"
            aria-valuemin={0}
            aria-valuemax={duration}
            aria-valuenow={range.start}
            aria-valuetext={fmt(range.start)}
            onPointerDown={onPointerDown('start')}
            onKeyDown={onKey('start')}
          />
          <div
            className="qr-trim-handle end"
            style={{ left: pct(range.end) }}
            role="slider"
            tabIndex={0}
            aria-label="Lead out"
            aria-valuemin={0}
            aria-valuemax={duration}
            aria-valuenow={range.end}
            aria-valuetext={fmt(range.end)}
            onPointerDown={onPointerDown('end')}
            onKeyDown={onKey('end')}
          />
          <div className="qr-trim-playhead" style={{ left: pct(playhead) }} />
        </div>
      </div>
      <div className="qr-trim-times">
        <span>In {fmt(range.start)}</span>
        <strong>{fmt(range.end - range.start)}</strong>
        <span>Out {fmt(range.end)}</span>
      </div>
    </div>
  );
}
