import { useState, useEffect, useCallback, useRef } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { api } from '../../services/api';
import type {
  ConventionQueue,
  ConventionQueueItem,
  ConventionQueueItemStatus,
  ConventionGalleryPhoto,
  SchedulePreview,
  SocialAccount,
  Platform,
  Footer,
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
  Crop,
  Clapperboard,
  GalleryHorizontalEnd,
  Plus,
  Palette,
} from 'lucide-react';
import { arrayMove } from '@dnd-kit/sortable';
import toast from 'react-hot-toast';
import { PlatformIcon } from '../Common/PlatformIcon';
import { QueueFormModal } from './ConventionPage';
import { Modal } from '../Common/Modal';
import { ImageCropper } from '../Common/ImageCropper';
import { ImageEditor } from '../Common/ImageEditor/ImageEditor';
import { QuickReel } from './QuickReel';
import { GallerySection } from './GallerySection';
import { SortableTiles, SortableTile } from '../Common/SortableTiles';
import { RewriteInstagramButton } from '../PostEditor/InstagramTools';
import '../PostEditor/PostEditor.css';
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
  footers,
  queuePlatforms,
  queueFooterIds,
  overlayUrl,
  onUpdate,
  onDelete,
  onAnalyze,
  onEditImage,
  onCropImage,
  onCreatePage,
  onPostNow,
}: {
  item: ConventionQueueItem;
  index: number;
  queueId: string;
  footers: Footer[];
  queuePlatforms: Platform[];
  queueFooterIds: Record<string, string>;
  overlayUrl?: string;
  onUpdate: (item: ConventionQueueItem) => void;
  onDelete: () => void;
  onAnalyze: () => Promise<void>;
  onEditImage: (imageIndex: number) => void;
  onCropImage: (imageIndex: number) => void;
  /** Opens the image editor for a new carousel page at imageIndex. */
  onCreatePage: (imageIndex: number) => void;
  onPostNow: () => Promise<void>;
}) {
  const [caption, setCaption] = useState(item.caption ?? '');
  const [bggUrl, setBggUrl] = useState(item.bggUrl ?? '');
  const [itemFooterIds, setItemFooterIds] = useState<Record<string, string>>(item.footerIds ?? {});
  const [analyzing, setAnalyzing] = useState(false);
  const [galleryIndex, setGalleryIndex] = useState(0);
  const galleryRef = useRef<HTMLDivElement>(null);
  const [saving, setSaving] = useState(false);
  const [posting, setPosting] = useState(false);
  const [captionSaved, setCaptionSaved] = useState(false);
  // "Customize per platform": a text per platform instead of the one caption.
  const [overrides, setOverrides] = useState<Record<string, string>>(item.contentOverrides ?? {});
  const [customize, setCustomize] = useState(Object.keys(item.contentOverrides ?? {}).length > 0);
  const [rewriting, setRewriting] = useState(false);
  const saveTimer = useRef<ReturnType<typeof setTimeout>>(undefined);
  const overridesSaveTimer = useRef<ReturnType<typeof setTimeout>>(undefined);
  const savedFlashTimer = useRef<ReturnType<typeof setTimeout>>(undefined);
  const bggSaveTimer = useRef<ReturnType<typeof setTimeout>>(undefined);
  const footerSaveTimer = useRef<ReturnType<typeof setTimeout>>(undefined);
  // Where a new carousel page goes while the add choices are open.
  const [addPageAt, setAddPageAt] = useState<number | null>(null);
  const [savingPages, setSavingPages] = useState(false);
  const pageInputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    setCaption(item.caption ?? '');
  }, [item.caption]);

  useEffect(() => {
    setBggUrl(item.bggUrl ?? '');
  }, [item.bggUrl]);

  // Follow the stored texts, e.g. after a new AI caption cleared them.
  const overridesKey = JSON.stringify(item.contentOverrides ?? {});
  useEffect(() => {
    const stored: Record<string, string> = JSON.parse(overridesKey);
    setOverrides(stored);
    setCustomize(Object.keys(stored).length > 0);
  }, [overridesKey]);

  const flashSaved = () => {
    setCaptionSaved(true);
    clearTimeout(savedFlashTimer.current);
    savedFlashTimer.current = setTimeout(() => setCaptionSaved(false), 2000);
  };

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
        flashSaved();
      } catch {
        // silently fail
      }
    }, 800);
  };

  const debouncedSaveOverride = (platform: string, value: string) => {
    const next = { ...overrides, [platform]: value };
    setOverrides(next);
    setCaptionSaved(false);
    clearTimeout(overridesSaveTimer.current);
    overridesSaveTimer.current = setTimeout(async () => {
      try {
        onUpdate(await api.updateConventionItem(queueId, item.id, { contentOverrides: next }));
        flashSaved();
      } catch {
        // silently fail
      }
    }, 800);
  };

  // Turning it on starts every platform from the caption; turning it off
  // keeps the longest text as the caption, like the News form.
  const toggleCustomize = async (on: boolean) => {
    clearTimeout(saveTimer.current);
    clearTimeout(overridesSaveTimer.current);
    let data: { caption?: string; contentOverrides: Record<string, string> };
    if (on) {
      const next: Record<string, string> = {};
      for (const p of effectivePlatforms) next[p] = caption;
      setOverrides(next);
      data = { caption, contentOverrides: next };
    } else {
      let longest = caption;
      for (const p of effectivePlatforms) {
        const val = overrides[p] ?? '';
        if (val.length > longest.length) longest = val;
      }
      setCaption(longest);
      setOverrides({});
      data = { caption: longest, contentOverrides: {} };
    }
    setCustomize(on);
    try {
      onUpdate(await api.updateConventionItem(queueId, item.id, data));
      flashSaved();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to save');
    }
  };

  const rewriteForInstagram = async () => {
    const source = (overrides.instagram ?? '').trim() || (overrides.bluesky ?? '').trim() || caption.trim();
    if (!source) { toast.error('Write a text first'); return; }
    setRewriting(true);
    try {
      const { text } = await api.rewriteForInstagram(source);
      debouncedSaveOverride('instagram', text);
      toast.success('Instagram caption rewritten');
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Rewrite failed');
    } finally {
      setRewriting(false);
    }
  };

  // Clear pending timers on unmount so no state updates fire afterwards.
  useEffect(() => () => {
    clearTimeout(saveTimer.current);
    clearTimeout(overridesSaveTimer.current);
    clearTimeout(savedFlashTimer.current);
    clearTimeout(bggSaveTimer.current);
    clearTimeout(footerSaveTimer.current);
  }, []);

  const saveFooterIds = (newFooterIds: Record<string, string>) => {
    clearTimeout(footerSaveTimer.current);
    footerSaveTimer.current = setTimeout(async () => {
      try {
        const updated = await api.updateConventionItem(queueId, item.id, { footerIds: newFooterIds });
        onUpdate(updated);
      } catch {
        // silently fail
      }
    }, 600);
  };

  const handleFooterChange = (platform: string, value: string) => {
    const next = { ...itemFooterIds };
    if (value) next[platform] = value;
    else delete next[platform];
    setItemFooterIds(next);
    saveFooterIds(next);
  };

  const toggleApprove = async () => {
    if (item.status === 'scheduled') return;
    // Block approving a caption that exceeds the character limit, matching the
    // PostEditor which refuses to schedule over-limit content.
    if (item.status !== 'approved' && overLimit) {
      toast.error(overLimitMessage);
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
      toast.error(overLimitMessage);
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

  const footerPlatformLabels: Record<string, string> = {
    bluesky: 'Bluesky',
    instagram: 'Instagram',
    twitter: 'X/Twitter',
    mastodon: 'Mastodon',
    threads: 'Threads',
    linkedin: 'LinkedIn',
  };

  const effectivePlatforms = item.platforms && item.platforms.length > 0 ? item.platforms : queuePlatforms;

  const footerLen = (platform: Platform) => {
    const id = itemFooterIds[platform] ?? queueFooterIds[platform];
    if (!id) return 0;
    const s = footers.find(x => x.id === id);
    return s ? s.content.length + 1 : 0;
  };
  const effectiveLimit = (platform: Platform) => platformLimit(platform) - footerLen(platform);
  const charLimit = effectivePlatforms.length === 0
    ? effectiveLimit('instagram')
    : Math.min(...effectivePlatforms.map(effectiveLimit));
  const charCount = caption.length;
  const perPlatform = customize && effectivePlatforms.length > 1;
  // A platform's text: its own, or the caption when it has none.
  const platformText = (platform: Platform) => overrides[platform] || caption;
  const overPlatform = perPlatform
    ? effectivePlatforms.find(p => platformText(p).length > effectiveLimit(p))
    : undefined;
  const overLimit = perPlatform ? overPlatform !== undefined : charCount > charLimit;
  const overLimitMessage = overPlatform
    ? `The ${footerPlatformLabels[overPlatform] ?? overPlatform} text exceeds the ${effectiveLimit(overPlatform)} character limit`
    : `Caption exceeds the ${charLimit} character limit`;
  const charClass = overLimit ? 'danger' : charCount > charLimit * 0.9 ? 'warning' : '';

  // Scheduled items are locked from editing/approval, but can still be pulled
  // back from the queue (which cancels the pending post). Once a post is
  // published its item is removed from the queue entirely.
  const isLocked = item.status === 'scheduled';

  // Gallery items carry multiple images; fall back to the single primary image.
  const galleryImages = item.imageUrls && item.imageUrls.length > 0 ? item.imageUrls : [item.imageUrl];
  // An item with an image list is a carousel even while it has one image, so
  // it shows as one that gallery photos can still be added to.
  const isGallery = (item.imageUrls?.length ?? 0) > 0;

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

  // Sortable IDs for the pages: the URL, numbered should one repeat.
  const pageIds = galleryImages.map((url, i) => {
    const n = galleryImages.slice(0, i).filter(u => u === url).length;
    return n ? `${url}#${n}` : url;
  });

  const savePages = async (next: string[]) => {
    const before = item;
    // Show the new order at once; the server's answer replaces it.
    onUpdate({ ...item, imageUrls: next, imageUrl: next[0] });
    setSavingPages(true);
    try {
      onUpdate(await api.setConventionItemImages(queueId, item.id, next));
    } catch (err) {
      onUpdate(before);
      toast.error(err instanceof Error ? err.message : 'Failed to save the pages');
    } finally {
      setSavingPages(false);
    }
  };

  const movePage = (from: number, to: number) => {
    savePages(arrayMove(galleryImages, from, to));
    setGalleryIndex(to);
    scrollToImage(to);
  };

  const removePage = (idx: number) => {
    if (!confirm('Remove this page from the carousel?')) return;
    savePages(galleryImages.filter((_, i) => i !== idx));
  };

  // Uploads the picked photos as pages from addPageAt on, in the picked order.
  const uploadPages = async (files: File[]) => {
    const at = addPageAt ?? galleryImages.length;
    setAddPageAt(null);
    const imageFiles = files.filter(f => f.type.startsWith('image/'));
    if (imageFiles.length === 0) return;
    setSavingPages(true);
    try {
      for (const [n, file] of imageFiles.entries()) {
        onUpdate(await api.addConventionItemImage(queueId, item.id, file, at + n));
      }
      toast.success(imageFiles.length === 1 ? 'Page added' : `${imageFiles.length} pages added`);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to add the page');
    } finally {
      setSavingPages(false);
    }
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
          <>
            <button
              className="conv-item-edit-btn conv-item-crop-btn"
              onClick={e => { e.stopPropagation(); onCropImage(isGallery ? galleryIndex : 0); }}
              title="Crop image"
            >
              <Crop size={12} />
            </button>
            <button
              className="conv-item-edit-btn"
              onClick={e => { e.stopPropagation(); onEditImage(isGallery ? galleryIndex : 0); }}
              title="Edit image"
            >
              <Pencil size={12} />
            </button>
          </>
        )}
      </div>

      {isGallery && !isLocked && (
        <div className="conv-pages">
          <div className="conv-pages-row">
            <button
              type="button"
              className={`conv-page-add ${addPageAt === 0 ? 'active' : ''}`}
              onClick={() => setAddPageAt(addPageAt === 0 ? null : 0)}
              disabled={savingPages}
              title="Add a front page"
            >
              <Plus size={14} />
            </button>
            <SortableTiles ids={pageIds} onMove={movePage} className="conv-pages-list">
              {galleryImages.map((src, i) => (
                <SortableTile
                  key={pageIds[i]}
                  id={pageIds[i]}
                  className={`conv-page ${i === galleryIndex ? 'current' : ''}`}
                  title="Drag to reorder"
                >
                  <img src={src} alt="" loading="lazy" draggable={false} onClick={() => scrollToImage(i)} />
                  <span className="conv-page-num">{i + 1}</span>
                  {galleryImages.length > 1 && (
                    <button
                      type="button"
                      className="conv-page-remove"
                      onClick={() => removePage(i)}
                      disabled={savingPages}
                      title="Remove this page"
                    >
                      <X size={10} />
                    </button>
                  )}
                </SortableTile>
              ))}
            </SortableTiles>
            <button
              type="button"
              className={`conv-page-add ${addPageAt === galleryImages.length ? 'active' : ''}`}
              onClick={() => setAddPageAt(addPageAt === galleryImages.length ? null : galleryImages.length)}
              disabled={savingPages}
              title="Add a back page"
            >
              <Plus size={14} />
            </button>
            {savingPages && <Loader size={14} className="spin conv-pages-busy" />}
          </div>
          {addPageAt !== null && (
            <div className="conv-pages-add-choices">
              <span>{addPageAt === 0 ? 'New front page:' : 'New back page:'}</span>
              <button type="button" className="btn btn-sm btn-secondary" onClick={() => pageInputRef.current?.click()}>
                <Upload size={13} /> Upload photo
              </button>
              <button
                type="button"
                className="btn btn-sm btn-secondary"
                onClick={() => { const at = addPageAt; setAddPageAt(null); onCreatePage(at); }}
              >
                <Palette size={13} /> Create in editor
              </button>
              <button type="button" className="btn btn-sm btn-ghost" onClick={() => setAddPageAt(null)}>Cancel</button>
            </div>
          )}
          <input
            ref={pageInputRef}
            type="file"
            accept="image/*"
            multiple
            style={{ display: 'none' }}
            onChange={e => {
              if (e.target.files) uploadPages(Array.from(e.target.files));
              e.target.value = '';
            }}
          />
          <p className="conv-upload-hint">Drag the pages to reorder them · + adds a front or back page</p>
        </div>
      )}

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

        {effectivePlatforms.length > 1 && (
          <div className="conv-customize-row">
            <label className="conv-customize-toggle">
              <input
                type="checkbox"
                checked={customize}
                onChange={e => toggleCustomize(e.target.checked)}
                disabled={isLocked}
              />
              Customize per platform
            </label>
          </div>
        )}

        {perPlatform ? (
          effectivePlatforms.map(platform => {
            const text = overrides[platform] ?? '';
            const limit = effectiveLimit(platform);
            const cls = text.length > limit ? 'danger' : text.length > limit * 0.9 ? 'warning' : '';
            return (
              <div key={platform} className="conv-platform-text">
                <div className="conv-platform-text-head">
                  <PlatformIcon platform={platform} size={11} />
                  <span>{footerPlatformLabels[platform] ?? platform}</span>
                  {platform === 'instagram' && !isLocked && (
                    <RewriteInstagramButton
                      style={{ marginLeft: 'auto', padding: '2px 8px' }}
                      rewriting={rewriting}
                      disabled={!text.trim() && !(overrides.bluesky ?? '').trim() && !caption.trim()}
                      onRewrite={rewriteForInstagram}
                    />
                  )}
                </div>
                <textarea
                  className="conv-caption-input"
                  value={text}
                  onChange={e => debouncedSaveOverride(platform, e.target.value)}
                  placeholder={caption ? 'Empty: the caption is used' : `Text for ${footerPlatformLabels[platform] ?? platform}…`}
                  rows={4}
                  disabled={isLocked}
                />
                <div className="conv-caption-meta">
                  <span />
                  <span className={`char-counter ${cls}`}>{text.length} / {limit}</span>
                </div>
              </div>
            );
          })
        ) : (
          <textarea
            className="conv-caption-input"
            value={caption}
            onChange={e => debouncedSaveCaption(e.target.value)}
            placeholder="Caption will appear here after AI analysis, or type your own…"
            rows={4}
            disabled={isLocked}
          />
        )}

        <div className="conv-caption-meta">
          <span className={`conv-caption-saved ${captionSaved ? 'visible' : ''}`} aria-live="polite">
            <Check size={11} /> Saved
          </span>
          {!perPlatform && <span className={`char-counter ${charClass}`}>{charCount} / {charLimit}</span>}
        </div>

        <div className="conv-item-platforms">
          {effectivePlatforms.map(p => <PlatformIcon key={p} platform={p} />)}
        </div>

        {footers.length > 0 && (
          <div className="conv-footer-selectors">
            {effectivePlatforms.map(platform => (
              <div key={platform} className="conv-footer-row">
                <label>
                  <PlatformIcon platform={platform} size={11} />
                  {footerPlatformLabels[platform] ?? platform}
                </label>
                <select
                  className="select"
                  value={itemFooterIds[platform] ?? queueFooterIds[platform] ?? ''}
                  onChange={e => handleFooterChange(platform, e.target.value)}
                  disabled={isLocked}
                >
                  <option value="">— queue default —</option>
                  {footers.map(s => (
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

interface ItemImage {
  item: ConventionQueueItem;
  index: number;
}

// The URL of one image of an item: an entry of a gallery's imageUrls, or the
// plain item's only image.
function itemImageUrl({ item, index }: ItemImage) {
  return item.imageUrls && item.imageUrls.length > 0 ? item.imageUrls[index] ?? item.imageUrl : item.imageUrl;
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
    <Modal onClose={onClose} className="conv-preview-modal">
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
    </Modal>
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
  const [gallery, setGallery] = useState<ConventionGalleryPhoto[]>([]);
  const [accounts, setAccounts] = useState<SocialAccount[]>([]);
  const [footers, setFooters] = useState<Footer[]>([]);
  const [loading, setLoading] = useState(true);
  const [uploading, setUploading] = useState(false);
  const [bggUrls, setBggUrls] = useState('');
  const [importingBgg, setImportingBgg] = useState(false);
  const [dragOver, setDragOver] = useState(false);
  const [uploadingGallery, setUploadingGallery] = useState(false);
  const [galleryDragOver, setGalleryDragOver] = useState(false);
  const [uploadingPhotos, setUploadingPhotos] = useState(false);
  const [photosDragOver, setPhotosDragOver] = useState(false);
  const [showEdit, setShowEdit] = useState(false);
  const [showQuickReel, setShowQuickReel] = useState(false);
  const [preview, setPreview] = useState<SchedulePreview | null>(null);
  const [loadingPreview, setLoadingPreview] = useState(false);
  const [filterPending, setFilterPending] = useState(false);
  // The image being edited or cropped: an item plus which of its images, since
  // a gallery item carries several.
  const [editingImage, setEditingImage] = useState<ItemImage | null>(null);
  const [croppingImage, setCroppingImage] = useState<ItemImage | null>(null);
  // A carousel page being made in the image editor: the item and where it goes.
  const [creatingPage, setCreatingPage] = useState<ItemImage | null>(null);
  const [overlayImg, setOverlayImg] = useState<HTMLImageElement | null>(null);
  const [watermarks, setWatermarks] = useState<Watermark[]>([]);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const galleryInputRef = useRef<HTMLInputElement>(null);
  const photosInputRef = useRef<HTMLInputElement>(null);
  const pollRef = useRef<ReturnType<typeof setInterval>>(undefined);

  const loadQueue = useCallback(async () => {
    if (!id) return;
    try {
      const [data, accs] = await Promise.all([api.getConventionQueue(id), api.getActiveAccounts()]);
      setQueue(data.queue);
      setItems(data.items);
      setGallery(data.gallery ?? []);
      setAccounts(accs);
    } catch {
      toast.error('Failed to load queue');
    } finally {
      setLoading(false);
    }
  }, [id]);

  useEffect(() => { loadQueue(); }, [loadQueue]);

  useEffect(() => {
    api.getFooters().then(setFooters).catch(() => {});
    api.getWatermarks().then(setWatermarks).catch(() => {});
  }, []);

  const resolveWatermarkUrl = (url: string) => {
    const base = import.meta.env.VITE_API_URL || '';
    return url.startsWith('/') ? base + url : url;
  };

  // The queue's selected watermark. Its overlay is rendered on top of each
  // preview image, and over the crop area, so the user sees the overlay before
  // scheduling; the backend composites it when the item is posted.
  const queueWatermark = queue?.watermarkId
    ? watermarks.find(w => w.id === queue.watermarkId)
    : undefined;
  const overlayUrl = queueWatermark ? resolveWatermarkUrl(queueWatermark.url) : undefined;

  useEffect(() => {
    if (!overlayUrl) {
      setOverlayImg(null);
      return;
    }
    let cancelled = false;
    const img = new window.Image();
    img.onload = () => { if (!cancelled) setOverlayImg(img); };
    img.onerror = () => { if (!cancelled) setOverlayImg(null); };
    img.src = overlayUrl;
    return () => { cancelled = true; };
  }, [overlayUrl]);

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

  // Carousel upload: every selected image is combined into a single queue
  // item, which is published as one post with multiple images.
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
      toast.success(`Created a carousel post with ${imageFiles.length} image${imageFiles.length > 1 ? 's' : ''}`);
    } catch (err: any) {
      toast.error(err.message || 'Failed to create carousel post');
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

  // Gallery upload: the photos go into the queue's gallery, to be moved into
  // single posts or carousels later.
  const uploadPhotos = async (files: File[]) => {
    if (!id || files.length === 0) return;
    const imageFiles = files.filter(f => f.type.startsWith('image/'));
    if (imageFiles.length === 0) {
      toast.error('Only image files are supported');
      return;
    }
    setUploadingPhotos(true);
    try {
      const photos = await api.addConventionGalleryPhotos(id, imageFiles);
      setGallery(prev => [...prev, ...photos]);
      toast.success(`Added ${photos.length} photo${photos.length > 1 ? 's' : ''} to the gallery`);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to upload to the gallery');
    } finally {
      setUploadingPhotos(false);
    }
  };

  const handlePhotosInput = (e: React.ChangeEvent<HTMLInputElement>) => {
    if (e.target.files) uploadPhotos(Array.from(e.target.files));
    e.target.value = '';
  };

  const handlePhotosDrop = (e: React.DragEvent) => {
    e.preventDefault();
    setPhotosDragOver(false);
    if (e.dataTransfer.files) uploadPhotos(Array.from(e.dataTransfer.files));
  };

  // Items a gallery move created or added photos to.
  const handleGalleryItems = (changed: ConventionQueueItem[]) => {
    setItems(prev => {
      const next = prev.map(i => changed.find(c => c.id === i.id) ?? i);
      const added = changed.filter(c => !prev.some(i => i.id === c.id));
      return [...next, ...added];
    });
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

  // Like the crop, the edit is uploaded without the queue's overlay, which the
  // editor only shows for reference.
  const handleImageEditSave = async (blob: Blob) => {
    if (!id || !editingImage) return;
    const { item, index } = editingImage;
    const file = new File([blob], 'edited.jpg', { type: 'image/jpeg' });
    // A failed upload is reported by the editor, which stays open for a retry.
    const updated = await api.replaceConventionItemImage(id, item.id, file, index);
    setItems(prev => prev.map(i => (i.id === item.id ? updated : i)));
    setEditingImage(null);
    toast.success('Image updated');
  };

  // The crop is uploaded without the overlay: the backend stamps the queue's
  // watermark onto the image when it posts it, so baking it in here would
  // apply it twice.
  const handleCropApply = async (blob: Blob) => {
    if (!id || !croppingImage) return;
    const { item, index } = croppingImage;
    setCroppingImage(null);
    const file = new File([blob], 'cropped.jpg', { type: 'image/jpeg' });
    try {
      const updated = await api.replaceConventionItemImage(id, item.id, file, index);
      setItems(prev => prev.map(i => (i.id === item.id ? updated : i)));
      toast.success('Image cropped');
    } catch (err: any) {
      toast.error(err.message || 'Failed to upload cropped image');
    }
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

      {/* Quick Reel: a clip in the convention's reel template, out on Instagram */}
      <button
        className={`conv-quick-reel ${queue.reelTemplateId ? '' : 'unset'}`}
        onClick={() => {
          if (queue.reelTemplateId) setShowQuickReel(true);
          else toast('Pick a reel template under Edit first');
        }}
      >
        <Clapperboard size={22} />
        <span className="conv-quick-reel-text">
          <span className="conv-quick-reel-title">Quick Reel</span>
          <span className="conv-quick-reel-hint">
            {queue.reelTemplateId
              ? 'Upload a clip, get an Instagram reel in the convention template'
              : 'Set a reel template under Edit to use it'}
          </span>
        </span>
      </button>

      {/* Upload zones: one post per photo, one carousel post from many photos, or
          the gallery, to decide later */}
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
              <p>Drop photos for one carousel post</p>
              <p className="conv-upload-hint">All photos become a single post with multiple images</p>
            </>
          )}
        </div>

        <div
          className={`conv-upload-zone ${photosDragOver ? 'drag-over' : ''} ${uploadingPhotos ? 'uploading' : ''}`}
          onDragOver={e => { e.preventDefault(); setPhotosDragOver(true); }}
          onDragLeave={() => setPhotosDragOver(false)}
          onDrop={handlePhotosDrop}
          onClick={() => photosInputRef.current?.click()}
        >
          <input
            ref={photosInputRef}
            type="file"
            accept="image/*"
            multiple
            style={{ display: 'none' }}
            onChange={handlePhotosInput}
          />
          {uploadingPhotos ? (
            <>
              <Loader size={28} className="spin" color="var(--accent)" />
              <p>Uploading…</p>
            </>
          ) : (
            <>
              <GalleryHorizontalEnd size={28} color="var(--text-muted)" />
              <p>Drop photos into the gallery</p>
              <p className="conv-upload-hint">Decide later: move them into single posts or carousels</p>
            </>
          )}
        </div>
      </div>

      <GallerySection
        queueId={queue.id}
        photos={gallery}
        items={items}
        onPhotosRemoved={ids => setGallery(prev => prev.filter(p => !ids.includes(p.id)))}
        onItemsChanged={handleGalleryItems}
      />

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
              footers={footers}
              queuePlatforms={queue.platforms}
              queueFooterIds={queue.footerIds ?? {}}
              overlayUrl={overlayUrl}
              onUpdate={handleUpdateItem}
              onDelete={() => handleDeleteItem(item)}
              onAnalyze={() => handleAnalyzeItem(item)}
              onEditImage={index => setEditingImage({ item, index })}
              onCropImage={index => setCroppingImage({ item, index })}
              onCreatePage={index => setCreatingPage({ item, index })}
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

      {showQuickReel && (
        <QuickReel queue={queue} onClose={() => setShowQuickReel(false)} />
      )}

      {preview && (
        <SchedulePreviewModal
          preview={preview}
          onPostNow={handlePostNow}
          onClose={() => setPreview(null)}
        />
      )}

      {croppingImage && (
        <ImageCropper
          imageUrl={itemImageUrl(croppingImage)}
          onApply={handleCropApply}
          onCancel={() => setCroppingImage(null)}
          watermarkImg={overlayImg}
          watermarkName={overlayImg ? queueWatermark?.name : undefined}
          bakeWatermark={false}
        />
      )}

      {creatingPage && (
        <ImageEditor
          canvas={{ width: 1080, height: 1080, background: '#ffffff' }}
          title={creatingPage.index === 0 ? 'Create front page' : 'Create carousel page'}
          guideOverlay={overlayImg}
          onCancel={() => setCreatingPage(null)}
          onSave={async blob => {
            const { item, index } = creatingPage;
            const file = new File([blob], 'page.jpg', { type: 'image/jpeg' });
            // A failed upload is reported by the editor, which stays open for a retry.
            const updated = await api.addConventionItemImage(queue.id, item.id, file, index);
            handleUpdateItem(updated);
            setCreatingPage(null);
            toast.success('Page added');
          }}
        />
      )}

      {editingImage && (
        <ImageEditor
          imageUrl={itemImageUrl(editingImage)}
          guideOverlay={overlayImg}
          onSave={handleImageEditSave}
          onCancel={() => setEditingImage(null)}
        />
      )}
    </div>
  );
}
