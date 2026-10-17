import { useRef, type PointerEvent as ReactPointerEvent } from 'react';
import { Music, Pause, Play, SkipBack } from 'lucide-react';
import { formatTime, layerAnim, layerSpan, musicSpan, type LayerAnim } from './animation';
import { clamp, layerName, type AudioTrack, type EditorLayer } from './editorModel';

interface Props {
  // In drawing order, like the Objects list; shown top first.
  layers: EditorLayer[];
  duration: number;
  time: number;
  playing: boolean;
  selectedId: string | null;
  onSeek: (t: number) => void;
  onTogglePlay: () => void;
  onSelect: (layer: EditorLayer) => void;
  // A drag of a bar: key keeps the whole drag one undo step.
  onTiming: (layer: EditorLayer, anim: LayerAnim, key: string) => void;
  onDragStart: () => void;
  // The music, a bar of its own, dragged to move where it starts.
  audio: AudioTrack | null;
  onMusicStart: (start: number, key: string) => void;
  onSelectMusic: () => void;
}

// Spans snap to a tenth of a second.
const STEP = 0.1;
const snap = (v: number) => Math.round(v / STEP) * STEP;

// The video's timeline under the picture: play and scrub, and a bar per layer
// for the span it shows, dragged to move it or by its ends to change it.
export function Timeline({ layers, duration, time, playing, selectedId, onSeek, onTogglePlay, onSelect, onTiming, onDragStart, audio, onMusicStart, onSelectMusic }: Props) {
  const tracksRef = useRef<HTMLDivElement>(null);
  const topFirst = [...layers].reverse();

  // Seconds per screen pixel along the tracks.
  const secondsPerPx = () => {
    const w = tracksRef.current?.querySelector('.ie-track-lane')?.clientWidth ?? 1;
    return duration / Math.max(1, w);
  };

  const startDrag = (e: ReactPointerEvent, l: EditorLayer, mode: 'move' | 'start' | 'end') => {
    e.stopPropagation();
    e.preventDefault();
    onSelect(l);
    onDragStart();
    const from = layerSpan(l, duration);
    const anim = layerAnim(l);
    const x0 = e.clientX;
    const perPx = secondsPerPx();
    const key = `gesture:timing:${l.id}`;
    let moved = false;
    const move = (ev: PointerEvent) => {
      const d = (ev.clientX - x0) * perPx;
      if (!moved && Math.abs(ev.clientX - x0) < 3) return;
      moved = true;
      let start = from.start;
      let end = from.end;
      if (mode === 'move') {
        const len = end - start;
        start = clamp(snap(from.start + d), 0, duration - len);
        end = start + len;
      } else if (mode === 'start') {
        start = clamp(snap(from.start + d), 0, end - STEP);
      } else {
        end = clamp(snap(from.end + d), start + STEP, duration);
      }
      // A span reaching the end keeps following the video's length.
      onTiming(l, { ...anim, start, end: end >= duration - 1e-6 ? undefined : end }, key);
    };
    const up = () => {
      window.removeEventListener('pointermove', move);
      window.removeEventListener('pointerup', up);
      window.removeEventListener('pointercancel', up);
    };
    window.addEventListener('pointermove', move);
    window.addEventListener('pointerup', up);
    window.addEventListener('pointercancel', up);
  };

  const startMusicDrag = (e: ReactPointerEvent) => {
    if (!audio) return;
    e.stopPropagation();
    e.preventDefault();
    onSelectMusic();
    onDragStart();
    const from = audio.start;
    const x0 = e.clientX;
    const perPx = secondsPerPx();
    const move = (ev: PointerEvent) => {
      if (Math.abs(ev.clientX - x0) < 3) return;
      onMusicStart(clamp(snap(from + (ev.clientX - x0) * perPx), 0, duration - STEP), 'gesture:music');
    };
    const up = () => {
      window.removeEventListener('pointermove', move);
      window.removeEventListener('pointerup', up);
      window.removeEventListener('pointercancel', up);
    };
    window.addEventListener('pointermove', move);
    window.addEventListener('pointerup', up);
    window.addEventListener('pointercancel', up);
  };

  // A press on the ruler or an empty part of a lane moves the playhead.
  const seekAt = (e: ReactPointerEvent<HTMLDivElement>) => {
    const lane = e.currentTarget.getBoundingClientRect();
    const to = (x: number) => clamp(((x - lane.left) / lane.width) * duration, 0, duration);
    onSeek(to(e.clientX));
    const move = (ev: PointerEvent) => onSeek(to(ev.clientX));
    const up = () => {
      window.removeEventListener('pointermove', move);
      window.removeEventListener('pointerup', up);
      window.removeEventListener('pointercancel', up);
    };
    window.addEventListener('pointermove', move);
    window.addEventListener('pointerup', up);
    window.addEventListener('pointercancel', up);
  };

  const pct = (t: number) => `${(t / duration) * 100}%`;
  const ticks = Array.from({ length: Math.floor(duration) + 1 }, (_, i) => i).filter(i => duration <= 20 || i % 5 === 0);

  return (
    <div className="ie-timeline" ref={tracksRef}>
      <div className="ie-timeline-bar">
        <button className="ie-icon-btn" onClick={() => onSeek(0)} title="Back to the start" aria-label="Back to the start">
          <SkipBack size={16} />
        </button>
        <button className="ie-icon-btn active" onClick={onTogglePlay} title={playing ? 'Pause (Space)' : 'Play (Space)'} aria-label={playing ? 'Pause' : 'Play'}>
          {playing ? <Pause size={18} /> : <Play size={18} />}
        </button>
        <span className="ie-timeline-time">{formatTime(time)} / {formatTime(duration)}</span>
      </div>
      <div className="ie-tracks">
        <div className="ie-track">
          <span className="ie-track-name" />
          <div className="ie-track-lane ie-ruler" onPointerDown={seekAt}>
            {ticks.map(i => (
              // The first and last stay inside the lane.
              <span key={i} className="ie-tick" style={{ left: pct(i), transform: `translateX(${i === 0 ? 0 : i >= duration ? -100 : -50}%)` }}>{i}</span>
            ))}
            <span className="ie-playhead" style={{ left: pct(time) }} />
          </div>
        </div>
        {audio && (() => {
          const m = musicSpan(audio, duration);
          return (
            <div className="ie-track">
              <button className="ie-track-name" onClick={onSelectMusic} title={audio.name}><Music size={11} /> {audio.name}</button>
              <div className="ie-track-lane" onPointerDown={seekAt}>
                <div
                  className="ie-track-span music"
                  style={{ left: pct(m.start), width: pct(Math.max(STEP, m.end - m.start)) }}
                  onPointerDown={startMusicDrag}
                  title={`Music from ${formatTime(m.start)} to ${formatTime(m.end)}: drag to move`}
                />
                <span className="ie-playhead" style={{ left: pct(time) }} />
              </div>
            </div>
          );
        })()}
        {topFirst.length === 0 && <p className="ie-hint">Add texts, images and shapes; each gets a bar here for when it shows.</p>}
        {topFirst.map(l => {
          const { start, end } = layerSpan(l, duration);
          const a = layerAnim(l);
          // Effects as a share of the bar.
          const share = (len: number) => `${Math.min(1, len / Math.max(STEP, end - start)) * 100}%`;
          return (
            <div key={l.id} className={`ie-track ${l.id === selectedId ? 'active' : ''} ${l.kind === 'image' && l.clip ? 'clip' : ''}`}>
              <button className="ie-track-name" onClick={() => onSelect(l)} title={layerName(l)}>{layerName(l)}</button>
              <div className="ie-track-lane" onPointerDown={seekAt}>
                <div
                  className="ie-track-span"
                  style={{ left: pct(start), width: pct(end - start) }}
                  onPointerDown={e => startDrag(e, l, 'move')}
                  title={`${formatTime(start)} to ${formatTime(end)}: drag to move, drag an end to change`}
                >
                  {a.enter !== 'none' && <span className="ie-track-fx in" style={{ width: share(a.enterDuration) }} />}
                  {a.exit !== 'none' && <span className="ie-track-fx out" style={{ width: share(a.exitDuration) }} />}
                  <span className="ie-track-handle start" onPointerDown={e => startDrag(e, l, 'start')} />
                  <span className="ie-track-handle end" onPointerDown={e => startDrag(e, l, 'end')} />
                </div>
                <span className="ie-playhead" style={{ left: pct(time) }} />
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
}
