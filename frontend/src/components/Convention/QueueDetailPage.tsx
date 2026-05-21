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
  Suffix,
} from '../../types';
import { format, parseISO } from 'date-fns';
import {
  ArrowLeft,
  Upload,
  Sparkles,
  CheckCircle,
  XCircle,
  X,
  Loader,
  ChevronUp,
  ChevronDown,
  Trash2,
  RefreshCw,
  Edit3,
  Calendar,
  AlertTriangle,
  Pencil,
  Filter,
} from 'lucide-react';
import toast from 'react-hot-toast';
import FilerobotImageEditor, { TABS } from 'react-filerobot-image-editor';
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
  suffixes,
  queuePlatforms,
  queueSuffixIds,
  onUpdate,
  onDelete,
  onAnalyze,
  onMoveUp,
  onMoveDown,
  onEditImage,
}: {
  item: ConventionQueueItem;
  index: number;
  total: number;
  queueId: string;
  accounts: SocialAccount[];
  suffixes: Suffix[];
  queuePlatforms: Platform[];
  queueSuffixIds: Record<string, string>;
  onUpdate: (item: ConventionQueueItem) => void;
  onDelete: () => void;
  onAnalyze: () => Promise<void>;
  onMoveUp: () => void;
  onMoveDown: () => void;
  onEditImage: () => void;
}) {
  const [caption, setCaption] = useState(item.caption ?? '');
  const [showOverrides, setShowOverrides] = useState(false);
  const [itemPlatforms, setItemPlatforms] = useState<Platform[]>(item.platforms ?? []);
  const [itemAccountIds, setItemAccountIds] = useState<Record<string, string>>(item.accountIds ?? {});
  const [itemSuffixIds, setItemSuffixIds] = useState<Record<string, string>>(item.suffixIds ?? {});
  const [analyzing, setAnalyzing] = useState(false);
  const [saving, setSaving] = useState(false);
  const saveTimer = useRef<ReturnType<typeof setTimeout>>(undefined);
  const suffixSaveTimer = useRef<ReturnType<typeof setTimeout>>(undefined);

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
        // silently fail
      }
    }, 800);
  };

  const saveSuffixIds = (newSuffixIds: Record<string, string>) => {
    clearTimeout(suffixSaveTimer.current);
    suffixSaveTimer.current = setTimeout(async () => {
      try {
        const updated = await api.updateConventionItem(queueId, item.id, { suffixIds: newSuffixIds });
        onUpdate(updated);
      } catch {
        // silently fail
      }
    }, 600);
  };

  const handleSuffixChange = (platform: string, value: string) => {
    const next = { ...itemSuffixIds };
    if (value) next[platform] = value;
    else delete next[platform];
    setItemSuffixIds(next);
    saveSuffixIds(next);
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

  const suffixPlatformLabels: Record<string, string> = {
    bluesky: 'Bluesky',
    instagram: 'Instagram',
    twitter: 'X/Twitter',
    mastodon: 'Mastodon',
    threads: 'Threads',
    linkedin: 'LinkedIn',
  };

  return (
    <div className={`conv-item-card ${item.status}`}>
      <div className="conv-item-thumb-wrap">
        <img src={item.imageUrl} alt="" className="conv-item-thumb" loading="lazy" />
        <span className={`conv-item-status-badge status-${item.status}`}>
          <ItemStatusIcon status={item.status} aiError={item.aiError} />
          {item.status}
        </span>
        <span className="conv-item-num">#{index + 1}</span>
        {!isLocked && (
          <button
            className="conv-item-edit-btn"
            onClick={e => { e.stopPropagation(); onEditImage(); }}
            title="Edit image"
          >
            <Pencil size={12} />
          </button>
        )}
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

        {suffixes.length > 0 && (
          <div className="conv-suffix-selectors">
            {effectivePlatforms.map(platform => (
              <div key={platform} className="conv-suffix-row">
                <label>
                  <PlatformIcon platform={platform} size={11} />
                  {suffixPlatformLabels[platform] ?? platform}
                </label>
                <select
                  className="select"
                  value={itemSuffixIds[platform] ?? queueSuffixIds[platform] ?? ''}
                  onChange={e => handleSuffixChange(platform, e.target.value)}
                  disabled={isLocked}
                >
                  <option value="">— queue default —</option>
                  {suffixes.map(s => (
                    <option key={s.id} value={s.id}>{s.name}</option>
                  ))}
                </select>
              </div>
            ))}
          </div>
        )}

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
                  className="select"
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
          <button className="btn btn-ghost btn-sm" onClick={onClose}><X size={18} /></button>
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
  const [suffixes, setSuffixes] = useState<Suffix[]>([]);
  const [loading, setLoading] = useState(true);
  const [uploading, setUploading] = useState(false);
  const [analyzingAll, setAnalyzingAll] = useState(false);
  const [dragOver, setDragOver] = useState(false);
  const [showEdit, setShowEdit] = useState(false);
  const [preview, setPreview] = useState<SchedulePreview | null>(null);
  const [loadingPreview, setLoadingPreview] = useState(false);
  const [filterPending, setFilterPending] = useState(false);
  const [editingImageItem, setEditingImageItem] = useState<ConventionQueueItem | null>(null);
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

  useEffect(() => {
    api.getSuffixes().then(setSuffixes).catch(() => {});
  }, []);

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

  const handleImageEditSave = (savedImageData: any) => {
    if (!id || !editingImageItem) return;
    const canvas = savedImageData.imageCanvas as HTMLCanvasElement | undefined;
    const base64 = savedImageData.imageBase64 as string | undefined;
    const src = base64 || canvas?.toDataURL('image/png');
    if (!src) {
      toast.error('Failed to get edited image');
      setEditingImageItem(null);
      return;
    }
    const itemId = editingImageItem.id;
    fetch(src)
      .then(res => res.blob())
      .then(blob => {
        const ext = savedImageData.extension || 'png';
        const file = new File([blob], `edited.${ext}`, { type: savedImageData.mimeType || 'image/png' });
        return api.replaceConventionItemImage(id!, itemId, file);
      })
      .then(updated => {
        setItems(prev => prev.map(i => (i.id === itemId ? updated : i)));
        setEditingImageItem(null);
        toast.success('Image updated');
      })
      .catch(() => {
        toast.error('Failed to upload edited image');
        setEditingImageItem(null);
      });
  };

  if (loading) return <div className="page"><div className="loading-screen"><div className="spinner" /></div></div>;
  if (!queue) return <div className="page"><p>Queue not found.</p></div>;

  const approvedCount = items.filter(i => i.status === 'approved').length;
  const scheduledCount = items.filter(i => i.status === 'scheduled' || i.status === 'published').length;
  const pendingCount = items.filter(i => i.status === 'pending').length;
  const analyzingCount = items.filter(i => i.status === 'pending' && !i.caption && !i.aiError).length;

  const visibleItems = filterPending
    ? items.filter(i => i.status === 'pending')
    : items;

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
          <div style={{ marginLeft: 'auto' }}>
            <button
              className={`conv-filter-btn ${filterPending ? 'active' : ''}`}
              onClick={() => setFilterPending(v => !v)}
              title={filterPending ? 'Show all items' : 'Show only pending'}
            >
              <Filter size={13} />
              {filterPending ? 'Pending only' : 'All items'}
            </button>
          </div>
        </div>
      )}

      {/* Item grid */}
      {visibleItems.length > 0 ? (
        <div className="conv-item-grid">
          {visibleItems.map((item) => (
            <QueueItemCard
              key={item.id}
              item={item}
              index={items.indexOf(item)}
              total={items.length}
              queueId={queue.id}
              accounts={accounts}
              suffixes={suffixes}
              queuePlatforms={queue.platforms}
              queueSuffixIds={queue.suffixIds ?? {}}
              onUpdate={handleUpdateItem}
              onDelete={() => handleDeleteItem(item)}
              onAnalyze={() => handleAnalyzeItem(item)}
              onMoveUp={() => handleMoveItem(items.indexOf(item), 'up')}
              onMoveDown={() => handleMoveItem(items.indexOf(item), 'down')}
              onEditImage={() => setEditingImageItem(item)}
            />
          ))}
        </div>
      ) : (
        <div className="empty-state" style={{ marginTop: 24 }}>
          <XCircle size={36} color="var(--text-muted)" />
          <p style={{ marginTop: 12, color: 'var(--text-muted)' }}>
            {filterPending && items.length > 0
              ? 'No pending photos — all items are approved or scheduled'
              : 'No photos yet — upload some above'}
          </p>
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

      {editingImageItem && (
        <div className="image-editor-overlay">
          <FilerobotImageEditor
            source={editingImageItem.imageUrl}
            tabsIds={[TABS.ADJUST, TABS.ANNOTATE, TABS.WATERMARK, TABS.FILTERS, TABS.FINETUNE, TABS.RESIZE]}
            defaultTabId={TABS.ANNOTATE}
            savingPixelRatio={2}
            previewPixelRatio={2}
            defaultSavedImageType="png"
            theme={{
              palette: {
                'bg-secondary': '#1e293b',
                'bg-primary': '#0f172a',
                'bg-primary-active': '#334155',
                'bg-primary-hover': '#263348',
                'bg-primary-light': '#1e293b',
                'bg-primary-stateless': '#334155',
                'bg-primary-0-5-opacity': 'rgba(30, 41, 59, 0.5)',
                'bg-stateless': '#1e293b',
                'bg-hover': '#263348',
                'bg-active': '#334155',
                'bg-grey': '#334155',
                'bg-tooltip': '#475569',
                'txt-primary': '#f1f5f9',
                'txt-secondary': '#94a3b8',
                'txt-secondary-invert': '#f1f5f9',
                'txt-placeholder': '#64748b',
                'accent-primary': '#818cf8',
                'accent-primary-hover': '#a5b4fc',
                'accent-primary-active': '#6366f1',
                'accent-stateless': '#818cf8',
                'accent-stateless_0_4_opacity': 'rgba(129, 140, 248, 0.4)',
                'accent_0_5_opacity': 'rgba(129, 140, 248, 0.05)',
                'accent_1_2_opacity': 'rgba(129, 140, 248, 0.12)',
                'accent_1_8_opacity': 'rgba(129, 140, 248, 0.18)',
                'accent_2_8_opacity': 'rgba(129, 140, 248, 0.28)',
                'accent_4_0_opacity': 'rgba(129, 140, 248, 0.4)',
                'accent-primary-disabled': '#334155',
                'accent-secondary-disabled': '#1e293b',
                'icon-primary': '#cbd5e1',
                'icons-primary-opacity-0-6': 'rgba(203, 213, 225, 0.6)',
                'icons-secondary': '#94a3b8',
                'icons-secondary-hover': '#cbd5e1',
                'icons-primary-hover': '#f1f5f9',
                'icons-invert': '#f1f5f9',
                'icons-placeholder': '#475569',
                'icons-muted': '#64748b',
                'borders-primary': '#334155',
                'borders-primary-hover': '#64748b',
                'borders-secondary': '#1e293b',
                'borders-strong': '#475569',
                'borders-invert': '#1e293b',
                'borders-item': '#334155',
                'borders-button': '#64748b',
                'borders-disabled': 'rgba(99, 102, 241, 0.3)',
                'border-primary-stateless': '#334155',
                'border-hover-bottom': 'rgba(129, 140, 248, 0.18)',
                'border-active-bottom': '#818cf8',
                'btn-primary-text': '#ffffff',
                'btn-primary-text-0-6': 'rgba(255, 255, 255, 0.6)',
                'btn-primary-text-0-4': 'rgba(255, 255, 255, 0.4)',
                'btn-secondary-text': '#f1f5f9',
                'btn-disabled-text': '#64748b',
                'link-primary': '#94a3b8',
                'link-hover': '#f1f5f9',
                'link-active': '#f1f5f9',
                'link-stateless': '#94a3b8',
                'link-pressed': '#818cf8',
                'link-muted': '#64748b',
                'active-secondary': '#1e293b',
                'active-secondary-hover': 'rgba(99, 102, 241, 0.15)',
                'error': '#ef4444',
                'error-hover': '#dc2626',
                'error-active': '#b91c1c',
                'success': '#22c55e',
                'success-hover': '#16a34a',
                'warning': '#f59e0b',
                'warning-hover': '#d97706',
                'info': '#3b82f6',
                'tag': '#94a3b8',
                'bg-base-light': '#1e293b',
                'bg-base-medium': '#334155',
                'borders-base-light': '#334155',
                'borders-base-medium': '#475569',
                'extra-0-3-overlay': 'rgba(15, 23, 42, 0.3)',
                'extra-0-5-overlay': 'rgba(15, 23, 42, 0.5)',
                'extra-0-7-overlay': 'rgba(15, 23, 42, 0.7)',
                'extra-0-9-overlay': 'rgba(15, 23, 42, 0.9)',
                'white-0-7-8-overlay': 'rgba(15, 23, 42, 0.78)',
                'gradient-right': 'linear-gradient(270deg, #0f172a 1.56%, rgba(15,23,42,0.89) 52.4%, rgba(15,23,42,0.53) 76.04%, rgba(15,23,42,0) 100%)',
                'gradient-right-active': 'linear-gradient(270deg, #1e293b 1.56%, #1e293b 52.4%, rgba(30,41,59,0.53) 76.04%, rgba(30,41,59,0) 100%)',
                'gradient-right-hover': 'linear-gradient(270deg, #263348 1.56%, #263348 52.4%, rgba(38,51,72,0.53) 76.04%, rgba(38,51,72,0) 100%)',
              },
              typography: {
                fontFamily: 'Inter, -apple-system, BlinkMacSystemFont, sans-serif',
              },
            }}
            Crop={{
              presetsItems: [
                { titleKey: 'Square (1:1)', ratio: 1 },
                { titleKey: 'Story (9:16)', ratio: 9 / 16 },
                { titleKey: 'Landscape (16:9)', ratio: 16 / 9 },
                { titleKey: 'Portrait (4:5)', ratio: 4 / 5 },
              ],
            }}
            onBeforeSave={() => false}
            onSave={handleImageEditSave}
            onClose={() => setEditingImageItem(null)}
          />
        </div>
      )}
    </div>
  );
}
