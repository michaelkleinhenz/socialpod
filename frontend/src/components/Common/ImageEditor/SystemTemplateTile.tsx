import { useEffect, useState } from 'react';
import { Loader2 } from 'lucide-react';
import { SYSTEM_FORMATS, systemTemplatePreview, type SystemTemplate } from './systemTemplates';

interface Props {
  template: SystemTemplate;
  loading: boolean;
  disabled: boolean;
  onPick: () => void;
}

// A built-in template in the Templates tab, its preview drawn the first time
// it shows.
export function SystemTemplateTile({ template, loading, disabled, onPick }: Props) {
  const [preview, setPreview] = useState<string | null>(null);
  useEffect(() => {
    let cancelled = false;
    systemTemplatePreview(template).then(url => { if (!cancelled) setPreview(url); }).catch(() => {});
    return () => { cancelled = true; };
  }, [template]);
  const f = SYSTEM_FORMATS.find(x => x.id === template.format)!;
  return (
    <button className="ie-tile" onClick={onPick} disabled={disabled} title={template.name}>
      {preview ? (
        <img src={preview} alt="" style={{ aspectRatio: `${f.width} / ${f.height}` }} />
      ) : (
        <div className="ie-tile-placeholder" style={{ aspectRatio: `${f.width} / ${f.height}` }}><Loader2 size={14} className="ie-spin" /></div>
      )}
      <span>{loading ? 'Loading…' : template.name}</span>
    </button>
  );
}
