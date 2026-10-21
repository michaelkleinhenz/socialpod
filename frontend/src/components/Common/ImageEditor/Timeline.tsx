import { useRef, useState, type PointerEvent as ReactPointerEvent } from 'react';
import { Film, Music, Pause, Play, Repeat, SkipBack } from 'lucide-react';
import { formatTime, KEY_SNAP, layerAnim, layerSpan, musicSpan, sortKeys, type LayerAnim } from './animation';
import { clamp, layerName, type AudioTrack, type EditorLayer } from './editorModel';
import { clipLength, isSeqClip, MIN_CLIP, seqClips, transitionLabel, type SeqClip } from './sequence';

interface Props {
  // In drawing order, like the Objects list; shown top first.
  layers: EditorLayer[];
  duration: number;
  time: number;
  playing: boolean;
  // Repeat: playback starts over at the end.
  loop: boolean;
  selectedId: string | null;
  onSeek: (t: number) => void;
  onTogglePlay: () => void;
  onToggleLoop: () => void;
  onSelect: (layer: EditorLayer) => void;
  // A drag of a bar: key keeps the whole drag one undo step.
  onTiming: (layer: EditorLayer, anim: LayerAnim, key: string) => void;
  onDragStart: () => void;
  // The music, a bar of its own, dragged to move where it starts.
  audio: AudioTrack | null;
  onMusicStart: (start: number, key: string) => void;
  onSelectMusic: () => void;
  // The clip sequence, a track of its own: a clip dragged to another place
  // in it, or trimmed by its ends (trimStart into its video, for length).
  onReorderClip: (id: string, index: number, key: string) => void;
  onTrimClip: (id: string, trimStart: number, length: number, key: string) => void;
  // The mark between two clips: picks the transition into the second.
  onPickTransition: (layer: EditorLayer) => void;
}

// Spans snap to a tenth of a second.
const STEP = 0.1;
const snap = (v: number) => Math.round(v / STEP) * STEP;

