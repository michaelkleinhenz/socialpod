import { useState, useEffect, useCallback } from 'react';
import { useNavigate } from 'react-router-dom';
import { api } from '../../services/api';
import type { ConventionQueue, Platform, SocialAccount, Footer, Watermark } from '../../types';
import { format, parseISO } from 'date-fns';
import { Plus, Trash2, Calendar, Hash, ExternalLink, Tent, X } from 'lucide-react';
import toast from 'react-hot-toast';
import { PlatformIcon } from '../Common/PlatformIcon';
import { Modal, DISCARD_PROMPT } from '../Common/Modal';
import './Convention.css';

const PLATFORM_OPTIONS: Platform[] = ['bluesky', 'instagram', 'twitter', 'mastodon', 'threads', 'linkedin'];

/** Order-insensitive compare, for the lists the toggles build up. */
function sameSet(a: readonly string[], b: readonly string[]) {
  return a.length === b.length && [...a].sort().join('\u0000') === [...b].sort().join('\u0000');
}

/** Compare two pick-per-platform maps, treating an empty pick as no pick. */
function sameMap(a: Record<string, string>, b: Record<string, string>) {
  const keys = (m: Record<string, string>) => Object.keys(m).filter(k => m[k]);
  const ka = keys(a);
  return ka.length === keys(b).length && ka.every(k => a[k] === b[k]);
}

