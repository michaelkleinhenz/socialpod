import { useState, useEffect, useCallback, useRef } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { api } from '../../services/api';
import type {
  ConventionQueue,
  ConventionQueueItem,
  ConventionQueueItemStatus,
  SchedulePreview,
  SocialAccount,
  Platform,
} from '../../types';
import { format, parseISO } from 'date-fns';
import {
  ArrowLeft,
  Upload,
  Sparkles,
  CheckCircle,
  XCircle,
  Loader,
  ChevronUp,
  ChevronDown,
  Trash2,
  RefreshCw,
  Edit3,
  Calendar,
  AlertTriangle,
} from 'lucide-react';
import toast from 'react-hot-toast';
import { PlatformIcon } from '../Common/PlatformIcon';
import { QueueFormModal } from './ConventionPage';
import './Convention.css';

const PLATFORM_OPTIONS: Platform[] = ['bluesky', 'instagram', 'twitter', 'mastodon', 'threads', 'linkedin'];

function ItemStatusIcon({ status, aiError }: { status: ConventionQueueItemStatus; aiError?: string }) {
  if (status === 'published') return <CheckCircle size={14} color="var(--success)" />;
  if (status === 'scheduled') return <Calendar size={14} color="var(--accent)" />;
  if (status === 'approved') return <CheckCircle size={14} color="var(--warning)" />;
  if (aiError) return <AlertTriangle size={14} color="var(--danger)" />;
  return <Loader size={14} color="var(--text-muted)" className="spin" />;
}

