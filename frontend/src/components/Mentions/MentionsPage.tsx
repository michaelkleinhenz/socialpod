import { useState, useEffect, useRef } from 'react';
import { api } from '../../services/api';
import type { MentionEntry, Platform } from '../../types';
import { Plus, Pencil, Trash2, X, Check, Download, Upload } from 'lucide-react';
import { PlatformIcon } from '../Common/PlatformIcon';
import toast from 'react-hot-toast';
import './Mentions.css';

const PLATFORMS: Platform[] = ['bluesky', 'instagram', 'twitter', 'mastodon', 'threads', 'linkedin', 'youtube'];

const PLATFORM_LABELS: Record<Platform, string> = {
  bluesky: 'Bluesky',
  instagram: 'Instagram',
  twitter: 'X / Twitter',
  mastodon: 'Mastodon',
  threads: 'Threads',
  linkedin: 'LinkedIn',
  youtube: 'YouTube',
};

const PLATFORM_PLACEHOLDERS: Record<Platform, string> = {
  bluesky: '@alice.bsky.social',
  instagram: '@alice',
  twitter: '@alice',
  mastodon: '@alice@mastodon.social',
  threads: '@alice',
  linkedin: '@alice',
  youtube: '@channel',
};

function getHandleUrl(platform: Platform, handle: string): string | null {
  const clean = handle.replace(/^@/, '');
  if (!clean) return null;
  switch (platform) {
    case 'instagram':
      return `https://instagram.com/${clean}`;
    case 'bluesky':
      return `https://bsky.app/profile/${clean}`;
    case 'twitter':
      return `https://x.com/${clean}`;
    case 'mastodon': {
      const match = clean.match(/^(.+)@(.+)$/);
      if (match) return `https://${match[2]}/@${match[1]}`;
      return null;
    }
    case 'threads':
      return `https://threads.net/@${clean}`;
    case 'linkedin':
      return `https://linkedin.com/company/${clean}`;
    case 'youtube':
      return `https://youtube.com/@${clean}`;
    default:
      return null;
  }
}

interface HandleFormState {
  [platform: string]: string;
}

function emptyHandles(): HandleFormState {
  return Object.fromEntries(PLATFORMS.map(p => [p, '']));
}

