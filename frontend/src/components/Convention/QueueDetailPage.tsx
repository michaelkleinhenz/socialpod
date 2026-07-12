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
  Watermark,
} from '../../types';
import { format, parseISO } from 'date-fns';
import {
  ArrowLeft,
  Upload,
  CheckCircle,
  XCircle,
  X,
  Loader,
  Trash2,
  RefreshCw,
  Calendar,
  AlertTriangle,
  Pencil,
  Filter,
  Link,
  Smartphone,
  Copy,
  Check,
  Send,
  Images,
} from 'lucide-react';
import toast from 'react-hot-toast';
import FilerobotImageEditor, { TABS } from 'react-filerobot-image-editor';
import { PlatformIcon } from '../Common/PlatformIcon';
import { QueueFormModal } from './ConventionPage';
import './Convention.css';

// Per-platform character limits — mirror the PostEditor logic so captions are
// validated the same way here.
const BLUESKY_LIMIT = 300;
const INSTAGRAM_LIMIT = 2200;
const TWITTER_LIMIT = 280;
const MASTODON_LIMIT = 500;
const LINKEDIN_LIMIT = 3000;

function platformLimit(platform: Platform) {
  if (platform === 'bluesky') return BLUESKY_LIMIT;
  if (platform === 'twitter') return TWITTER_LIMIT;
  if (platform === 'mastodon') return MASTODON_LIMIT;
  if (platform === 'linkedin') return LINKEDIN_LIMIT;
  return INSTAGRAM_LIMIT;
}

function ItemStatusIcon({ status, aiError }: { status: ConventionQueueItemStatus; aiError?: string }) {
  if (status === 'scheduled') return <Calendar size={14} color="var(--accent)" />;
  if (status === 'approved') return <CheckCircle size={14} color="var(--warning)" />;
  if (aiError) return <AlertTriangle size={14} color="var(--danger)" />;
  return <Loader size={14} color="var(--text-muted)" className="spin" />;
}