function QueueItemCard({
  item,
  index,
  total,
  queueId,
  accounts,
  queuePlatforms,
  onUpdate,
  onDelete,
  onAnalyze,
  onMoveUp,
  onMoveDown,
}: {
  item: ConventionQueueItem;
  index: number;
  total: number;
  queueId: string;
  accounts: SocialAccount[];
  queuePlatforms: Platform[];
  onUpdate: (item: ConventionQueueItem) => void;
  onDelete: () => void;
  onAnalyze: () => Promise<void>;
  onMoveUp: () => void;
  onMoveDown: () => void;
}) {
  const [caption, setCaption] = useState(item.caption ?? '');
  const [showOverrides, setShowOverrides] = useState(false);
  const [itemPlatforms, setItemPlatforms] = useState<Platform[]>(item.platforms ?? []);
  const [itemAccountIds, setItemAccountIds] = useState<Record<string, string>>(item.accountIds ?? {});
  const [analyzing, setAnalyzing] = useState(false);
  const [saving, setSaving] = useState(false);
  const saveTimer = useRef<ReturnType<typeof setTimeout>>(undefined);

  // Sync when item updates from parent (e.g. after AI analysis)
  useEffect(() => {
    setCaption(item.caption ?? '');
  }, [item.caption]);

  const debouncedSaveCaption = (value: string) => {
    setCaption(value);
    clearTimeout(saveTimer.current);
    saveTimer.current = setTimeout(async () => {
      try {
        const updated = await api.updateConventionItem(queueId, item.id, { caption: value });
        onUpdate(updated);
      } catch {
        // silently fail — user can retry
      }
    }, 800);
  };

  const toggleApprove = async () => {
    if (item.status === 'scheduled' || item.status === 'published') return;
    const newStatus = item.status === 'approved' ? 'pending' : 'approved';
    setSaving(true);
    try {
      const updated = await api.updateConventionItem(queueId, item.id, { status: newStatus });
      onUpdate(updated);
    } catch (err: any) {
      toast.error(err.message || 'Failed to update');
    } finally {
      setSaving(false);
    }
  };

  const handleAnalyze = async () => {
    setAnalyzing(true);
    try {
      await onAnalyze();
    } finally {
      setAnalyzing(false);
    }
  };

  const saveOverrides = async () => {
    try {
      const updated = await api.updateConventionItem(queueId, item.id, {
        platforms: itemPlatforms.length > 0 ? itemPlatforms : null,
        accountIds: Object.keys(itemAccountIds).length > 0 ? itemAccountIds : null,
      });
      onUpdate(updated);
      toast.success('Overrides saved');
    } catch (err: any) {
      toast.error(err.message || 'Failed to save overrides');
    }
  };

  const effectivePlatforms = itemPlatforms.length > 0 ? itemPlatforms : queuePlatforms;
  const isLocked = item.status === 'scheduled' || item.status === 'published';

  return (
    <div className={`conv-item-card ${item.status}`}>
      <div className="conv-item-thumb-wrap">
        <img src={item.imageUrl} alt="" className="conv-item-thumb" loading="lazy" />
        <span className={`conv-item-status-badge status-${item.status}`}>
          <ItemStatusIcon status={item.status} aiError={item.aiError} />
          {item.status}
        </span>
        <span className="conv-item-num">#{index + 1}</span>
      </div>

      <div className="conv-item-body">
        {item.aiError && (
          <div className="conv-ai-error">
            <AlertTriangle size={12} /> AI error: {item.aiError}
          </div>
        )}

        <textarea
          className="conv-caption-input"
          value={caption}
          onChange={e => debouncedSaveCaption(e.target.value)}
          placeholder="Caption will appear here after AI analysis, or type your own…"
          rows={4}
          disabled={isLocked}
        />

        <div className="conv-item-platforms">
          {effectivePlatforms.map(p => <PlatformIcon key={p} platform={p} />)}
          {itemPlatforms.length > 0 && <span className="conv-override-badge">overridden</span>}
        </div>

        <div className="conv-item-actions">
          <button
            className={`btn btn-sm ${item.status === 'approved' ? 'btn-success' : 'btn-secondary'}`}
            onClick={toggleApprove}
            disabled={saving || isLocked}
            title={item.status === 'approved' ? 'Click to unapprove' : 'Approve'}
          >
            <CheckCircle size={13} />
            {item.status === 'approved' ? 'Approved' : 'Approve'}
          </button>

          <button
            className="btn btn-sm btn-secondary"
            onClick={handleAnalyze}
            disabled={analyzing || isLocked}
            title="Regenerate AI caption"
          >
            {analyzing ? <Loader size={13} className="spin" /> : <RefreshCw size={13} />}
            {analyzing ? 'Analyzing…' : 'AI caption'}
          </button>

          <button
            className="btn btn-sm btn-secondary"
            onClick={() => setShowOverrides(v => !v)}
            title="Per-item platform overrides"
          >
            <Edit3 size={13} />
            Overrides
          </button>

          <button
            className="icon-btn danger"
            onClick={onDelete}
            disabled={isLocked}
            title="Remove from queue"
          >
            <Trash2 size={14} />
          </button>
        </div>

        {showOverrides && !isLocked && (
          <div className="conv-overrides-panel">
            <p className="conv-overrides-label">Platform overrides for this photo</p>
            <div className="platform-toggles small">
              {PLATFORM_OPTIONS.map(p => (
                <button
                  key={p}
                  type="button"
                  className={`platform-toggle small ${itemPlatforms.includes(p) ? 'active' : ''}`}
                  onClick={() =>
                    setItemPlatforms(prev =>
                      prev.includes(p) ? prev.filter(x => x !== p) : [...prev, p]
                    )
                  }
                >
                  <PlatformIcon platform={p} />
                </button>
              ))}
            </div>
            {accounts.filter(a => (itemPlatforms.length > 0 ? itemPlatforms : queuePlatforms).includes(a.platform)).map(a => (
              <div key={a.id} className="account-select-row small">
                <PlatformIcon platform={a.platform} />
                <select
                  className="form-input"
                  value={itemAccountIds[a.platform] ?? ''}
                  onChange={e => setItemAccountIds(prev => ({ ...prev, [a.platform]: e.target.value }))}
                >
                  <option value="">— use queue default —</option>
                  {accounts.filter(x => x.platform === a.platform).map(x => (
                    <option key={x.id} value={x.id}>{x.displayName || x.accountName}</option>
                  ))}
                </select>
              </div>
            ))}
            <button className="btn btn-sm btn-primary" onClick={saveOverrides}>Save overrides</button>
          </div>
        )}
      </div>

      <div className="conv-item-reorder">
        <button className="icon-btn" onClick={onMoveUp} disabled={index === 0} title="Move up">
          <ChevronUp size={16} />
        </button>
        <button className="icon-btn" onClick={onMoveDown} disabled={index === total - 1} title="Move down">
          <ChevronDown size={16} />
        </button>
      </div>
    </div>
  );
}