export function MentionsPage() {
  const [mentions, setMentions] = useState<MentionEntry[]>([]);
  const [showForm, setShowForm] = useState(false);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [name, setName] = useState('');
  const [country, setCountry] = useState('');
  const [handles, setHandles] = useState<HandleFormState>(emptyHandles());
  const [saving, setSaving] = useState(false);
  const fileInputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    api.getMentions().then(setMentions).catch(() => toast.error('Failed to load mentions'));
  }, []);

  const resetForm = () => {
    setName('');
    setCountry('');
    setHandles(emptyHandles());
    setShowForm(false);
    setEditingId(null);
  };

  const buildHandles = (): Record<string, string> =>
    Object.fromEntries(
      Object.entries(handles)
        .filter(([, v]) => v.trim())
        .map(([k, v]) => [k, v.trim()]),
    );

  const handleCreate = async () => {
    if (!name.trim()) {
      toast.error('Name is required');
      return;
    }
    setSaving(true);
    try {
      const created = await api.createMention({ name: name.trim(), country: country.trim() || undefined, handles: buildHandles() });
      setMentions(prev => [...prev, created]);
      resetForm();
      toast.success('Mention created');
    } catch (err: any) {
      toast.error(err.message);
    } finally {
      setSaving(false);
    }
  };

  const startEdit = (mention: MentionEntry) => {
    setEditingId(mention.id);
    setName(mention.name);
    setCountry(mention.country || '');
    const h = emptyHandles();
    for (const [k, v] of Object.entries(mention.handles || {})) {
      h[k] = v;
    }
    setHandles(h);
    setShowForm(false);
  };

  const handleUpdate = async (id: string) => {
    if (!name.trim()) {
      toast.error('Name is required');
      return;
    }
    setSaving(true);
    try {
      const updated = await api.updateMention(id, { name: name.trim(), country: country.trim(), handles: buildHandles() });
      setMentions(prev => prev.map(m => (m.id === id ? updated : m)));
      resetForm();
      toast.success('Mention updated');
    } catch (err: any) {
      toast.error(err.message);
    } finally {
      setSaving(false);
    }
  };

  const handleDelete = async (id: string) => {
    if (!confirm('Delete this mention?')) return;
    try {
      await api.deleteMention(id);
      setMentions(prev => prev.filter(m => m.id !== id));
      toast.success('Mention deleted');
    } catch (err: any) {
      toast.error(err.message);
    }
  };

  const handleExport = async () => {
    try {
      const data = await api.exportMentions();
      const blob = new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' });
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = 'mentions.json';
      a.click();
      URL.revokeObjectURL(url);
      toast.success(`Exported ${data.length} mentions`);
    } catch (err: any) {
      toast.error(err.message);
    }
  };

  const handleImport = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;
    try {
      const text = await file.text();
      const data = JSON.parse(text);
      if (!Array.isArray(data)) {
        toast.error('JSON must be an array');
        return;
      }
      const result = await api.importMentions(data);
      toast.success(`Imported ${result.imported} mentions`);
      const updated = await api.getMentions();
      setMentions(updated);
    } catch (err: any) {
      toast.error(err.message || 'Failed to import');
    } finally {
      if (fileInputRef.current) fileInputRef.current.value = '';
    }
  };

  return (
    <div className="page">
      <div className="page-header">
        <div>
          <h1>Mentions</h1>
          <p className="page-subtitle">
            Accounts you can @mention in posts. Each entry maps to per-platform handles so
            mentions are resolved correctly when publishing.
          </p>
        </div>
        <div className="mention-header-actions">
          <button className="btn btn-secondary" onClick={handleExport} title="Export mentions as JSON">
            <Download size={16} /> Export
          </button>
          <button className="btn btn-secondary" onClick={() => fileInputRef.current?.click()} title="Import mentions from JSON">
            <Upload size={16} /> Import
          </button>
          <input
            ref={fileInputRef}
            type="file"
            accept=".json,application/json"
            style={{ display: 'none' }}
            onChange={handleImport}
          />
          <button
            className="btn btn-primary"
            onClick={() => {
              resetForm();
              setShowForm(true);
            }}
          >
            <Plus size={16} /> New Mention
          </button>
        </div>
      </div>

      {showForm && (
        <MentionForm
          title="New Mention"
          name={name}
          country={country}
          handles={handles}
          saving={saving}
          onNameChange={setName}
          onCountryChange={setCountry}
          onHandleChange={(p, v) => setHandles(prev => ({ ...prev, [p]: v }))}
          onSubmit={handleCreate}
          onCancel={resetForm}
          submitLabel="Create"
        />
      )}

      {mentions.length === 0 && !showForm ? (
        <div className="card empty-state">
          <p>
            No mentions yet. Add one to get @-mention autocomplete while composing posts.
          </p>
        </div>
      ) : (
        <div className="mention-list">
          {mentions.map(mention => (
            <div key={mention.id} className="card mention-card">
              {editingId === mention.id ? (
                <MentionForm
                  title={null}
                  name={name}
                  country={country}
                  handles={handles}
                  saving={saving}
                  onNameChange={setName}
                  onCountryChange={setCountry}
                  onHandleChange={(p, v) => setHandles(prev => ({ ...prev, [p]: v }))}
                  onSubmit={() => handleUpdate(mention.id)}
                  onCancel={resetForm}
                  submitLabel="Save"
                />
              ) : (
                <div className="mention-row">
                  <div className="mention-info">
                    <div className="mention-name">
                      {mention.name}
                      {mention.country && (
                        <span className="mention-country">{mention.country}</span>
                      )}
                    </div>
                    <div className="mention-handles">
                      {PLATFORMS.filter(p => mention.handles?.[p]).map(p => {
                        const url = getHandleUrl(p, mention.handles[p]);
                        return url ? (
                          <a
                            key={p}
                            className="mention-handle-chip mention-handle-link"
                            href={url}
                            target="_blank"
                            rel="noopener noreferrer"
                            title={`Open ${PLATFORM_LABELS[p]} profile`}
                          >
                            <PlatformIcon platform={p} size={11} />
                            {mention.handles[p]}
                          </a>
                        ) : (
                          <span key={p} className="mention-handle-chip">
                            <PlatformIcon platform={p} size={11} />
                            {mention.handles[p]}
                          </span>
                        );
                      })}
                      {!Object.values(mention.handles || {}).some(Boolean) && (
                        <span className="mention-no-handles">No handles configured</span>
                      )}
                    </div>
                  </div>
                  <div className="mention-actions">
                    <button
                      className="btn btn-ghost btn-sm"
                      onClick={() => startEdit(mention)}
                      title="Edit"
                    >
                      <Pencil size={14} />
                    </button>
                    <button
                      className="btn btn-ghost btn-sm"
                      onClick={() => handleDelete(mention.id)}
                      title="Delete"
                    >
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

interface MentionFormProps {
  title: string | null;
  name: string;
  country: string;
  handles: HandleFormState;
  saving: boolean;
  onNameChange: (v: string) => void;
  onCountryChange: (v: string) => void;
  onHandleChange: (platform: Platform, value: string) => void;
  onSubmit: () => void;
  onCancel: () => void;
  submitLabel: string;
}

function MentionForm({
  title,
  name,
  country,
  handles,
  saving,
  onNameChange,
  onCountryChange,
  onHandleChange,
  onSubmit,
  onCancel,
  submitLabel,
}: MentionFormProps) {
  return (
    <div className={title ? 'card mention-form-card' : 'mention-edit-form'}>
      {title && (
        <div className="mention-form-header">
          <h3>{title}</h3>
          <button className="btn btn-ghost btn-sm" onClick={onCancel}>
            <X size={16} />
          </button>
        </div>
      )}
      <div className="mention-name-country-row">
        <div className="form-group mention-name-field">
          <label>Display name</label>
          <input
            className="input"
            placeholder="e.g. Pegasus Spiele"
            value={name}
            onChange={e => onNameChange(e.target.value)}
            autoFocus
          />
        </div>
        <div className="form-group mention-country-field">
          <label>Country</label>
          <input
            className="input"
            placeholder="e.g. Deutschland"
            value={country}
            onChange={e => onCountryChange(e.target.value)}
          />
        </div>
      </div>
      <div className="mention-handles-grid">
        {PLATFORMS.map(p => (
          <div key={p} className="form-group mention-handle-field">
            <label className="mention-handle-label">
              <PlatformIcon platform={p} size={12} />
              {PLATFORM_LABELS[p]}
            </label>
            <input
              className="input"
              placeholder={PLATFORM_PLACEHOLDERS[p]}
              value={handles[p] || ''}
              onChange={e => onHandleChange(p, e.target.value)}
            />
          </div>
        ))}
      </div>
      <div className="mention-form-actions">
        <button className="btn btn-secondary" onClick={onCancel}>
          {title ? 'Cancel' : <><X size={14} /> Cancel</>}
        </button>
        <button className="btn btn-primary" onClick={onSubmit} disabled={saving}>
          {title ? (
            saving ? `${submitLabel.replace(/e$/, '')}ing...` : submitLabel
          ) : (
            <><Check size={14} /> {saving ? 'Saving...' : submitLabel}</>
          )}
        </button>
      </div>
    </div>
  );
}
