import { useState, useRef, useEffect, useCallback } from 'react';
import { format } from 'date-fns';
import { api } from '../../services/api';
import type { Post, Platform } from '../../types';
import { X, Image, Send, Trash2, Clock, Tag, Hash, Wand2 } from 'lucide-react';
import toast from 'react-hot-toast';
import './PostEditor.css';

// Module-level cache so the SDK is initialized once per page load and login
// state persists across PostEditor open/close cycles.
let _ccEditor: any = null;
let _ccClientId = '';

interface Props {
  post?: Post | null;
  defaultDate?: Date | null;
  onSave: (data: any) => void;
  onDelete?: () => void;
  onClose: () => void;
}

const BLUESKY_LIMIT = 300;
const INSTAGRAM_LIMIT = 2200;

export function PostEditor({ post, defaultDate, onSave, onDelete, onClose }: Props) {
  const defaultTime = defaultDate
    ? format(defaultDate, "yyyy-MM-dd'T'HH:mm")
    : format(new Date(Date.now() + 3600000), "yyyy-MM-dd'T'HH:mm");

  const [content, setContent] = useState(post?.content || '');
  const [platforms, setPlatforms] = useState<Platform[]>(post?.platforms || ['bluesky']);
  const [scheduledAt, setScheduledAt] = useState(
    post ? format(new Date(post.scheduledAt), "yyyy-MM-dd'T'HH:mm") : defaultTime
  );
  const [imageUrls, setImageUrls] = useState<string[]>(post?.imageUrls || []);
  const [tags, setTags] = useState<string[]>(post?.tags || []);
  const [tagInput, setTagInput] = useState('');
  const [status, setStatus] = useState(post?.status || 'scheduled');
  const [uploading, setUploading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [adobeClientId, setAdobeClientId] = useState('');
  const [adobeLoading, setAdobeLoading] = useState(false);
  const [adobeActive, setAdobeActive] = useState(false);
  const fileRef = useRef<HTMLInputElement>(null);
  const textareaRef = useRef<HTMLTextAreaElement>(null);

  useEffect(() => {
    textareaRef.current?.focus();
    api.getPublicSettings().then(s => {
      if (s.adobeExpressClientId) setAdobeClientId(s.adobeExpressClientId);
    }).catch(() => {});
    return () => {
      // Clean up in case the component unmounts while Adobe is open.
      document.body.classList.remove('adobe-express-open');
      document.getElementById('adobe-zindex-fix')?.remove();
    };
  }, []);

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
      // The SDK is pre-loaded via <script defer> in index.html so the OAuth
      // popup (which redirects back to this app's origin) can complete the
      // auth handshake via postMessage. The dynamic import here is a fallback
      // for environments where the script tag hasn't executed yet.
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
          canvasSize: platforms.includes('instagram')
            ? { width: 1080, height: 1080, unit: 'px' }
            : { width: 1200, height: 675, unit: 'px' },
        },
        {
          callbacks: {
            // SDK v4 passes a single publishParams argument (not intent + params).
            onPublish: async (publishParams: any) => {
              closeAdobe();
              const dataUrl = publishParams?.asset?.[0]?.data;
              if (!dataUrl) { toast.error('No image data received'); return; }
              try {
                const res = await fetch(dataUrl);
                const blob = await res.blob();
                const file = new File([blob], 'design.png', { type: 'image/png' });
                const uploaded = await api.uploadImage(file);
                setImageUrls(prev => [...prev, uploaded.url]);
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

  const togglePlatform = (p: Platform) => {
    setPlatforms(prev =>
      prev.includes(p) ? prev.filter(x => x !== p) : [...prev, p]
    );
  };

  const charLimit = platforms.length === 0 ? INSTAGRAM_LIMIT : Math.min(
    ...(platforms.includes('bluesky') ? [BLUESKY_LIMIT] : []),
    ...(platforms.includes('instagram') ? [INSTAGRAM_LIMIT] : []),
  );
  const charCount = content.length;
  const overLimit = charCount > charLimit;
  const charClass = overLimit ? 'danger' : charCount > charLimit * 0.9 ? 'warning' : '';

  const handleImageUpload = async (files: FileList | null) => {
    if (!files) return;
    setUploading(true);
    try {
      for (const file of Array.from(files)) {
        const res = await api.uploadImage(file);
        setImageUrls(prev => [...prev, res.url]);
      }
    } catch {
      toast.error('Upload failed');
    } finally {
      setUploading(false);
    }
  };

  const removeImage = (idx: number) => {
    setImageUrls(prev => prev.filter((_, i) => i !== idx));
  };

  const addTag = () => {
    const tag = tagInput.trim().replace(/^#/, '');
    if (tag && !tags.includes(tag)) {
      setTags(prev => [...prev, tag]);
    }
    setTagInput('');
  };

  const handleSubmit = async () => {
    if (!content.trim()) { toast.error('Content is required'); return; }
    if (platforms.length === 0) { toast.error('Select at least one platform'); return; }
    if (content.length > charLimit) { toast.error(`Content exceeds ${charLimit} character limit`); return; }

    setSaving(true);
    try {
      await onSave({
        content,
        platforms,
        scheduledAt: new Date(scheduledAt).toISOString(),
        imageUrls,
        tags,
        status,
      });
    } finally {
      setSaving(false);
    }
  };

  const apiUrl = import.meta.env.VITE_API_URL || '';

  return (
    <div className="modal-overlay" style={adobeActive ? { display: 'none' } : undefined} onClick={onClose}>
      <div className="modal post-editor-modal" onClick={e => e.stopPropagation()}>
        <div className="editor-header">
          <h2>{post ? 'Edit Post' : 'New Post'}</h2>
          <button className="btn btn-ghost btn-sm" onClick={onClose}>
            <X size={18} />
          </button>
        </div>

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
            <div className={`char-counter ${charClass}`}>
              {charCount} / {charLimit}
            </div>
          </div>

          {/* Images */}
          <div className="editor-images">
            {imageUrls.length > 0 && (
              <div className="image-preview-grid">
                {imageUrls.map((url, i) => (
                  <div key={i} className="image-preview">
                    <img src={url.startsWith('/') ? apiUrl + url : url} alt="" />
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

          {/* Tags */}
          <div className="form-group">
            <label><Hash size={14} /> Tags</label>
            <div className="tag-input-row">
              <input
                className="input"
                placeholder="Add a tag..."
                value={tagInput}
                onChange={e => setTagInput(e.target.value)}
                onKeyDown={e => { if (e.key === 'Enter') { e.preventDefault(); addTag(); } }}
              />
              <button className="btn btn-secondary btn-sm" onClick={addTag}>Add</button>
            </div>
            {tags.length > 0 && (
              <div className="tags-list">
                {tags.map(t => (
                  <span key={t} className="tag">
                    #{t}
                    <button onClick={() => setTags(prev => prev.filter(x => x !== t))}>x</button>
                  </span>
                ))}
              </div>
            )}
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

        <div className="editor-footer">
          <div className="footer-left">
            <button className="btn btn-ghost btn-sm" onClick={() => fileRef.current?.click()} disabled={uploading}>
              <Image size={16} /> {uploading ? 'Uploading...' : 'Upload'}
            </button>
            {adobeClientId && (
              <button className="btn btn-ghost btn-sm" onClick={launchAdobeExpress} disabled={adobeLoading}>
                <Wand2 size={16} /> {adobeLoading ? 'Opening...' : 'Create Design'}
              </button>
            )}
            {onDelete && (
              <button className="btn btn-danger btn-sm" onClick={onDelete}>
                <Trash2 size={16} /> Delete
              </button>
            )}
          </div>
          <div className="footer-right">
            <button className="btn btn-secondary" onClick={onClose}>Cancel</button>
            <button className="btn btn-primary" onClick={handleSubmit} disabled={saving || overLimit}>
              <Send size={16} /> {saving ? 'Saving...' : post ? 'Update' : 'Schedule'}
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}
