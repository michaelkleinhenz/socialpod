import { useState, useEffect, useRef } from 'react';
import { api } from '../../services/api';
import { Trash2, Upload, Image } from 'lucide-react';
import toast from 'react-hot-toast';
import './Admin.css';

interface Watermark {
  id: string;
  name: string;
  filename: string;
  url: string;
  createdAt: string;
}

export function WatermarksPage() {
  const [watermarks, setWatermarks] = useState<Watermark[]>([]);
  const [name, setName] = useState('');
  const [uploading, setUploading] = useState(false);
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
        await api.uploadWatermark(name || file.name.replace(/\.[^.]+$/, ''), file);
      }
      setName('');
      load();
      toast.success('Watermark uploaded');
    } catch (err: any) {
      toast.error(err.message || 'Upload failed');
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

  return (
    <div className="page">
      <div className="page-header">
        <h1>Watermarks</h1>
      </div>

      <p className="watermarks-description">
        Upload images that will be available as watermarks in the image editor. Users can place them on any image when editing.
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
              <div className="watermark-info">
                <span className="watermark-name">{wm.name}</span>
                <button className="btn btn-ghost btn-sm btn-danger-text" onClick={() => handleDelete(wm.id)}>
                  <Trash2 size={14} />
                </button>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
