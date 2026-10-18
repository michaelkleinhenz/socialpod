import { useEffect, useRef, useState, type PointerEvent } from 'react';
import Konva from 'konva';
import { Film, Loader2 } from 'lucide-react';
import { drawSystemFrame, SYSTEM_FORMATS, systemTemplateLayers, systemTemplatePreview, type SystemTemplate } from './systemTemplates';

interface Props {
  template: SystemTemplate;
  // In a video: the template plays while the pointer is over it.
  animate: boolean;
  loading: boolean;
  disabled: boolean;
  onPick: () => void;
}

// A built-in template in the Templates tab: its preview drawn the first time
// it shows, and in a video the template itself playing on hover (or keyboard
// focus), drawn frame by frame like the editor's export.
export function SystemTemplateTile({ template, animate, loading, disabled, onPick }: Props) {
  const [preview, setPreview] = useState<string | null>(null);
  const [playing, setPlaying] = useState(false);
  const hostRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    let cancelled = false;
    systemTemplatePreview(template).then(url => { if (!cancelled) setPreview(url); }).catch(() => {});
    return () => { cancelled = true; };
  }, [template]);

  useEffect(() => {
    const host = hostRef.current;
    if (!playing || !animate || !host) return;
    let stopped = false;
    let frame = 0;
    let stage: Konva.Stage | null = null;
    void systemTemplateLayers(template, true).then(({ template: design, layers }) => {
      if (stopped) return;
      const width = host.clientWidth;
      const scale = width / design.width;
      stage = new Konva.Stage({ container: host, width, height: Math.round(design.height * scale) });
      const layer = new Konva.Layer({ listening: false });
      stage.add(layer);
      const group = new Konva.Group({ scaleX: scale, scaleY: scale });
      layer.add(group);
      const duration = design.duration ?? template.duration;
      const began = performance.now();
      const tick = () => {
        drawSystemFrame(group, design, layers, ((performance.now() - began) / 1000) % duration);
        layer.draw();
        frame = requestAnimationFrame(tick);
      };
      tick();
    }).catch(() => {});
    return () => {
      stopped = true;
      cancelAnimationFrame(frame);
      stage?.destroy();
    };
  }, [playing, animate, template]);

  const f = SYSTEM_FORMATS.find(x => x.id === template.format)!;
  const ratio = { aspectRatio: `${f.width} / ${f.height}` };
  // Touch has no hover: a tap picks the template.
  const start = (e: PointerEvent) => { if (e.pointerType !== 'touch') setPlaying(true); };
  return (
    <button
      className="ie-tile"
      onClick={onPick}
      disabled={disabled}
      title={template.name}
      onPointerEnter={animate ? start : undefined}
      onPointerLeave={animate ? () => setPlaying(false) : undefined}
      onFocus={animate ? () => setPlaying(true) : undefined}
      onBlur={animate ? () => setPlaying(false) : undefined}
    >
      <div className="ie-tile-media" style={ratio}>
        {preview ? <img src={preview} alt="" style={ratio} /> : <div className="ie-tile-placeholder" style={ratio}><Loader2 size={14} className="ie-spin" /></div>}
        {playing && animate && <div ref={hostRef} className="ie-tile-player" aria-hidden="true" />}
      </div>
      {template.videoOnly && <Film size={14} className="ie-tile-badge" aria-label="Reel template" />}
      <span>{loading ? 'Loading…' : template.name}</span>
    </button>
  );
}
