import { useState, useEffect } from 'react';
import { api } from '../../services/api';
import type { Suffix } from '../../types';
import { Plus, Pencil, Trash2, X, Check } from 'lucide-react';
import toast from 'react-hot-toast';
import './Suffixes.css';

export function SuffixesPage() {
  const [suffixes, setSuffixes] = useState<Suffix[]>([]);
  const [showForm, setShowForm] = useState(false);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [name, setName] = useState('');
  const [content, setContent] = useState('');
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    api.getSuffixes().then(setSuffixes).catch(() => toast.error('Failed to load suffixes'));
  }, []);

  const resetForm = () => {
    setName('');
    setContent('');
    setShowForm(false);
    setEditingId(null);
  };

  const handleCreate = async () => {
    if (!name.trim() || !content.trim()) {
      toast.error('Name and content are required');
      return;
    }
    setSaving(true);
    try {
      const created = await api.createSuffix({ name: name.trim(), content: content.trim() });
      setSuffixes(prev => [...prev, created]);
      resetForm();
      toast.success('Suffix created');
    } catch (err: any) {
      toast.error(err.message);
    } finally {
      setSaving(false);
    }
  };

  const startEdit = (suffix: Suffix) => {
    setEditingId(suffix.id);
    setName(suffix.name);
    setContent(suffix.content);
    setShowForm(false);
  };

  const handleUpdate = async (id: string) => {
    if (!name.trim() || !content.trim()) {
      toast.error('Name and content are required');
      return;
    }
    setSaving(true);
    try {
      const updated = await api.updateSuffix(id, { name: name.trim(), content: content.trim() });
      setSuffixes(prev => prev.map(s => s.id === id ? updated : s));
      resetForm();
      toast.success('Suffix updated');
    } catch (err: any) {
      toast.error(err.message);
    } finally {
      setSaving(false);
    }
  };

  const handleDelete = async (id: string) => {
    if (!confirm('Delete this suffix?')) return;
    try {
      await api.deleteSuffix(id);
      setSuffixes(prev => prev.filter(s => s.id !== id));
      toast.success('Suffix deleted');
    } catch (err: any) {
      toast.error(err.message);
    }
  };

  return (
    <div className="page">
      <div className="page-header">
        <div>
          <h1>Suffixes</h1>
          <p className="page-subtitle">Automatically append text to posts per platform when publishing.</p>
        </div>
        <button className="btn btn-primary" onClick={() => { resetForm(); setShowForm(true); }}>
          <Plus size={16} /> New Suffix
        </button>
      </div>

      {showForm && (
        <div className="card suffix-form-card">
          <div className="suffix-form-header">
            <h3>New Suffix</h3>
            <button className="btn btn-ghost btn-sm" onClick={resetForm}><X size={16} /></button>
          </div>
          <div className="form-group">
            <label>Name</label>
            <input
              className="input"
              placeholder="e.g. Website footer"
              value={name}
              onChange={e => setName(e.target.value)}
              autoFocus
            />
          </div>
          <div className="form-group">
            <label>Content</label>
            <textarea
              className="textarea"
              placeholder="Text to append to posts..."
              value={content}
              onChange={e => setContent(e.target.value)}
              rows={3}
            />
            <div className="suffix-char-hint">{content.length} characters</div>
          </div>
          <div className="modal-actions">
            <button className="btn btn-secondary" onClick={resetForm}>Cancel</button>
            <button className="btn btn-primary" onClick={handleCreate} disabled={saving}>
              {saving ? 'Creating...' : 'Create'}
            </button>
          </div>
        </div>
      )}

      {suffixes.length === 0 && !showForm ? (
        <div className="card empty-state">
          <p>No suffixes yet. Create one to automatically append text when publishing.</p>
        </div>
      ) : (
        <div className="suffix-list">
          {suffixes.map(suffix => (
            <div key={suffix.id} className="card suffix-card">
              {editingId === suffix.id ? (
                <div className="suffix-edit-form">
                  <div className="form-group">
                    <label>Name</label>
                    <input
                      className="input"
                      value={name}
                      onChange={e => setName(e.target.value)}
                      autoFocus
                    />
                  </div>
                  <div className="form-group">
                    <label>Content</label>
                    <textarea
                      className="textarea"
                      value={content}
                      onChange={e => setContent(e.target.value)}
                      rows={3}
                    />
                    <div className="suffix-char-hint">{content.length} characters</div>
                  </div>
                  <div className="suffix-edit-actions">
                    <button className="btn btn-ghost btn-sm" onClick={resetForm}>
                      <X size={14} /> Cancel
                    </button>
                    <button className="btn btn-primary btn-sm" onClick={() => handleUpdate(suffix.id)} disabled={saving}>
                      <Check size={14} /> {saving ? 'Saving...' : 'Save'}
                    </button>
                  </div>
                </div>
              ) : (
                <div className="suffix-row">
                  <div className="suffix-info">
                    <div className="suffix-name">{suffix.name}</div>
                    <div className="suffix-content">{suffix.content}</div>
                  </div>
                  <div className="suffix-actions">
                    <button className="btn btn-ghost btn-sm" onClick={() => startEdit(suffix)} title="Edit">
                      <Pencil size={14} />
                    </button>
                    <button className="btn btn-ghost btn-sm" onClick={() => handleDelete(suffix.id)} title="Delete">
                      <Trash2 size={14} color="var(--danger)" />
                    </button>
                  </div>
                </div>
              )}
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