// The video's timeline under the picture: play and scrub, and a bar per layer
// for the span it shows, dragged to move it or by its ends to change it.
export function Timeline({
  layers, duration, time, playing, loop, selectedId, onSeek, onTogglePlay, onToggleLoop, onSelect, onTiming, onDragStart, audio, onMusicStart, onSelectMusic,
  onReorderClip, onTrimClip, onPickTransition,
}: Props) {
  const tracksRef = useRef<HTMLDivElement>(null);
  // Sequence clips have the sequence's track instead of bars of their own.
  const topFirst = [...layers].reverse().filter(l => !isSeqClip(l));
  const clips = seqClips(layers);
  // The clip being dragged to another place, and where it is held now.
  const [held, setHeld] = useState<{ id: string; start: number } | null>(null);

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
      // Keys count from the start of the span: trimming the start keeps
      // them where they are in the video.
      const keys = mode === 'start' && anim.keys ? anim.keys.map(k => ({ ...k, t: k.t + from.start - start })) : anim.keys;
      // A span reaching the end keeps following the video's length.
      onTiming(l, { ...anim, keys, start, end: end >= duration - 1e-6 ? undefined : end }, key);
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

  // A keyframe's diamond: a click goes to it, a drag moves it along the bar.
  const startKeyDrag = (e: ReactPointerEvent, l: EditorLayer, index: number) => {
    e.stopPropagation();
    e.preventDefault();
    onSelect(l);
    const anim = layerAnim(l);
    const keys = anim.keys ?? [];
    const { start, end } = layerSpan(l, duration);
    const from = keys[index].t;
    const x0 = e.clientX;
    const perPx = secondsPerPx();
    const key = `gesture:key:${l.id}`;
    let moved = false;
    onDragStart();
    const move = (ev: PointerEvent) => {
      if (!moved && Math.abs(ev.clientX - x0) < 3) return;
      moved = true;
      // To a frame, within the span, and not onto another key.
      const t = clamp(Math.round((from + (ev.clientX - x0) * perPx) * 30) / 30, 0, end - start);
      if (keys.some((k, i) => i !== index && Math.abs(k.t - t) <= KEY_SNAP)) return;
      onTiming(l, { ...anim, keys: sortKeys(keys.map((k, i) => (i === index ? { ...k, t } : k))) }, key);
      onSeek(start + t);
    };
    const up = () => {
      if (!moved) onSeek(start + from);
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

  // A sequence clip dragged along its track: it follows the pointer and takes
  // the place in the sequence nearest to where it is held, the others making
  // room as it goes. A press that does not move only selects it.
  const startClipMove = (e: ReactPointerEvent, l: SeqClip) => {
    e.stopPropagation();
    e.preventDefault();
    onSelect(l);
    const others = clips.filter(c => c.id !== l.id).map(clipLength);
    let index = clips.findIndex(c => c.id === l.id);
    const from = layerSpan(l, duration).start;
    const x0 = e.clientX;
    const perPx = secondsPerPx();
    const key = `gesture:sequence:${l.id}`;
    let moved = false;
    const move = (ev: PointerEvent) => {
      if (!moved && Math.abs(ev.clientX - x0) < 3) return;
      if (!moved) onDragStart();
      moved = true;
      const at = clamp(from + (ev.clientX - x0) * perPx, 0, duration);
      setHeld({ id: l.id, start: at });
      // The place whose start is nearest.
      let best = 0;
      let bestGap = Infinity;
      let pos = 0;
      for (let i = 0; i <= others.length; i++) {
        if (Math.abs(pos - at) < bestGap) { bestGap = Math.abs(pos - at); best = i; }
        pos += others[i] ?? 0;
      }
      if (best !== index) {
        index = best;
        onReorderClip(l.id, best, key);
      }
    };
    const up = () => {
      setHeld(null);
      window.removeEventListener('pointermove', move);
      window.removeEventListener('pointerup', up);
      window.removeEventListener('pointercancel', up);
    };
    window.addEventListener('pointermove', move);
    window.addEventListener('pointerup', up);
    window.addEventListener('pointercancel', up);
  };

  // A sequence clip's end dragged: the start trims its head (it starts later
  // in its video), the end its tail; the clips after it follow.
  const startClipTrim = (e: ReactPointerEvent, l: SeqClip, edge: 'start' | 'end') => {
    e.stopPropagation();
    e.preventDefault();
    onSelect(l);
    onDragStart();
    const trim0 = l.clip.trimStart;
    const len0 = clipLength(l);
    const max = l.clip.duration > 0 ? l.clip.duration : Infinity;
    const x0 = e.clientX;
    const perPx = secondsPerPx();
    const key = `gesture:trim:${l.id}`;
    const move = (ev: PointerEvent) => {
      if (Math.abs(ev.clientX - x0) < 3) return;
      const d = snap((ev.clientX - x0) * perPx);
      if (edge === 'start') {
        const trim = clamp(trim0 + d, 0, trim0 + len0 - MIN_CLIP);
        onTrimClip(l.id, trim, len0 - (trim - trim0), key);
      } else {
        onTrimClip(l.id, trim0, clamp(len0 + d, MIN_CLIP, max - trim0), key);
      }
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
    // Nor start a text selection while dragging.
    e.preventDefault();
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
        <button className={`ie-icon-btn${loop ? ' active' : ''}`} onClick={onToggleLoop} title={loop ? 'Repeat on: plays round and round, and you can edit while it plays' : 'Repeat off'} aria-label="Repeat" aria-pressed={loop}>
          <Repeat size={16} />
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
        {clips.length > 0 && (
          <div className={`ie-track clip ie-seq-track ${clips.some(c => c.id === selectedId) ? 'active' : ''}`}>
            <span className="ie-track-name" title="The clips play one after the other: drag one to move it, drag its ends to trim it"><Film size={11} /> Clips</span>
            <div className="ie-track-lane" onPointerDown={seekAt}>
              {clips.map((c, i) => {
                const { start, end } = layerSpan(c, duration);
                const left = held?.id === c.id ? held.start : start;
                const t = c.clip.into;
                return (
                  <div
                    key={c.id}
                    className={`ie-track-span ie-seq-clip${c.id === selectedId ? ' selected' : ''}${held?.id === c.id ? ' held' : ''}`}
                    style={{ left: pct(left), width: pct(Math.max(0.05, end - start)), zIndex: held?.id === c.id ? 3 : 1 }}
                    onPointerDown={e => startClipMove(e, c)}
                    title={`${layerName(c)} ${i + 1}: ${formatTime(end - start)} from ${formatTime(c.clip.trimStart)} into the clip. Drag to move it, drag an end to trim it`}
                  >
                    <span className="ie-seq-label">{i + 1}</span>
                    {t && <span className="ie-track-fx in" style={{ width: `${Math.min(1, t.duration / Math.max(0.1, end - start)) * 100}%` }} />}
                    <span className="ie-track-handle start" onPointerDown={e => startClipTrim(e, c, 'start')} />
                    <span className="ie-track-handle end" onPointerDown={e => startClipTrim(e, c, 'end')} />
                  </div>
                );
              })}
              {!held && clips.slice(1).map(c => {
                const { start } = layerSpan(c, duration);
                const t = c.clip.into;
                const label = transitionLabel(c.clip.transition?.kind);
                return (
                  <button
                    key={`t-${c.id}`}
                    className={`ie-seq-transition${t ? ' set' : ''}`}
                    style={{ left: pct(start + (t?.duration ?? 0) / 2) }}
                    onPointerDown={e => e.stopPropagation()}
                    onClick={() => onPickTransition(c)}
                    title={`Transition: ${label}. Click to change it`}
                    aria-label={`Transition: ${label}`}
                  >
                    {t ? '⇄' : '|'}
                  </button>
                );
              })}
              <span className="ie-playhead" style={{ left: pct(time) }} />
            </div>
          </div>
        )}
        {topFirst.length === 0 && clips.length === 0 && <p className="ie-hint">Add texts, images and shapes; each gets a bar here for when it shows.</p>}
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
                {(a.keys ?? []).map((k, i) => (k.t >= -1e-6 && start + k.t <= end + 1e-6 ? (
                  <span
                    key={i}
                    className={`ie-track-key${Math.abs(start + k.t - time) <= KEY_SNAP ? ' current' : ''}`}
                    style={{ left: pct(start + k.t) }}
                    onPointerDown={e => startKeyDrag(e, l, i)}
                    title={`Keyframe at ${formatTime(start + k.t)}: click to go to it, drag to move it`}
                  />
                ) : null))}
                <span className="ie-playhead" style={{ left: pct(time) }} />
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
}
