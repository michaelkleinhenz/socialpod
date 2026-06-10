import { useState, useEffect, useRef, useCallback } from 'react';
import { format } from 'date-fns';
import { api } from '../../services/api';
import type { Platform, Suffix, SocialAccount, MentionEntry, TeamSettings, Watermark } from '../../types';
import { Newspaper, Image, Send, Clock, Tag, MessageSquare, Upload, Crop, X, Loader, Dice5 } from 'lucide-react';
import { PlatformIcon } from '../Common/PlatformIcon';
import { MentionTextarea } from '../PostEditor/MentionTextarea';
import toast from 'react-hot-toast';
import '../PostEditor/PostEditor.css';
import './News.css';

const MAX_IMAGE_BYTES = 1_000_000;

const BLUESKY_LIMIT = 300;
const INSTAGRAM_LIMIT = 2200;
const TWITTER_LIMIT = 280;
const MASTODON_LIMIT = 500;
const LINKEDIN_LIMIT = 3000;

interface CropRect {
  x: number;
  y: number;
  size: number;
}

export function NewsPage() {
  const [pluginReady, setPluginReady] = useState<boolean | null>(null);

  // News fields
  const [episodeNumber, setEpisodeNumber] = useState('');
  const [newsTagline, setNewsTagline] = useState('');
  const [articleUrl, setArticleUrl] = useState('');
  const [shownotes, setShownotes] = useState('');

  // Single image handling
  const [imageFile, setImageFile] = useState<File | null>(null);
  const [imagePreviewUrl, setImagePreviewUrl] = useState<string | null>(null);
  const fileRef = useRef<HTMLInputElement>(null);

  // Cropping state
  const [showCropper, setShowCropper] = useState(false);
  const [cropRect, setCropRect] = useState<CropRect | null>(null);
  const [croppedBlob, setCroppedBlob] = useState<Blob | null>(null);
  const [croppedPreviewUrl, setCroppedPreviewUrl] = useState<string | null>(null);
  const [croppedSize, setCroppedSize] = useState<number | null>(null);
  const cropImgRef = useRef<HTMLImageElement>(null);
  const cropContainerRef = useRef<HTMLDivElement>(null);
  const [imgNaturalSize, setImgNaturalSize] = useState<{ w: number; h: number } | null>(null);
  const [dragging, setDragging] = useState<{ startX: number; startY: number; origRect: CropRect } | null>(null);
  const [resizing, setResizing] = useState<{ corner: string; startX: number; startY: number; origRect: CropRect } | null>(null);

  // Watermark
  const [watermark, setWatermark] = useState<Watermark | null>(null);
  const [watermarkImg, setWatermarkImg] = useState<HTMLImageElement | null>(null);

  // BGG import
  const [bggUrl, setBggUrl] = useState('');
  const [fetchingBgg, setFetchingBgg] = useState(false);
  const [bggError, setBggError] = useState('');
  const [bggEnabled, setBggEnabled] = useState(false);

  // Drag & drop zone
  const [dragOver, setDragOver] = useState(false);

  // Social posting toggle
  const [addSocialPost, setAddSocialPost] = useState(false);

  // Social posting fields
  const [content, setContent] = useState('');
  const [contentOverrides, setContentOverrides] = useState<Record<string, string>>({});
  const [customizePerPlatform, setCustomizePerPlatform] = useState(false);
  const [firstComment, setFirstComment] = useState('');
  const [platforms, setPlatforms] = useState<Platform[]>([]);
  const [scheduledAt, setScheduledAt] = useState(
    format(new Date(Date.now() + 3600000), "yyyy-MM-dd'T'HH:mm")
  );
  const [status, setStatus] = useState<'scheduled' | 'draft'>('scheduled');
  const [suffixes, setSuffixes] = useState<Suffix[]>([]);
  const [suffixIds, setSuffixIds] = useState<Record<string, string>>({});
  const [mentions, setMentions] = useState<MentionEntry[]>([]);
  const [accounts, setAccounts] = useState<SocialAccount[]>([]);
  const [accountsLoaded, setAccountsLoaded] = useState(false);

  const [submitting, setSubmitting] = useState(false);

  const apiUrl = import.meta.env.VITE_API_URL || '';

  useEffect(() => {
    api.getTeamSettings()
      .then((s: TeamSettings) => {
        if (s.enabledPlugins?.includes('news_creator') && s.newsCreatorUrl && s.hasNewsCreatorBearerToken) {
          setPluginReady(true);
          if (s.newsCreatorWatermarkId) {
            api.getWatermarks().then((wms: Watermark[]) => {
              const wm = wms.find(w => w.id === s.newsCreatorWatermarkId);
              if (wm) setWatermark(wm);
            }).catch(() => {});
          }
        } else {
          setPluginReady(false);
        }
      })
      .catch(() => setPluginReady(false));
    api.getPublicSettings().then(s => {
      if (s.hasBggApiToken) setBggEnabled(true);
    }).catch(() => {});
  }, []);

  // Load watermark image when watermark is set
  useEffect(() => {
    if (!watermark) { setWatermarkImg(null); return; }
    const img = new window.Image();
    img.crossOrigin = 'anonymous';
    img.onload = () => setWatermarkImg(img);
    img.src = watermark.url.startsWith('/') ? apiUrl + watermark.url : watermark.url;
  }, [watermark, apiUrl]);

  // Clean up preview URLs
  useEffect(() => {
    return () => {
      if (imagePreviewUrl) URL.revokeObjectURL(imagePreviewUrl);
      if (croppedPreviewUrl) URL.revokeObjectURL(croppedPreviewUrl);
    };
  }, []);

  // Load social posting data when toggle is enabled
  useEffect(() => {
    if (!addSocialPost || accountsLoaded) return;
    api.getActiveAccounts().then(accs => { setAccounts(accs); setAccountsLoaded(true); }).catch(() => setAccountsLoaded(true));
    api.getSuffixes().then(setSuffixes).catch(() => {});
    api.getMentions().then(setMentions).catch(() => {});
  }, [addSocialPost, accountsLoaded]);

  useEffect(() => {
    if (!accountsLoaded) return;
    setPlatforms(prev => {
      if (prev.length === 0) {
        const first = accounts[0];
        return first ? [first.platform] : [];
      }
      return prev.filter(p => accounts.some(a => a.platform === p));
    });
  }, [accountsLoaded]); // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    if (customizePerPlatform && platforms.length <= 1) {
      if (platforms.length === 1) {
        const override = contentOverrides[platforms[0]];
        if (override) setContent(override);
      }
      setContentOverrides({});
      setCustomizePerPlatform(false);
    }
  }, [platforms.length]); // eslint-disable-line react-hooks/exhaustive-deps

  // Handle file selection (from input, drop, or paste)
  const handleFile = useCallback((file: File) => {
    if (!file.type.startsWith('image/')) {
      toast.error('Please select an image file');
      return;
    }
    // Clean up old previews
    if (imagePreviewUrl) URL.revokeObjectURL(imagePreviewUrl);
    if (croppedPreviewUrl) URL.revokeObjectURL(croppedPreviewUrl);

    setImageFile(file);
    setImagePreviewUrl(URL.createObjectURL(file));
    setCroppedBlob(null);
    setCroppedPreviewUrl(null);
    setCroppedSize(null);
    setCropRect(null);
    setShowCropper(true);
  }, [imagePreviewUrl, croppedPreviewUrl]);

  // Paste handler
  useEffect(() => {
    const onPaste = (e: ClipboardEvent) => {
      const items = e.clipboardData?.items;
      if (!items) return;
      for (const item of items) {
        if (item.type.startsWith('image/')) {
          e.preventDefault();
          const file = item.getAsFile();
          if (file) handleFile(file);
          return;
        }
      }
    };
    document.addEventListener('paste', onPaste);
    return () => document.removeEventListener('paste', onPaste);
  }, [handleFile]);

  // Drag & drop handlers
  const onDragOver = (e: React.DragEvent) => {
    e.preventDefault();
    setDragOver(true);
  };
  const onDragLeave = (e: React.DragEvent) => {
    e.preventDefault();
    setDragOver(false);
  };
  const onDrop = (e: React.DragEvent) => {
    e.preventDefault();
    setDragOver(false);
    const file = e.dataTransfer.files[0];
    if (file) handleFile(file);
  };

  const removeImage = () => {
    if (imagePreviewUrl) URL.revokeObjectURL(imagePreviewUrl);
    if (croppedPreviewUrl) URL.revokeObjectURL(croppedPreviewUrl);
    setImageFile(null);
    setImagePreviewUrl(null);
    setCroppedBlob(null);
    setCroppedPreviewUrl(null);
    setCroppedSize(null);
    setCropRect(null);
    setShowCropper(false);
    if (fileRef.current) fileRef.current.value = '';
  };

  // Cropper helpers
  const getDisplayScale = () => {
    if (!cropImgRef.current || !imgNaturalSize) return 1;
    return cropImgRef.current.clientWidth / imgNaturalSize.w;
  };

  const onCropImageLoad = () => {
    const img = cropImgRef.current;
    if (!img) return;
    const w = img.naturalWidth;
    const h = img.naturalHeight;
    setImgNaturalSize({ w, h });
    const minDim = Math.min(w, h);
    const size = Math.floor(minDim * 0.8);
    setCropRect({
      x: Math.floor((w - size) / 2),
      y: Math.floor((h - size) / 2),
      size,
    });
  };

  const clampRect = (rect: CropRect, natW: number, natH: number): CropRect => {
    let { x, y, size } = rect;
    size = Math.round(Math.max(50, Math.min(size, natW, natH)));
    x = Math.round(Math.max(0, Math.min(x, natW - size)));
    y = Math.round(Math.max(0, Math.min(y, natH - size)));
    return { x, y, size };
  };

  const handleCropStart = (clientX: number, clientY: number) => {
    if (!cropRect || !imgNaturalSize || !cropImgRef.current) return;
    const scale = getDisplayScale();
    const imgBounds = cropImgRef.current.getBoundingClientRect();
    const mx = (clientX - imgBounds.left) / scale;
    const my = (clientY - imgBounds.top) / scale;

    const { x, y, size } = cropRect;
    // 28px hit area in display coordinates — large enough for touch targets
    const handleHit = 28 / scale;

    const corners = [
      { corner: 'tl', cx: x, cy: y },
      { corner: 'tr', cx: x + size, cy: y },
      { corner: 'bl', cx: x, cy: y + size },
      { corner: 'br', cx: x + size, cy: y + size },
    ];
    for (const c of corners) {
      if (Math.abs(mx - c.cx) < handleHit && Math.abs(my - c.cy) < handleHit) {
        setResizing({ corner: c.corner, startX: clientX, startY: clientY, origRect: { ...cropRect } });
        return;
      }
    }

    if (mx >= x && mx <= x + size && my >= y && my <= y + size) {
      setDragging({ startX: clientX, startY: clientY, origRect: { ...cropRect } });
    }
  };

  const onCropMouseDown = (e: React.MouseEvent) => {
    e.preventDefault();
    handleCropStart(e.clientX, e.clientY);
  };

  const onCropTouchStart = (e: React.TouchEvent) => {
    e.preventDefault();
    if (e.touches.length > 0) {
      handleCropStart(e.touches[0].clientX, e.touches[0].clientY);
    }
  };

  const onCropMouseMove = useCallback((clientX: number, clientY: number) => {
    if (!imgNaturalSize) return;
    const scale = getDisplayScale();

    if (dragging) {
      const dx = (clientX - dragging.startX) / scale;
      const dy = (clientY - dragging.startY) / scale;
      setCropRect(clampRect({
        x: dragging.origRect.x + dx,
        y: dragging.origRect.y + dy,
        size: dragging.origRect.size,
      }, imgNaturalSize.w, imgNaturalSize.h));
    }

    if (resizing) {
      const dx = (clientX - resizing.startX) / scale;
      const dy = (clientY - resizing.startY) / scale;
      const { corner, origRect } = resizing;
      let newSize = origRect.size;
      let newX = origRect.x;
      let newY = origRect.y;

      if (corner === 'br') {
        newSize = origRect.size + Math.max(dx, dy);
      } else if (corner === 'tl') {
        const delta = Math.min(dx, dy);
        newSize = origRect.size - delta;
        newX = origRect.x + delta;
        newY = origRect.y + delta;
      } else if (corner === 'tr') {
        const delta = Math.max(dx, -dy);
        newSize = origRect.size + delta;
        newY = origRect.y - delta;
      } else if (corner === 'bl') {
        const delta = Math.max(-dx, dy);
        newSize = origRect.size + delta;
        newX = origRect.x - delta;
      }

      setCropRect(clampRect({ x: newX, y: newY, size: newSize }, imgNaturalSize.w, imgNaturalSize.h));
    }
  }, [dragging, resizing, imgNaturalSize]);

  const onCropMouseUp = useCallback(() => {
    setDragging(null);
    setResizing(null);
  }, []);

  useEffect(() => {
    if (dragging || resizing) {
      const handleMouseMove = (e: MouseEvent) => onCropMouseMove(e.clientX, e.clientY);
      const handleTouchMove = (e: TouchEvent) => {
        e.preventDefault();
        if (e.touches.length > 0) onCropMouseMove(e.touches[0].clientX, e.touches[0].clientY);
      };
      window.addEventListener('mousemove', handleMouseMove);
      window.addEventListener('mouseup', onCropMouseUp);
      window.addEventListener('touchmove', handleTouchMove, { passive: false });
      window.addEventListener('touchend', onCropMouseUp);
      return () => {
        window.removeEventListener('mousemove', handleMouseMove);
        window.removeEventListener('mouseup', onCropMouseUp);
        window.removeEventListener('touchmove', handleTouchMove);
        window.removeEventListener('touchend', onCropMouseUp);
      };
    }
  }, [dragging, resizing, onCropMouseMove, onCropMouseUp]);

  // Render the crop at a given output size and return the blob
  const renderCropToBlob = async (outputSize: number): Promise<Blob> => {
    const canvas = document.createElement('canvas');
    canvas.width = outputSize;
    canvas.height = outputSize;
    const ctx = canvas.getContext('2d')!;

    const img = new window.Image();
    img.crossOrigin = 'anonymous';
    await new Promise<void>((resolve, reject) => {
      img.onload = () => resolve();
      img.onerror = reject;
      img.src = imagePreviewUrl!;
    });

    ctx.drawImage(img, cropRect!.x, cropRect!.y, cropRect!.size, cropRect!.size, 0, 0, outputSize, outputSize);

    if (watermarkImg) {
      ctx.drawImage(watermarkImg, 0, 0, outputSize, outputSize);
    }

    return new Promise<Blob>((resolve, reject) => {
      canvas.toBlob(b => b ? resolve(b) : reject(new Error('Failed to create blob')), 'image/jpeg', 0.92);
    });
  };

  // Apply crop + watermark
  const applyCrop = async () => {
    if (!cropRect || !imagePreviewUrl || !imgNaturalSize) return;

    let outputSize = cropRect.size;
    let blob = await renderCropToBlob(outputSize);

    // If over 1MB, progressively shrink until it fits
    while (blob.size > MAX_IMAGE_BYTES && outputSize > 100) {
      const scale = Math.sqrt(MAX_IMAGE_BYTES / blob.size) * 0.95;
      outputSize = Math.max(100, Math.floor(outputSize * scale));
      blob = await renderCropToBlob(outputSize);
    }

    if (croppedPreviewUrl) URL.revokeObjectURL(croppedPreviewUrl);
    setCroppedBlob(blob);
    setCroppedPreviewUrl(URL.createObjectURL(blob));
    setCroppedSize(outputSize);
    setShowCropper(false);
  };

  // Account lookups
  const blueskyAccount = accounts.find(a => a.platform === 'bluesky') ?? null;
  const instagramAccount = accounts.find(a => a.platform === 'instagram') ?? null;
  const twitterAccount = accounts.find(a => a.platform === 'twitter') ?? null;
  const mastodonAccount = accounts.find(a => a.platform === 'mastodon') ?? null;
  const threadsAccount = accounts.find(a => a.platform === 'threads') ?? null;
  const linkedinAccount = accounts.find(a => a.platform === 'linkedin') ?? null;

  const togglePlatform = (p: Platform) => {
    setPlatforms(prev =>
      prev.includes(p) ? prev.filter(x => x !== p) : [...prev, p]
    );
  };

  const suffixLen = (platform: Platform) => {
    const id = suffixIds[platform];
    if (!id) return 0;
    const s = suffixes.find(x => x.id === id);
    return s ? s.content.length + 1 : 0;
  };

  const platformLimit = (platform: Platform) => {
    if (platform === 'bluesky') return BLUESKY_LIMIT;
    if (platform === 'twitter') return TWITTER_LIMIT;
    if (platform === 'mastodon') return MASTODON_LIMIT;
    if (platform === 'linkedin') return LINKEDIN_LIMIT;
    return INSTAGRAM_LIMIT;
  };

  const effectiveLimit = (platform: Platform) =>
    platformLimit(platform) - suffixLen(platform);

  const charLimit = platforms.length === 0 ? effectiveLimit('instagram') : Math.min(
    ...(platforms.includes('bluesky') ? [effectiveLimit('bluesky')] : []),
    ...(platforms.includes('instagram') ? [effectiveLimit('instagram')] : []),
    ...(platforms.includes('twitter') ? [effectiveLimit('twitter')] : []),
    ...(platforms.includes('mastodon') ? [effectiveLimit('mastodon')] : []),
    ...(platforms.includes('linkedin') ? [effectiveLimit('linkedin')] : []),
  );
  const charCount = content.length;
  const overLimit = (customizePerPlatform && platforms.length > 1)
    ? platforms.some(p => {
        const val = contentOverrides[p];
        return val != null && val.length > effectiveLimit(p);
      })
    : charCount > charLimit;
  const charClass = charCount > charLimit ? 'danger' : charCount > charLimit * 0.9 ? 'warning' : '';

  const fetchBGGData = async () => {
    if (!bggUrl.trim()) return;
    setFetchingBgg(true);
    setBggError('');
    try {
      const data = await api.fetchBGGGame(bggUrl.trim());

      setArticleUrl(bggUrl.trim());

      if (data.title) setNewsTagline(`"${data.title}"`);

      const lines: string[] = [];
      if (data.title) lines.push(data.title + (data.yearPublished ? ` (${data.yearPublished})` : ''));
      if (data.designers?.length) lines.push(`Designer: ${data.designers.join(', ')}`);
      if (data.artists?.length) lines.push(`Artist: ${data.artists.join(', ')}`);
      if (data.publishers?.length) lines.push(`Publisher: ${data.publishers.join(', ')}`);
      if (data.minPlayers && data.maxPlayers) lines.push(`Players: ${data.minPlayers}–${data.maxPlayers}`);
      if (data.minPlaytime && data.maxPlaytime) {
        lines.push(data.minPlaytime === data.maxPlaytime ? `Playtime: ${data.minPlaytime} min` : `Playtime: ${data.minPlaytime}–${data.maxPlaytime} min`);
      }
      if (data.minAge) lines.push(`Age: ${data.minAge}+`);
      if (data.rating) lines.push(`Rating: ${data.rating}`);
      if (data.weight) lines.push(`Weight: ${data.weight}`);
      if (data.description) lines.push(data.description);
      setShownotes(lines.filter(l => l.trim()).join('\n'));

      if (data.imageBase64) {
        const byteStr = atob(data.imageBase64);
        const arr = new Uint8Array(byteStr.length);
        for (let i = 0; i < byteStr.length; i++) arr[i] = byteStr.charCodeAt(i);
        const blob = new Blob([arr], { type: 'image/jpeg' });
        const file = new File([blob], data.imageFilename || 'bgg-cover.jpg', { type: 'image/jpeg' });
        if (imagePreviewUrl) URL.revokeObjectURL(imagePreviewUrl);
        if (croppedPreviewUrl) URL.revokeObjectURL(croppedPreviewUrl);
        setImageFile(file);
        setImagePreviewUrl(URL.createObjectURL(file));
        setCroppedBlob(null);
        setCroppedPreviewUrl(null);
        setCroppedSize(null);
        setCropRect(null);
        setShowCropper(false);
      }

      toast.success('BGG data imported');
    } catch (e: any) {
      setBggError(e.message || 'Failed to fetch BGG data');
    } finally {
      setFetchingBgg(false);
    }
  };

  const resetForm = (keepEpisodeNumber = false) => {
    if (!keepEpisodeNumber) setEpisodeNumber('');
    setNewsTagline('');
    setArticleUrl('');
    setShownotes('');
    removeImage();
    setBggUrl('');
    setBggError('');
    setAddSocialPost(false);
    setContent('');
    setContentOverrides({});
    setCustomizePerPlatform(false);
    setFirstComment('');
    setPlatforms([]);
    setScheduledAt(format(new Date(Date.now() + 3600000), "yyyy-MM-dd'T'HH:mm"));
    setStatus('scheduled');
    setSuffixIds({});
  };

  const handleSubmit = async () => {
    if (!episodeNumber.trim()) { toast.error('Episode number is required'); return; }
    if (!newsTagline.trim()) { toast.error('News tagline is required'); return; }
    if (!articleUrl.trim()) { toast.error('Article URL is required'); return; }

    if (addSocialPost) {
      if (platforms.length === 0) { toast.error('Select at least one platform'); return; }
      if (customizePerPlatform && platforms.length > 1) {
        const missing = platforms.filter(p => !(contentOverrides[p] || '').trim());
        if (missing.length > 0) { toast.error(`Content is required for ${missing.join(', ')}`); return; }
        const over = platforms.find(p => (contentOverrides[p] || '').length > effectiveLimit(p));
        if (over) { toast.error(`Content for ${over} exceeds ${effectiveLimit(over)} character limit`); return; }
      } else {
        if (!content.trim()) { toast.error('Post content is required'); return; }
        if (content.length > charLimit) { toast.error(`Content exceeds ${charLimit} character limit`); return; }
      }
    }

    // Use cropped blob if available, otherwise original file
    const fileToSend = croppedBlob
      ? new File([croppedBlob], (imageFile?.name || 'cropped').replace(/\.[^.]+$/, '') + '.jpg', { type: 'image/jpeg' })
      : imageFile;

    const data: any = {
      episodeNumber: episodeNumber.trim(),
      newsTagline: newsTagline.trim(),
      articleUrl: articleUrl.trim(),
      shownotes: shownotes.trim() || undefined,
      imageUrls: [],
    };

    data.addSocialPosting = addSocialPost;

    if (addSocialPost) {
      const accountIds: Record<string, string> = {};
      if (platforms.includes('bluesky') && blueskyAccount) accountIds['bluesky'] = blueskyAccount.id;
      if (platforms.includes('instagram') && instagramAccount) accountIds['instagram'] = instagramAccount.id;
      if (platforms.includes('twitter') && twitterAccount) accountIds['twitter'] = twitterAccount.id;
      if (platforms.includes('mastodon') && mastodonAccount) accountIds['mastodon'] = mastodonAccount.id;
      if (platforms.includes('threads') && threadsAccount) accountIds['threads'] = threadsAccount.id;
      if (platforms.includes('linkedin') && linkedinAccount) accountIds['linkedin'] = linkedinAccount.id;

      data.content = content || '';
      data.firstComment = firstComment.trim() || undefined;
      data.platforms = platforms;
      data.scheduledAt = new Date(scheduledAt).toISOString();
      data.status = status;
      data.suffixIds = suffixIds;
      data.contentOverrides = customizePerPlatform ? contentOverrides : {};
      data.accountIds = accountIds;
      data.postType = 'post';
    }

    setSubmitting(true);
    try {
      await api.submitNews(data, fileToSend ? [fileToSend] : undefined);
      toast.success('News submitted successfully');
      resetForm(true);
    } catch (err: any) {
      toast.error(err.message || 'Failed to submit news');
    } finally {
      setSubmitting(false);
    }
  };

  if (pluginReady === null) {
    return (
      <div className="page">
        <div className="loading-screen"><div className="spinner" /></div>
      </div>
    );
  }

  if (!pluginReady) {
    return (
      <div className="page">
        <div className="page-header">
          <h1><Newspaper size={24} /> News Creator</h1>
        </div>
        <div className="empty-state">
          <Newspaper size={48} />
          <p>The News Creator plugin is not enabled for your team.</p>
          <p style={{ color: 'var(--text-muted)', fontSize: 14 }}>
            Ask your team admin to enable the News Creator plugin and configure the webhook settings.
          </p>
        </div>
      </div>
    );
  }

  const suffixPlatformLabels: Record<string, string> = {
    bluesky: 'Bluesky',
    instagram: 'Instagram',
    twitter: 'X/Twitter',
    mastodon: 'Mastodon',
    threads: 'Threads',
    linkedin: 'LinkedIn',
  };

  const renderCropOverlay = () => {
    if (!cropRect || !imgNaturalSize) return null;
    const scale = getDisplayScale();
    const x = cropRect.x * scale;
    const y = cropRect.y * scale;
    const size = cropRect.size * scale;
    const handleSz = 14;

    return (
      <div
        style={{ position: 'absolute', top: 0, left: 0, right: 0, bottom: 0, cursor: 'default', touchAction: 'none' }}
        onMouseDown={onCropMouseDown}
        onTouchStart={onCropTouchStart}
      >
        {/* Dimmed overlay outside crop */}
        <div style={{ position: 'absolute', top: 0, left: 0, right: 0, height: y, background: '#000' }} />
        <div style={{ position: 'absolute', top: y, left: 0, width: x, height: size, background: '#000' }} />
        <div style={{ position: 'absolute', top: y, left: x + size, right: 0, height: size, background: '#000' }} />
        <div style={{ position: 'absolute', top: y + size, left: 0, right: 0, bottom: 0, background: '#000' }} />

        {/* Crop border */}
        <div style={{
          position: 'absolute', left: x, top: y, width: size, height: size,
          border: '2px solid #fff',
          boxShadow: '0 0 0 1px rgba(0,0,0,0.3)',
          cursor: 'move',
        }}>
          {/* Grid lines */}
          <div style={{ position: 'absolute', left: '33.33%', top: 0, bottom: 0, width: 1, background: 'rgba(255,255,255,0.3)' }} />
          <div style={{ position: 'absolute', left: '66.66%', top: 0, bottom: 0, width: 1, background: 'rgba(255,255,255,0.3)' }} />
          <div style={{ position: 'absolute', top: '33.33%', left: 0, right: 0, height: 1, background: 'rgba(255,255,255,0.3)' }} />
          <div style={{ position: 'absolute', top: '66.66%', left: 0, right: 0, height: 1, background: 'rgba(255,255,255,0.3)' }} />
        </div>

        {/* Resize handles */}
        {[
          { corner: 'tl', left: x - handleSz / 2, top: y - handleSz / 2, cursor: 'nw-resize' },
          { corner: 'tr', left: x + size - handleSz / 2, top: y - handleSz / 2, cursor: 'ne-resize' },
          { corner: 'bl', left: x - handleSz / 2, top: y + size - handleSz / 2, cursor: 'sw-resize' },
          { corner: 'br', left: x + size - handleSz / 2, top: y + size - handleSz / 2, cursor: 'se-resize' },
        ].map(h => (
          <div
            key={h.corner}
            style={{
              position: 'absolute', left: h.left, top: h.top,
              width: handleSz, height: handleSz,
              background: '#fff', border: '1px solid rgba(0,0,0,0.3)',
              borderRadius: 2, cursor: h.cursor,
            }}
          />
        ))}

        {/* Watermark preview inside crop area */}
        {watermarkImg && (
          <img
            src={watermarkImg.src}
            alt="watermark"
            style={{
              position: 'absolute', left: x, top: y,
              width: size, height: size,
              opacity: 0.4,
              pointerEvents: 'none',
            }}
          />
        )}
      </div>
    );
  };

  return (
    <div className="page">
      <div className="page-header">
        <h1><Newspaper size={24} /> News Creator</h1>
      </div>

      <div>
        <div className="card" style={{ padding: 24 }}>
          {/* Episode Number & Tagline in a row */}
          <div className="news-meta-row">
            <div className="form-group">
              <label>Episode Number <span style={{ color: 'var(--danger)' }}>*</span></label>
              <input
                type="text"
                className="input"
                value={episodeNumber}
                onChange={e => setEpisodeNumber(e.target.value)}
                placeholder="e.g. 42"
              />
            </div>
            <div className="form-group">
              <label>News Tagline <span style={{ color: 'var(--danger)' }}>*</span></label>
              <input
                type="text"
                className="input"
                value={newsTagline}
                onChange={e => setNewsTagline(e.target.value)}
                placeholder="Short tagline for the episode"
              />
            </div>
          </div>

          {/* BGG Import */}
          {bggEnabled && (
            <div className="form-group">
              <label><Dice5 size={14} /> BoardGameGeek Import</label>
              <div style={{ display: 'flex', gap: 8 }}>
                <input
                  type="url"
                  className="input"
                  value={bggUrl}
                  onChange={e => { setBggUrl(e.target.value); setBggError(''); }}
                  onKeyDown={e => e.key === 'Enter' && fetchBGGData()}
                  placeholder="https://boardgamegeek.com/boardgame/822/carcassonne"
                  style={{ flex: 1 }}
                  disabled={fetchingBgg}
                />
                <button
                  className="btn btn-secondary btn-sm"
                  onClick={fetchBGGData}
                  disabled={fetchingBgg || !bggUrl.trim()}
                  style={{ whiteSpace: 'nowrap' }}
                >
                  {fetchingBgg ? <><Loader size={14} className="post-editor-spin" /> Importing…</> : 'Import'}
                </button>
              </div>
              {bggError && (
                <span style={{ marginTop: 4, fontSize: '0.8rem', color: 'var(--danger)', display: 'block' }}>{bggError}</span>
              )}
            </div>
          )}

          {/* Article URL */}
          <div className="form-group">
            <label>Article URL <span style={{ color: 'var(--danger)' }}>*</span></label>
            <input
              type="url"
              className="input"
              value={articleUrl}
              onChange={e => setArticleUrl(e.target.value)}
              placeholder="https://example.com/article"
            />
          </div>

          {/* Shownotes */}
          <div className="form-group">
            <label>Shownotes <span style={{ color: 'var(--text-muted)', fontWeight: 400 }}>(optional)</span></label>
            <textarea
              className="textarea"
              value={shownotes}
              onChange={e => setShownotes(e.target.value)}
              placeholder="Additional show notes..."
              rows={3}
            />
          </div>

          {/* Image upload area */}
          <div className="form-group">
            <label><Image size={14} /> Image</label>

            {!imageFile ? (
              <div
                onDragOver={onDragOver}
                onDragLeave={onDragLeave}
                onDrop={onDrop}
                onClick={() => fileRef.current?.click()}
                style={{
                  border: `2px dashed ${dragOver ? 'var(--accent)' : 'var(--border)'}`,
                  borderRadius: 'var(--radius-sm, 8px)',
                  padding: '32px 16px',
                  textAlign: 'center',
                  cursor: 'pointer',
                  background: dragOver ? 'rgba(99, 102, 241, 0.05)' : 'transparent',
                  transition: 'all 0.15s',
                  color: 'var(--text-muted)',
                  fontSize: 14,
                }}
              >
                <Upload size={28} style={{ marginBottom: 8, opacity: 0.5 }} />
                <div>Drop an image here, click to browse, or paste from clipboard</div>
              </div>
            ) : (
              <div>
                {/* Show cropped result or original */}
                <div style={{ position: 'relative', display: 'inline-block', marginBottom: 8 }}>
                  <img
                    src={croppedPreviewUrl || imagePreviewUrl || ''}
                    alt="preview"
                    style={{
                      maxWidth: '100%',
                      maxHeight: 200,
                      borderRadius: 'var(--radius-sm, 8px)',
                      border: '1px solid var(--border)',
                      display: 'block',
                    }}
                  />
                  <button
                    className="remove-btn"
                    onClick={removeImage}
                    style={{
                      position: 'absolute', top: 6, right: 6,
                      width: 24, height: 24, borderRadius: '50%',
                      background: 'rgba(0,0,0,0.7)', border: 'none',
                      color: '#fff', cursor: 'pointer',
                      display: 'flex', alignItems: 'center', justifyContent: 'center',
                      fontSize: 14,
                    }}
                  >
                    <X size={14} />
                  </button>
                </div>
                {croppedSize && croppedBlob && (
                  <div style={{ fontSize: 12, color: 'var(--text-muted)', marginBottom: 4 }}>
                    {croppedSize} × {croppedSize} px · {(croppedBlob.size / 1024).toFixed(0)} KB
                  </div>
                )}
                <div style={{ display: 'flex', gap: 8 }}>
                  <button
                    className="btn btn-ghost btn-sm"
                    onClick={() => setShowCropper(true)}
                  >
                    <Crop size={14} /> {croppedBlob ? 'Re-crop' : 'Crop'} Image
                  </button>
                  <button
                    className="btn btn-ghost btn-sm"
                    onClick={() => fileRef.current?.click()}
                  >
                    <Image size={14} /> Replace
                  </button>
                </div>
              </div>
            )}

            <input
              ref={fileRef}
              type="file"
              accept="image/*"
              style={{ display: 'none' }}
              onChange={e => {
                const file = e.target.files?.[0];
                if (file) handleFile(file);
              }}
            />
          </div>

          {/* Crop Modal */}
          {showCropper && imagePreviewUrl && (
            <div
              className="news-crop-overlay"
              onClick={() => setShowCropper(false)}
            >
              <div
                className="card news-crop-card"
                onClick={e => e.stopPropagation()}
              >
                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 16 }}>
                  <h3 style={{ margin: 0, fontSize: 16, display: 'flex', alignItems: 'center', gap: 8 }}>
                    <Crop size={18} /> Crop Image
                    {cropRect && <span style={{ fontSize: 12, color: 'var(--text-muted)', fontWeight: 400 }}>
                      ({cropRect.size} × {cropRect.size} px)
                    </span>}
                    {watermark && <span style={{ fontSize: 12, color: 'var(--text-muted)', fontWeight: 400 }}>
                      · watermark: {watermark.name}
                    </span>}
                  </h3>
                  <button className="btn btn-ghost btn-sm" onClick={() => setShowCropper(false)}>
                    <X size={18} />
                  </button>
                </div>
                <div
                  ref={cropContainerRef}
                  style={{
                    position: 'relative',
                    flex: 1,
                    minHeight: 0,
                    overflow: 'hidden',
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'center',
                    background: 'var(--bg-secondary, #0f172a)',
                    borderRadius: 8,
                    userSelect: 'none',
                    touchAction: 'none',
                  }}
                >
                  <img
                    ref={cropImgRef}
                    src={imagePreviewUrl}
                    alt="crop source"
                    onLoad={onCropImageLoad}
                    draggable={false}
                    style={{
                      maxWidth: '100%',
                      maxHeight: 'calc(90vh - 140px)',
                      display: 'block',
                    }}
                  />
                  {renderCropOverlay()}
                </div>
                <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 8, marginTop: 16, flexWrap: 'wrap' }}>
                  <button className="btn btn-secondary" onClick={() => setShowCropper(false)}>
                    Cancel
                  </button>
                  <button className="btn btn-primary" onClick={applyCrop} disabled={!cropRect}>
                    <Crop size={14} /> Apply Crop{watermark ? ' & Watermark' : ''}
                  </button>
                </div>
              </div>
            </div>
          )}

          {/* Social Posting Toggle */}
          <div className="form-group" style={{ borderTop: '1px solid var(--border)', paddingTop: 20, marginTop: 12 }}>
            <label style={{ display: 'flex', alignItems: 'center', gap: 8, cursor: 'pointer', margin: 0 }}>
              <input
                type="checkbox"
                checked={addSocialPost}
                onChange={e => setAddSocialPost(e.target.checked)}
              />
              Add as Social Media Posting
            </label>
          </div>

          {/* Social Posting Fields */}
          {addSocialPost && (
            <div style={{ display: 'flex', flexDirection: 'column', gap: 0, padding: '16px', background: 'var(--bg-secondary, #1e293b)', borderRadius: 8, border: '1px solid var(--border)', marginTop: 8 }}>
              {/* Platform selector */}
              <div className="form-group">
                <label>Platforms</label>
                <div className="platform-selector">
                  <div
                    className={`platform-option bluesky ${platforms.includes('bluesky') ? 'selected' : ''} ${!blueskyAccount ? 'disabled' : ''}`}
                    onClick={() => blueskyAccount && togglePlatform('bluesky')}
                    title={!blueskyAccount ? 'No Bluesky account configured' : `Will post as @${blueskyAccount.accountName}`}
                  >
                    <PlatformIcon platform="bluesky" size={18} />
                  </div>
                  <div
                    className={`platform-option instagram ${platforms.includes('instagram') ? 'selected' : ''} ${!instagramAccount ? 'disabled' : ''}`}
                    onClick={() => instagramAccount && togglePlatform('instagram')}
                    title={!instagramAccount ? 'No Instagram account configured' : `Will post as @${instagramAccount.accountName}`}
                  >
                    <PlatformIcon platform="instagram" size={18} />
                  </div>
                  <div
                    className={`platform-option twitter ${platforms.includes('twitter') ? 'selected' : ''} ${!twitterAccount ? 'disabled' : ''}`}
                    onClick={() => twitterAccount && togglePlatform('twitter')}
                    title={!twitterAccount ? 'No X/Twitter account configured' : `Will post as @${twitterAccount.accountName}`}
                  >
                    <PlatformIcon platform="twitter" size={18} />
                  </div>
                  <div
                    className={`platform-option mastodon ${platforms.includes('mastodon') ? 'selected' : ''} ${!mastodonAccount ? 'disabled' : ''}`}
                    onClick={() => mastodonAccount && togglePlatform('mastodon')}
                    title={!mastodonAccount ? 'No Mastodon account configured' : `Will post as @${mastodonAccount.accountName}`}
                  >
                    <PlatformIcon platform="mastodon" size={18} />
                  </div>
                  <div
                    className={`platform-option threads ${platforms.includes('threads') ? 'selected' : ''} ${!threadsAccount ? 'disabled' : ''}`}
                    onClick={() => threadsAccount && togglePlatform('threads')}
                    title={!threadsAccount ? 'No Threads account configured' : `Will post as @${threadsAccount.accountName}`}
                  >
                    <PlatformIcon platform="threads" size={18} />
                  </div>
                  <div
                    className={`platform-option linkedin ${platforms.includes('linkedin') ? 'selected' : ''} ${!linkedinAccount ? 'disabled' : ''}`}
                    onClick={() => linkedinAccount && togglePlatform('linkedin')}
                    title={!linkedinAccount ? 'No LinkedIn account configured' : `Will post as ${linkedinAccount.displayName || linkedinAccount.accountName}`}
                  >
                    <PlatformIcon platform="linkedin" size={18} />
                  </div>
                </div>
              </div>

              {/* Content */}
              <div className="form-group">
                <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 4 }}>
                  <label style={{ margin: 0 }}>Content</label>
                  {platforms.length > 1 && (
                    <label className="customize-per-platform-toggle" style={{ display: 'flex', alignItems: 'center', gap: 6, fontSize: '0.8rem', cursor: 'pointer', margin: 0 }}>
                      <input
                        type="checkbox"
                        checked={customizePerPlatform}
                        onChange={e => {
                          const checked = e.target.checked;
                          if (checked) {
                            const baseText = content;
                            setCustomizePerPlatform(true);
                            setContent('');
                            const newOverrides: Record<string, string> = {};
                            for (const p of platforms) {
                              newOverrides[p] = baseText;
                            }
                            setContentOverrides(newOverrides);
                          } else {
                            let longest = '';
                            for (const p of platforms) {
                              const val = contentOverrides[p] || '';
                              if (val.length > longest.length) longest = val;
                            }
                            setContent(longest);
                            setContentOverrides({});
                            setCustomizePerPlatform(false);
                          }
                        }}
                      />
                      Customize per platform
                    </label>
                  )}
                </div>

                {customizePerPlatform && platforms.length > 1 ? (
                  <>
                    {platforms.map(platform => {
                      const overrideVal = contentOverrides[platform] ?? '';
                      const limit = effectiveLimit(platform);
                      const count = overrideVal.length;
                      const cls = count > limit ? 'danger' : count > limit * 0.9 ? 'warning' : '';
                      return (
                        <div key={platform} style={{ marginBottom: 8 }}>
                          <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginBottom: 4 }}>
                            <PlatformIcon platform={platform} size={12} />
                            <span style={{ fontSize: '0.8rem', textTransform: 'capitalize' }}>{platform}</span>
                          </div>
                          <MentionTextarea
                            className="textarea post-textarea"
                            value={overrideVal}
                            onChange={val => setContentOverrides(prev => ({ ...prev, [platform]: val }))}
                            mentions={mentions}
                            platform={platform}
                            placeholder={`Text for ${platform}...`}
                            rows={4}
                          />
                          <div className={`char-counter ${cls}`} style={{ textAlign: 'right' }}>{count} / {limit}</div>
                        </div>
                      );
                    })}
                  </>
                ) : (
                  <>
                    <MentionTextarea
                      className="textarea post-textarea"
                      value={content}
                      onChange={setContent}
                      mentions={mentions}
                      placeholder="What's on your mind? Use #hashtags for tags..."
                      rows={5}
                    />
                    <div className={`char-counter ${charClass}`} style={{ textAlign: 'right' }}>
                      {charCount} / {charLimit}
                    </div>
                  </>
                )}
              </div>

              {/* Suffix selectors */}
              {suffixes.length > 0 && platforms.length > 0 && (
                <div className="suffix-selectors">
                  {platforms.map(platform => (
                    <div key={platform} className="suffix-selector-row">
                      <label className="suffix-label">
                        <PlatformIcon platform={platform} size={12} /> {suffixPlatformLabels[platform] ?? platform} suffix
                      </label>
                      <select
                        className="select suffix-select"
                        value={suffixIds[platform] || ''}
                        onChange={e => setSuffixIds(prev => {
                          const next = { ...prev };
                          if (e.target.value) next[platform] = e.target.value;
                          else delete next[platform];
                          return next;
                        })}
                      >
                        <option value="">None</option>
                        {suffixes.map(s => (
                          <option key={s.id} value={s.id}>{s.name}</option>
                        ))}
                      </select>
                    </div>
                  ))}
                </div>
              )}

              {/* First Comment */}
              <div className="form-group">
                <label style={{ whiteSpace: 'nowrap' }}><MessageSquare size={14} /> First Comment <span style={{ color: 'var(--text-muted)', fontWeight: 400 }}>(optional)</span></label>
                <textarea
                  className="textarea first-comment-textarea"
                  value={firstComment}
                  onChange={e => setFirstComment(e.target.value)}
                  placeholder="Add a first comment to your post..."
                  rows={3}
                />
              </div>

              {/* Schedule */}
              <div className="form-group">
                <label><Clock size={14} /> Schedule</label>
                <input
                  type="datetime-local"
                  className="input"
                  value={scheduledAt}
                  onChange={e => setScheduledAt(e.target.value)}
                />
              </div>

              {/* Status */}
              <div className="form-group">
                <label><Tag size={14} /> Status</label>
                <select className="select" value={status} onChange={e => setStatus(e.target.value as 'scheduled' | 'draft')}>
                  <option value="scheduled">Scheduled</option>
                  <option value="draft">Draft</option>
                </select>
              </div>
            </div>
          )}

          {/* Submit */}
          <div style={{ marginTop: 24, display: 'flex', justifyContent: 'flex-end' }}>
            <button
              className="btn btn-primary"
              onClick={handleSubmit}
              disabled={submitting || overLimit || !episodeNumber.trim() || !newsTagline.trim() || !articleUrl.trim()}
            >
              <Send size={16} /> {submitting ? 'Submitting...' : 'Submit News'}
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}