function QueueItemCard({
  item,
  index,
  queueId,
  suffixes,
  queuePlatforms,
  queueSuffixIds,
  overlayUrl,
  onUpdate,
  onDelete,
  onAnalyze,
  onEditImage,
  onPostNow,
}: {
  item: ConventionQueueItem;
  index: number;
  queueId: string;
  suffixes: Suffix[];
  queuePlatforms: Platform[];
  queueSuffixIds: Record<string, string>;
  overlayUrl?: string;
  onUpdate: (item: ConventionQueueItem) => void;
  onDelete: () => void;
  onAnalyze: () => Promise<void>;
  onEditImage: () => void;
  onPostNow: () => Promise<void>;
}) {
  const [caption, setCaption] = useState(item.caption ?? '');
  const [bggUrl, setBggUrl] = useState(item.bggUrl ?? '');
  const [itemSuffixIds, setItemSuffixIds] = useState<Record<string, string>>(item.suffixIds ?? {});
  const [analyzing, setAnalyzing] = useState(false);
  const [galleryIndex, setGalleryIndex] = useState(0);
  const galleryRef = useRef<HTMLDivElement>(null);
  const [saving, setSaving] = useState(false);
  const [posting, setPosting] = useState(false);
  const [captionSaved, setCaptionSaved] = useState(false);
  const saveTimer = useRef<ReturnType<typeof setTimeout>>(undefined);
  const savedFlashTimer = useRef<ReturnType<typeof setTimeout>>(undefined);
  const bggSaveTimer = useRef<ReturnType<typeof setTimeout>>(undefined);
  const suffixSaveTimer = useRef<ReturnType<typeof setTimeout>>(undefined);

  useEffect(() => {
    setCaption(item.caption ?? '');
  }, [item.caption]);

  useEffect(() => {
    setBggUrl(item.bggUrl ?? '');
  }, [item.bggUrl]);

  const debouncedSaveBggUrl = (value: string) => {
    setBggUrl(value);
    clearTimeout(bggSaveTimer.current);
    bggSaveTimer.current = setTimeout(async () => {
      try {
        const updated = await api.updateConventionItem(queueId, item.id, { bggUrl: value });
        onUpdate(updated);
      } catch {
        // silently fail
      }
    }, 800);
  };

  const debouncedSaveCaption = (value: string) => {
    setCaption(value);
    // Hide the "saved" hint while the user is still typing.
    setCaptionSaved(false);
    clearTimeout(saveTimer.current);
    saveTimer.current = setTimeout(async () => {
      try {
        const updated = await api.updateConventionItem(queueId, item.id, { caption: value });
        onUpdate(updated);
        // Briefly surface a subtle confirmation that the caption was saved.
        setCaptionSaved(true);
        clearTimeout(savedFlashTimer.current);
        savedFlashTimer.current = setTimeout(() => setCaptionSaved(false), 2000);
      } catch {
        // silently fail
      }
    }, 800);
  };

  // Clear pending timers on unmount so no state updates fire afterwards.
  useEffect(() => () => {
    clearTimeout(saveTimer.current);
    clearTimeout(savedFlashTimer.current);
    clearTimeout(bggSaveTimer.current);
    clearTimeout(suffixSaveTimer.current);
  }, []);

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
    if (item.status === 'scheduled') return;
    // Block approving a caption that exceeds the character limit, matching the
    // PostEditor which refuses to schedule over-limit content.
    if (item.status !== 'approved' && overLimit) {
      toast.error(`Caption exceeds the ${charLimit} character limit`);
      return;
    }
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

  const handlePostNow = async () => {
    if (overLimit) {
      toast.error(`Caption exceeds the ${charLimit} character limit`);
      return;
    }
    if (!confirm('Post this photo now? It will go out on the next scheduler run.')) return;
    setPosting(true);
    try {
      await onPostNow();
    } finally {
      setPosting(false);
    }
  };

  const effectivePlatforms = item.platforms && item.platforms.length > 0 ? item.platforms : queuePlatforms;

  // Character limit: strictest of the effective platforms, minus the length of
  // the selected suffix for each (an item override falls back to the queue
  // default). Mirrors the PostEditor so captions are validated identically.
  const suffixLen = (platform: Platform) => {
    const id = itemSuffixIds[platform] ?? queueSuffixIds[platform];
    if (!id) return 0;
    const s = suffixes.find(x => x.id === id);
    return s ? s.content.length + 1 : 0; // +1 for the newline separator
  };
  const effectiveLimit = (platform: Platform) => platformLimit(platform) - suffixLen(platform);
  const charLimit = effectivePlatforms.length === 0
    ? effectiveLimit('instagram')
    : Math.min(...effectivePlatforms.map(effectiveLimit));
  const charCount = caption.length;
  const overLimit = charCount > charLimit;
  const charClass = overLimit ? 'danger' : charCount > charLimit * 0.9 ? 'warning' : '';

  // Scheduled items are locked from editing/approval, but can still be pulled
  // back from the queue (which cancels the pending post). Once a post is
  // published its item is removed from the queue entirely.
  const isLocked = item.status === 'scheduled';

  const suffixPlatformLabels: Record<string, string> = {
    bluesky: 'Bluesky',
    instagram: 'Instagram',
    twitter: 'X/Twitter',
    mastodon: 'Mastodon',
    threads: 'Threads',
    linkedin: 'LinkedIn',
  };

  // Gallery items carry multiple images; fall back to the single primary image.
  const galleryImages = item.imageUrls && item.imageUrls.length > 0 ? item.imageUrls : [item.imageUrl];
  const isGallery = galleryImages.length > 1;

  // Track the visible slide while the user swipes horizontally so the counter
  // and dots reflect the current image.
  const handleGalleryScroll = () => {
    const el = galleryRef.current;
    if (!el) return;
    const idx = Math.round(el.scrollLeft / el.clientWidth);
    setGalleryIndex(prev => (prev === idx ? prev : idx));
  };

  const scrollToImage = (idx: number) => {
    const el = galleryRef.current;
    if (!el) return;
    el.scrollTo({ left: idx * el.clientWidth, behavior: 'smooth' });
  };

  return (
    <div className={`conv-item-card ${item.status}`}>
      <div className="conv-item-thumb-wrap">
        {isGallery ? (
          <div
            className="conv-item-gallery"
            ref={galleryRef}
            onScroll={handleGalleryScroll}
          >
            {galleryImages.map((src, i) => (
              <img
                key={i}
                src={src}
                alt=""
                className="conv-item-thumb conv-item-gallery-slide"
                loading="lazy"
              />
            ))}
          </div>
        ) : (
          <img src={item.imageUrl} alt="" className="conv-item-thumb" loading="lazy" />
        )}
        {overlayUrl && (
          <img src={overlayUrl} alt="" className="conv-item-overlay" aria-hidden="true" />
        )}
        <span className={`conv-item-status-badge status-${item.status}`}>
          <ItemStatusIcon status={item.status} aiError={item.aiError} />
          {item.status}
        </span>
        <span className="conv-item-num">#{index + 1}</span>
        {isGallery && (
          <>
            <span className="conv-item-gallery-badge" title={`${galleryImages.length} images`}>
              <Images size={11} /> {galleryIndex + 1} / {galleryImages.length}
            </span>
            <div className="conv-item-gallery-dots" aria-hidden="true">
              {galleryImages.map((_, i) => (
                <button
                  key={i}
                  type="button"
                  className={`conv-item-gallery-dot ${i === galleryIndex ? 'active' : ''}`}
                  onClick={e => { e.stopPropagation(); scrollToImage(i); }}
                  title={`Image ${i + 1}`}
                />
              ))}
            </div>
          </>
        )}
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

        <div className="conv-bgg-row">
          <Link size={12} className="conv-bgg-icon" />
          <input
            type="url"
            className="conv-bgg-input"
            value={bggUrl}
            onChange={e => debouncedSaveBggUrl(e.target.value)}
            placeholder="BGG URL (optional, used by AI caption)"
            disabled={isLocked}
          />
        </div>

        <textarea
          className="conv-caption-input"
          value={caption}
          onChange={e => debouncedSaveCaption(e.target.value)}
          placeholder="Caption will appear here after AI analysis, or type your own…"
          rows={4}
          disabled={isLocked}
        />

        <div className="conv-caption-meta">
          <span className={`conv-caption-saved ${captionSaved ? 'visible' : ''}`} aria-live="polite">
            <Check size={11} /> Saved
          </span>
          <span className={`char-counter ${charClass}`}>{charCount} / {charLimit}</span>
        </div>

        <div className="conv-item-platforms">
          {effectivePlatforms.map(p => <PlatformIcon key={p} platform={p} />)}
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
            className="btn btn-sm btn-primary"
            onClick={handlePostNow}
            disabled={posting || isLocked}
            title="Post this photo immediately"
          >
            {posting ? <Loader size={13} className="spin" /> : <Send size={13} />}
            {posting ? 'Posting…' : 'Post now'}
          </button>

          <button
            className="icon-btn danger"
            onClick={onDelete}
            title={item.status === 'scheduled' ? 'Remove from queue and cancel the pending post' : 'Remove from queue'}
          >
            <Trash2 size={14} />
          </button>
        </div>
      </div>
    </div>
  );
}