function QueueFormModal({
  initial,
  accounts,
  onSave,
  onClose,
}: {
  initial?: ConventionQueue;
  accounts: SocialAccount[];
  onSave: (data: any) => Promise<void>;
  onClose: () => void;
}) {
  const initialName = initial?.name ?? '';
  const initialUrl = initial?.conventionUrl ?? '';
  const initialHashtags = initial?.hashtags ?? [];
  const initialStart = initial ? format(parseISO(initial.startDate), "yyyy-MM-dd'T'HH:mm") : '';
  const initialEnd = initial ? format(parseISO(initial.endDate), "yyyy-MM-dd'T'HH:mm") : '';
  const initialPostsPerDay = initial?.postsPerDay ?? 2;
  const initialMinHours = initial?.minHoursBetweenPosts ?? 0;
  const initialPlatforms = initial?.platforms ?? [];
  const initialAccountIds = initial?.accountIds ?? {};
  const initialFooterIds = initial?.footerIds ?? {};
  const initialWatermarkId = initial?.watermarkId ?? '';

  const [name, setName] = useState(initialName);
  const [conventionUrl, setConventionUrl] = useState(initialUrl);
  const [hashtagInput, setHashtagInput] = useState('');
  const [hashtags, setHashtags] = useState<string[]>(initialHashtags);
  const [startDate, setStartDate] = useState(initialStart);
  const [endDate, setEndDate] = useState(initialEnd);
  const [postsPerDay, setPostsPerDay] = useState(initialPostsPerDay);
  const [minHoursBetween, setMinHoursBetween] = useState(initialMinHours);
  const [platforms, setPlatforms] = useState<Platform[]>(initialPlatforms);
  const [accountIds, setAccountIds] = useState<Record<string, string>>(initialAccountIds);
  const [footerIds, setFooterIds] = useState<Record<string, string>>(initialFooterIds);
  const [footers, setFooters] = useState<Footer[]>([]);
  const [watermarkId, setWatermarkId] = useState(initialWatermarkId);
  const [watermarks, setWatermarks] = useState<Watermark[]>([]);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    api.getFooters().then(setFooters).catch(() => {});
    api.getWatermarks().then(setWatermarks).catch(() => {});
  }, []);

  const addHashtag = () => {
    const tag = hashtagInput.trim().replace(/^#/, '');
    if (tag && !hashtags.includes('#' + tag)) {
      setHashtags(prev => [...prev, '#' + tag]);
    }
    setHashtagInput('');
  };

  const handleHashtagKey = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' || e.key === ' ' || e.key === ',') {
      e.preventDefault();
      addHashtag();
    }
  };

  const togglePlatform = (p: Platform) => {
    setPlatforms(prev =>
      prev.includes(p) ? prev.filter(x => x !== p) : [...prev, p]
    );
  };

  // A whole convention queue is too much work to lose to a slip, so this form
  // is one of the dialogs that only its own buttons can dismiss: a click
  // outside does nothing at all (closeOnBackdrop below). Cancel and the X then
  // still ask before throwing away a filled-in form. The Modal's own
  // unsaved-input watch would not see most of this form — the platform
  // toggles and hashtag chips are buttons, not inputs — so work out here
  // whether anything differs from what the form opened with.
  const isDirty =
    name !== initialName ||
    conventionUrl !== initialUrl ||
    hashtagInput.trim() !== '' ||
    !sameSet(hashtags, initialHashtags) ||
    startDate !== initialStart ||
    endDate !== initialEnd ||
    postsPerDay !== initialPostsPerDay ||
    minHoursBetween !== initialMinHours ||
    !sameSet(platforms, initialPlatforms) ||
    !sameMap(accountIds, initialAccountIds) ||
    !sameMap(footerIds, initialFooterIds) ||
    watermarkId !== initialWatermarkId;

  const handleClose = () => {
    if (isDirty && !window.confirm(DISCARD_PROMPT)) return;
    onClose();
  };

  // The minimum delay takes precedence over posts-per-day: at most
  // floor(24 / minHours) posts can go out on any given day.
  const effectivePerDay = minHoursBetween > 0
    ? Math.min(postsPerDay, Math.max(1, Math.floor(24 / minHoursBetween)))
    : postsPerDay;

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!name || !startDate || !endDate || platforms.length === 0) {
      toast.error('Fill in name, dates, and at least one platform');
      return;
    }
    setSaving(true);
    try {
      await onSave({
        name,
        conventionUrl,
        hashtags,
        startDate: new Date(startDate).toISOString(),
        endDate: new Date(endDate).toISOString(),
        postsPerDay,
        minHoursBetweenPosts: minHoursBetween,
        platforms,
        accountIds,
        footerIds,
        watermarkId: watermarkId || null,
      });
    } finally {
      setSaving(false);
    }
  };

  const platformAccounts = accounts.filter(a => platforms.includes(a.platform));

  return (
    <Modal onClose={handleClose} closeOnBackdrop={false} className="conv-modal">
      <div className="modal-header">
        <h2>{initial ? 'Edit Queue' : 'New Convention Queue'}</h2>
        <button className="btn btn-ghost btn-sm" onClick={handleClose}><X size={18} /></button>
      </div>
      <form onSubmit={handleSubmit} className="conv-form">
        <div className="form-group">
          <label>Convention name *</label>
          <input
            className="input"
            value={name}
            onChange={e => setName(e.target.value)}
            placeholder="SPIEL 2026"
            required
          />
        </div>

        <div className="form-group">
          <label>Convention URL</label>
          <input
            className="input"
            value={conventionUrl}
            onChange={e => setConventionUrl(e.target.value)}
            placeholder="https://spiel.de"
            type="url"
          />
        </div>

        <div className="form-group">
          <label>Hashtags</label>
          <div className="hashtag-input-row">
            <input
              className="input"
              value={hashtagInput}
              onChange={e => setHashtagInput(e.target.value)}
              onKeyDown={handleHashtagKey}
              placeholder="#Essen2026 — press Enter to add"
            />
            <button type="button" className="btn btn-sm btn-secondary" onClick={addHashtag}>Add</button>
          </div>
          {hashtags.length > 0 && (
            <div className="tag-list">
              {hashtags.map(tag => (
                <span key={tag} className="tag">
                  {tag}
                  <button type="button" onClick={() => setHashtags(prev => prev.filter(t => t !== tag))}>×</button>
                </span>
              ))}
            </div>
          )}
        </div>

        {watermarks.length > 0 && (
          <div className="form-group">
            <label>Overlay watermark</label>
            <div className="conv-watermark-select">
              <select
                className="select"
                value={watermarkId}
                onChange={e => setWatermarkId(e.target.value)}
              >
                <option value="">None</option>
                {watermarks.map(w => (
                  <option key={w.id} value={w.id}>{w.name}</option>
                ))}
              </select>
              {watermarkId && (() => {
                const wm = watermarks.find(w => w.id === watermarkId);
                if (!wm) return null;
                const base = import.meta.env.VITE_API_URL || '';
                const src = wm.url.startsWith('/') ? base + wm.url : wm.url;
                return <img src={src} alt={wm.name} className="conv-watermark-preview" />;
              })()}
            </div>
            <p className="conv-upload-hint">
              Applied automatically to every image in this convention when scheduled.
            </p>
          </div>
        )}

        <div className="form-row">
          <div className="form-group">
            <label>Drip window start *</label>
            <input
              className="input"
              type="datetime-local"
              value={startDate}
              onChange={e => setStartDate(e.target.value)}
              required
            />
          </div>
          <div className="form-group">
            <label>Drip window end *</label>
            <input
              className="input"
              type="datetime-local"
              value={endDate}
              onChange={e => setEndDate(e.target.value)}
              required
            />
          </div>
        </div>

        <div className="form-row">
          <div className="form-group">
            <label>Posts per day</label>
            <input
              className="input"
              type="number"
              min={1}
              step={1}
              value={postsPerDay}
              onChange={e => setPostsPerDay(Math.max(1, parseInt(e.target.value, 10) || 1))}
            />
          </div>
          <div className="form-group">
            <label>Min. hours between posts</label>
            <input
              className="input"
              type="number"
              min={0}
              step={0.5}
              value={minHoursBetween}
              onChange={e => setMinHoursBetween(Math.max(0, parseFloat(e.target.value) || 0))}
              placeholder="0 = no minimum"
            />
          </div>
        </div>
        <p className="conv-schedule-hint">
          Posts are scattered randomly across each day, with a random gap of the
          minimum delay ±60 minutes between them. The minimum delay takes
          precedence: {' '}
          {minHoursBetween > 0
            ? <>at most <strong>{effectivePerDay}</strong> post{effectivePerDay !== 1 ? 's' : ''}/day will go out ({minHoursBetween}h apart caps it at {Math.max(1, Math.floor(24 / minHoursBetween))}/day).</>
            : <>with no minimum set, all <strong>{postsPerDay}</strong> post{postsPerDay !== 1 ? 's' : ''}/day are spread evenly across the day.</>}
        </p>

        <div className="form-group">
          <label>Platforms *</label>
          <div className="platform-toggles">
            {PLATFORM_OPTIONS.map(p => (
              <button
                key={p}
                type="button"
                className={`platform-toggle ${platforms.includes(p) ? 'active' : ''}`}
                onClick={() => togglePlatform(p)}
              >
                <PlatformIcon platform={p} />
                <span>{p}</span>
              </button>
            ))}
          </div>
        </div>

        {platformAccounts.length > 0 && (
          <div className="form-group">
            <label>Accounts</label>
            <div className="account-selects">
              {platforms.map(p => {
                const opts = platformAccounts.filter(a => a.platform === p);
                if (opts.length === 0) return null;
                return (
                  <div key={p} className="account-select-row">
                    <PlatformIcon platform={p} />
                    <select
                      className="select"
                      value={accountIds[p] ?? ''}
                      onChange={e => setAccountIds(prev => ({ ...prev, [p]: e.target.value }))}
                    >
                      <option value="">— select account —</option>
                      {opts.map(a => (
                        <option key={a.id} value={a.id}>{a.displayName || a.accountName}</option>
                      ))}
                    </select>
                  </div>
                );
              })}
            </div>
          </div>
        )}

        {footers.length > 0 && platforms.length > 0 && (
          <div className="form-group">
            <label>Default footers</label>
            <div className="account-selects">
              {platforms.map(p => (
                <div key={p} className="account-select-row">
                  <PlatformIcon platform={p} />
                  <select
                    className="select"
                    value={footerIds[p] ?? ''}
                    onChange={e => setFooterIds(prev => {
                      const next = { ...prev };
                      if (e.target.value) next[p] = e.target.value;
                      else delete next[p];
                      return next;
                    })}
                  >
                    <option value="">None</option>
                    {footers.map(s => (
                      <option key={s.id} value={s.id}>{s.name}</option>
                    ))}
                  </select>
                </div>
              ))}
            </div>
          </div>
        )}

        <div className="modal-footer">
          <button type="button" className="btn btn-secondary" onClick={handleClose}>Cancel</button>
          <button type="submit" className="btn btn-primary" disabled={saving}>
            {saving ? 'Saving…' : initial ? 'Save changes' : 'Create queue'}
          </button>
        </div>
      </form>
    </Modal>
  );
}

