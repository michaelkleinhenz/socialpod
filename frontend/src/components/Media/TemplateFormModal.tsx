import { useState } from 'react';
import { X } from 'lucide-react';
import { Modal } from '../Common/Modal';
import { CANVAS_FORMATS } from '../Common/ImageEditor/editorModel';

export interface NewTemplate {
  name: string;
  description: string;
  tags: string[];
  width: number;
  height: number;
  // A video template: designed with a length, music and animations.
  video: boolean;
}

interface Props {
  onClose: () => void;
  onContinue: (t: NewTemplate) => void;
}

/** Names a new template and picks its format; the design follows in the image editor. */
export function TemplateFormModal({ onClose, onContinue }: Props) {
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [tags, setTags] = useState('');
  const [formatId, setFormatId] = useState('4:5');
  const [video, setVideo] = useState(false);

  // A video is usually a reel or story, a still usually a feed post.
  const changeType = (v: boolean) => {
    setVideo(v);
    setFormatId(v ? '9:16' : '4:5');
  };

  const proceed = () => {
    const format = CANVAS_FORMATS.find(f => f.id === formatId) ?? CANVAS_FORMATS[0];
    onContinue({
      name: name.trim() || 'Template',
      description: description.trim(),
      tags: tags.split(/[,;]/).map(t => t.trim().replace(/^#/, '').toLowerCase()).filter(Boolean),
      width: format.width,
      height: format.height,
      video,
    });
  };

  return (
    <Modal onClose={onClose}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 20 }}>
        <h2>New Template</h2>
        <button className="btn btn-ghost btn-sm" onClick={onClose}>
          <X size={18} />
        </button>
      </div>
      <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
        <div className="form-group">
          <label>Name</label>
          <input className="input" value={name} onChange={e => setName(e.target.value)} placeholder="e.g. News card" autoFocus />
        </div>
        <div className="form-group">
          <label>Type</label>
          <select className="select" value={video ? 'video' : 'image'} onChange={e => changeType(e.target.value === 'video')}>
            <option value="image">Image</option>
            <option value="video">Video (length, music and animations, for reels and stories)</option>
          </select>
        </div>
        <div className="form-group">
          <label>Format</label>
          <select className="select" value={formatId} onChange={e => setFormatId(e.target.value)}>
            {CANVAS_FORMATS.map(f => (
              <option key={f.id} value={f.id}>{f.label} ({f.width} × {f.height})</option>
            ))}
          </select>
        </div>
        <div className="form-group">
          <label>Description</label>
          <textarea
            className="input"
            rows={3}
            value={description}
            onChange={e => setDescription(e.target.value)}
            placeholder="What the template is for"
          />
        </div>
        <div className="form-group">
          <label>Tags</label>
          <input className="input" value={tags} onChange={e => setTags(e.target.value)} placeholder="Comma-separated, e.g. news, story" />
        </div>
      </div>
      <div className="modal-actions">
        <button className="btn btn-secondary" onClick={onClose}>Cancel</button>
        <button className="btn btn-primary" onClick={proceed}>Design it</button>
      </div>
    </Modal>
  );
}