function SchedulePreviewModal({
  preview,
  items,
  onConfirm,
  onClose,
}: {
  preview: SchedulePreview;
  items: ConventionQueueItem[];
  onConfirm: () => Promise<void>;
  onClose: () => void;
}) {
  const [confirming, setConfirming] = useState(false);
  const itemMap = Object.fromEntries(items.map(i => [i.id, i]));

  const handleConfirm = async () => {
    setConfirming(true);
    try {
      await onConfirm();
    } finally {
      setConfirming(false);
    }
  };

  return (
    <div className="modal-overlay" onClick={e => e.target === e.currentTarget && onClose()}>
      <div className="modal conv-preview-modal">
        <div className="modal-header">
          <h2>Schedule preview</h2>
          <button className="modal-close" onClick={onClose}>×</button>
        </div>

        <div className="conv-preview-body">
          {preview.overflow > 0 && (
            <div className="conv-preview-warning">
              <AlertTriangle size={14} />
              {preview.overflow} photo{preview.overflow > 1 ? 's' : ''} won't fit in the current window.
              Extend the date range or increase posts per day to schedule all.
            </div>
          )}

          <p className="conv-preview-summary">
            Scheduling {preview.slots.length} of {preview.approvedCount} approved photos
            across {preview.availableSlots} available slots.
          </p>

          <div className="conv-preview-list">
            {preview.slots.map(slot => {
              const item = itemMap[slot.itemId];
              return (
                <div key={slot.itemId} className="conv-preview-row">
                  {item && <img src={item.imageUrl} alt="" className="conv-preview-thumb" />}
                  <div className="conv-preview-info">
                    <div className="conv-preview-time">
                      {format(parseISO(slot.scheduledAt), 'EEE, MMM d, yyyy — HH:mm')} UTC
                    </div>
                    {item?.caption && (
                      <div className="conv-preview-caption">
                        {item.caption.length > 80 ? item.caption.slice(0, 80) + '…' : item.caption}
                      </div>
                    )}
                  </div>
                </div>
              );
            })}
          </div>
        </div>

        <div className="modal-footer">
          <button className="btn btn-secondary" onClick={onClose}>Cancel</button>
          <button className="btn btn-primary" onClick={handleConfirm} disabled={confirming}>
            {confirming ? 'Scheduling…' : `Schedule ${preview.slots.length} posts`}
          </button>
        </div>
      </div>
    </div>
  );
}