function SchedulePreviewModal({
  preview,
  onPostNow,
  onClose,
}: {
  preview: SchedulePreview;
  onPostNow: () => Promise<void>;
  onClose: () => void;
}) {
  const [posting, setPosting] = useState(false);

  const handlePostNow = async () => {
    setPosting(true);
    try {
      await onPostNow();
    } finally {
      setPosting(false);
    }
  };

  return (
    <div className="modal-overlay" onClick={e => e.target === e.currentTarget && onClose()}>
      <div className="modal conv-preview-modal">
        <div className="modal-header">
          <h2>Upcoming schedule</h2>
          <button className="btn btn-ghost btn-sm" onClick={onClose}><X size={18} /></button>
        </div>

        <div className="conv-preview-body">
          <p className="conv-preview-summary">
            {preview.approvedCount} approved photo{preview.approvedCount !== 1 ? 's' : ''} in the queue.
            About {preview.postsPerDay} post{preview.postsPerDay !== 1 ? 's' : ''}/day go out until{' '}
            {format(parseISO(preview.windowEnd), 'EEE, MMM d, yyyy')}. At each time below a
            <strong> random approved photo</strong> is picked and posted — keep the queue topped up and
            it keeps posting.
          </p>

          {preview.approvedCount === 0 && (
            <div className="conv-preview-warning">
              <AlertTriangle size={14} />
              No approved photos yet — approve some photos so the queue has something to post.
            </div>
          )}

          <div className="conv-preview-list">
            {preview.slots.map((slot, i) => (
              <div key={i} className="conv-preview-row">
                <div className="conv-preview-info">
                  <div className="conv-preview-time">
                    {format(parseISO(slot.scheduledAt), 'EEE, MMM d, yyyy — HH:mm')} UTC
                    {i === 0 && <span className="conv-override-badge"> next</span>}
                  </div>
                </div>
              </div>
            ))}
          </div>
          <p className="conv-upload-hint">
            Times are approximate — each gap carries a small random jitter.
          </p>
        </div>

        <div className="modal-footer">
          <button className="btn btn-secondary" onClick={onClose}>Close</button>
          <button
            className="btn btn-primary"
            onClick={handlePostNow}
            disabled={posting || preview.approvedCount === 0}
          >
            {posting ? 'Posting…' : 'Post one now'}
          </button>
        </div>
      </div>
    </div>
  );
}

