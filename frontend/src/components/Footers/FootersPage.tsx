import { useState, useEffect } from 'react';
import { api } from '../../services/api';
import type { Footer } from '../../types';
import { Plus, Pencil, Trash2, X, Check } from 'lucide-react';
import toast from 'react-hot-toast';
import './Footers.css';

export function FootersPage() {
  const [footers, setFooters] = useState<Footer[]>([]);
  const [showForm, setShowForm] = useState(false);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [name, setName] = useState('');
  const [content, setContent] = useState('');
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    api.getFooters().then(setFooters).catch(() => toast.error('Failed to load footers'));
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
      const created = await api.createFooter({ name: name.trim(), content: content.trim() });
      setFooters(prev => [...prev, created]);
      resetForm();
      toast.success('Footer created');
    } catch (err: any) {
      toast.error(err.message);
    } finally {
      setSaving(false);
    }
  };

  const startEdit = (footer: Footer) => {
    setEditingId(footer.id);
    setName(footer.name);
    setContent(footer.content);
    setShowForm(false);
  };

  const handleUpdate = async (id: string) => {
    if (!name.trim() || !content.trim()) {
      toast.error('Name and content are required');
      return;
    }
    setSaving(true);
    try {
      const updated = await api.updateFooter(id, { name: name.trim(), content: content.trim() });
      setFooters(prev => prev.map(s => s.id === id ? updated : s));
      resetForm();
      toast.success('Footer updated');
    } catch (err: any) {
      toast.error(err.message);
    } finally {
      setSaving(false);
    }
  };

  const handleDelete = async (id: string) => {
    if (!confirm('Delete this footer?')) return;
    try {
      await api.deleteFooter(id);
      setFooters(prev => prev.filter(s => s.id !== id));
      toast.success('Footer deleted');
    } catch (err: any) {
      toast.error(err.message);
    }
  };

  return (
    <div className="page">
      <div className="page-header">
        <div>
          <h1>Footers</h1>
          <p className="page-subtitle">Automatically append text to posts per platform when publishing.</p>
        </div>
        <button className="btn btn-primary" onClick={() => { resetForm(); setShowForm(true); }}>
          <Plus size={16} /> New Footer
        </button>
      </div>

      {showForm && (
        <div className="card footer-form-card">
          <div className="footer-form-header">
            <h3>New Footer</h3>
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
            <div className="footer-char-hint">{content.length} characters</div>
          </div>
          <div className="modal-actions">
            <button className="btn btn-secondary" onClick={resetForm}>Cancel</button>
            <button className="btn btn-primary" onClick={handleCreate} disabled={saving}>
              {saving ? 'Creating...' : 'Create'}
            </button>
          </div>
        </div>
      )}

      {footers.length === 0 && !showForm ? (
        <div className="card empty-state">
          <p>No footers yet. Create one to automatically append text when publishing.</p>
        </div>
      ) : (
        <div className="footer-list">
          {footers.map(footer => (
            <div key={footer.id} className="card footer-card">
              {editingId === footer.id ? (
                <div className="footer-edit-form">
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
                    <div className="footer-char-hint">{content.length} characters</div>
                  </div>
                  <div className="footer-edit-actions">
                    <button className="btn btn-ghost btn-sm" onClick={resetForm}>
                      <X size={14} /> Cancel
                    </button>
                    <button className="btn btn-primary btn-sm" onClick={() => handleUpdate(footer.id)} disabled={saving}>
                      <Check size={14} /> {saving ? 'Saving...' : 'Save'}
                    </button>
                  </div>
                </div>
              ) : (
                <div className="footer-row">
                  <div className="footer-info">
                    <div className="footer-name">{footer.name}</div>
                    <div className="footer-content">{footer.content}</div>
                  </div>
                  <div className="footer-actions">
                    <button className="btn btn-ghost btn-sm" onClick={() => startEdit(footer)} title="Edit">
                      <Pencil size={14} />
                    </button>
                    <button className="btn btn-ghost btn-sm" onClick={() => handleDelete(footer.id)} title="Delete">
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