export function QueueDetailPage() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();

  const [queue, setQueue] = useState<ConventionQueue | null>(null);
  const [items, setItems] = useState<ConventionQueueItem[]>([]);
  const [accounts, setAccounts] = useState<SocialAccount[]>([]);
  const [loading, setLoading] = useState(true);
  const [uploading, setUploading] = useState(false);
  const [analyzingAll, setAnalyzingAll] = useState(false);
  const [dragOver, setDragOver] = useState(false);
  const [showEdit, setShowEdit] = useState(false);
  const [preview, setPreview] = useState<SchedulePreview | null>(null);
  const [loadingPreview, setLoadingPreview] = useState(false);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const pollRef = useRef<ReturnType<typeof setInterval>>(undefined);

  const loadQueue = useCallback(async () => {
    if (!id) return;
    try {
      const [data, accs] = await Promise.all([api.getConventionQueue(id), api.getActiveAccounts()]);
      setQueue(data.queue);
      setItems(data.items);
      setAccounts(accs);
    } catch {
      toast.error('Failed to load queue');
    } finally {
      setLoading(false);
    }
  }, [id]);

  useEffect(() => { loadQueue(); }, [loadQueue]);

  // Poll every 3s while any item is in pending-but-being-analyzed state
  useEffect(() => {
    const hasPending = items.some(i => i.status === 'pending' && !i.aiError && !i.caption);
    if (hasPending) {
      pollRef.current = setInterval(loadQueue, 3000);
    } else {
      clearInterval(pollRef.current);
    }
    return () => clearInterval(pollRef.current);
  }, [items, loadQueue]);

  const uploadFiles = async (files: File[]) => {
    if (!id || files.length === 0) return;
    setUploading(true);
    const imageFiles = files.filter(f => f.type.startsWith('image/'));
    if (imageFiles.length === 0) {
      toast.error('Only image files are supported');
      setUploading(false);
      return;
    }
    let added = 0;
    for (const file of imageFiles) {
      try {
        const item = await api.addConventionItem(id, file);
        setItems(prev => [...prev, item]);
        added++;
      } catch (err: any) {
        toast.error(`Failed to upload ${file.name}: ${err.message}`);
      }
    }
    if (added > 0) toast.success(`Added ${added} photo${added > 1 ? 's' : ''}`);
    setUploading(false);
  };

  const handleFileInput = (e: React.ChangeEvent<HTMLInputElement>) => {
    if (e.target.files) uploadFiles(Array.from(e.target.files));
    e.target.value = '';
  };

  const handleDrop = (e: React.DragEvent) => {
    e.preventDefault();
    setDragOver(false);
    if (e.dataTransfer.files) uploadFiles(Array.from(e.dataTransfer.files));
  };

  const handleAnalyzeAll = async () => {
    if (!id) return;
    setAnalyzingAll(true);
    try {
      const res = await api.analyzeAllConventionItems(id);
      if (res.count === 0) {
        toast('No pending photos to analyze');
      } else {
        toast.success(`AI analyzing ${res.count} photos in the background…`);
      }
    } catch (err: any) {
      toast.error(err.message || 'Failed to start analysis');
    } finally {
      setAnalyzingAll(false);
    }
  };

  const handleAnalyzeItem = async (item: ConventionQueueItem) => {
    if (!id) return;
    try {
      const updated = await api.analyzeConventionItem(id, item.id);
      setItems(prev => prev.map(i => (i.id === item.id ? updated : i)));
    } catch (err: any) {
      toast.error(err.message || 'AI analysis failed');
    }
  };

  const handleUpdateItem = (updated: ConventionQueueItem) => {
    setItems(prev => prev.map(i => (i.id === updated.id ? updated : i)));
  };

  const handleDeleteItem = async (item: ConventionQueueItem) => {
    if (!id) return;
    try {
      await api.deleteConventionItem(id, item.id);
      setItems(prev => prev.filter(i => i.id !== item.id));
    } catch (err: any) {
      toast.error(err.message || 'Failed to delete item');
    }
  };

  const handleMoveItem = async (index: number, direction: 'up' | 'down') => {
    if (!id) return;
    const newItems = [...items];
    const swapIdx = direction === 'up' ? index - 1 : index + 1;
    [newItems[index], newItems[swapIdx]] = [newItems[swapIdx], newItems[index]];
    setItems(newItems);
    await api.reorderConventionItems(id, newItems.map(i => i.id));
  };

  const handlePreview = async () => {
    if (!id) return;
    setLoadingPreview(true);
    try {
      const data = await api.previewConventionSchedule(id);
      if (data.approvedCount === 0) {
        toast('No approved photos yet — approve some photos first');
        return;
      }
      setPreview(data);
    } catch (err: any) {
      toast.error(err.message || 'Failed to load preview');
    } finally {
      setLoadingPreview(false);
    }
  };

  const handleSchedule = async () => {
    if (!id) return;
    try {
      const res = await api.scheduleConventionItems(id);
      toast.success(`Scheduled ${res.scheduled} post${res.scheduled !== 1 ? 's' : ''}`);
      if (res.overflow > 0) {
        toast(`${res.overflow} photo${res.overflow > 1 ? 's' : ''} didn't fit — extend the date range`, { icon: '⚠️' });
      }
      setPreview(null);
      loadQueue();
    } catch (err: any) {
      toast.error(err.message || 'Failed to schedule');
    }
  };

  const handleEditQueue = async (data: any) => {
    if (!id) return;
    const updated = await api.updateConventionQueue(id, data);
    setQueue(updated);
    toast.success('Queue updated');
    setShowEdit(false);
  };

  if (loading) return <div className="page"><div className="loading-screen"><div className="spinner" /></div></div>;
  if (!queue) return <div className="page"><p>Queue not found.</p></div>;

  const approvedCount = items.filter(i => i.status === 'approved').length;
  const scheduledCount = items.filter(i => i.status === 'scheduled' || i.status === 'published').length;
  const pendingCount = items.filter(i => i.status === 'pending').length;
  const analyzingCount = items.filter(i => i.status === 'pending' && !i.caption && !i.aiError).length;

  return (
    <div className="page conv-detail-page">
      {/* Header */}
      <div className="conv-detail-header">
        <button className="icon-btn" onClick={() => navigate('/convention')} title="Back">
          <ArrowLeft size={18} />
        </button>
        <div className="conv-detail-title">
          <h1>{queue.name}</h1>
          <div className="conv-detail-meta">
            <span>{format(parseISO(queue.startDate), 'MMM d')} – {format(parseISO(queue.endDate), 'MMM d, yyyy')}</span>
            <span>·</span>
            <span>{queue.postsPerDay}×/day</span>
            {queue.hashtags && queue.hashtags.length > 0 && (
              <>
                <span>·</span>
                <span>{queue.hashtags.join(' ')}</span>
              </>
            )}
          </div>
        </div>
        <button className="btn btn-secondary btn-sm" onClick={() => setShowEdit(true)}>
          Edit
        </button>
      </div>

      {/* Stats bar */}
      <div className="conv-stats-bar">
        <div className="conv-stat-pill">{items.length} photos</div>
        <div className="conv-stat-pill pending">{pendingCount} pending</div>
        <div className="conv-stat-pill approved">{approvedCount} approved</div>
        <div className="conv-stat-pill scheduled">{scheduledCount} scheduled/published</div>
        {analyzingCount > 0 && (
          <div className="conv-stat-pill analyzing">
            <Loader size={11} className="spin" /> {analyzingCount} analyzing…
          </div>
        )}
      </div>

      {/* Upload zone */}
      <div
        className={`conv-upload-zone ${dragOver ? 'drag-over' : ''} ${uploading ? 'uploading' : ''}`}
        onDragOver={e => { e.preventDefault(); setDragOver(true); }}
        onDragLeave={() => setDragOver(false)}
        onDrop={handleDrop}
        onClick={() => fileInputRef.current?.click()}
      >
        <input
          ref={fileInputRef}
          type="file"
          accept="image/*"
          multiple
          style={{ display: 'none' }}
          onChange={handleFileInput}
        />
        {uploading ? (
          <>
            <Loader size={28} className="spin" color="var(--accent)" />
            <p>Uploading…</p>
          </>
        ) : (
          <>
            <Upload size={28} color="var(--text-muted)" />
            <p>Drop photos here or tap to select</p>
            <p className="conv-upload-hint">Supports JPG, PNG, WebP · Multiple files OK</p>
          </>
        )}
      </div>

      {/* Toolbar */}
      {items.length > 0 && (
        <div className="conv-toolbar">
          <button
            className="btn btn-secondary"
            onClick={handleAnalyzeAll}
            disabled={analyzingAll || pendingCount === 0}
          >
            {analyzingAll ? <Loader size={15} className="spin" /> : <Sparkles size={15} />}
            {analyzingAll ? 'Starting…' : `AI captions for ${pendingCount} pending`}
          </button>
          <span className="conv-toolbar-hint">
            {items.filter(i => i.status === 'pending' && i.caption).length} have captions,{' '}
            {approvedCount} approved
          </span>
        </div>
      )}

      {/* Item grid */}
      {items.length > 0 ? (
        <div className="conv-item-grid">
          {items.map((item, idx) => (
            <QueueItemCard
              key={item.id}
              item={item}
              index={idx}
              total={items.length}
              queueId={queue.id}
              accounts={accounts}
              queuePlatforms={queue.platforms}
              onUpdate={handleUpdateItem}
              onDelete={() => handleDeleteItem(item)}
              onAnalyze={() => handleAnalyzeItem(item)}
              onMoveUp={() => handleMoveItem(idx, 'up')}
              onMoveDown={() => handleMoveItem(idx, 'down')}
            />
          ))}
        </div>
      ) : (
        <div className="empty-state" style={{ marginTop: 24 }}>
          <XCircle size={36} color="var(--text-muted)" />
          <p style={{ marginTop: 12, color: 'var(--text-muted)' }}>No photos yet — upload some above</p>
        </div>
      )}

      {/* Sticky schedule footer */}
      {approvedCount > 0 && (
        <div className="conv-schedule-bar">
          <span className="conv-schedule-info">
            <CheckCircle size={16} color="var(--warning)" />
            {approvedCount} photo{approvedCount !== 1 ? 's' : ''} approved and ready to schedule
          </span>
          <button
            className="btn btn-primary"
            onClick={handlePreview}
            disabled={loadingPreview}
          >
            {loadingPreview ? <Loader size={15} className="spin" /> : <Calendar size={15} />}
            Preview &amp; schedule
          </button>
        </div>
      )}

      {showEdit && (
        <QueueFormModal
          initial={queue}
          accounts={accounts}
          onSave={handleEditQueue}
          onClose={() => setShowEdit(false)}
        />
      )}

      {preview && (
        <SchedulePreviewModal
          preview={preview}
          items={items}
          onConfirm={handleSchedule}
          onClose={() => setPreview(null)}
        />
      )}
    </div>
  );
}