export function QueueDetailPage({ mobile = false }: { mobile?: boolean } = {}) {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const [linkCopied, setLinkCopied] = useState(false);

  const mobileUrl = id ? `${window.location.origin}/m/convention/${id}` : '';

  const copyMobileLink = async () => {
    try {
      await navigator.clipboard.writeText(mobileUrl);
      setLinkCopied(true);
      setTimeout(() => setLinkCopied(false), 2000);
    } catch {
      toast.error('Could not copy link');
    }
  };

  const [queue, setQueue] = useState<ConventionQueue | null>(null);
  const [items, setItems] = useState<ConventionQueueItem[]>([]);
  const [accounts, setAccounts] = useState<SocialAccount[]>([]);
  const [suffixes, setSuffixes] = useState<Suffix[]>([]);
  const [loading, setLoading] = useState(true);
  const [uploading, setUploading] = useState(false);
  const [bggUrls, setBggUrls] = useState('');
  const [importingBgg, setImportingBgg] = useState(false);
  const [dragOver, setDragOver] = useState(false);
  const [uploadingGallery, setUploadingGallery] = useState(false);
  const [galleryDragOver, setGalleryDragOver] = useState(false);
  const [showEdit, setShowEdit] = useState(false);
  const [preview, setPreview] = useState<SchedulePreview | null>(null);
  const [loadingPreview, setLoadingPreview] = useState(false);
  const [filterPending, setFilterPending] = useState(false);
  const [editingImageItem, setEditingImageItem] = useState<ConventionQueueItem | null>(null);
  const [watermarks, setWatermarks] = useState<Watermark[]>([]);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const galleryInputRef = useRef<HTMLInputElement>(null);
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
    api.getWatermarks().then(setWatermarks).catch(() => {});
  }, []);

  const resolveWatermarkUrl = (url: string) => {
    const base = import.meta.env.VITE_API_URL || '';
    return url.startsWith('/') ? base + url : url;
  };

  const watermarkGallery = watermarks.map(w => {
    const src = resolveWatermarkUrl(w.url);
    return { url: src, previewUrl: src };
  });

  // The overlay URL for the queue's selected watermark, rendered on top of each
  // preview image so the user sees the overlay before scheduling.
  const overlayUrl = (() => {
    if (!queue?.watermarkId) return undefined;
    const wm = watermarks.find(w => w.id === queue.watermarkId);
    return wm ? resolveWatermarkUrl(wm.url) : undefined;
  })();

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

  // Gallery upload: every selected image is combined into a single queue item,
  // which is published as one post with multiple images.
  const uploadGallery = async (files: File[]) => {
    if (!id || files.length === 0) return;
    const imageFiles = files.filter(f => f.type.startsWith('image/'));
    if (imageFiles.length === 0) {
      toast.error('Only image files are supported');
      return;
    }
    setUploadingGallery(true);
    try {
      const item = await api.addConventionGalleryItem(id, imageFiles);
      setItems(prev => [...prev, item]);
      toast.success(`Created a gallery post with ${imageFiles.length} image${imageFiles.length > 1 ? 's' : ''}`);
    } catch (err: any) {
      toast.error(err.message || 'Failed to create gallery post');
    } finally {
      setUploadingGallery(false);
    }
  };

  const handleGalleryInput = (e: React.ChangeEvent<HTMLInputElement>) => {
    if (e.target.files) uploadGallery(Array.from(e.target.files));
    e.target.value = '';
  };

  const handleGalleryDrop = (e: React.DragEvent) => {
    e.preventDefault();
    setGalleryDragOver(false);
    if (e.dataTransfer.files) uploadGallery(Array.from(e.dataTransfer.files));
  };

  const handleBGGImport = async () => {
    if (!id) return;
    const urls = bggUrls.trim().split(/\s+/).filter(Boolean);
    if (urls.length === 0) return;
    setImportingBgg(true);
    try {
      const res = await api.addBGGItems(id, urls);
      if (res.items && res.items.length > 0) {
        setItems(prev => [...prev, ...res.items]);
        toast.success(`Added ${res.items.length} BGG item${res.items.length > 1 ? 's' : ''}`);
        setBggUrls('');
      }
      if (res.errors && res.errors.length > 0) {
        res.errors.forEach(e => toast.error(`${e.url}: ${e.error}`));
      }
      if (!res.items || res.items.length === 0) {
        toast.error('No items could be imported');
      }
    } catch (err: any) {
      toast.error(err.message || 'BGG import failed');
    } finally {
      setImportingBgg(false);
    }
  };

  const handleDrop = (e: React.DragEvent) => {
    e.preventDefault();
    setDragOver(false);
    if (e.dataTransfer.files) uploadFiles(Array.from(e.dataTransfer.files));
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

  const handlePostItemNow = async (item: ConventionQueueItem) => {
    if (!id) return;
    try {
      await api.postConventionItemNow(id, item.id);
      toast.success('Photo queued to post now');
      loadQueue();
    } catch (err: any) {
      toast.error(err.message || 'Failed to post');
    }
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

  const handlePostNow = async () => {
    if (!id) return;
    try {
      const res = await api.scheduleConventionItems(id);
      toast.success('Posted a random approved photo');
      if (res.remaining === 0) {
        toast('Queue is empty — approve more photos to keep it going', { icon: '📭' });
      }
      setPreview(null);
      loadQueue();
    } catch (err: any) {
      toast.error(err.message || 'Failed to post');
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

  const rootClass = mobile ? 'conv-mobile-page' : 'page conv-detail-page';

  if (loading) return <div className={rootClass}><div className="loading-screen"><div className="spinner" /></div></div>;
  if (!queue) return <div className={rootClass}><p style={{ padding: 20 }}>Queue not found.</p></div>;

  const approvedCount = items.filter(i => i.status === 'approved').length;
  const pendingCount = items.filter(i => i.status === 'pending').length;
  const postedCount = queue.postedCount ?? 0;
  const analyzingCount = items.filter(i => i.status === 'pending' && !i.caption && !i.aiError).length;

  const visibleItems = filterPending
    ? items.filter(i => i.status === 'pending')
    : items;
  // Show the most recently added entries first.
  const displayItems = [...visibleItems].reverse();

  return (
    <div className={rootClass}>
      {/* Bookmarkable mobile-view link (desktop only) */}
      {!mobile && (
        <div className="conv-mobile-link-bar">
          <Smartphone size={16} className="conv-mobile-link-icon" />
          <div className="conv-mobile-link-text">
            <span className="conv-mobile-link-label">Phone view for this convention — bookmark it to post on the go:</span>
            <a href={mobileUrl} className="conv-mobile-link" target="_blank" rel="noopener noreferrer">{mobileUrl}</a>
          </div>
          <button className="btn btn-sm btn-secondary conv-mobile-link-copy" onClick={copyMobileLink}>
            {linkCopied ? <Check size={14} /> : <Copy size={14} />}
            {linkCopied ? 'Copied' : 'Copy link'}
          </button>
        </div>
      )}

      {/* Header */}
      <div className="conv-detail-header">
        {!mobile && (
          <button className="icon-btn" onClick={() => navigate('/convention')} title="Back">
            <ArrowLeft size={18} />
          </button>
        )}
        <div className="conv-detail-title">
          <h1>{queue.name}</h1>
          <div className="conv-detail-meta">
            <span>{format(parseISO(queue.startDate), 'MMM d')} – {format(parseISO(queue.endDate), 'MMM d, yyyy')}</span>
            <span>·</span>
            <span>{queue.postsPerDay}×/day</span>
            {queue.minHoursBetweenPosts ? (
              <>
                <span>·</span>
                <span>≥{queue.minHoursBetweenPosts}h apart</span>
              </>
            ) : null}
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
        <div className="conv-stat-pill pending">{pendingCount} pending</div>
        <div className="conv-stat-pill approved">{approvedCount} approved</div>
        <div className="conv-stat-pill posted">{postedCount} posted</div>
        {analyzingCount > 0 && (
          <div className="conv-stat-pill analyzing">
            <Loader size={11} className="spin" /> {analyzingCount} analyzing…
          </div>
        )}
      </div>

      {/* Upload zones: one post per photo, or one gallery post from many photos */}
      <div className="conv-upload-zones">
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
              <p className="conv-upload-hint">One post per photo · JPG, PNG, WebP</p>
            </>
          )}
        </div>

        <div
          className={`conv-upload-zone ${galleryDragOver ? 'drag-over' : ''} ${uploadingGallery ? 'uploading' : ''}`}
          onDragOver={e => { e.preventDefault(); setGalleryDragOver(true); }}
          onDragLeave={() => setGalleryDragOver(false)}
          onDrop={handleGalleryDrop}
          onClick={() => galleryInputRef.current?.click()}
        >
          <input
            ref={galleryInputRef}
            type="file"
            accept="image/*"
            multiple
            style={{ display: 'none' }}
            onChange={handleGalleryInput}
          />
          {uploadingGallery ? (
            <>
              <Loader size={28} className="spin" color="var(--accent)" />
              <p>Uploading…</p>
            </>
          ) : (
            <>
              <Images size={28} color="var(--text-muted)" />
              <p>Drop photos for one gallery post</p>
              <p className="conv-upload-hint">All photos become a single post with multiple images</p>
            </>
          )}
        </div>
      </div>

      {/* BGG import */}
      <div className="conv-bgg-import">
        <div className="conv-bgg-import-header">
          <Link size={14} />
          <span>Import from BoardGameGeek</span>
        </div>
        <div className="conv-bgg-import-row">
          <textarea
            className="conv-bgg-import-input"
            value={bggUrls}
            onChange={e => setBggUrls(e.target.value)}
            placeholder="Paste one or more BGG URLs separated by spaces or newlines&#10;e.g. https://boardgamegeek.com/boardgame/174430/gloomhaven"
            rows={3}
            disabled={importingBgg}
          />
          <button
            className="btn btn-secondary conv-bgg-import-btn"
            onClick={handleBGGImport}
            disabled={importingBgg || !bggUrls.trim()}
          >
            {importingBgg ? <Loader size={14} className="spin" /> : <Upload size={14} />}
            {importingBgg ? 'Importing…' : 'Import'}
          </button>
        </div>
        <p className="conv-upload-hint">Fetches cover image and generates post text for each game</p>
      </div>

      {/* Toolbar */}
      {items.length > 0 && (
        <div className="conv-toolbar">
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
      {displayItems.length > 0 ? (
        <div className="conv-item-grid">
          {displayItems.map((item, idx) => (
            <QueueItemCard
              key={item.id}
              item={item}
              index={idx}
              queueId={queue.id}
              suffixes={suffixes}
              queuePlatforms={queue.platforms}
              queueSuffixIds={queue.suffixIds ?? {}}
              overlayUrl={overlayUrl}
              onUpdate={handleUpdateItem}
              onDelete={() => handleDeleteItem(item)}
              onAnalyze={() => handleAnalyzeItem(item)}
              onEditImage={() => setEditingImageItem(item)}
              onPostNow={() => handlePostItemNow(item)}
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
            {approvedCount} approved photo{approvedCount !== 1 ? 's' : ''} in the queue — posting
            automatically at random on the schedule
          </span>
          <button
            className="btn btn-primary"
            onClick={handlePreview}
            disabled={loadingPreview}
          >
            {loadingPreview ? <Loader size={15} className="spin" /> : <Calendar size={15} />}
            View schedule
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
          onPostNow={handlePostNow}
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
            Watermark={{
              gallery: watermarkGallery,
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
