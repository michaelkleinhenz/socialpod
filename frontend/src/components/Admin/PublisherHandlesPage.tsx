import { useState, useEffect } from 'react';
import { api } from '../../services/api';
import type { PublisherHandle } from '../../types';
import { Trash2, Plus, X, Pencil } from 'lucide-react';
import toast from 'react-hot-toast';
import './Admin.css';

const PLATFORMS = ['instagram', 'bluesky', 'threads', 'mastodon', 'twitter'];

function HandleEditor({
  handles,
  onChange,
}: {
  handles: Record<string, string>;
  onChange: (h: Record<string, string>) => void;
}) {
  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
      {PLATFORMS.map(platform => (
        <div key={platform} style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
          <span style={{ width: 90, fontSize: 13, color: 'var(--text-secondary)', textTransform: 'capitalize' }}>
            {platform}
          </span>
          <input
            className="input"
            style={{ flex: 1 }}
            placeholder={`@handle`}
            value={handles[platform] || ''}
            onChange={e => {
              const val = e.target.value.trim();
              const next = { ...handles };
              if (val) {
                next[platform] = val.startsWith('@') ? val : '@' + val;
              } else {
                delete next[platform];
              }
              onChange(next);
            }}
          />
        </div>
      ))}
    </div>
  );
}

export function PublisherHandlesPage() {
  const [entries, setEntries] = useState<PublisherHandle[]>([]);
  const [loading, setLoading] = useState(true);
  const [showForm, setShowForm] = useState(false);
  const [editEntry, setEditEntry] = useState<PublisherHandle | null>(null);
  const [formName, setFormName] = useState('');
  const [formHandles, setFormHandles] = useState<Record<string, string>>({});
  const [saving, setSaving] = useState(false);

  const load = () => {
    setLoading(true);
    api.getPublisherHandles()
      .then(setEntries)
      .catch(() => toast.error('Failed to load publisher handles'))
      .finally(() => setLoading(false));
  };

  useEffect(() => { load(); }, []);

  const openNew = () => {
    setEditEntry(null);
    setFormName('');
    setFormHandles({});
    setShowForm(true);
  };

  const openEdit = (entry: PublisherHandle) => {
    setEditEntry(entry);
    setFormName(entry.name);
    setFormHandles({ ...entry.handles });
    setShowForm(true);
  };

  const closeForm = () => {
    setShowForm(false);
    setEditEntry(null);
  };

  const save = async () => {
    if (!formName.trim()) { toast.error('Name is required'); return; }
    setSaving(true);
    try {
      if (editEntry) {
        await api.updatePublisherHandle(editEntry.id, { name: formName.trim(), handles: formHandles });
        toast.success('Entry updated');
      } else {
        await api.createPublisherHandle({ name: formName.trim(), handles: formHandles });
        toast.success('Entry created');
      }
      closeForm();
      load();
    } catch (err: any) {
      toast.error(err.message || 'Failed to save');
    } finally {
      setSaving(false);
    }
  };

  const remove = async (entry: PublisherHandle) => {
    if (!confirm(`Delete handle entry for "${entry.name}"?`)) return;
    try {
      await api.deletePublisherHandle(entry.id);
      load();
      toast.success('Deleted');
    } catch (err: any) {
      toast.error(err.message || 'Failed to delete');
    }
  };

  const platformCount = (h: Record<string, string>) => Object.values(h).filter(Boolean).length;

  return (
    <div className="page">
      <div className="page-header">
        <div>
          <h1>Publisher Handles</h1>
          <p style={{ margin: 0, color: 'var(--text-secondary)', fontSize: 14 }}>
            Map publisher, designer, and artist names from BGG to their social media handles.
            These are used as hints for AI handle lookup during BGG imports.
          </p>
        </div>
        <button className="btn btn-primary" onClick={openNew}>
          <Plus size={16} /> Add Entry
        </button>
      </div>

      {loading ? (
        <div style={{ textAlign: 'center', padding: 40, color: 'var(--text-muted)' }}>Loading…</div>
      ) : entries.length === 0 ? (
        <div className="card" style={{ textAlign: 'center', padding: 40, color: 'var(--text-muted)' }}>
          No entries yet. Add publishers, designers, or artists with their Instagram/Bluesky handles.
        </div>
      ) : (
        <div className="card" style={{ padding: 0, overflow: 'hidden' }}>
          <table style={{ width: '100%', borderCollapse: 'collapse' }}>
            <thead>
              <tr style={{ borderBottom: '1px solid var(--border)' }}>
                <th style={{ padding: '10px 16px', textAlign: 'left', fontSize: 13, fontWeight: 600 }}>Name</th>
                <th style={{ padding: '10px 16px', textAlign: 'left', fontSize: 13, fontWeight: 600 }}>Handles</th>
                <th style={{ padding: '10px 16px', width: 80 }}></th>
              </tr>
            </thead>
            <tbody>
              {entries.map((entry, i) => (
                <tr key={entry.id} style={{ borderBottom: i < entries.length - 1 ? '1px solid var(--border)' : 'none' }}>
                  <td style={{ padding: '10px 16px', fontWeight: 500, fontSize: 14 }}>{entry.name}</td>
                  <td style={{ padding: '10px 16px' }}>
                    <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4 }}>
                      {platformCount(entry.handles) === 0 ? (
                        <span style={{ fontSize: 12, color: 'var(--text-muted)' }}>No handles</span>
                      ) : (
                        Object.entries(entry.handles).map(([platform, handle]) => (
                          handle ? (
                            <span key={platform} className="badge badge-scheduled" style={{ fontSize: 12 }}>
                              {platform}: {handle}
                            </span>
                          ) : null
                        ))
                      )}
                    </div>
                  </td>
                  <td style={{ padding: '10px 16px' }}>
                    <div style={{ display: 'flex', gap: 4, justifyContent: 'flex-end' }}>
                      <button className="btn btn-ghost btn-sm" onClick={() => openEdit(entry)}>
                        <Pencil size={14} />
                      </button>
                      <button className="btn btn-ghost btn-sm" onClick={() => remove(entry)}>
                        <Trash2 size={14} color="var(--danger)" />
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {showForm && (
        <div className="modal-overlay" onClick={closeForm}>
          <div className="modal" onClick={e => e.stopPropagation()}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 20 }}>
              <h2>{editEntry ? 'Edit Entry' : 'New Publisher Handle'}</h2>
              <button className="btn btn-ghost btn-sm" onClick={closeForm}><X size={18} /></button>
            </div>

            <div className="form-group">
              <label>Name (as it appears on BGG)</label>
              <input
                className="input"
                placeholder="e.g. Lookout Games"
                value={formName}
                onChange={e => setFormName(e.target.value)}
              />
            </div>

            <div className="form-group" style={{ marginBottom: 20 }}>
              <label>Social handles</label>
              <p style={{ fontSize: 12, color: 'var(--text-muted)', margin: '4px 0 10px' }}>
                Leave a platform blank to skip it. The @ is added automatically.
              </p>
              <HandleEditor handles={formHandles} onChange={setFormHandles} />
            </div>

            <div className="modal-actions">
              <button className="btn btn-secondary" onClick={closeForm}>Cancel</button>
              <button className="btn btn-primary" onClick={save} disabled={saving}>
                {saving ? 'Saving…' : 'Save'}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