export function ConventionPage() {
  const [queues, setQueues] = useState<ConventionQueue[]>([]);
  const [accounts, setAccounts] = useState<SocialAccount[]>([]);
  const [loading, setLoading] = useState(true);
  const [showNew, setShowNew] = useState(false);
  const navigate = useNavigate();

  const load = useCallback(async () => {
    try {
      const [qs, accs] = await Promise.all([api.getConventionQueues(), api.getActiveAccounts()]);
      setQueues(qs);
      setAccounts(accs);
    } catch {
      toast.error('Failed to load convention queues');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { load(); }, [load]);

  const handleCreate = async (data: any) => {
    await api.createConventionQueue(data);
    toast.success('Queue created');
    setShowNew(false);
    load();
  };

  const handleDelete = async (q: ConventionQueue) => {
    if (!confirm(`Delete "${q.name}" and all its photos?`)) return;
    await api.deleteConventionQueue(q.id);
    toast.success('Queue deleted');
    load();
  };

  if (loading) return <div className="page"><div className="loading-screen"><div className="spinner" /></div></div>;

  return (
    <div className="page">
      <div className="page-header">
        <div className="page-header-left">
          <h1>Convention Mode</h1>
          <p className="page-subtitle">Queue convention photos and drip-post them over days or weeks</p>
        </div>
        <button className="btn btn-primary" onClick={() => setShowNew(true)}>
          <Plus size={16} /> New Queue
        </button>
      </div>

      {queues.length === 0 ? (
        <div className="empty-state">
          <Tent size={48} color="var(--text-muted)" />
          <p style={{ marginTop: 16 }}>No convention queues yet</p>
          <p style={{ color: 'var(--text-muted)', fontSize: 14 }}>
            Create a queue for your next show to start uploading photos.
          </p>
          <button className="btn btn-primary" style={{ marginTop: 20 }} onClick={() => setShowNew(true)}>
            <Plus size={16} /> Create your first queue
          </button>
        </div>
      ) : (
        <div className="conv-queue-list">
          {queues.map(q => (
            <div key={q.id} className="conv-queue-card" onClick={() => navigate(`/convention/${q.id}`)}>
              <div className="conv-queue-card-header">
                <div>
                  <h3>{q.name}</h3>
                  <div className="conv-queue-meta">
                    <span className="conv-meta-item">
                      <Calendar size={13} />
                      {format(parseISO(q.startDate), 'MMM d')} – {format(parseISO(q.endDate), 'MMM d, yyyy')}
                    </span>
                    {q.hashtags && q.hashtags.length > 0 && (
                      <span className="conv-meta-item">
                        <Hash size={13} />
                        {q.hashtags.slice(0, 3).join(' ')}
                        {q.hashtags.length > 3 && ` +${q.hashtags.length - 3}`}
                      </span>
                    )}
                    {q.conventionUrl && (
                      <a
                        href={q.conventionUrl}
                        target="_blank"
                        rel="noopener noreferrer"
                        className="conv-meta-item conv-url-link"
                        onClick={e => e.stopPropagation()}
                      >
                        <ExternalLink size={13} /> Website
                      </a>
                    )}
                  </div>
                </div>
                <button
                  className="icon-btn danger"
                  onClick={e => { e.stopPropagation(); handleDelete(q); }}
                  title="Delete queue"
                >
                  <Trash2 size={16} />
                </button>
              </div>

              <div className="conv-queue-platforms">
                {q.platforms.map(p => <PlatformIcon key={p} platform={p} />)}
              </div>

              <div className="conv-queue-progress">
                <div className="conv-progress-stats">
                  <span className="conv-stat">{q.itemCount ?? 0} photos</span>
                  <span className="conv-stat approved">{q.approvedCount ?? 0} approved</span>
                  <span className="conv-stat scheduled">{q.scheduledCount ?? 0} scheduled</span>
                </div>
                <div className="conv-progress-bar">
                  <div
                    className="conv-progress-fill"
                    style={{ width: q.itemCount ? `${((q.scheduledCount ?? 0) / q.itemCount) * 100}%` : '0%' }}
                  />
                </div>
              </div>

              <div className="conv-queue-footer">
                <span className={`badge badge-${q.status}`}>{q.status}</span>
                <span className="conv-ppd">
                  {q.postsPerDay}×/day{q.minHoursBetweenPosts ? ` · ≥${q.minHoursBetweenPosts}h apart` : ''}
                </span>
              </div>
            </div>
          ))}
        </div>
      )}

      {showNew && (
        <QueueFormModal
          accounts={accounts}
          onSave={handleCreate}
          onClose={() => setShowNew(false)}
        />
      )}
    </div>
  );
}

export { QueueFormModal };
