import { useState, useEffect, useCallback } from 'react';
import { useNavigate } from 'react-router-dom';
import { api } from '../../services/api';
import type { ConventionQueue, Platform, SocialAccount } from '../../types';
import { format, parseISO } from 'date-fns';
import { Plus, Trash2, Calendar, Hash, ExternalLink, Tent } from 'lucide-react';
import toast from 'react-hot-toast';
import { PlatformIcon } from '../Common/PlatformIcon';
import './Convention.css';

const PLATFORM_OPTIONS: Platform[] = ['bluesky', 'instagram', 'twitter', 'mastodon', 'threads', 'linkedin'];
const DEFAULT_TIME_SLOTS = ['09:00', '14:00', '19:00'];

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
  const [name, setName] = useState(initial?.name ?? '');
  const [conventionUrl, setConventionUrl] = useState(initial?.conventionUrl ?? '');
  const [hashtagInput, setHashtagInput] = useState('');
  const [hashtags, setHashtags] = useState<string[]>(initial?.hashtags ?? []);
  const [startDate, setStartDate] = useState(
    initial ? format(parseISO(initial.startDate), "yyyy-MM-dd'T'HH:mm") : ''
  );
  const [endDate, setEndDate] = useState(
    initial ? format(parseISO(initial.endDate), "yyyy-MM-dd'T'HH:mm") : ''
  );
  const [postsPerDay, setPostsPerDay] = useState(initial?.postsPerDay ?? 2);
  const [timeSlots, setTimeSlots] = useState<string[]>(initial?.timeSlots ?? DEFAULT_TIME_SLOTS.slice(0, 2));
  const [platforms, setPlatforms] = useState<Platform[]>(initial?.platforms ?? []);
  const [accountIds, setAccountIds] = useState<Record<string, string>>(initial?.accountIds ?? {});
  const [saving, setSaving] = useState(false);

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

  const updateTimeSlot = (idx: number, val: string) => {
    setTimeSlots(prev => prev.map((s, i) => (i === idx ? val : s)));
  };

  const syncSlotCount = (count: number) => {
    setPostsPerDay(count);
    setTimeSlots(prev => {
      const next = [...prev];
      while (next.length < count) next.push(DEFAULT_TIME_SLOTS[next.length] ?? '12:00');
      return next.slice(0, count);
    });
  };

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
        timeSlots: timeSlots.slice(0, postsPerDay),
        platforms,
        accountIds,
      });
    } finally {
      setSaving(false);
    }
  };

  const platformAccounts = accounts.filter(a => platforms.includes(a.platform));

  return (
    <div className="modal-overlay" onClick={e => e.target === e.currentTarget && onClose()}>
      <div className="modal conv-modal">
        <div className="modal-header">
          <h2>{initial ? 'Edit Queue' : 'New Convention Queue'}</h2>
          <button className="modal-close" onClick={onClose}>×</button>
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

          <div className="form-group">
            <label>Posts per day</label>
            <div className="posts-per-day-btns">
              {[1, 2, 3].map(n => (
                <button
                  key={n}
                  type="button"
                  className={`ppd-btn ${postsPerDay === n ? 'active' : ''}`}
                  onClick={() => syncSlotCount(n)}
                >
                  {n}
                </button>
              ))}
            </div>
          </div>

          <div className="form-group">
            <label>Posting times (UTC)</label>
            <div className="time-slots">
              {Array.from({ length: postsPerDay }).map((_, i) => (
                <input
                  key={i}
                  className="input time-slot-input"
                  type="time"
                  value={timeSlots[i] ?? '12:00'}
                  onChange={e => updateTimeSlot(i, e.target.value)}
                />
              ))}
            </div>
          </div>

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

          <div className="modal-footer">
            <button type="button" className="btn btn-secondary" onClick={onClose}>Cancel</button>
            <button type="submit" className="btn btn-primary" disabled={saving}>
              {saving ? 'Saving…' : initial ? 'Save changes' : 'Create queue'}
            </button>
          </div>
        </form>
      </div>
    </div>
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
                <span className="conv-ppd">{q.postsPerDay}×/day</span>
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
