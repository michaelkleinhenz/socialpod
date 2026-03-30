import { useState, useRef, useEffect, useCallback, useMemo } from 'react';
import { format, parseISO } from 'date-fns';
import { api } from '../../services/api';
import type { Post, Platform, Suffix, SocialAccount } from '../../types';
import { X, Image, Send, Trash2, Clock, Tag, Wand2, MessageSquare, Sparkles } from 'lucide-react';
import toast from 'react-hot-toast';
import './PostEditor.css';

// Module-level cache so the SDK is initialized once per page load and login
// state persists across PostEditor open/close cycles.
let _ccEditor: any = null;
let _ccClientId = '';

interface Props {
  post?: Post | null;
  defaultDate?: Date | null;
  onSave: (data: any, files?: File[]) => void;
  onDelete?: () => void;
  onClose: () => void;
}

const BLUESKY_LIMIT = 300;
const INSTAGRAM_LIMIT = 2200;

function renderWithHashtags(text: string) {
  if (!text) return null;
  const parts = text.split(/(#[\w\u00C0-\u024F]+)/g);
  return parts.map((part, i) =>
    part.startsWith('#')
      ? <span key={i} className="preview-hashtag">{part}</span>
      : <span key={i}>{part}</span>
  );
}

interface PreviewProps {
  content: string;
  platforms: Platform[];
  imageUrls: string[];
  scheduledAt: string;
  apiUrl: string;
  blueskyAccount?: SocialAccount | null;
  instagramAccount?: SocialAccount | null;
  bluskySuffix?: string;
  instagramSuffix?: string;
}

function PostPreview({ content, platforms, imageUrls, scheduledAt, apiUrl, blueskyAccount, instagramAccount, bluskySuffix, instagramSuffix }: PreviewProps) {
  const time = scheduledAt
    ? format(parseISO(new Date(scheduledAt).toISOString()), 'MMM d, yyyy · HH:mm')
    : '';

  const bskyDisplayName = blueskyAccount?.displayName || blueskyAccount?.accountName || 'Your Name';
  const bskyHandle = blueskyAccount?.accountName
    ? (blueskyAccount.accountName.includes('.') ? blueskyAccount.accountName : `${blueskyAccount.accountName}.bsky.social`)
    : 'yourhandle.bsky.social';
  const igHandle = instagramAccount?.accountName || 'yourhandle';

  const bskyContent = bluskySuffix ? content + '\n' + bluskySuffix : content;
  const igContent = instagramSuffix ? content + '\n' + instagramSuffix : content;

  if (platforms.length === 0) {
    return (
      <div className="preview-empty">
        <p>Select a platform to see a preview</p>
      </div>
    );
  }

  return (
    <div className="preview-cards">
      {platforms.includes('bluesky') && (
        <div className="preview-card preview-bluesky">
          <div className="preview-card-header">
            <div className="preview-avatar" />
            <div className="preview-user-info">
              <span className="preview-display-name">{bskyDisplayName}</span>
              <span className="preview-handle">@{bskyHandle}</span>
            </div>
            <div className="preview-platform-badge bluesky">Bluesky</div>
          </div>
          <div className="preview-content">
            {content
              ? <p className="preview-text">{renderWithHashtags(bskyContent)}</p>
              : <p className="preview-placeholder">Start typing to see a preview…</p>
            }
          </div>
          {imageUrls.length > 0 && (
            <div className={`preview-images preview-images-${Math.min(imageUrls.length, 4)}`}>
              {imageUrls.slice(0, 4).map((url, i) => (
                <img key={i} src={url.startsWith('/') ? apiUrl + url : url} alt="" className="preview-image" />
              ))}
            </div>
          )}
          <div className="preview-footer">
            <span className="preview-time">{time}</span>
          </div>
        </div>
      )}

      {platforms.includes('instagram') && (
        <div className="preview-card preview-instagram">
          <div className="preview-card-header">
            <div className="preview-avatar" />
            <div className="preview-user-info">
              <span className="preview-display-name">{igHandle}</span>
            </div>
            <div className="preview-platform-badge instagram">Instagram</div>
          </div>
          {imageUrls.length > 0 ? (
            <div className="preview-ig-image">
              <img src={imageUrls[0].startsWith('/') ? apiUrl + imageUrls[0] : imageUrls[0]} alt="" />
            </div>
          ) : (
            <div className="preview-ig-image-placeholder">
              <Image size={32} />
              <span>No image selected</span>
            </div>
          )}
          <div className="preview-content">
            {content
              ? <p className="preview-text"><strong className="preview-display-name">{igHandle}</strong>{' '}{renderWithHashtags(igContent)}</p>
              : <p className="preview-placeholder">Start typing to see a preview…</p>
            }
          </div>
          <div className="preview-footer">
            <span className="preview-time">{time}</span>
          </div>
        </div>
      )}
    </div>
  );
}

export function PostEditor({ post, defaultDate, onSave, onDelete, onClose }: Props) {
  const defaultTime = defaultDate
    ? format(defaultDate, "yyyy-MM-dd'T'HH:mm")
    : format(new Date(Date.now() + 3600000), "yyyy-MM-dd'T'HH:mm");

  const [content, setContent] = useState(post?.content || '');
  const [firstComment, setFirstComment] = useState(post?.firstComment || '');
  const [platforms, setPlatforms] = useState<Platform[]>(post?.platforms || ['bluesky']);
  const [scheduledAt, setScheduledAt] = useState(
    post ? format(new Date(post.scheduledAt), "yyyy-MM-dd'T'HH:mm") : defaultTime
  );
  // Unified image list: existing server URLs and local File objects not yet uploaded.
  type ImageItem = { kind: 'url'; url: string } | { kind: 'file'; file: File };
  const [images, setImages] = useState<ImageItem[]>(
    (post?.imageUrls || []).map(url => ({ kind: 'url' as const, url }))
  );
  // Cache for object URLs so we don't create a new one on every render.
  const objUrlCache = useRef<Map<File, string>>(new Map());
  const [tags] = useState<string[]>(post?.tags || []);
  const [status, setStatus] = useState(post?.status || 'scheduled');
  const [suffixes, setSuffixes] = useState<Suffix[]>([]);
  const [suffixIds, setSuffixIds] = useState<Record<string, string>>(post?.suffixIds || {});
  const [saving, setSaving] = useState(false);
  const [accounts, setAccounts] = useState<SocialAccount[]>([]);
  const [adobeClientId, setAdobeClientId] = useState('');
  const [adobeLoading, setAdobeLoading] = useState(false);
  const [adobeActive, setAdobeActive] = useState(false);
  const [aiEnabled, setAiEnabled] = useState(false);
  const [generating, setGenerating] = useState(false);
  const fileRef = useRef<HTMLInputElement>(null);
  const textareaRef = useRef<HTMLTextAreaElement>(null);

  useEffect(() => {
    textareaRef.current?.focus();
    api.getPublicSettings().then(s => {
      if (s.adobeExpressClientId) setAdobeClientId(s.adobeExpressClientId);
      if (s.openRouterEnabled) setAiEnabled(true);
    }).catch(() => {});
    api.getSuffixes().then(setSuffixes).catch(() => {});
    api.getActiveAccounts().then(setAccounts).catch(() => {});
    return () => {
      document.body.classList.remove('adobe-express-open');
      document.getElementById('adobe-zindex-fix')?.remove();
      objUrlCache.current.forEach(url => URL.revokeObjectURL(url));
    };
  }, []);

  const apiUrl = import.meta.env.VITE_API_URL || '';

  const launchAdobeExpress = useCallback(async () => {
    if (!adobeClientId) return;
    setAdobeLoading(true);

    // Hide the PostEditor overlay and sidebar BEFORE any async work so
    // the Adobe auth/editor UI is never blocked by our overlay.
    setAdobeActive(true);
    document.body.classList.add('adobe-express-open');

    // Inject CSS so the Adobe SDK's body-level container (appended outside
    // #root) always stacks above the sidebar and any other positioned element,
    // regardless of which class/id the SDK uses internally.
    if (!document.getElementById('adobe-zindex-fix')) {
      const s = document.createElement('style');
      s.id = 'adobe-zindex-fix';
      s.textContent = 'body > div:not(#root) { z-index: 9999 !important; }';
      document.head.appendChild(s);
    }

    const closeAdobe = () => {
      setAdobeActive(false);
      document.body.classList.remove('adobe-express-open');
      document.getElementById('adobe-zindex-fix')?.remove();
    };

    try {
      // Re-initialize only if the client ID changed or SDK was never loaded.
      if (!_ccEditor || _ccClientId !== adobeClientId) {
        if (!(window as any).CCEverywhere) {
          await import('https://cc-embed.adobe.com/sdk/v4/CCEverywhere.js' as any);
        }
        const { editor } = await (window as any).CCEverywhere.initialize(
          { clientId: adobeClientId, appName: 'SocialPod' },
          { loginMode: 'delayed' },
        );
        _ccEditor = editor;
        _ccClientId = adobeClientId;
      }

      _ccEditor.create(
        {
          canvasSize: { width: 1080, height: 1080, unit: 'px' },
        },
        {
          callbacks: {
            // v4 still passes two arguments: (intent, publishParams).
            onPublish: async (_intent: any, publishParams: any) => {
              closeAdobe();
              const dataUrl = publishParams?.asset?.[0]?.data;
              if (!dataUrl) { toast.error('No image data received'); return; }
              try {
                const res = await fetch(dataUrl);
                const blob = await res.blob();
                const file = new File([blob], 'design.png', { type: 'image/png' });
                setImages(prev => [...prev, { kind: 'file' as const, file }]);
                toast.success('Design added');
              } catch {
                toast.error('Failed to save design');
              }
            },
            onCancel: closeAdobe,
            onError: (err: any) => {
              closeAdobe();
              toast.error('Adobe Express error: ' + err.toString());
            },
          },
        },
        [
          {
            id: 'save-to-post',
            label: 'Add to Post',
            action: { target: 'publish' },
            style: { uiType: 'button' },
          },
        ],
      );
    } catch {
      closeAdobe();
      toast.error('Failed to open Adobe Express');
    } finally {
      setAdobeLoading(false);
    }
  }, [adobeClientId, platforms]);

  const generateText = useCallback(async () => {
    if (!content.trim()) { toast.error('Enter a prompt first'); return; }
    setGenerating(true);
    try {
      const { text } = await api.generateText(content, platforms);
      setContent(text);
    } catch (err: any) {
      toast.error(err.message || 'Text generation failed');
    } finally {
      setGenerating(false);
    }
  }, [content, platforms]);

  const togglePlatform = (p: Platform) => {
    setPlatforms(prev =>
      prev.includes(p) ? prev.filter(x => x !== p) : [...prev, p]
    );
  };

  const suffixLen = (platform: Platform) => {
    const id = suffixIds[platform];
    if (!id) return 0;
    const s = suffixes.find(x => x.id === id);
    return s ? s.content.length + 1 : 0; // +1 for newline separator
  };

  const effectiveLimit = (platform: Platform) =>
    (platform === 'bluesky' ? BLUESKY_LIMIT : INSTAGRAM_LIMIT) - suffixLen(platform);

  const charLimit = platforms.length === 0 ? effectiveLimit('instagram') : Math.min(
    ...(platforms.includes('bluesky') ? [effectiveLimit('bluesky')] : []),
    ...(platforms.includes('instagram') ? [effectiveLimit('instagram')] : []),
  );
  const charCount = content.length;
  const overLimit = charCount > charLimit;
  const charClass = overLimit ? 'danger' : charCount > charLimit * 0.9 ? 'warning' : '';

  const handleImageUpload = (files: FileList | null) => {
    if (!files) return;
    setImages(prev => [
      ...prev,
      ...Array.from(files).map(file => ({ kind: 'file' as const, file })),
    ]);
  };

  const removeImage = (idx: number) => {
    setImages(prev => {
      const item = prev[idx];
      if (item.kind === 'file') {
        const cached = objUrlCache.current.get(item.file);
        if (cached) { URL.revokeObjectURL(cached); objUrlCache.current.delete(item.file); }
      }
      return prev.filter((_, i) => i !== idx);
    });
  };

  // Returns a displayable URL for any ImageItem without leaking object URLs.
  const previewUrl = useMemo(() => (item: ImageItem) => {
    if (item.kind === 'url') return item.url.startsWith('/') ? apiUrl + item.url : item.url;
    if (!objUrlCache.current.has(item.file)) {
      objUrlCache.current.set(item.file, URL.createObjectURL(item.file));
    }
    return objUrlCache.current.get(item.file)!;
  }, [apiUrl]);

  // Image URLs for the preview panel (server URLs + object URLs for local files).
  const previewImageUrls = useMemo(() =>
    images.map(item => previewUrl(item)),
  [images, previewUrl]);

  const handleSubmit = async () => {
    if (!content.trim()) { toast.error('Content is required'); return; }
    if (platforms.length === 0) { toast.error('Select at least one platform'); return; }
    if (content.length > charLimit) { toast.error(`Content exceeds ${charLimit} character limit`); return; }

    const imageUrls = images.filter(i => i.kind === 'url').map(i => (i as { kind: 'url'; url: string }).url);
    const imageFiles = images.filter(i => i.kind === 'file').map(i => (i as { kind: 'file'; file: File }).file);

    setSaving(true);
    try {
      await onSave(
        {
          content,
          firstComment: firstComment.trim() || undefined,
          platforms,
          scheduledAt: new Date(scheduledAt).toISOString(),
          imageUrls,
          tags,
          status,
          suffixIds,
        },
        imageFiles.length > 0 ? imageFiles : undefined,
      );
    } finally {
      setSaving(false);
    }
  };

  const blueskyAccount = accounts.find(a => a.platform === 'bluesky') ?? null;
  const instagramAccount = accounts.find(a => a.platform === 'instagram') ?? null;

  const isDirty = content !== (post?.content || '')
    || firstComment !== (post?.firstComment || '')
    || scheduledAt !== (post ? format(new Date(post.scheduledAt), "yyyy-MM-dd'T'HH:mm") : defaultTime)
    || status !== (post?.status || 'scheduled')
    || JSON.stringify(platforms) !== JSON.stringify(post?.platforms || ['bluesky'])
    || images.length !== (post?.imageUrls || []).length
    || JSON.stringify(suffixIds) !== JSON.stringify(post?.suffixIds || {});

  const handleClose = () => {
    if (isDirty && !window.confirm('You have unsaved changes. Discard them?')) return;
    onClose();
  };

  return (
    <div className="modal-overlay" style={adobeActive ? { display: 'none' } : undefined}>
      <div className="modal post-editor-modal" onClick={e => e.stopPropagation()}>
        <div className="editor-header">
          <h2>{post ? 'Edit Post' : 'New Post'}</h2>
          <button className="btn btn-ghost btn-sm" onClick={handleClose}>
            <X size={18} />
          </button>
        </div>

        <div className="editor-layout">
          <div className="editor-body">
            {/* Platform selector */}
            <div className="platform-selector">
              <div
                className={`platform-option bluesky ${platforms.includes('bluesky') ? 'selected' : ''}`}
                onClick={() => togglePlatform('bluesky')}
              >
                <span className="platform-dot bluesky" />
                Bluesky
              </div>
              <div
                className={`platform-option instagram ${platforms.includes('instagram') ? 'selected' : ''}`}
                onClick={() => togglePlatform('instagram')}
              >
                <span className="platform-dot instagram" />
                Instagram
              </div>
            </div>

            {/* Suffix selectors */}
            {suffixes.length > 0 && (platforms.includes('bluesky') || platforms.includes('instagram')) && (
              <div className="suffix-selectors">
                {platforms.includes('bluesky') && (
                  <div className="suffix-selector-row">
                    <label className="suffix-label">
                      <span className="platform-dot bluesky" /> Bluesky suffix
                    </label>
                    <select
                      className="select suffix-select"
                      value={suffixIds['bluesky'] || ''}
                      onChange={e => setSuffixIds(prev => {
                        const next = { ...prev };
                        if (e.target.value) next['bluesky'] = e.target.value;
                        else delete next['bluesky'];
                        return next;
                      })}
                    >
                      <option value="">None</option>
                      {suffixes.map(s => (
                        <option key={s.id} value={s.id}>{s.name}</option>
                      ))}
                    </select>
                  </div>
                )}
                {platforms.includes('instagram') && (
                  <div className="suffix-selector-row">
                    <label className="suffix-label">
                      <span className="platform-dot instagram" /> Instagram suffix
                    </label>
                    <select
                      className="select suffix-select"
                      value={suffixIds['instagram'] || ''}
                      onChange={e => setSuffixIds(prev => {
                        const next = { ...prev };
                        if (e.target.value) next['instagram'] = e.target.value;
                        else delete next['instagram'];
                        return next;
                      })}
                    >
                      <option value="">None</option>
                      {suffixes.map(s => (
                        <option key={s.id} value={s.id}>{s.name}</option>
                      ))}
                    </select>
                  </div>
                )}
              </div>
            )}

            {/* Content */}
            <div className="form-group">
              <textarea
                ref={textareaRef}
                className="textarea post-textarea"
                value={content}
                onChange={e => setContent(e.target.value)}
                placeholder="What's on your mind? Use #hashtags for tags..."
                rows={5}
              />
              <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                {aiEnabled && (
                  <button className="btn btn-ghost btn-sm" onClick={generateText} disabled={generating || !content.trim()}>
                    <Sparkles size={14} /> {generating ? 'Generating...' : 'Generate'}
                  </button>
                )}
                <div className={`char-counter ${charClass}`} style={{ marginLeft: 'auto' }}>
                  {charCount} / {charLimit}
                </div>
              </div>
            </div>

            {/* First Comment */}
            <div className="form-group">
              <label><MessageSquare size={14} /> First Comment <span className="first-comment-label-hint">(optional — posted right after)</span></label>
              <textarea
                className="textarea first-comment-textarea"
                value={firstComment}
                onChange={e => setFirstComment(e.target.value)}
                placeholder="Add a first comment to your post…"
                rows={3}
              />
            </div>

            {/* Images */}
            <div className="editor-images">
              {images.length > 0 && (
                <div className="image-preview-grid">
                  {images.map((item, i) => (
                    <div key={i} className="image-preview">
                      <img src={previewUrl(item)} alt="" />
                      <button className="remove-btn" onClick={() => removeImage(i)}>x</button>
                    </div>
                  ))}
                </div>
              )}
              <input
                ref={fileRef}
                type="file"
                accept="image/*"
                multiple
                style={{ display: 'none' }}
                onChange={e => handleImageUpload(e.target.files)}
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
              <select className="select" value={status} onChange={e => setStatus(e.target.value as any)}>
                <option value="scheduled">Scheduled</option>
                <option value="draft">Draft</option>
              </select>
            </div>
          </div>

          {/* Live preview */}
          <div className="editor-preview">
            <div className="editor-preview-label">Preview</div>
            <PostPreview
              content={content}
              platforms={platforms}
              imageUrls={previewImageUrls}
              scheduledAt={scheduledAt}
              apiUrl={apiUrl}
              blueskyAccount={blueskyAccount}
              instagramAccount={instagramAccount}
              bluskySuffix={suffixes.find(s => s.id === suffixIds['bluesky'])?.content}
              instagramSuffix={suffixes.find(s => s.id === suffixIds['instagram'])?.content}
            />
          </div>
        </div>

        <div className="editor-footer">
          <div className="footer-left">
            <button className="btn btn-ghost btn-sm" onClick={() => fileRef.current?.click()}>
              <Image size={16} /> Upload
            </button>
            {adobeClientId && (
              <button className="btn btn-ghost btn-sm" onClick={launchAdobeExpress} disabled={adobeLoading}>
                <Wand2 size={16} /> {adobeLoading ? 'Opening...' : 'Create Design'}
              </button>
            )}
            {onDelete && (
              <button className="btn btn-danger btn-sm" onClick={() => { if (window.confirm('Are you sure you want to delete this post?')) onDelete(); }}>
                <Trash2 size={16} /> Delete
              </button>
            )}
          </div>
          <div className="footer-right">
            <button className="btn btn-secondary" onClick={handleClose}>Cancel</button>
            <button className="btn btn-primary" onClick={handleSubmit} disabled={saving || overLimit}>
              <Send size={16} /> {saving ? 'Saving...' : post ? 'Update' : 'Schedule'}
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}
