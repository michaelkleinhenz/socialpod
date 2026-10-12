import { useState, useEffect, useRef } from 'react';
import { api } from '../../services/api';
import { Trash2, Upload, Image, Pencil, Check, X } from 'lucide-react';
import toast from 'react-hot-toast';
import type { Watermark } from '../../types';
import './Admin.css';

export function WatermarksPage() {
  const [watermarks, setWatermarks] = useState<Watermark[]>([]);
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [uploading, setUploading] = useState(false);
  const [editing, setEditing] = useState<string | null>(null);
  const [editName, setEditName] = useState('');
  const [editDescription, setEditDescription] = useState('');
  const fileRef = useRef<HTMLInputElement>(null);
  const apiUrl = import.meta.env.VITE_API_URL || '';

  const load = () => {
    api.getWatermarks().then(setWatermarks).catch(() => toast.error('Failed to load watermarks'));
  };

  useEffect(load, []);

  const handleUpload = async (files: FileList | null) => {
    if (!files || files.length === 0) return;
    setUploading(true);
    try {
      for (const file of Array.from(files)) {
        await api.uploadWatermark(name || file.name.replace(/\.[^.]+$/, ''), file, description.trim());
      }
      setName('');
      setDescription('');
      load();
      toast.success('Watermark uploaded');
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Upload failed');
    } finally {
      setUploading(false);
      if (fileRef.current) fileRef.current.value = '';
    }
  };

  const handleDelete = async (id: string) => {
    if (!window.confirm('Delete this watermark?')) return;
    try {
      await api.deleteWatermark(id);
      setWatermarks(prev => prev.filter(w => w.id !== id));
      toast.success('Watermark deleted');
    } catch {
      toast.error('Failed to delete watermark');
    }
  };

  const startEdit = (wm: Watermark) => {
    setEditing(wm.id);
    setEditName(wm.name);
    setEditDescription(wm.description || '');
  };

  const saveEdit = async (id: string) => {
    try {
      const updated = await api.updateWatermark(id, { name: editName.trim(), description: editDescription.trim() });
      setWatermarks(prev => prev.map(w => (w.id === id ? updated : w)));
      setEditing(null);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to save watermark');
    }
  };

  return (
    <div className="page">
      <div className="page-header">
        <h1>Watermarks</h1>
      </div>

      <p className="watermarks-description">
        Upload images that will be available as watermarks in the image editor. Users can place them on any image when editing.
        The description tells MCP agents what a watermark looks like and how to use it, e.g. "round logo for a corner" or
        "full-size frame with a banner at the bottom".
      </p>

      <div className="watermark-upload-row">
        <input
          className="input"
          placeholder="Watermark name (optional)"
          value={name}
          onChange={e => setName(e.target.value)}
          style={{ flex: 1 }}
        />
        <input
          className="input"
          placeholder="Description (optional)"
          value={description}
          onChange={e => setDescription(e.target.value)}
          style={{ flex: 2 }}
        />
        <input
          ref={fileRef}
          type="file"
          accept="image/png,image/jpeg,image/gif,image/webp,image/svg+xml"
          style={{ display: 'none' }}
          onChange={e => handleUpload(e.target.files)}
        />
        <button className="btn btn-primary" onClick={() => fileRef.current?.click()} disabled={uploading}>
          <Upload size={16} /> {uploading ? 'Uploading...' : 'Upload'}
        </button>
      </div>

      {watermarks.length === 0 ? (
        <div className="empty-state">
          <Image size={32} />
          <p>No watermarks yet. Upload images to make them available in the image editor.</p>
        </div>
      ) : (
        <div className="watermark-grid">
          {watermarks.map(wm => (
            <div key={wm.id} className="watermark-card">
              <div className="watermark-preview">
                <img src={wm.url.startsWith('/') ? apiUrl + wm.url : wm.url} alt={wm.name} />
              </div>
              {editing === wm.id ? (
                <div className="watermark-edit">
                  <input className="input" value={editName} onChange={e => setEditName(e.target.value)} placeholder="Name" />
                  <textarea
                    className="input"
                    rows={3}
                    value={editDescription}
                    onChange={e => setEditDescription(e.target.value)}
                    placeholder="What it looks like and what it is for"
                  />
                  <div className="watermark-edit-actions">
                    <button className="btn btn-ghost btn-sm" onClick={() => setEditing(null)} title="Cancel">
                      <X size={14} />
                    </button>
                    <button className="btn btn-primary btn-sm" onClick={() => saveEdit(wm.id)} title="Save">
                      <Check size={14} />
                    </button>
                  </div>
                </div>
              ) : (
                <>
                  <div className="watermark-info">
                    <span className="watermark-name">{wm.name}</span>
                    <span className="watermark-actions">
                      <button className="btn btn-ghost btn-sm" onClick={() => startEdit(wm)} title="Edit name and description">
                        <Pencil size={14} />
                      </button>
                      <button className="btn btn-ghost btn-sm btn-danger-text" onClick={() => handleDelete(wm.id)} title="Delete">
                        <Trash2 size={14} />
                      </button>
                    </span>
                  </div>
                  {wm.description && <p className="watermark-desc">{wm.description}</p>}
                </>
              )}
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
