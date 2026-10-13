import { useState } from 'react';
import { X } from 'lucide-react';
import toast from 'react-hot-toast';
import { Modal } from '../Common/Modal';
import { api } from '../../services/api';
import type { MediaItem } from '../../types';
import { HEX_COLOR, pickerValue } from './mediaColor';

/** Hex text field with a colour picker beside it. The picker keeps an alpha the text gave. */
export function ColorField({ value, onChange }: { value: string; onChange: (v: string) => void }) {
  const alpha = value.trim().replace(/^#/, '').slice(6, 8);
  return (
    <div className="media-color-field">
      <input type="color" value={pickerValue(value)} onChange={e => onChange(e.target.value + alpha)} />
      <input className="input" value={value} onChange={e => onChange(e.target.value)} placeholder="#rrggbb or #rrggbbaa" />
    </div>
  );
}

interface Props {
  onClose: () => void;
  onCreated: (item: MediaItem) => void;
}

/** Adds a colour to the media library, with a description of how to use it. */
export function ColorFormModal({ onClose, onCreated }: Props) {
  const [color, setColor] = useState('#');
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [tags, setTags] = useState('');
  const [saving, setSaving] = useState(false);

  const save = async () => {
    if (!HEX_COLOR.test(color.trim())) {
      toast.error('Enter the colour as #rrggbb or #rrggbbaa');
      return;
    }
    setSaving(true);
    try {
      const item = await api.createMediaColor({
        color: color.trim(),
        name: name.trim(),
        description: description.trim(),
        tags: tags.split(/[,;]/).map(t => t.trim().replace(/^#/, '').toLowerCase()).filter(Boolean),
      });
      onCreated(item);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to add the colour');
    } finally {
      setSaving(false);
    }
  };

  return (
    <Modal onClose={onClose}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 20 }}>
        <h2>Add Colour</h2>
        <button className="btn btn-ghost btn-sm" onClick={onClose}>
          <X size={18} />
        </button>
      </div>
      <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
        <div className="form-group">
          <label>Colour</label>
          <ColorField value={color} onChange={setColor} />
        </div>
        <div className="form-group">
          <label>Name</label>
          <input className="input" value={name} onChange={e => setName(e.target.value)} placeholder="e.g. Brand red" />
        </div>
        <div className="form-group">
          <label>How to use it</label>
          <textarea
            className="input"
            rows={4}
            value={description}
            onChange={e => setDescription(e.target.value)}
            placeholder="e.g. Accent for headlines and badges; never as a background behind photos"
          />
          <span style={{ fontSize: 12, color: 'var(--text-muted)' }}>
            MCP agents read this and follow it when they compose post images.
          </span>
        </div>
        <div className="form-group">
          <label>Tags</label>
          <input className="input" value={tags} onChange={e => setTags(e.target.value)} placeholder="Comma-separated, e.g. brand, accent" />
        </div>
      </div>
      <div className="modal-actions">
        <button className="btn btn-secondary" onClick={onClose}>Cancel</button>
        <button className="btn btn-primary" onClick={save} disabled={saving}>
          {saving ? 'Adding...' : 'Add Colour'}
        </button>
      </div>
    </Modal>
  );
}
